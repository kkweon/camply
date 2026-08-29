package usedirect

// UseDirect grids carry no equipment data. These four names are synthesized in
// FindCampsites from a unit's category, type group and vehicle length, so this
// list is closed by construction: nothing else can ever be emitted.
//
// The producer references these same constants, so a validator built on
// SynthesizedEquipment cannot drift away from what searches actually return.
const (
	EquipmentTent    = "Tent"
	EquipmentRV      = "RV"
	EquipmentTrailer = "Trailer"
	EquipmentVehicle = "Vehicle"
)

// SynthesizedEquipment is every equipment name this provider can report.
var SynthesizedEquipment = []string{
	EquipmentTent,
	EquipmentRV,
	EquipmentTrailer,
	EquipmentVehicle,
}
