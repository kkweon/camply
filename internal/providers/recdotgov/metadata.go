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

			// Access is not classified here: the rule also consults
			// campsite_type, which only the availability response carries. The
			// raw attributes travel to the hydrate step in recdotgov.go, where
			// both halves are finally on one struct.
			siteAccessRaw, maxVehicles := site.accessAttributes()

			campsiteMap[site.CampsiteID] = core.AvailableCampsite{
				PermittedEquipment: equipment,
				FacilityName:       site.ParentName, // RecDotGov metadata sometimes brings back parent names
				SiteAccessRaw:      siteAccessRaw,
				MaxVehicles:        maxVehicles,
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
	Attributes         []recdotgovAttribute `json:"attributes"`
}

type recdotgovAttribute struct {
	AttributeName  string `json:"attribute_name"`
	AttributeValue string `json:"attribute_value"`
}

// accessAttributes pulls the two attributes that bear on vehicle access out of
// the flat, per-campground attribute list.
func (c campsiteSearchItem) accessAttributes() (siteAccess string, maxVehicles *int) {
	for _, attr := range c.Attributes {
		switch attr.AttributeName {
		case attrSiteAccess:
			siteAccess = attr.AttributeValue
		case attrMaxNumVehicles:
			maxVehicles = parseMaxVehicles(attr.AttributeValue)
		}
	}
	return siteAccess, maxVehicles
}

type recdotgovEquipment struct {
	EquipmentName string  `json:"equipment_name"`
	MaxLength     float64 `json:"max_length"`
}
