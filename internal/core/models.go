package core

import "time"

// Facility is the campground a site belongs to.
type Facility struct {
	ID               string
	Name             string
	RecreationArea   string
	RecreationAreaID string
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
	// ExcludeNoVehicleAccess drops sites proven to need a walk from the car, or
	// to have no car access at all. Sites whose access the provider did not
	// report are kept, not dropped — see Parking.RequiresWalk.
	ExcludeNoVehicleAccess bool
	Equipment              []Equipment
	Query                  string
	State                  string
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
