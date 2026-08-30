package recdotgov

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/kkweon/camply/internal/core"
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

// FindCampsites queries Recreation.gov for availability
func (p *Provider) FindCampsites(ctx context.Context, req core.SearchRequest) ([]core.AvailableCampsite, error) {
	var allCampsites []core.AvailableCampsite

	// Recreation.gov availability is queried by campground ID and month.
	// For each campground and each month spanning the search dates, we make a request.
	months := getSearchMonths(req.StartDates, req.EndDates)

	for _, campgroundID := range req.Campgrounds {
		// 1. Fetch Metadata (for equipment & facility names)
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

		fmt.Printf("🏕  Fetched metadata for %s (#%s) - %d total campsites\n", facilityName, campgroundID, len(metadata))
		// 2. Fetch Availabilities
		for _, month := range months {
			campsites, err := p.getAvailability(ctx, campgroundID, month)
			if err != nil {
				return nil, fmt.Errorf("failed to get availability for campground %s: %w", campgroundID, err)
			}

			// 3. Hydrate with metadata
			for i := range campsites {
				meta, ok := metadata[campsites[i].CampsiteID]
				if ok {
					campsites[i].PermittedEquipment = meta.PermittedEquipment
					if meta.FacilityName != "" {
						campsites[i].FacilityName = meta.FacilityName
					}
					campsites[i].SiteAccessRaw = meta.SiteAccessRaw
					campsites[i].MaxVehicles = meta.MaxVehicles
				}
				// Classified here rather than in either fetch: the attributes
				// come from the metadata endpoint and campsite_type from the
				// availability endpoint, and the rule needs both. A campsite
				// missing from the metadata map keeps the zero value, which is
				// SiteAccessUnknown — reported, never assumed drive-in.
				campsites[i].SiteAccess = classifySiteAccess(
					campsites[i].SiteAccessRaw,
					campsites[i].MaxVehicles,
					campsites[i].CampsiteType,
				)
			}

			allCampsites = append(allCampsites, campsites...)
		}
	}

	return allCampsites, nil
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

			// Some campgrounds are in Rec Areas, some are independent (National Parks)
			recAreaName := "National Park"
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
func (p *Provider) getAvailability(ctx context.Context, campgroundID string, month time.Time) ([]core.AvailableCampsite, error) {
	urlStr := fmt.Sprintf("%s://%s/%s/%s/month?start_date=%s",
		p.apiScheme, p.apiNetLoc, p.apiBasePath, campgroundID, url.QueryEscape(month.Format("2006-01-02T00:00:00.000Z")))

	var apiResp monthAvailabilityResponse
	if err := p.getJSON(ctx, urlStr, recdotgovHeaders, &apiResp); err != nil {
		return nil, err
	}

	return parseAvailability(apiResp, campgroundID)
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

func parseAvailability(apiResp monthAvailabilityResponse, campgroundID string) ([]core.AvailableCampsite, error) {
	var available []core.AvailableCampsite

	for id, data := range apiResp.Campsites {
		for dateStr, status := range data.Availabilities {
			// Recreation.gov uses "Available" for open slots
			if status != "Available" {
				continue
			}

			// Date comes in as 2006-01-02T00:00:00Z
			bookingDate, err := time.Parse(time.RFC3339, dateStr)
			if err != nil {
				continue
			}

			available = append(available, core.AvailableCampsite{
				CampsiteID:         id,
				BookingDate:        bookingDate,
				BookingEndDate:     bookingDate.AddDate(0, 0, 1), // Default 1 night, Filter handles consecutive merging
				BookingNights:      1,
				CampsiteSiteName:   data.Site,
				CampsiteLoopName:   data.Loop,
				CampsiteType:       data.Type,
				MinOccupancy:       data.MinNumPeople,
				MaxOccupancy:       data.MaxNumPeople,
				CampsiteUseType:    data.TypeOfUse,
				AvailabilityStatus: status,
				FacilityID:         campgroundID,
				BookingURL:         fmt.Sprintf("https://www.recreation.gov/camping/campsites/%s", id),
			})
		}
	}
	return available, nil
}
