package usedirect

import (
	"testing"

	"github.com/kkweon/camply/internal/core"
)

func TestClassifySiteAccess(t *testing.T) {
	tests := []struct {
		name            string
		driveIn         bool
		campsiteType    string
		campsiteUseType string
		want            core.SiteAccess
	}{
		{"drive-in unit", true, "Tent Site", "Camping", core.SiteAccessDriveIn},
		{"boat in", false, "Remote Camping", "Boat In", core.SiteAccessBoatIn},
		{"hike and bike", false, "Remote Camping", "Hike & Bike", core.SiteAccessHikeIn},
		{"bike in counts as hike-in", false, "Remote Camping", "Bike In", core.SiteAccessHikeIn},
		{"walk in", false, "Remote Camping", "Walk In", core.SiteAccessWalkIn},
		{"remote with no mode named", false, "Remote Camping", "", core.SiteAccessNoVehicle},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifySiteAccess(tt.driveIn, tt.campsiteType, tt.campsiteUseType)
			if got != tt.want {
				t.Errorf("classifySiteAccess(%v, %q, %q) = %v, want %v",
					tt.driveIn, tt.campsiteType, tt.campsiteUseType, got, tt.want)
			}
			// ReserveCalifornia always has the category and type group
			// populated, so it always owes a definite answer. If it could
			// report Unknown, every RC alert would carry the warning label and
			// the label would stop meaning anything.
			if got == core.SiteAccessUnknown {
				t.Error("UseDirect must never report Unknown")
			}
		})
	}
}
