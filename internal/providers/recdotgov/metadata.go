package recdotgov

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/kkweon/camply/internal/core"
)

// fetchMetadata paginates through the api/search/campsites endpoint to gather equipment and location data
func (p *Provider) fetchMetadata(ctx context.Context, campgroundID string) (map[string]core.AvailableCampsite, error) {
	// 1. Fetch metadata
	campsiteMap := make(map[string]core.AvailableCampsite)

	start := 0
	size := 1000
	total := 1

	for start < total {
		urlStr := fmt.Sprintf("%s://%s/api/search/campsites?start=%d&size=%d&fq=asset_id:%s&include_non_site_specific_campsites=true",
			p.apiScheme, p.apiNetLoc, start, size, campgroundID)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "camply/go-rewrite")

		resp, err := p.client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("recreation.gov API returned status: %d", resp.StatusCode)
		}

		var metaResp campsiteSearchResponse
		if err := json.NewDecoder(resp.Body).Decode(&metaResp); err != nil {
			return nil, err
		}

		total = metaResp.Total
		start += metaResp.Size

		for _, site := range metaResp.Campsites {
			var equipment []core.Equipment
			for _, eq := range site.PermittedEquipment {
				equipment = append(equipment, core.Equipment{
					EquipmentName: eq.EquipmentName,
					MaxLength:     int(eq.MaxLength),
				})
			}

			campsiteMap[site.CampsiteID] = core.AvailableCampsite{
				PermittedEquipment: equipment,
				FacilityName:       site.ParentName, // RecDotGov metadata sometimes brings back parent names
			}
		}
	}

	return campsiteMap, nil
}

type campsiteSearchResponse struct {
	Campsites []campsiteSearchItem `json:"campsites"`
	Size      int                  `json:"size"`
	Start     string               `json:"start"`
	Total     int                  `json:"total"`
}

type campsiteSearchItem struct {
	CampsiteID         string               `json:"campsite_id"`
	ParentName         string               `json:"parent_name"`
	PermittedEquipment []recdotgovEquipment `json:"permitted_equipment"`
}

type recdotgovEquipment struct {
	EquipmentName string  `json:"equipment_name"`
	MaxLength     float64 `json:"max_length"`
}
