package usedirect

import (
	"strings"

	"github.com/kkweon/camply/internal/core"
)

// classifySiteAccess names the arrival mode for a UseDirect unit.
//
// It takes the driveIn verdict FindCampsites already computed rather than
// recomputing it, so the access shown in an alert and the equipment offered for
// the same unit can never contradict each other.
//
// UseDirect never yields core.SiteAccessUnknown: unlike recreation.gov, its unit
// category and type group are always populated, so an honest answer is always
// available and the Unknown warning stays reserved for sites that truly have no
// data.
func classifySiteAccess(driveIn bool, campsiteType, campsiteUseType string) core.SiteAccess {
	if driveIn {
		return core.SiteAccessDriveIn
	}

	haystack := strings.ToLower(campsiteType + " " + campsiteUseType)
	switch {
	case strings.Contains(haystack, "boat"):
		return core.SiteAccessBoatIn
	case strings.Contains(haystack, "hike"), strings.Contains(haystack, "bike"):
		return core.SiteAccessHikeIn
	case strings.Contains(haystack, "walk"):
		return core.SiteAccessWalkIn
	default:
		// "Remote Camping" with no mode named. Non-drivable is the part that
		// matters and the part driveIn already established.
		return core.SiteAccessNoVehicle
	}
}
