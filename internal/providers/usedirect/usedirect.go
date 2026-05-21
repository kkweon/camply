package usedirect

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"strconv"
	"time"

	"github.com/kkweon/camply/internal/core"
)

type Provider struct {
	client       *http.Client
	baseURL      string
	providerName string
}

func NewProvider(providerName, baseURL string) *Provider {
	jar, _ := cookiejar.New(nil)
	return &Provider{
		client:       &http.Client{Timeout: 30 * time.Second, Jar: jar},
		baseURL:      baseURL,
		providerName: providerName,
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
			UnitId int    `json:"UnitId"`
			Name   string `json:"Name"`
			Slices map[string]struct {
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
	err := p.warmupCookies(ctx)
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
						// Slice dates come in as "2026-06-01T00:00:00"
						bookingDate, err := time.Parse("2006-01-02T15:04:05", slice.Date)
						if err != nil {
							continue
						}

						// ReserveCalifornia format booking link: https://www.reservecalifornia.com/park/0/1121
						bookingURL := fmt.Sprintf("%s/park/0/%d", p.baseURL, facilityID)

						allCampsites = append(allCampsites, core.AvailableCampsite{
							CampsiteID:         strconv.Itoa(unit.UnitId),
							CampsiteSiteName:   unit.Name,
							BookingDate:        bookingDate,
							BookingEndDate:     bookingDate.AddDate(0, 0, 1), // Default 1 night, Filter merges them
							BookingNights:      1,
							AvailabilityStatus: "Available",
							FacilityID:         strconv.Itoa(facilityID),
							FacilityName:       grid.Facility.FacilityName,
							BookingURL:         bookingURL,
						})
					}
				}
			}
		}
	}

	return allCampsites, nil
}

func (p *Provider) warmupCookies(ctx context.Context) error {
	// Hit the public places endpoint to trigger the AWS Application Load Balancer to issue stickounet cookies
	url := fmt.Sprintf("%s/rdr/fd/places", p.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/116.0.0.0 Safari/537.36")

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

type facilitiesResponse []struct {
	FacilityId int    `json:"FacilityId"`
	Name       string `json:"Name"`
	PlaceId    int    `json:"PlaceId"`
}

func (p *Provider) FindCampgrounds(ctx context.Context, req core.SearchRequest) ([]core.CampgroundFacility, error) {
	var facilities []core.CampgroundFacility

	err := p.warmupCookies(ctx)
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

	for _, data := range parsedResp {
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

	err := p.warmupCookies(ctx)
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
