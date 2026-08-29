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
	EquipmentID    string
	Equipment      []Equipment
	Query          string
	State          string
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
