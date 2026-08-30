package recdotgov

import "github.com/kkweon/camply/internal/core"

// KnownCampsiteTypes lists the campsite_type values recreation.gov is known to
// return, gathered from a 13-campground sample of the availability API.
//
// This field does NOT reliably separate drive-in sites from walk-in ones,
// though it was once used that way. Measured against the live API it disagrees
// with the Site Access attribute in both directions: Zephyr Cove types all 47
// of its hike-in sites TENT ONLY NONELECTRIC, and Lodgepole types 3 of its
// Drive-In sites WALK TO. Use SiteAccess for that question; this list is for
// the coarse cut between tent, RV and cabin.
//
// Like the equipment names, the real set is open — a campground can return
// anything — so this is a curated list for suggestions and warnings, not a gate.
var KnownCampsiteTypes = []string{
	"STANDARD NONELECTRIC",
	"STANDARD ELECTRIC",
	"TENT ONLY NONELECTRIC",
	"TENT ONLY ELECTRIC",
	"RV NONELECTRIC",
	"RV ELECTRIC",
	"WALK TO",
	"HIKE TO",
	"BOAT IN",
	"CABIN NONELECTRIC",
	"CABIN ELECTRIC",
	"GROUP STANDARD NONELECTRIC",
	"GROUP TENT ONLY AREA NONELECTRIC",
	"GROUP HIKE TO",
	"MANAGEMENT",
}

// The campsite_type values partitioned by whether a car reaches them.
//
// These back only the fallback steps of classifySiteAccess: the Site Access
// attribute outranks them wherever it is present, because the two genuinely
// disagree. Lodgepole types 5 of its Hike-In/Walk-In sites TENT ONLY
// NONELECTRIC, and 3 of its Drive-In sites WALK TO.
//
// campsiteTypeUnclassified keeps the partition total. TestCampsiteTypePartition
// asserts every KnownCampsiteTypes value lands in exactly one of the three, so
// a type added to that list cannot slip through unconsidered.
var (
	// noVehicleCampsiteTypes name their own arrival mode, so the fallback can
	// report which one rather than a bare "no vehicle".
	noVehicleCampsiteTypes = map[string]core.SiteAccess{
		"WALK TO":       core.SiteAccessWalkIn,
		"HIKE TO":       core.SiteAccessHikeIn,
		"GROUP HIKE TO": core.SiteAccessHikeIn,
		"BOAT IN":       core.SiteAccessBoatIn,
	}

	driveInCampsiteTypes = map[string]bool{
		"STANDARD NONELECTRIC":             true,
		"STANDARD ELECTRIC":                true,
		"TENT ONLY NONELECTRIC":            true,
		"TENT ONLY ELECTRIC":               true,
		"RV NONELECTRIC":                   true,
		"RV ELECTRIC":                      true,
		"CABIN NONELECTRIC":                true,
		"CABIN ELECTRIC":                   true,
		"GROUP STANDARD NONELECTRIC":       true,
		"GROUP TENT ONLY AREA NONELECTRIC": true,
	}

	// campsiteTypeUnclassified says nothing about access. MANAGEMENT is a
	// staff site, not a camper's choice.
	campsiteTypeUnclassified = map[string]bool{
		"MANAGEMENT": true,
	}
)
