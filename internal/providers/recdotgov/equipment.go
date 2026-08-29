package recdotgov

// KnownEquipment lists the permitted_equipment names recreation.gov is known to
// use.
//
// Sourced from camply's own Python EquipmentConfig.EQUIPMENT_MAPPING
// (camply/config/search_config.py) rather than invented, and confirmed against
// a 12-campground sample which produced no name outside this set.
//
// The real vocabulary is open: recreation.gov returns whatever a campground
// configured, and it varies between campgrounds in the same search. So this is
// a curated list for suggestions and warnings, never a gate — rejecting a
// genuinely valid but uncatalogued name would be worse than the problem.
var KnownEquipment = []string{
	// tent
	"Tent",
	"Small Tent",
	"Large Tent Over 9X12",
	"Large Tent Over 9X12`",
	// rv
	"RV",
	"RV/Motorhome",
	"Pop up",
	"Fifth Wheel",
	"Caravan/Camper Van",
	// trailer
	"Trailer",
	// vehicle
	"Vehicle",
	"Car",
	"Pickup Camper",
	// other
	"Boat",
	"Horse",
	"Hammock",
}
