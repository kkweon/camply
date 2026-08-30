package core

import (
	"strings"
	"testing"
)

func vehicles(n int) *int { return &n }

// TestFilterExcludeNoVehicleAccess is the decision this feature turns on: the
// filter removes only what is proven unreachable by car, and never a site the
// provider said nothing about.
func TestFilterExcludeNoVehicleAccess(t *testing.T) {
	tests := []struct {
		name       string
		access     SiteAccess
		wantKept   bool
		wantKeptOn bool // kept when the filter is enabled
	}{
		{"drive-in", SiteAccessDriveIn, true, true},
		{"walk-in", SiteAccessWalkIn, true, false},
		{"hike-in", SiteAccessHikeIn, true, false},
		{"boat-in", SiteAccessBoatIn, true, false},
		{"unrecognised non-drivable label", SiteAccessNoVehicle, true, false},
		// The whole point: missing data is surfaced, never acted on. Dropping
		// these would repeat the equipment filter's silent loss of 423 sites.
		{"unknown survives the filter", SiteAccessUnknown, true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			site := night("1", "f1", day(2026, 9, 4))
			site.SiteAccess = tt.access
			f := Filter{}

			off := f.Apply([]AvailableCampsite{site}, SearchRequest{Nights: 1})
			if got := len(off) == 1; got != tt.wantKept {
				t.Errorf("filter off: kept = %v, want %v", got, tt.wantKept)
			}

			on := f.Apply([]AvailableCampsite{site}, SearchRequest{Nights: 1, ExcludeNoVehicleAccess: true})
			if got := len(on) == 1; got != tt.wantKeptOn {
				t.Errorf("filter on: kept = %v, want %v", got, tt.wantKeptOn)
			}
		})
	}
}

// TestFilterZephyrCoveIncident pins the exact alert that prompted this work:
// campsite 10300345, a hike-in site typed identically to the campground's
// drive-in tent sites. It passes --campsite-types and must not survive
// --exclude-no-vehicle-access.
func TestFilterZephyrCoveIncident(t *testing.T) {
	site := night("10300345", "10300216", day(2026, 9, 4))
	site.CampsiteType = "TENT ONLY NONELECTRIC"
	site.SiteAccess = SiteAccessHikeIn
	site.SiteAccessRaw = "Hike-In"
	site.MaxVehicles = vehicles(0)

	f := Filter{}
	req := SearchRequest{Nights: 1, CampsiteTypes: []string{"TENT ONLY NONELECTRIC"}}

	if got := f.Apply([]AvailableCampsite{site}, req); len(got) != 1 {
		t.Fatalf("campsite-types alone should still pass this site, got %d results", len(got))
	}

	req.ExcludeNoVehicleAccess = true
	if got := f.Apply([]AvailableCampsite{site}, req); len(got) != 0 {
		t.Errorf("expected the hike-in site to be dropped, got %d results", len(got))
	}
}

func TestSiteAccessSummaryIsNeverEmpty(t *testing.T) {
	all := []SiteAccess{
		SiteAccessUnknown, SiteAccessDriveIn, SiteAccessWalkIn,
		SiteAccessHikeIn, SiteAccessBoatIn, SiteAccessNoVehicle,
	}
	for _, access := range all {
		c := AvailableCampsite{SiteAccess: access}
		if strings.TrimSpace(c.SiteAccessSummary()) == "" {
			t.Errorf("SiteAccessSummary() is empty for %v; an omitted line reads as reassurance", access)
		}
	}
}

func TestSiteAccessSummaryAndAlert(t *testing.T) {
	tests := []struct {
		name          string
		site          AvailableCampsite
		wantSummary   string
		wantAlert     string
		wantSubstring string
	}{
		{
			name:        "drive-in reports its vehicle count and raises no alert",
			site:        AvailableCampsite{SiteAccess: SiteAccessDriveIn, SiteAccessRaw: "Drive-In", MaxVehicles: vehicles(2)},
			wantSummary: "Drive-In (Max Vehicles: 2)",
			wantAlert:   "",
		},
		{
			name:        "hike-in spells out that no vehicle reaches it",
			site:        AvailableCampsite{SiteAccess: SiteAccessHikeIn, SiteAccessRaw: "Hike-In", MaxVehicles: vehicles(0)},
			wantSummary: "⚠️ HIKE-IN — no vehicle access (Max Vehicles: 0)",
			wantAlert:   "⚠️ HIKE-IN",
		},
		{
			// Boat-in is reported as itself, not folded into walk-in.
			name:        "boat-in keeps its own label",
			site:        AvailableCampsite{SiteAccess: SiteAccessBoatIn, SiteAccessRaw: "Boat-In"},
			wantSummary: "⚠️ BOAT-IN — no vehicle access",
			wantAlert:   "⚠️ BOAT-IN",
		},
		{
			name:        "an unrecognised label still reaches the reader verbatim",
			site:        AvailableCampsite{SiteAccess: SiteAccessNoVehicle, SiteAccessRaw: "Ferry-In"},
			wantSummary: "⚠️ FERRY-IN — no vehicle access",
			wantAlert:   "⚠️ FERRY-IN",
		},
		{
			name:          "unknown says so and says what to do about it",
			site:          AvailableCampsite{SiteAccess: SiteAccessUnknown},
			wantAlert:     "⚠️ UNKNOWN ACCESS",
			wantSubstring: "verify on the booking page",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantSummary != "" {
				if got := tt.site.SiteAccessSummary(); got != tt.wantSummary {
					t.Errorf("SiteAccessSummary() = %q, want %q", got, tt.wantSummary)
				}
			}
			if tt.wantSubstring != "" && !strings.Contains(tt.site.SiteAccessSummary(), tt.wantSubstring) {
				t.Errorf("SiteAccessSummary() = %q, want it to contain %q", tt.site.SiteAccessSummary(), tt.wantSubstring)
			}
			if got := tt.site.SiteAccessAlert(); got != tt.wantAlert {
				t.Errorf("SiteAccessAlert() = %q, want %q", got, tt.wantAlert)
			}
		})
	}
}

// TestSiteAccessUnknownIsTheZeroValue guards the reason the enum is ordered the
// way it is: a provider that never populates the field must not read as
// drive-in.
func TestSiteAccessUnknownIsTheZeroValue(t *testing.T) {
	var zero SiteAccess
	if zero != SiteAccessUnknown {
		t.Fatalf("zero value is %v, want SiteAccessUnknown", zero)
	}

	var site AvailableCampsite
	if site.SiteAccess.HasVehicleAccess() {
		t.Error("an unpopulated campsite reports vehicle access")
	}
	if site.SiteAccess.NoVehicleAccess() {
		t.Error("an unpopulated campsite claims to be proven unreachable")
	}
}

func TestAnalyzeSiteAccess(t *testing.T) {
	mk := func(id, facility string, access SiteAccess) AvailableCampsite {
		s := night(id, facility, day(2026, 9, 4))
		s.SiteAccess = access
		return s
	}

	sites := []AvailableCampsite{
		mk("1", "kaspian", SiteAccessWalkIn),
		mk("2", "kaspian", SiteAccessWalkIn),
		// The same campsite over two nights is still one campsite.
		mk("2", "kaspian", SiteAccessWalkIn),
		mk("3", "fallenleaf", SiteAccessDriveIn),
		mk("4", "fallenleaf", SiteAccessUnknown),
	}

	c := AnalyzeSiteAccess(sites)
	if c.TotalSites != 4 {
		t.Errorf("TotalSites = %d, want 4 (distinct campsites, not availability rows)", c.TotalSites)
	}
	if c.NoVehicle != 2 || c.Unknown != 1 {
		t.Errorf("NoVehicle = %d, Unknown = %d; want 2 and 1", c.NoVehicle, c.Unknown)
	}

	dropped := c.FacilitiesFullyDropped()
	if len(dropped) != 1 || dropped[0].FacilityID != "kaspian" {
		t.Fatalf("FacilitiesFullyDropped() = %v, want only kaspian", dropped)
	}
}
