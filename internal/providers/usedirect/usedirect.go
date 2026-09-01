package usedirect

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kkweon/camply/internal/core"
	"github.com/kkweon/camply/internal/logger"
)

type Provider struct {
	client        *http.Client
	baseURL       string
	campgroundURL string
	providerName  string
	// Cached metadata
	unitCategories  map[int]string
	unitTypeGroups  map[int]string
	facilityToPlace map[int]int
	places          map[int]core.RecreationArea
	facilities      map[int]core.CampgroundFacility
	metadataFetched bool
	// Cached unit occupancy from /rdr/search/details/<unitId>
	mu            sync.Mutex
	unitOccupancy map[int]unitOccupancyData
}

type unitOccupancyData struct {
	MinOccupancy int
	MaxOccupancy int
}

func NewProvider(providerName, baseURL, campgroundURL string) *Provider {
	jar, _ := cookiejar.New(nil)
	return &Provider{
		client:        &http.Client{Timeout: 30 * time.Second, Jar: jar},
		baseURL:       baseURL,
		campgroundURL: campgroundURL,
		providerName:  providerName,
		places:        make(map[int]core.RecreationArea),
		facilities:    make(map[int]core.CampgroundFacility),
	}
}

type gridRequest struct {
	StartDate    string `json:"StartDate"`
	EndDate      string `json:"EndDate"`
	FacilityId   int    `json:"FacilityId"`
	UnitSort     string `json:"UnitSort"`
	InSeasonOnly bool   `json:"InSeasonOnly"`
	WebOnly      bool   `json:"WebOnly"`
}

type gridResponse struct {
	Message  string `json:"Message"`
	Facility struct {
		FacilityId   int    `json:"FacilityId"`
		FacilityName string `json:"Name"`
		Units        map[string]struct {
			UnitId          int    `json:"UnitId"`
			Name            string `json:"Name"`
			UnitCategoryId  int    `json:"UnitCategoryId"`
			UnitTypeGroupId int    `json:"UnitTypeGroupId"`
			VehicleLength   int    `json:"VehicleLength"`
			Slices          map[string]struct {
				Date   string `json:"Date"`
				IsFree bool   `json:"IsFree"`
			} `json:"Slices"`
		} `json:"Units"`
	} `json:"Facility"`
}

func (p *Provider) FindCampsites(ctx context.Context, req core.SearchRequest) ([]core.Availability, error) {
	var all []core.Availability

	if len(req.StartDates) == 0 || len(req.EndDates) == 0 {
		return nil, fmt.Errorf("UseDirect requires both start and end dates")
	}

	// 1. Bypass the TylerTech WAF by grabbing session cookies from a public metadata endpoint
	err := p.refreshMetadata(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to warmup usedirect session: %w", err)
	}

	// Every unit at every facility searched — the roster the --campsites check
	// needs in order to tell a typo from a booked-out site.
	var roster []core.KnownCampsite

	for _, campgroundID := range req.Campgrounds {
		facilityID, err := strconv.Atoi(campgroundID)
		if err != nil {
			return nil, fmt.Errorf("invalid campground ID %s: %w", campgroundID, err)
		}

		fmt.Printf("🏕  Searching %s (#%s) for availability...\n", p.providerName, campgroundID)

		for i := range req.StartDates {
			start := req.StartDates[i]
			end := req.EndDates[i]

			// The API expects exactly the range bounded by today at the earliest
			greatestStart := start
			today := time.Now().Truncate(24*time.Hour).AddDate(0, 0, -1)
			if greatestStart.Before(today) {
				greatestStart = today
			}

			payload := gridRequest{
				StartDate:    greatestStart.Format("01-02-2006"), // UseDirect uses MM-DD-YYYY
				EndDate:      end.Format("01-02-2006"),
				FacilityId:   facilityID,
				UnitSort:     "orderby",
				InSeasonOnly: true,
				WebOnly:      true,
			}

			jsonData, err := json.Marshal(payload)
			if err != nil {
				return nil, err
			}

			url := fmt.Sprintf("%s/rdr/search/grid", p.baseURL)
			httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonData))
			if err != nil {
				return nil, err
			}
			httpReq.Header.Set("Content-Type", "application/json")
			httpReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/116.0.0.0 Safari/537.36")

			resp, err := p.client.Do(httpReq)
			if err != nil {
				return nil, err
			}

			if resp.StatusCode >= 400 {
				_ = resp.Body.Close()
				return nil, fmt.Errorf("UseDirect API returned status: %d", resp.StatusCode)
			}

			var grid gridResponse
			if err := json.NewDecoder(resp.Body).Decode(&grid); err != nil {
				_ = resp.Body.Close()
				return nil, err
			}
			_ = resp.Body.Close()

			if grid.Message != "" {
				// TylerTech sometimes returns a debug message here even on 200 OK. Don't fail the request.
				// e.g. "Built in 3.5325 ms size 11737 bytes on rdr-cali-prod-..."
				_ = grid.Message // no-op to satisfy staticcheck empty branch
			}

			// One Site per unit, shared by every night it is free.
			sites := map[string]*core.Site{}

			// Pre-fetch occupancy concurrently for any unit with a free slice
			var wg sync.WaitGroup
			sem := make(chan struct{}, 5) // limit concurrency to 5
			uniqueUnitsToFetch := make(map[int]bool)
			for _, unit := range grid.Facility.Units {
				for _, slice := range unit.Slices {
					if slice.IsFree {
						uniqueUnitsToFetch[unit.UnitId] = true
						break
					}
				}
			}

			for unitID := range uniqueUnitsToFetch {
				wg.Add(1)
				go func(id int) {
					defer wg.Done()
					sem <- struct{}{} // acquire
					p.fetchUnitOccupancy(ctx, id)
					<-sem // release
				}(unitID)
			}
			wg.Wait()

			for _, unit := range grid.Facility.Units {
				for _, slice := range unit.Slices {
					if slice.IsFree {
						// Slice dates come in as "2026-06-01" or "2026-06-01T00:00:00" depending on TylerTech version
						bookingDate, err := time.Parse("2006-01-02", slice.Date)
						if err != nil {
							bookingDate, err = time.Parse("2006-01-02T15:04:05", slice.Date)
							if err != nil {
								continue
							}
						}

						// ReserveCalifornia format booking link: https://www.reservecalifornia.com/park/691/616
						placeID := p.facilityToPlace[facilityID]
						bookingURL := fmt.Sprintf("%s/park/%d/%d", p.campgroundURL, placeID, facilityID)
						// The grid endpoint ignores MinVehicleLength in the
						// request (verified: identical unit counts with and
						// without it), so filter on the VehicleLength it does
						// return per unit.
						if req.MinVehicleLength > 0 && unit.VehicleLength < req.MinVehicleLength {
							continue
						}

						campsiteType := p.unitCategories[unit.UnitCategoryId]
						campsiteUseType := p.unitTypeGroups[unit.UnitTypeGroupId]

						// Fetch occupancy lazily (cached per unit)
						p.fetchUnitOccupancy(ctx, unit.UnitId)

						logger.Debug("Evaluating campsite: %s (Type: %s, UseType: %s, VehicleLength: %d)",
							unit.Name, campsiteType, campsiteUseType, unit.VehicleLength)

						parking, parkingBasis := classifyParking(campsiteType, campsiteUseType, unit.VehicleLength)
						permits, permitsBasis := classifyPermits(campsiteType, campsiteUseType, unit.VehicleLength, parking)

						// UseDirect grids carry no equipment, so camply
						// synthesises it from what the unit permits. A unit no
						// car reaches must not advertise vehicle equipment even
						// when the API reports a length for it.
						var equipment []core.Equipment
						if permits.Has(core.PermitsTent) {
							equipment = append(equipment, core.Equipment{EquipmentName: EquipmentTent})
						}
						if permits.Has(core.PermitsRV) {
							equipment = append(equipment,
								core.Equipment{EquipmentName: EquipmentRV, MaxLength: unit.VehicleLength},
								core.Equipment{EquipmentName: EquipmentTrailer, MaxLength: unit.VehicleLength},
								core.Equipment{EquipmentName: EquipmentVehicle, MaxLength: unit.VehicleLength})
						}

						// The place record carries the town as well as the
						// name. Both travel with the facility: a reader
						// watching several parks cannot place a campground
						// from its name alone.
						var recreationAreaName, recreationAreaLocation string
						if ra, ok := p.places[placeID]; ok {
							recreationAreaName = ra.RecreationArea
							recreationAreaLocation = ra.RecreationAreaLocation
						}

						minOcc := 0
						maxOcc := 1
						if occ, ok := p.unitOccupancy[unit.UnitId]; ok {
							minOcc = occ.MinOccupancy
							maxOcc = occ.MaxOccupancy
						}

						unitID := strconv.Itoa(unit.UnitId)
						site, ok := sites[unitID]
						if !ok {
							site = &core.Site{
								ID:   unitID,
								Name: unit.Name,
								Facility: core.Facility{
									ID:               strconv.Itoa(facilityID),
									Name:             grid.Facility.FacilityName,
									RecreationArea:   recreationAreaName,
									RecreationAreaID: strconv.Itoa(placeID),
									Location:         recreationAreaLocation,
								},
								Permits:      permits,
								Parking:      parking,
								Hookups:      classifyHookups(campsiteType),
								Equipment:    equipment,
								PermitsBasis: permitsBasis,
								ParkingBasis: parkingBasis,
								AccessLabel:  accessLabel(parking, campsiteUseType),
								RawType:      campsiteType,
								UseType:      campsiteUseType,
								MinOccupancy: minOcc,
								MaxOccupancy: maxOcc,
								BookingURL:   bookingURL,
							}
							sites[unitID] = site
							roster = append(roster, core.KnownCampsite{
								CampsiteID:   unitID,
								SiteName:     unit.Name,
								FacilityName: grid.Facility.FacilityName,
							})
						}

						all = append(all, core.Availability{
							Site:   site,
							Start:  bookingDate,
							End:    bookingDate.AddDate(0, 0, 1), // one night; Filter merges consecutive ones
							Nights: 1,
							Status: "Available",
						})
					}
				}
			}
		}
	}

	// Checked after every facility is known: an ID absent from one may belong
	// to another in the same search.
	if err := core.ValidateRequestedCampsites(req.Campsites, roster); err != nil {
		return nil, err
	}

	return all, nil
}

// accessLabel is the word an alert uses for how a unit is reached.
//
// UseDirect has no Site Access field, but its type group is the provider's own
// word for the same thing — "Hike & Bike", "Bike In", "Boat In" — and it is more
// use to a reader than camply's generic one. A walk of some kind is where that
// detail matters, so the type group is preferred there and camply's own word
// fills in elsewhere.
func accessLabel(p core.Parking, campsiteUseType string) string {
	if p.RequiresWalk() && strings.TrimSpace(campsiteUseType) != "" {
		return campsiteUseType
	}
	switch p {
	case core.ParkingAtSite:
		return "Drive-In"
	case core.ParkingWalk:
		return "Walk-In"
	case core.ParkingNone:
		return "No Vehicle Access"
	default:
		return ""
	}
}

type unitDetailResponse struct {
	NightlyUnit struct {
		MinOccupancy int `json:"MinOccupancy"`
		MaxOccupancy int `json:"MaxOccupancy"`
	} `json:"NightlyUnit"`
	DayUseUnit struct {
		MinOccupancy int `json:"MinOccupancy"`
		MaxOccupancy int `json:"MaxOccupancy"`
	} `json:"DayUseUnit"`
}

type filterResponse struct {
	UnitCategories []struct {
		ID   int    `json:"UnitCategoryId"`
		Name string `json:"UnitCategoryName"`
	} `json:"UnitCategories"`
	UnitTypesGroups []struct {
		ID   int    `json:"UnitTypesGroupId"`
		Name string `json:"UnitTypesGroupName"`
	} `json:"UnitTypesGroups"`
}

// fetchUnitOccupancy fetches occupancy data for a single unit and caches it
func (p *Provider) fetchUnitOccupancy(ctx context.Context, unitID int) {
	p.mu.Lock()
	if p.unitOccupancy == nil {
		p.unitOccupancy = make(map[int]unitOccupancyData)
	}
	if _, ok := p.unitOccupancy[unitID]; ok {
		p.mu.Unlock()
		return
	}
	p.mu.Unlock()

	url := fmt.Sprintf("%s/rdr/search/details/%d/startdate/2000-01-01/nights/1/0/0", p.baseURL, unitID)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := p.client.Do(req)
	if err != nil {
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		return
	}

	var det unitDetailResponse
	if err := json.NewDecoder(resp.Body).Decode(&det); err != nil {
		return
	}

	occ := unitOccupancyData{MinOccupancy: 0, MaxOccupancy: 1}
	if det.NightlyUnit.MaxOccupancy > 0 {
		occ.MinOccupancy = det.NightlyUnit.MinOccupancy
		occ.MaxOccupancy = det.NightlyUnit.MaxOccupancy
	} else if det.DayUseUnit.MaxOccupancy > 0 {
		occ.MinOccupancy = det.DayUseUnit.MinOccupancy
		occ.MaxOccupancy = det.DayUseUnit.MaxOccupancy
	}

	p.mu.Lock()
	p.unitOccupancy[unitID] = occ
	p.mu.Unlock()
}

// refreshMetadata caches all categorical types and resolves WAF session cookies exactly like Python
func (p *Provider) refreshMetadata(ctx context.Context) error {
	if p.metadataFetched {
		return nil
	}

	// 1. Fetch Categories for mapping and WAF cookies
	req1, _ := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/rdr/search/filters", p.baseURL), nil)
	req1.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := p.client.Do(req1)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	var filters filterResponse
	if err := json.NewDecoder(resp.Body).Decode(&filters); err == nil {
		p.unitCategories = make(map[int]string)
		for _, c := range filters.UnitCategories {
			p.unitCategories[c.ID] = c.Name
		}
		p.unitTypeGroups = make(map[int]string)
		for _, t := range filters.UnitTypesGroups {
			p.unitTypeGroups[t.ID] = t.Name
		}
	}

	// 2. Fetch Places for Recreation Area mapping
	p.places = make(map[int]core.RecreationArea)
	req2, _ := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/rdr/fd/places", p.baseURL), nil)
	req2.Header.Set("User-Agent", "Mozilla/5.0")
	resp2, err := p.client.Do(req2)
	if err == nil {
		defer func() { _ = resp2.Body.Close() }()
		var places placesResponse
		if err := json.NewDecoder(resp2.Body).Decode(&places); err == nil {
			for _, pl := range places {
				location := fmt.Sprintf("%s, %s", pl.City, pl.State)
				if pl.City == "" {
					location = pl.State
				}
				p.places[pl.PlaceId] = core.RecreationArea{
					RecreationAreaID:       strconv.Itoa(pl.PlaceId),
					RecreationArea:         pl.Name,
					RecreationAreaLocation: location,
				}
			}
		}
	}

	// 3. Fetch Facilities for URL mapping and Campground info
	p.facilities = make(map[int]core.CampgroundFacility)
	req3, _ := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/rdr/fd/facilities", p.baseURL), nil)
	req3.Header.Set("User-Agent", "Mozilla/5.0")
	resp3, err := p.client.Do(req3)
	if err == nil {
		defer func() { _ = resp3.Body.Close() }()
		var facResponse facilitiesResponse
		if err := json.NewDecoder(resp3.Body).Decode(&facResponse); err == nil {
			p.facilityToPlace = make(map[int]int)
			for _, f := range facResponse {
				p.facilityToPlace[f.FacilityId] = f.PlaceId

				recAreaName := ""
				if ra, ok := p.places[f.PlaceId]; ok {
					recAreaName = ra.RecreationArea
				}
				p.facilities[f.FacilityId] = core.CampgroundFacility{
					FacilityID:       strconv.Itoa(f.FacilityId),
					FacilityName:     f.Name,
					RecreationArea:   recAreaName,
					RecreationAreaID: strconv.Itoa(f.PlaceId),
				}
			}
		}
	}

	p.metadataFetched = true
	return nil
}

type facilitiesResponse []struct {
	FacilityId int    `json:"FacilityId"`
	Name       string `json:"Name"`
	PlaceId    int    `json:"PlaceId"`
}

func (p *Provider) FindCampgrounds(ctx context.Context, req core.SearchRequest) ([]core.CampgroundFacility, error) {
	var facilities []core.CampgroundFacility

	err := p.refreshMetadata(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to warmup usedirect session: %w", err)
	}

	url := fmt.Sprintf("%s/rdr/fd/facilities", p.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("UseDirect API returned status: %d", resp.StatusCode)
	}

	var parsedResp facilitiesResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsedResp); err != nil {
		return nil, err
	}

	// Filter down by RecreationArea if provided
	recAreaMap := make(map[int]bool)

	// req.RecreationArea is a string based on the core.SearchRequest interface we built
	if req.RecreationArea != "" {
		if id, err := strconv.Atoi(req.RecreationArea); err == nil {
			recAreaMap[id] = true
		}
	}

	for _, data := range parsedResp {
		// Apply search filter if query is provided
		if req.Query != "" && !strings.Contains(strings.ToLower(data.Name), strings.ToLower(req.Query)) {
			continue
		}

		if len(recAreaMap) > 0 && !recAreaMap[data.PlaceId] {
			continue
		}

		recAreaName := ""
		if ra, ok := p.places[data.PlaceId]; ok {
			recAreaName = ra.RecreationArea
		}

		facilities = append(facilities, core.CampgroundFacility{
			FacilityID:       strconv.Itoa(data.FacilityId),
			FacilityName:     data.Name,
			RecreationArea:   recAreaName,
			RecreationAreaID: strconv.Itoa(data.PlaceId),
		})
	}

	return facilities, nil
}

type placesResponse []struct {
	PlaceId int    `json:"PlaceId"`
	Name    string `json:"Name"`
	City    string `json:"City"`
	State   string `json:"State"`
}

func (p *Provider) FindRecreationAreas(ctx context.Context, req core.SearchRequest) ([]core.RecreationArea, error) {
	var areas []core.RecreationArea

	err := p.refreshMetadata(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to warmup usedirect session: %w", err)
	}

	url := fmt.Sprintf("%s/rdr/fd/places", p.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("UseDirect API returned status: %d", resp.StatusCode)
	}

	var parsedResp placesResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsedResp); err != nil {
		return nil, err
	}

	for _, data := range parsedResp {
		// Apply search filter if query is provided
		if req.Query != "" && !strings.Contains(strings.ToLower(data.Name), strings.ToLower(req.Query)) {
			continue
		}

		location := fmt.Sprintf("%s, %s", data.City, data.State)
		if data.City == "" {
			location = data.State
		}

		areas = append(areas, core.RecreationArea{
			RecreationAreaID:       strconv.Itoa(data.PlaceId),
			RecreationArea:         data.Name,
			RecreationAreaLocation: location,
		})
	}

	return areas, nil
}
