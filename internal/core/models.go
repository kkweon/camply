package core

import "time"

// Facility is the campground a site belongs to.
//
// Name alone does not locate a campground. A reader who is watching a dozen of
// them cannot place "KIRK CREEK CAMPGROUND" from the name, so the recreation
// area and the town it sits in travel with it and are what the alert leads on.
type Facility struct {
	ID               string
	Name             string
	RecreationArea   string
	RecreationAreaID string
	// Location is the campground's town as the provider gives it, e.g.
	// "Zephyr Cove, NV". Empty when the provider does not report one.
	Location string
}

// Site is everything true of a campsite regardless of when you book it.
//
// It is separate from Availability because these facts do not vary by night.
// Zephyr Cove's 47 sites over a 30-day search used to mean 1410 copies of the
// same shelter, parking and equipment data, copied again for every sliding
// window of consecutive nights.
type Site struct {
	ID       string
	Name     string // the number on the post, e.g. "36"
	Loop     string
	Facility Facility

	// The three axes. See domain.go.
	Permits     Permitted
	Parking     Parking
	Hookups     Hookups
	SharedWater Tri // a spigot or tap somewhere in the campground

	Equipment  []Equipment
	WalkFeet   *int   // distance from parking, when the provider reports it
	Waterfront string // "Riverfront"/"Lakefront" — a location, not a water source
	Amps       *int   // electrical service, when reported (30/50)
	// MaxVehicles is how many vehicles the site parks. A pointer because "zero
	// vehicles" and "not reported" are different answers.
	MaxVehicles *int

	// Basis records which provider field decided each axis, so an alert can
	// explain itself and a misclassification is traceable to its source rather
	// than to the classifier as a whole.
	PermitsBasis string
	ParkingBasis string

	// AccessLabel is the provider's own word for how the site is reached
	// ("Hike-In", "Walk-In"). It is kept verbatim so a label outside the known
	// vocabulary still reaches the reader intact.
	AccessLabel string

	// RawType is the provider's campsite type, retained for --campsite-types,
	// which stays available as a raw escape hatch.
	RawType string
	UseType string

	MinOccupancy int
	MaxOccupancy int
	BookingURL   string
}

// Availability is one bookable window at a Site.
type Availability struct {
	Site   *Site
	Start  time.Time
	End    time.Time
	Nights int
	Status string

	// EquipmentUnverified is set when an equipment filter was active and the
	// provider reported no equipment for this site, so it survived the filter
	// without ever being shown to match it.
	//
	// It lives here rather than on Site because Sites are shared by pointer
	// across every night, and because the doubt is created by the filter: with
	// no filter, absent equipment data misleads nobody.
	EquipmentUnverified bool

	// ParkingRequested is set when a --parking filter was active and this
	// site's known parking level was one the user named. A proven walk the
	// user asked for is information, not an alarm: it downgrades the access
	// warning. ParkingUnknown never earns it — unknown stays flagged.
	ParkingRequested bool
}

type Equipment struct {
	EquipmentName string
	MaxLength     int
}

// SearchRequest defines the unified parameters across all providers
type SearchRequest struct {
	StartDates     []time.Time
	EndDates       []time.Time
	WeekendsOnly   bool
	Nights         int
	Campgrounds    []string
	RecreationArea string
	Campsites      []string
	CampsiteTypes  []string

	// MinVehicleLength filters UseDirect results on the VehicleLength its grid
	// response reports per unit. It has no recreation.gov equivalent.
	MinVehicleLength int
	// Shelter is the kind of camping this search is for: one choice, not a
	// list. It also decides what "water" means in Hookups — for an RV a pipe at
	// the site, for a tent somewhere to fill a jug.
	Shelter Shelter
	// Parking keeps only sites whose parking is one of these. A site whose
	// parking the provider never reported is kept regardless: missing data is
	// surfaced, never acted on.
	Parking []Parking
	// Hookups is additive: naming two requires both.
	Hookups   []Hookup
	Equipment []Equipment
	Query     string
	State     string
}

type CampgroundFacility struct {
	FacilityID       string
	FacilityName     string
	RecreationArea   string
	RecreationAreaID string
}

type RecreationArea struct {
	RecreationAreaID       string
	RecreationArea         string
	RecreationAreaLocation string
}
