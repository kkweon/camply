package core

import "time"

// AvailableCampsite represents a single available booking slot
type AvailableCampsite struct {
	CampsiteID         string
	BookingDate        time.Time
	BookingEndDate     time.Time
	BookingNights      int
	CampsiteSiteName   string
	CampsiteLoopName   string
	CampsiteType       string
	MinOccupancy       int
	MaxOccupancy       int
	CampsiteUseType    string
	AvailabilityStatus string
	RecreationArea     string
	RecreationAreaID   string
	FacilityName       string
	FacilityID         string
	BookingURL         string
	PermittedEquipment []Equipment

	// SiteAccess is how the site is reached. The zero value is
	// SiteAccessUnknown, so a provider that leaves it alone cannot be read as
	// drive-in.
	SiteAccess SiteAccess
	// SiteAccessRaw is the provider's own label, kept verbatim so a value
	// outside the known vocabulary still reaches the reader intact.
	SiteAccessRaw string
	// MaxVehicles is how many vehicles the site parks. It is a pointer because
	// "zero vehicles" and "not reported" are different answers and only the
	// first one means anything.
	MaxVehicles *int
	// EquipmentUnverified is set when an equipment filter was active and the
	// provider reported no equipment for this site, so it survived the filter
	// without ever being shown to match it.
	//
	// Set by Filter.Apply rather than by a provider: the doubt is created by
	// the filter. With no filter, absent equipment data misleads nobody, and
	// flagging it there would mark 18% of Meeks Bay's sites for no actionable
	// reason — noise that erodes the warnings that do matter.
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
	// ExcludeNoVehicleAccess drops sites proven unreachable by car. Sites whose
	// access the provider did not report are kept, not dropped — see
	// SiteAccess.NoVehicleAccess.
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
