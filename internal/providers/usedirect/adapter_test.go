package usedirect

import (
	"testing"

	"github.com/kkweon/camply/internal/core"
)

// TestClassifyParkingPrecedence pins the rule both providers share: an explicit
// access label outranks a numeric parking hint.
//
// Verified live — a Bike In unit reports VehicleLength 21, exactly as Lodgepole
// reports "Max Num of Vehicles: 1" on 54 hike-in sites. Checking the length
// first would call both of them drive-in.
func TestClassifyParkingPrecedence(t *testing.T) {
	tests := []struct {
		name          string
		category      string
		typeGroup     string
		vehicleLength int
		want          core.Parking
	}{
		{"bike in reporting a vehicle length", "Remote Camping", "Bike In", 21, core.ParkingNone},
		{"hike & bike", "Remote Camping", "Hike & Bike", 0, core.ParkingNone},
		{"boat in", "Remote Camping", "Boat In", 0, core.ParkingNone},
		{"walk in", "Remote Camping", "Walk In", 0, core.ParkingWalk},
		{"remote with no mode named", "Remote Camping", "", 0, core.ParkingWalk},
		{"a hook-up site parks a car at the unit", "Hook Up Camping", "Hook Up (E/W)", 36, core.ParkingAtSite},
		{"a tent site with a vehicle length", "Camping", "Tent Site", 18, core.ParkingAtSite},
		{"nothing to go on", "Camping", "Campsite", 0, core.ParkingUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := classifyParking(tt.category, tt.typeGroup, tt.vehicleLength)
			if got != tt.want {
				t.Errorf("classifyParking(%q, %q, %d) = %v, want %v",
					tt.category, tt.typeGroup, tt.vehicleLength, got, tt.want)
			}
		})
	}
}

func TestClassifyPermits(t *testing.T) {
	tests := []struct {
		name          string
		category      string
		typeGroup     string
		vehicleLength int
		parking       core.Parking
		want          core.Permitted
	}{
		{
			// The same reading as STANDARD on recreation.gov: a hook-up site
			// takes an RV and a tent both.
			name: "hook-up site", category: "Hook Up Camping", typeGroup: "Hook Up (E/W)",
			vehicleLength: 36, parking: core.ParkingAtSite,
			want: core.PermitsTent | core.PermitsRV,
		},
		{
			name: "tent site", category: "Camping", typeGroup: "Tent Site",
			vehicleLength: 18, parking: core.ParkingAtSite,
			want: core.PermitsTent | core.PermitsRV,
		},
		{
			// No car reaches it, so no RV may be advertised even though the API
			// quotes a length of 21.
			//
			// It resolves to Unknown rather than Tent, which is what this
			// provider has always done: the type group "Bike In" matches none of
			// the tent tokens and a length is quoted, so nothing is claimed. A
			// bike-in site plainly takes a tent, so this is a gap worth closing
			// — but closing it changes results, which R1 does not do.
			name: "bike in with a quoted length", category: "Remote Camping", typeGroup: "Bike In",
			vehicleLength: 21, parking: core.ParkingNone,
			want: core.PermittedUnknown,
		},
		{
			name: "lodging", category: "Lodging", typeGroup: "Cabin",
			vehicleLength: 0, parking: core.ParkingUnknown,
			want: core.PermitsCabin,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := classifyPermits(tt.category, tt.typeGroup, tt.vehicleLength, tt.parking)
			if got != tt.want {
				t.Errorf("classifyPermits() = %b, want %b", got, tt.want)
			}
		})
	}
}

// UseDirect always has a category and a type group, so Unknown should be rare:
// a provider that reports it for everything makes the ⚠️ marker meaningless.
func TestHookupsAreUnknownRatherThanDenied(t *testing.T) {
	if got := classifyHookups("Camping"); got.Electric != core.TriUnknown {
		t.Errorf("Electric = %v, want TriUnknown — this provider does not say", got.Electric)
	}
	if got := classifyHookups("Hook Up Camping"); got.Electric != core.TriYes {
		t.Errorf("Electric = %v, want TriYes", got.Electric)
	}
}
