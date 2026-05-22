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
	"time"

	"github.com/kkweon/camply/internal/core"
)

type Provider struct {
	client        *http.Client
	baseURL       string
	campgroundURL string
	providerName  string
	// Cached metadata
	unitCategories  map[int]string
	unitTypeGroups  map[int]string
	facilityMap     map[int]string
	placeMap        map[int]string
	facilityToPlace map[int]int
	metadataFetched bool
}

func NewProvider(providerName, baseURL, campgroundURL string) *Provider {
	jar, _ := cookiejar.New(nil)
	return &Provider{
		client:        &http.Client{Timeout: 30 * time.Second, Jar: jar},
		baseURL:       baseURL,
		campgroundURL: campgroundURL,
		providerName:  providerName,
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

func (p *Provider) FindCampsites(ctx context.Context, req core.SearchRequest) ([]core.AvailableCampsite, error) {
	var allCampsites []core.AvailableCampsite

	if len(req.StartDates) == 0 || len(req.EndDates) == 0 {
		return nil, fmt.Errorf("UseDirect requires both start and end dates")
	}

	// 1. Bypass the TylerTech WAF by grabbing session cookies from a public metadata endpoint
	err := p.refreshMetadata(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to warmup usedirect session: %w", err)
	}

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
				resp.Body.Close()
				return nil, fmt.Errorf("UseDirect API returned status: %d", resp.StatusCode)
			}

			var grid gridResponse
			if err := json.NewDecoder(resp.Body).Decode(&grid); err != nil {
				resp.Body.Close()
				return nil, err
			}
			resp.Body.Close()

			if grid.Message != "" {
				// TylerTech sometimes returns a debug message here even on 200 OK. Don't fail the request.
				// e.g. "Built in 3.5325 ms size 11737 bytes on rdr-cali-prod-..."
				_ = grid.Message // no-op to satisfy staticcheck empty branch
			}

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
						campsiteType := p.unitCategories[unit.UnitCategoryId]
						campsiteUseType := p.unitTypeGroups[unit.UnitTypeGroupId]

						// TylerTech maps Tent sites to Group 5 and Equipment mapping
						var permittedEquipment []core.Equipment

						lowerUseType := strings.ToLower(campsiteUseType)

						// If it's explicitly a "Tent Site" or "Tent and RV" category, or VehicleLength is 0 (primitive)
						// We also include generic campsites, sites, and hook ups as they are typically tent friendly.
						if strings.Contains(lowerUseType, "tent") || strings.Contains(lowerUseType, "campsite") || strings.Contains(lowerUseType, "site") || strings.Contains(lowerUseType, "hook up") || unit.VehicleLength == 0 {
							permittedEquipment = append(permittedEquipment, core.Equipment{
								EquipmentName: "Tent",
								MaxLength:     0,
							})
						}
						// Map raw Vehicle Lengths natively into the struct
						if unit.VehicleLength > 0 {
							permittedEquipment = append(permittedEquipment, core.Equipment{
								EquipmentName: "RV",
								MaxLength:     unit.VehicleLength,
							}, core.Equipment{
								EquipmentName: "Trailer",
								MaxLength:     unit.VehicleLength,
							}, core.Equipment{
								EquipmentName: "Vehicle",
								MaxLength:     unit.VehicleLength,
							})
						}

						allCampsites = append(allCampsites, core.AvailableCampsite{
							CampsiteID:         strconv.Itoa(unit.UnitId),
							CampsiteSiteName:   unit.Name,
							BookingDate:        bookingDate,
							BookingEndDate:     bookingDate.AddDate(0, 0, 1), // Default 1 night, Filter merges them
							BookingNights:      1,
							CampsiteType:       campsiteType,
							CampsiteUseType:    campsiteUseType,
							AvailabilityStatus: "Available",
							FacilityID:         strconv.Itoa(facilityID),
							FacilityName:       grid.Facility.FacilityName,
							PermittedEquipment: permittedEquipment,
							BookingURL:         bookingURL,
						})
					}
				}
			}
		}
	}

	return allCampsites, nil
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
	defer resp.Body.Close()

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

	// 2. Fetch Facilities for URL mapping
	req2, _ := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/rdr/fd/facilities", p.baseURL), nil)
	req2.Header.Set("User-Agent", "Mozilla/5.0")
	resp2, err := p.client.Do(req2)
	if err == nil {
		defer resp2.Body.Close()
		var facResponse facilitiesResponse
		if err := json.NewDecoder(resp2.Body).Decode(&facResponse); err == nil {
			p.facilityToPlace = make(map[int]int)
			for _, f := range facResponse {
				p.facilityToPlace[f.FacilityId] = f.PlaceId
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
	defer resp.Body.Close()

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

		facilities = append(facilities, core.CampgroundFacility{
			FacilityID:       strconv.Itoa(data.FacilityId),
			FacilityName:     data.Name,
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
	defer resp.Body.Close()

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
