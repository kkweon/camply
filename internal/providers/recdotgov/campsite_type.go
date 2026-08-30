package recdotgov

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
