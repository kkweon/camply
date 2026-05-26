package recdotgov

import (
	"context"
	"fmt"

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

		var metaResp campsiteSearchResponse
		if err := p.getJSON(ctx, urlStr, recdotgovHeaders, &metaResp); err != nil {
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
	ParentName         string               `json:"asset_name"`
	PermittedEquipment []recdotgovEquipment `json:"permitted_equipment"`
}

type recdotgovEquipment struct {
	EquipmentName string  `json:"equipment_name"`
	MaxLength     float64 `json:"max_length"`
}
