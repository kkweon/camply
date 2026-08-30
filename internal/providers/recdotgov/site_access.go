package recdotgov

import (
	"strconv"
	"strings"

	"github.com/kkweon/camply/internal/core"
)

// Attribute names on the api/search/campsites response.
//
// attrMaxVehicleLength is named here only to be ignored: it is a different
// attribute from attrMaxNumVehicles and reads 0 on 27 genuine drive-in sites at
// Manzanita Lake and on all 9 Kaspian sites. Filtering on it deletes valid
// results, so it must never be mistaken for the vehicle count.
const (
	attrSiteAccess     = "Site Access"
	attrMaxNumVehicles = "Max Num of Vehicles"
)

// siteAccessLabels maps recreation.gov's Site Access vocabulary.
//
// The vocabulary is open, so an unlisted label is not an error — see
// classifySiteAccess, which treats anything that is not Drive-In as
// non-drivable rather than guessing a mode for it.
var siteAccessLabels = map[string]core.SiteAccess{
	"drive-in": core.SiteAccessDriveIn,
	"drive in": core.SiteAccessDriveIn,
	"walk-in":  core.SiteAccessWalkIn,
	"walk in":  core.SiteAccessWalkIn,
	"hike-in":  core.SiteAccessHikeIn,
	"hike in":  core.SiteAccessHikeIn,
	"boat-in":  core.SiteAccessBoatIn,
	"boat in":  core.SiteAccessBoatIn,
}

// classifySiteAccess decides whether a car reaches a campsite.
//
// The signals are ranked by how well they held up against the live API, not by
// convenience:
//
//  1. Site Access is authoritative wherever it is present. It outranks the
//     vehicle count because Lodgepole reports "Max Num of Vehicles: 1" on all 54
//     of its Hike-In and Walk-In sites — that is trailhead parking, not access.
//  2. A vehicle count of exactly 0 is conclusive. Only 0 is: >= 1 proves
//     nothing, as Kaspian's 9 WALK TO sites all report 1.
//  3. Some campsite types name their own arrival mode. This is what catches
//     Kaspian, which omits Site Access entirely.
//  4. A vehicle count with a drive-in type is the weakest evidence, and is
//     accepted only together. It is what keeps Manzanita Lake's 170 sites and
//     Nevada Beach's 54 from all reading Unknown, which would drown the warning
//     label in noise.
//  5. Otherwise Unknown. Unknown is a real answer here, not a failure: it is
//     reported to the user rather than resolved by guessing.
func classifySiteAccess(rawSiteAccess string, maxVehicles *int, campsiteType string) core.SiteAccess {
	// Meeks Bay returns an empty string for sites it has no answer for, which
	// is absence, not a label.
	if raw := strings.TrimSpace(rawSiteAccess); raw != "" {
		if known, ok := siteAccessLabels[strings.ToLower(raw)]; ok {
			return known
		}
		// A label outside the known set still means "not Drive-In". Honouring
		// that without inventing a mode for it is why SiteAccessNoVehicle
		// exists.
		return core.SiteAccessNoVehicle
	}

	if maxVehicles != nil && *maxVehicles == 0 {
		return core.SiteAccessNoVehicle
	}

	normalizedType := strings.ToUpper(strings.TrimSpace(campsiteType))
	if access, ok := noVehicleCampsiteTypes[normalizedType]; ok {
		return access
	}

	if maxVehicles != nil && *maxVehicles >= 1 && driveInCampsiteTypes[normalizedType] {
		return core.SiteAccessDriveIn
	}

	return core.SiteAccessUnknown
}

// parseMaxVehicles returns nil for an absent or unparseable value, keeping
// "not reported" distinct from "zero vehicles".
func parseMaxVehicles(raw string) *int {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return nil
	}
	return &n
}
