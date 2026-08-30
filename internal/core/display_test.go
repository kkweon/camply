package core

import (
	"strings"
	"testing"
)

func TestEquipmentSummary(t *testing.T) {
	tests := []struct {
		name string
		site AvailableCampsite
		want string
	}{
		{
			name: "lists names and lengths",
			site: AvailableCampsite{PermittedEquipment: []Equipment{
				{EquipmentName: "Tent"},
				{EquipmentName: "RV", MaxLength: 30},
			}},
			want: "Tent, RV (max 30ft)",
		},
		{
			// The filter let this through without ever matching it, and this is
			// the only place a reader can learn that.
			name: "unverified says so and says what to do",
			site: AvailableCampsite{EquipmentUnverified: true},
			want: "⚠️ UNKNOWN — the provider reports no equipment data; verify on the booking page before reserving",
		},
		{
			// No filter was active, so nothing was assumed — but the line is
			// still never blank.
			name: "no data and no filter is stated plainly",
			site: AvailableCampsite{},
			want: "Not reported",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.site.EquipmentSummary(); got != tt.want {
				t.Errorf("EquipmentSummary() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSummariesAreNeverEmpty(t *testing.T) {
	all := []SiteAccess{
		SiteAccessUnknown, SiteAccessDriveIn, SiteAccessWalkIn,
		SiteAccessHikeIn, SiteAccessBoatIn, SiteAccessNoVehicle,
	}
	for _, access := range all {
		for _, unverified := range []bool{false, true} {
			c := AvailableCampsite{SiteAccess: access, EquipmentUnverified: unverified}
			if strings.TrimSpace(c.SiteAccessSummary()) == "" {
				t.Errorf("SiteAccessSummary() empty for %v", access)
			}
			if strings.TrimSpace(c.EquipmentSummary()) == "" {
				t.Errorf("EquipmentSummary() empty for %v/unverified=%v", access, unverified)
			}
		}
	}
}

// TestWarningsCarryEveryDoubt guards against a site being doubtful on two counts
// and only one of them reaching the title.
func TestWarningsCarryEveryDoubt(t *testing.T) {
	both := AvailableCampsite{
		SiteAccess:          SiteAccessHikeIn,
		SiteAccessRaw:       "Hike-In",
		EquipmentUnverified: true,
	}
	got := both.WarningPrefix()
	for _, want := range []string{"⚠️ HIKE-IN", "⚠️ NO EQUIPMENT DATA"} {
		if !strings.Contains(got, want) {
			t.Errorf("WarningPrefix() = %q, want it to contain %q", got, want)
		}
	}

	clean := AvailableCampsite{SiteAccess: SiteAccessDriveIn}
	if clean.WarningPrefix() != "" {
		t.Errorf("a confirmed drive-in site with equipment data should carry no warning, got %q",
			clean.WarningPrefix())
	}
}
