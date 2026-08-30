package usedirect

import (
	"strings"

	"github.com/kkweon/camply/internal/core"
)

// This file is the only place UseDirect's vocabulary appears.
//
// UseDirect carries less than recreation.gov: no attributes, no equipment, just
// a unit category, a type group and a vehicle length. Where it cannot tell,
// Unknown is the honest answer — but it should be rare, because a provider that
// reports Unknown for everything makes the ⚠️ marker meaningless.

// classifyParking decides how close a car gets to a unit.
//
// The category and type group are checked BEFORE VehicleLength, and that order
// matters: a Bike In unit reports VehicleLength 21. It is the same trap as
// recreation.gov's "Max Num of Vehicles: 1" on 54 Lodgepole hike-in sites — an
// explicit access label outranks a numeric parking hint, on both providers.
func classifyParking(campsiteType, campsiteUseType string, vehicleLength int) (core.Parking, string) {
	haystack := strings.ToLower(campsiteType + " " + campsiteUseType)

	switch {
	case strings.Contains(haystack, "boat"):
		return core.ParkingNone, "unit type: " + campsiteUseType
	case strings.Contains(haystack, "hike"), strings.Contains(haystack, "bike"):
		return core.ParkingNone, "unit type: " + campsiteUseType
	case strings.Contains(haystack, "walk"):
		return core.ParkingWalk, "unit type: " + campsiteUseType
	case strings.Contains(haystack, "remote"):
		return core.ParkingWalk, "unit category: " + campsiteType
	case vehicleLength > 0:
		return core.ParkingAtSite, "VehicleLength > 0"
	default:
		return core.ParkingUnknown, ""
	}
}

// classifyPermits decides what a unit accepts.
//
// UseDirect names no equipment, so this is derived from the category and type
// group. A tent fits wherever the unit is a campsite, a tent site, a hook-up
// site or a primitive one, and anywhere no vehicle length is quoted — a hook-up
// site takes an RV and a tent both, which is what "Campsite" means on this
// provider and what STANDARD means on the other.
//
// An RV is only permitted where a car actually stops at the unit: a Bike In unit
// reports VehicleLength 21, and advertising RV equipment for it would contradict
// the parking the same adapter reports.
func classifyPermits(campsiteType, campsiteUseType string, vehicleLength int, parking core.Parking) (core.Permitted, string) {
	category := strings.ToLower(campsiteType)
	useType := strings.ToLower(campsiteUseType)

	if strings.Contains(category, "lodging") {
		return core.PermitsCabin, "unit category: " + campsiteType
	}

	var permits core.Permitted
	for _, token := range []string{"tent", "camp", "site", "hook up", "primitive"} {
		if strings.Contains(useType, token) {
			permits |= core.PermitsTent
			break
		}
	}
	if vehicleLength == 0 {
		permits |= core.PermitsTent
	}
	if vehicleLength > 0 && parking == core.ParkingAtSite {
		permits |= core.PermitsRV
	}
	if permits == core.PermittedUnknown {
		return permits, ""
	}
	return permits, "unit type: " + campsiteUseType
}

// classifyHookups reads what little UseDirect says about utilities. Only the
// "Hook Up Camping" category is a positive signal; everything else is Unknown
// rather than a claim of absence.
func classifyHookups(campsiteType string) core.Hookups {
	if strings.Contains(strings.ToLower(campsiteType), "hook up") {
		return core.Hookups{Electric: core.TriYes, Water: core.TriYes}
	}
	return core.Hookups{}
}
