package recdotgov

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/kkweon/camply/internal/core"
	"github.com/kkweon/camply/internal/logger"
)

const (
	apiScheme   = "https"
	apiNetLoc   = "www.recreation.gov"
	apiBasePath = "api/camps/availability/campground"
	ridbApiKey  = "a7416471-1b5d-4a64-ad3d-a233e7cb5c44"
	ridbBaseURL = "https://ridb.recreation.gov/api/v1"
)

// Provider implements the core.Provider interface for Recreation.gov
type Provider struct {
	client      *http.Client
	apiScheme   string
	apiNetLoc   string
	apiBasePath string
	// ridbBaseURL is the root of the RIDB data API (no trailing slash). It is a
	// field rather than a const so tests can point it at an httptest server.
	ridbBaseURL string
}

// NewProvider creates a new Recreation.gov provider
func NewProvider() *Provider {
	return &Provider{
		client:      &http.Client{Timeout: 30 * time.Second},
		apiScheme:   "https",
		apiNetLoc:   "www.recreation.gov",
		apiBasePath: "api/camps/availability/campground",
		ridbBaseURL: ridbBaseURL,
	}
}

// NewProviderAt points a Provider at a different host.
//
// It exists so tests can replay recorded API responses through the real
// provider, adapter and filter rather than through a stand-in. Production
// always uses NewProvider.
func NewProviderAt(scheme, netLoc string) *Provider {
	p := NewProvider()
	p.apiScheme = scheme
	p.apiNetLoc = netLoc
	p.ridbBaseURL = scheme + "://" + netLoc
	return p
}

// FindCampsites queries Recreation.gov for availability.
//
// Classification happens here rather than in either fetch, because the two
// endpoints each hold half the evidence: attributes and notices come from the
// campsite search, campsite_type from the month availability, and the rules in
// adapter.go need both.
func (p *Provider) FindCampsites(ctx context.Context, req core.SearchRequest) ([]core.Availability, error) {
	var all []core.Availability

	// Recreation.gov availability is queried by campground ID and month.
	// For each campground and each month spanning the search dates, we make a request.
	months := getSearchMonths(req.StartDates, req.EndDates)

	// Every campsite at every campground searched, availability aside. It backs
	// the --campsites check below, which must not confuse "no such campsite"
	// with "that campsite is booked".
	var roster []core.KnownCampsite

	for _, campgroundID := range req.Campgrounds {
		// 1. Fetch Metadata (for equipment, attributes & facility names)
		metadata, err := p.fetchMetadata(ctx, campgroundID)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch metadata for %s: %w", campgroundID, err)
		}

		// Attempt to grab the human readable campground name from the first campsite's metadata
		facilityName := "Unknown Campground"
		for _, v := range metadata {
			if v.FacilityName != "" {
				facilityName = v.FacilityName
				break
			}
		}

		for id, meta := range metadata {
			roster = append(roster, core.KnownCampsite{
				CampsiteID:   id,
				SiteName:     meta.Name,
				FacilityName: facilityName,
			})
		}

		facility := p.describeFacility(ctx, campgroundID, facilityName)

		// One INFO line per campground, emitted after its result is known — the
		// same shape usedirect prints, so both providers' logs read alike.
		label := fmt.Sprintf("%s (#%s)", facilityName, campgroundID)
		if facility.RecreationArea != "" {
			label = fmt.Sprintf("%s (#%s, %s)", facilityName, campgroundID, facility.RecreationArea)
		}
		logger.Debug("Searching %s...", label)

		// Distinct campsites with at least one open night, across every month.
		sitesFree := map[string]bool{}

		// 2. Fetch Availabilities
		for _, month := range months {
			raw, err := p.getAvailability(ctx, campgroundID, month)
			if err != nil {
				return nil, fmt.Errorf("failed to get availability for campground %s: %w", campgroundID, err)
			}

			// 3. Build one Site per campsite, shared by every night it is free.
			sites := make(map[string]*core.Site, len(raw.Campsites))
			for id, data := range raw.Campsites {
				sites[id] = p.buildSite(id, data, facility, metadata[id])
			}

			for id, data := range raw.Campsites {
				for dateStr, status := range data.Availabilities {
					// Recreation.gov uses "Available" for open slots
					if status != "Available" {
						continue
					}
					// Date comes in as 2006-01-02T00:00:00Z
					start, err := time.Parse(time.RFC3339, dateStr)
					if err != nil {
						continue
					}
					sitesFree[id] = true
					all = append(all, core.Availability{
						Site:   sites[id],
						Start:  start,
						End:    start.AddDate(0, 0, 1), // one night; Filter merges consecutive ones
						Nights: 1,
						Status: status,
					})
				}
			}
		}

		logger.Info("🏕  %s: %d of %d sites have at least one open night",
			label, len(sitesFree), len(metadata))
	}

	// Checked after every campground is known: an ID absent from one campground
	// may well belong to another in the same search.
	if err := core.ValidateRequestedCampsites(req.Campsites, roster); err != nil {
		return nil, err
	}

	return all, nil
}

// buildSite is the whole adapter boundary for one campsite: raw API shapes in,
// camply's domain out.
func (p *Provider) buildSite(id string, data campsiteData, facility core.Facility, meta siteMetadata) *core.Site {
	parking, parkingBasis := classifyParking(meta.Attributes, data.Type, meta.Notices)
	permits, permitsBasis := classifyPermits(meta.Equipment, data.Type)

	if meta.FacilityName != "" {
		facility.Name = meta.FacilityName
	}

	return &core.Site{
		ID:           id,
		Name:         data.Site,
		Loop:         data.Loop,
		Facility:     facility,
		Permits:      permits,
		Parking:      parking,
		Hookups:      classifyHookups(meta.Attributes, data.Type),
		SharedWater:  meta.Attributes.yesNo(sharedWaterAttrs),
		Equipment:    meta.Equipment,
		WalkFeet:     meta.Attributes.firstNumber(hikeDistanceAttrs),
		Waterfront:   meta.Attributes.text("Proximity to Water"),
		Amps:         meta.Attributes.number("Electricity Hookup"),
		MaxVehicles:  meta.Attributes.number(attrMaxNumVehicles),
		PermitsBasis: permitsBasis,
		ParkingBasis: parkingBasis,
		AccessLabel:  meta.Attributes.text(attrSiteAccess),
		RawType:      data.Type,
		UseType:      data.TypeOfUse,
		MinOccupancy: data.MinNumPeople,
		MaxOccupancy: data.MaxNumPeople,
		BookingURL:   fmt.Sprintf("https://www.recreation.gov/camping/campsites/%s", id),
	}
}

type ridbFacilitiesResponse struct {
	RecData []struct {
		FacilityID              string `json:"FacilityID"`
		FacilityName            string `json:"FacilityName"`
		FacilityTypeDescription string `json:"FacilityTypeDescription"`
		Enabled                 bool   `json:"Enabled"`
		Reservable              bool   `json:"Reservable"`
		ParentRecAreaID         string `json:"ParentRecAreaID"`
		RecArea                 []struct {
			RecAreaID   string `json:"RecAreaID"`
			RecAreaName string `json:"RecAreaName"`
		} `json:"RECAREA"`
	} `json:"RECDATA"`
	MetaData struct {
		Results struct {
			CurrentCount int `json:"CURRENT_COUNT"`
			TotalCount   int `json:"TOTAL_COUNT"`
		} `json:"RESULTS"`
	} `json:"METADATA"`
}

// ridbFacilityResponse is RIDB's single-facility record. Unlike /facilities it
// returns the object itself rather than a RECDATA envelope.
type ridbFacilityResponse struct {
	FacilityID      string `json:"FacilityID"`
	FacilityName    string `json:"FacilityName"`
	ParentRecAreaID string `json:"ParentRecAreaID"`
	RecArea         []struct {
		RecAreaID   string `json:"RecAreaID"`
		RecAreaName string `json:"RecAreaName"`
	} `json:"RECAREA"`
	Address []struct {
		City             string `json:"City"`
		AddressStateCode string `json:"AddressStateCode"`
	} `json:"FACILITYADDRESS"`
}

// describeFacility resolves where a campground actually is.
//
// The campsite search the availability path already fetches carries only
// asset_id and asset_name -- no recreation area, no address -- so this is a
// second request, to RIDB, and it is the only place the answer exists. Skipping
// it is what left every recreation.gov alert titled with a bare campground name
// and a leading empty field.
//
// It degrades instead of failing. An alert that names the campground is still
// worth sending, and a search that did find availability must not be discarded
// because a lookup that only improves the wording went unanswered.
func (p *Provider) describeFacility(ctx context.Context, campgroundID, name string) core.Facility {
	facility := core.Facility{ID: campgroundID, Name: name}

	var resp ridbFacilityResponse
	urlStr := fmt.Sprintf("%s/facilities/%s?full=true", p.ridbBaseURL, url.PathEscape(campgroundID))
	if err := p.getJSON(ctx, urlStr, ridbHeaders, &resp); err != nil {
		logger.Debug("could not resolve where campground %s is: %v", campgroundID, err)
		return facility
	}

	// The nested RECAREA is the authority; ParentRecAreaID is the id alone, and
	// is all some facilities carry. Neither is invented when both are absent --
	// a blank field is a fact about the provider, and the notification says so.
	facility.RecreationAreaID = resp.ParentRecAreaID
	if len(resp.RecArea) > 0 {
		facility.RecreationArea = resp.RecArea[0].RecAreaName
		facility.RecreationAreaID = resp.RecArea[0].RecAreaID
	}

	// RIDB repeats the address once per type (physical, mailing). They agree on
	// the town for every campground in the corpus, so the first usable one wins.
	for _, a := range resp.Address {
		if loc := joinLocation(a.City, a.AddressStateCode); loc != "" {
			facility.Location = loc
			break
		}
	}

	return facility
}

// joinLocation renders a town as "City, ST", dropping whichever half is missing.
func joinLocation(city, state string) string {
	city, state = strings.TrimSpace(city), strings.TrimSpace(state)
	switch {
	case city != "" && state != "":
		return city + ", " + state
	case city != "":
		return city
	default:
		return state
	}
}

func (p *Provider) FindCampgrounds(ctx context.Context, req core.SearchRequest) ([]core.CampgroundFacility, error) {
	var facilities []core.CampgroundFacility

	start := 0
	size := 500
	total := 1

	// RIDB serves facilities from two endpoints: a flat /facilities list and a
	// /recareas/{id}/facilities list scoped to one recreation area. The flat
	// endpoint has NO rec-area filter, so scoping must be done by choosing the
	// nested path -- passing the ID to /facilities would silently return every
	// reservable campground in the country.
	endpoint := p.ridbBaseURL + "/facilities"
	if req.RecreationArea != "" {
		endpoint = fmt.Sprintf("%s/recareas/%s/facilities", p.ridbBaseURL, url.PathEscape(req.RecreationArea))
	}

	for start < total {
		urlStr := fmt.Sprintf("%s?full=true&limit=%d&offset=%d", endpoint, size, start)
		if req.Query != "" {
			urlStr += fmt.Sprintf("&query=%s", url.QueryEscape(req.Query))
		}
		if req.State != "" {
			urlStr += fmt.Sprintf("&state=%s", url.QueryEscape(req.State))
		}

		var parsedResp ridbFacilitiesResponse
		if err := p.getJSON(ctx, urlStr, ridbHeaders, &parsedResp); err != nil {
			return nil, err
		}

		total = parsedResp.MetaData.Results.TotalCount
		// Guard against a page that reports results but returns no rows: without
		// this, start never advances and the loop spins forever.
		if parsedResp.MetaData.Results.CurrentCount == 0 {
			break
		}
		start += parsedResp.MetaData.Results.CurrentCount

		for _, data := range parsedResp.RecData {
			// Camply filters out non-campgrounds
			if data.FacilityTypeDescription != "Campground" || !data.Enabled || !data.Reservable {
				continue
			}

			// Some campgrounds sit in a recreation area and some report none.
			// The blank stays blank. This branch used to guess "National Park",
			// and in a 475-campground sample of RIDB the three facilities that
			// reach it -- Joe T. Fallini Recreation Site, Sawtooth Canyon
			// Campground, LOON LAKE RECREATION SITE -- are BLM and national
			// forest land, so the guess was wrong every time it fired.
			// describeFacility decides this the same way, and the two paths
			// must not describe one campground differently.
			recAreaName := ""
			recAreaID := data.ParentRecAreaID
			if len(data.RecArea) > 0 {
				recAreaName = data.RecArea[0].RecAreaName
				recAreaID = data.RecArea[0].RecAreaID
			}

			facilities = append(facilities, core.CampgroundFacility{
				FacilityID:       data.FacilityID,
				FacilityName:     data.FacilityName,
				RecreationArea:   recAreaName,
				RecreationAreaID: recAreaID,
			})
		}
	}

	return facilities, nil
}

type ridbRecAreaResponse struct {
	RecData []struct {
		RecAreaID   string `json:"RecAreaID"`
		RecAreaName string `json:"RecAreaName"`
		Addresses   []struct {
			AddressStateCode string `json:"AddressStateCode"`
		} `json:"RECAREAADDRESS"`
	} `json:"RECDATA"`
	MetaData struct {
		Results struct {
			CurrentCount int `json:"CURRENT_COUNT"`
			TotalCount   int `json:"TOTAL_COUNT"`
		} `json:"RESULTS"`
	} `json:"METADATA"`
}

func (p *Provider) FindRecreationAreas(ctx context.Context, req core.SearchRequest) ([]core.RecreationArea, error) {
	var areas []core.RecreationArea

	start := 0
	size := 500
	total := 1

	for start < total {
		urlStr := fmt.Sprintf("%s/recareas?full=true&limit=%d&offset=%d", p.ridbBaseURL, size, start)
		if req.Query != "" {
			urlStr += fmt.Sprintf("&query=%s", url.QueryEscape(req.Query))
		}
		if req.State != "" {
			urlStr += fmt.Sprintf("&state=%s", url.QueryEscape(req.State))
		}

		var parsedResp ridbRecAreaResponse
		if err := p.getJSON(ctx, urlStr, ridbHeaders, &parsedResp); err != nil {
			return nil, err
		}

		total = parsedResp.MetaData.Results.TotalCount
		start += parsedResp.MetaData.Results.CurrentCount

		for _, data := range parsedResp.RecData {
			location := "USA"
			if len(data.Addresses) > 0 {
				location = data.Addresses[0].AddressStateCode
			}

			areas = append(areas, core.RecreationArea{
				RecreationAreaID:       data.RecAreaID,
				RecreationArea:         data.RecAreaName,
				RecreationAreaLocation: location,
			})
		}
	}

	return areas, nil
}

// getAvailability calls the /api/camps/availability/campground/{id}/month endpoint
func (p *Provider) getAvailability(ctx context.Context, campgroundID string, month time.Time) (monthAvailabilityResponse, error) {
	urlStr := fmt.Sprintf("%s://%s/%s/%s/month?start_date=%s",
		p.apiScheme, p.apiNetLoc, p.apiBasePath, campgroundID, url.QueryEscape(month.Format("2006-01-02T00:00:00.000Z")))

	var apiResp monthAvailabilityResponse
	if err := p.getJSON(ctx, urlStr, recdotgovHeaders, &apiResp); err != nil {
		return monthAvailabilityResponse{}, err
	}
	return apiResp, nil
}

// getSearchMonths extracts the unique YYYY-MM-01 times to query
func getSearchMonths(startDates, endDates []time.Time) []time.Time {
	// A simple set implementation in Go to find unique months
	monthsSet := make(map[string]time.Time)

	for i := range startDates {
		start := startDates[i]
		end := endDates[i]

		current := time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, time.UTC)
		endMonth := time.Date(end.Year(), end.Month(), 1, 0, 0, 0, 0, time.UTC)

		for !current.After(endMonth) {
			monthsSet[current.Format("2006-01")] = current
			current = current.AddDate(0, 1, 0)
		}
	}

	var months []time.Time
	for _, m := range monthsSet {
		months = append(months, m)
	}
	return months
}

// --- JSON Structs for Recreation.gov ---

type monthAvailabilityResponse struct {
	Campsites map[string]campsiteData `json:"campsites"`
}

type campsiteData struct {
	CampsiteID     string            `json:"campsite_id"`
	Site           string            `json:"site"`
	Loop           string            `json:"loop"`
	Type           string            `json:"campsite_type"`
	TypeOfUse      string            `json:"type_of_use"`
	MinNumPeople   int               `json:"min_num_people"`
	MaxNumPeople   int               `json:"max_num_people"`
	Availabilities map[string]string `json:"availabilities"`
}
