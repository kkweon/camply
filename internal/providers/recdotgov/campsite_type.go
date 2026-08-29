package recdotgov

// KnownCampsiteTypes lists the campsite_type values recreation.gov is known to
// return, gathered from a 13-campground sample of the availability API.
//
// This is the field that separates drive-in sites from walk-in ones — WALK TO,
// HIKE TO and BOAT IN are not reachable by car, while STANDARD NONELECTRIC and
// TENT ONLY NONELECTRIC are. Equipment cannot express that: a WALK TO site
// still permits a tent.
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
