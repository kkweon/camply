package recdotgov

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/kkweon/camply/internal/core"
)

// siteMetadata is one campsite as the search endpoint describes it, before the
// availability endpoint adds its campsite_type.
type siteMetadata struct {
	Name         string
	FacilityName string
	Equipment    []core.Equipment
	Attributes   attributes
	Notices      string
}

// fetchMetadata paginates through the api/search/campsites endpoint to gather equipment,
// attributes and location data.
func (p *Provider) fetchMetadata(ctx context.Context, campgroundID string) (map[string]siteMetadata, error) {
	campsiteMap := make(map[string]siteMetadata)

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

			campsiteMap[site.CampsiteID] = siteMetadata{
				Name:         site.SiteName,
				FacilityName: site.ParentName, // RecDotGov metadata sometimes brings back parent names
				Equipment:    equipment,
				Attributes:   newAttributes(site.Attributes),
				Notices:      site.noticeText(),
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
	SiteName           string               `json:"name"`
	ParentName         string               `json:"asset_name"`
	PermittedEquipment []recdotgovEquipment `json:"permitted_equipment"`
	Attributes         []recdotgovAttribute `json:"attributes"`
	Notices            []recdotgovNotice    `json:"notices"`
}

type recdotgovEquipment struct {
	EquipmentName string  `json:"equipment_name"`
	MaxLength     float64 `json:"max_length"`
}

type recdotgovAttribute struct {
	AttributeName  string `json:"attribute_name"`
	AttributeValue string `json:"attribute_value"`
}

type recdotgovNotice struct {
	Text string `json:"text"`
	Type string `json:"type"`
}

var noticeMarkup = regexp.MustCompile(`<[^>]*>`)

// noticeText flattens the notices into one searchable string. Some facts live
// only here: Zephyr Cove records "NO VEHICLE ACCESS" and a half-mile hike
// distance in prose, with no corresponding attribute.
func (c campsiteSearchItem) noticeText() string {
	parts := make([]string, 0, len(c.Notices))
	for _, n := range c.Notices {
		parts = append(parts, noticeMarkup.ReplaceAllString(n.Text, " "))
	}
	return strings.Join(parts, " ")
}
