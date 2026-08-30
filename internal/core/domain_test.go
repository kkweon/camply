package core

import (
	"strings"
	"testing"
)

func vehicles(n int) *int { return &n }

// TestUnknownIsTheZeroValue guards the reason every enum here is ordered the way
// it is: a provider that never populates a field must not read as a confident
// answer.
func TestUnknownIsTheZeroValue(t *testing.T) {
	var s Site
	if s.Parking != ParkingUnknown {
		t.Errorf("Parking zero value = %v, want ParkingUnknown", s.Parking)
	}
	if s.Permits != PermittedUnknown {
		t.Errorf("Permits zero value = %v, want PermittedUnknown", s.Permits)
	}
	if s.Hookups.Electric != TriUnknown || s.SharedWater != TriUnknown {
		t.Error("hookups and shared water must default to unknown, not to no")
	}
	if s.Parking.ReachableByCar() {
		t.Error("an unpopulated site reports its car reaches it")
	}
	if s.Parking.RequiresWalk() {
		t.Error("an unpopulated site claims to be proven unreachable")
	}
}

// TestPermittedIsASetNotAChoice is the distinction an earlier draft of this
// model got wrong. A STANDARD site takes both a tent and an RV; nobody goes
// "standard camping". Collapsing the two would have hidden 264 sites from a tent
// search, 260 of which take tents.
func TestPermittedIsASetNotAChoice(t *testing.T) {
	standard := PermitsTent | PermitsRV

	if !standard.Allows(ShelterTent) {
		t.Error("a site taking tents and RVs must answer yes to a tent camper")
	}
	if !standard.Allows(ShelterRV) {
		t.Error("the same site must answer yes to an RV camper")
	}
	if standard.Allows(ShelterCabin) {
		t.Error("it takes no cabin")
	}

	rvOnly := PermitsRV
	if rvOnly.Allows(ShelterTent) {
		t.Error("an RV-only site must not answer yes to a tent camper")
	}

	// Set membership and the filter's question are different, and named
	// differently: an unknown set is not known to allow anything, but it is not
	// a "no" either, so a filter must not drop it.
	if PermittedUnknown.Allows(ShelterTent) {
		t.Error("an unknown set is not known to allow a tent")
	}
	if !PermittedUnknown.CouldAllow(ShelterTent) {
		t.Error("an unknown permitted set must not exclude a camper")
	}
	if standard.CouldAllow(ShelterCabin) {
		t.Error("a known set that excludes cabins must still exclude them")
	}
}

// TestParkingSplitsUnknownFromProven is the whole reason Parking has four values
// rather than a boolean.
func TestParkingSplitsUnknownFromProven(t *testing.T) {
	tests := []struct {
		p        Parking
		reach    bool
		needWalk bool
	}{
		{ParkingAtSite, true, false},
		{ParkingWalk, false, true},
		{ParkingNone, false, true},
		// Neither: the split a drive-in/not flag cannot express.
		{ParkingUnknown, false, false},
	}
	for _, tt := range tests {
		if got := tt.p.ReachableByCar(); got != tt.reach {
			t.Errorf("%v.ReachableByCar() = %v, want %v", tt.p, got, tt.reach)
		}
		if got := tt.p.RequiresWalk(); got != tt.needWalk {
			t.Errorf("%v.RequiresWalk() = %v, want %v", tt.p, got, tt.needWalk)
		}
	}
}

func TestSummariesAreNeverEmpty(t *testing.T) {
	all := []Parking{ParkingUnknown, ParkingAtSite, ParkingWalk, ParkingNone}
	for _, p := range all {
		for _, unverified := range []bool{false, true} {
			a := Availability{Site: &Site{Parking: p}, EquipmentUnverified: unverified}
			if strings.TrimSpace(a.SiteAccessSummary()) == "" {
				t.Errorf("SiteAccessSummary() empty for %v; an omitted line reads as reassurance", p)
			}
			if strings.TrimSpace(a.EquipmentSummary()) == "" {
				t.Errorf("EquipmentSummary() empty for %v/unverified=%v", p, unverified)
			}
		}
	}
}

func TestSummaryAndAlertWording(t *testing.T) {
	tests := []struct {
		name        string
		booking     Availability
		wantSummary string
		wantAlert   string
		wantContain string
	}{
		{
			name:        "a car reaching the site reports its vehicle count and raises no alert",
			booking:     Availability{Site: &Site{Parking: ParkingAtSite, AccessLabel: "Drive-In", MaxVehicles: vehicles(2)}},
			wantSummary: "Drive-In (Max Vehicles: 2)",
		},
		{
			name:        "the reported incident",
			booking:     Availability{Site: &Site{Parking: ParkingNone, AccessLabel: "Hike-In", MaxVehicles: vehicles(0)}},
			wantSummary: "⚠️ HIKE-IN — no vehicle access (Max Vehicles: 0)",
			wantAlert:   "⚠️ HIKE-IN",
		},
		{
			// The provider's own word survives even outside the known
			// vocabulary, so a reader is never told a label camply invented.
			name:        "an unrecognised label reaches the reader verbatim",
			booking:     Availability{Site: &Site{Parking: ParkingWalk, AccessLabel: "Ferry-In"}},
			wantSummary: "⚠️ FERRY-IN — no vehicle access",
			wantAlert:   "⚠️ FERRY-IN",
		},
		{
			name:        "unknown says so and says what to do about it",
			booking:     Availability{Site: &Site{}},
			wantAlert:   "⚠️ UNKNOWN ACCESS",
			wantContain: "verify on the booking page",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantSummary != "" {
				if got := tt.booking.SiteAccessSummary(); got != tt.wantSummary {
					t.Errorf("SiteAccessSummary() = %q, want %q", got, tt.wantSummary)
				}
			}
			if tt.wantContain != "" && !strings.Contains(tt.booking.SiteAccessSummary(), tt.wantContain) {
				t.Errorf("SiteAccessSummary() = %q, want it to contain %q",
					tt.booking.SiteAccessSummary(), tt.wantContain)
			}
			if got := tt.booking.SiteAccessAlert(); got != tt.wantAlert {
				t.Errorf("SiteAccessAlert() = %q, want %q", got, tt.wantAlert)
			}
		})
	}
}

// TestWarningsCarryEveryDoubt guards against a site being doubtful on two counts
// and only one of them reaching the title.
func TestWarningsCarryEveryDoubt(t *testing.T) {
	both := Availability{
		Site:                &Site{Parking: ParkingWalk, AccessLabel: "Hike-In"},
		EquipmentUnverified: true,
	}
	got := both.WarningPrefix()
	for _, want := range []string{"⚠️ HIKE-IN", "⚠️ NO EQUIPMENT DATA"} {
		if !strings.Contains(got, want) {
			t.Errorf("WarningPrefix() = %q, want it to contain %q", got, want)
		}
	}

	clean := Availability{Site: &Site{Parking: ParkingAtSite}}
	if clean.WarningPrefix() != "" {
		t.Errorf("a site a car reaches, with equipment data, should carry no warning, got %q",
			clean.WarningPrefix())
	}
}

func TestAnalyzeParking(t *testing.T) {
	mk := func(id, facility string, p Parking) Availability {
		a := night(id, facility, day(2026, 9, 4))
		a.Site.Parking = p
		return a
	}

	availabilities := []Availability{
		mk("1", "kaspian", ParkingWalk),
		mk("2", "kaspian", ParkingWalk),
		// The same campsite over two nights is still one campsite.
		mk("2", "kaspian", ParkingWalk),
		mk("3", "fallenleaf", ParkingAtSite),
		mk("4", "fallenleaf", ParkingUnknown),
	}

	c := AnalyzeParking(availabilities)
	if c.TotalSites != 4 {
		t.Errorf("TotalSites = %d, want 4 (distinct campsites, not availability rows)", c.TotalSites)
	}
	if c.RequiresWalk != 2 || c.Unknown != 1 {
		t.Errorf("RequiresWalk = %d, Unknown = %d; want 2 and 1", c.RequiresWalk, c.Unknown)
	}

	dropped := c.FacilitiesFullyDropped()
	if len(dropped) != 1 || dropped[0].FacilityID != "kaspian" {
		t.Fatalf("FacilitiesFullyDropped() = %v, want only kaspian", dropped)
	}
}

// TestFilterParking is the decision this feature turns on: the filter removes
// only what is proven, and never a site the provider said nothing about.
func TestFilterParking(t *testing.T) {
	tests := []struct {
		name       string
		parking    Parking
		wantKeptOn bool
	}{
		{"a car reaches the site", ParkingAtSite, true},
		{"park and walk", ParkingWalk, false},
		{"no vehicle access at all", ParkingNone, false},
		// Missing data is surfaced, never acted on.
		{"unknown survives the filter", ParkingUnknown, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			booking := night("1", "f1", day(2026, 9, 4))
			booking.Site.Parking = tt.parking
			f := Filter{}

			if got := f.Apply([]Availability{booking}, SearchRequest{Nights: 1}); len(got) != 1 {
				t.Errorf("filter off: kept = %d, want 1", len(got))
			}

			on := f.Apply([]Availability{booking}, SearchRequest{Nights: 1, Parking: []Parking{ParkingAtSite}})
			if got := len(on) == 1; got != tt.wantKeptOn {
				t.Errorf("filter on: kept = %v, want %v", got, tt.wantKeptOn)
			}
		})
	}
}

// TestFilterZephyrCoveIncident pins the exact alert that prompted this work.
func TestFilterZephyrCoveIncident(t *testing.T) {
	booking := night("10300345", "10300216", day(2026, 9, 4))
	booking.Site.RawType = "TENT ONLY NONELECTRIC"
	booking.Site.Parking = ParkingNone
	booking.Site.AccessLabel = "Hike-In"
	booking.Site.MaxVehicles = vehicles(0)

	f := Filter{}
	req := SearchRequest{Nights: 1, CampsiteTypes: []string{"TENT ONLY NONELECTRIC"}}

	if got := f.Apply([]Availability{booking}, req); len(got) != 1 {
		t.Fatalf("campsite-types alone should still pass this site, got %d results", len(got))
	}

	req.Parking = []Parking{ParkingAtSite}
	if got := f.Apply([]Availability{booking}, req); len(got) != 0 {
		t.Errorf("expected the hike-in site to be dropped, got %d results", len(got))
	}
}

// TestFilterShelterIsAChoiceAgainstASet is the test that catches the modelling
// mistake: a STANDARD site takes a tent, so a tent search must return it.
func TestFilterShelterIsAChoiceAgainstASet(t *testing.T) {
	mk := func(id string, permits Permitted) Availability {
		a := night(id, "f1", day(2026, 9, 4))
		a.Site.Permits = permits
		return a
	}
	sites := []Availability{
		mk("standard", PermitsTent|PermitsRV),
		mk("rv-only", PermitsRV),
		mk("tent-only", PermitsTent),
		mk("unknown", PermittedUnknown),
	}

	got := func(s Shelter) map[string]bool {
		out := map[string]bool{}
		for _, a := range (&Filter{}).Apply(sites, SearchRequest{Nights: 1, Shelter: s}) {
			out[a.Site.ID] = true
		}
		return out
	}

	tent := got(ShelterTent)
	if !tent["standard"] {
		t.Error("a STANDARD site takes a tent; excluding it hides 264 sites at the campgrounds measured")
	}
	if tent["rv-only"] {
		t.Error("an RV-only site must not reach a tent camper")
	}
	if !tent["unknown"] {
		t.Error("a site whose permitted set is unknown must be kept, not dropped")
	}

	rv := got(ShelterRV)
	if !rv["standard"] || !rv["rv-only"] || rv["tent-only"] {
		t.Errorf("RV search returned %v", rv)
	}
}

// TestWaterMeansDifferentThingsByShelter pins the one context-dependent rule in
// the flag surface, both readings side by side.
//
// It is the rule most able to become the next context-dependent bug, so the two
// answers for the same site are asserted together rather than in separate tests.
func TestWaterMeansDifferentThingsByShelter(t *testing.T) {
	// The shelter only decides the answer where the hookup is explicitly
	// absent. Where it is merely unreported the site is kept either way, so
	// this is the case that actually discriminates.
	spigotOnly := &Site{Hookups: Hookups{Water: TriNo}, SharedWater: TriYes}

	if !spigotOnly.Satisfies(HookupWater, ShelterTent) {
		t.Error("a tent camper filling a jug is served by a shared tap")
	}
	if spigotOnly.Satisfies(HookupWater, ShelterRV) {
		t.Error("an RV camper asked for a hookup; a tap is not one, and this site says it has none")
	}

	// Unreported is not a no, for either kind of camper.
	unreported := &Site{SharedWater: TriYes}
	for _, sh := range []Shelter{ShelterTent, ShelterRV} {
		if !unreported.Satisfies(HookupWater, sh) {
			t.Errorf("%v: an unreported hookup must not exclude", sh)
		}
	}
	if got := spigotOnly.WaterSource(); got != "shared source" {
		t.Errorf("WaterSource() = %q; an alert must never report a tap as a hookup", got)
	}

	hookup := &Site{Hookups: Hookups{Water: TriYes}}
	for _, s := range []Shelter{ShelterTent, ShelterRV, ShelterCabin} {
		if !hookup.Satisfies(HookupWater, s) {
			t.Errorf("a real hookup serves %v", s)
		}
	}
	if got := hookup.WaterSource(); got != "hookup at site" {
		t.Errorf("WaterSource() = %q", got)
	}

	// Unknown is kept, as everywhere else. Hookups are recorded per campground,
	// so dropping unknowns would remove whole campgrounds from a search.
	silent := &Site{}
	for _, h := range []Hookup{HookupElectric, HookupWater, HookupSewer} {
		if !silent.Satisfies(h, ShelterRV) {
			t.Errorf("%v: a provider that said nothing has not said no", h)
		}
	}

	// An explicit no is a real answer and does exclude.
	denied := &Site{Hookups: Hookups{Electric: TriNo}}
	if denied.Satisfies(HookupElectric, ShelterRV) {
		t.Error("NONELECTRIC is an explicit no and must exclude")
	}
}

// TestFilterHookupsAreAdditive: naming two requires both.
func TestFilterHookupsAreAdditive(t *testing.T) {
	mk := func(id string, h Hookups) Availability {
		a := night(id, "f1", day(2026, 9, 4))
		a.Site.Hookups = h
		return a
	}
	sites := []Availability{
		mk("both", Hookups{Electric: TriYes, Water: TriYes}),
		mk("electric-only", Hookups{Electric: TriYes, Water: TriNo}),
	}

	req := SearchRequest{Nights: 1, Shelter: ShelterRV, Hookups: []Hookup{HookupElectric, HookupWater}}
	got := (&Filter{}).Apply(sites, req)
	if len(got) != 1 || got[0].Site.ID != "both" {
		t.Errorf("requiring electric and water returned %d sites, want only \"both\"", len(got))
	}
}

// TestNormalizeValueFoldsTheTypo is the live bug that prompted normalization:
// recreation.gov returns "Large Tent Over 9X12`" and camply advertised the clean
// spelling, which matched nothing.
func TestNormalizeValueFoldsTheTypo(t *testing.T) {
	if !EqualValue("Large Tent Over 9X12", "Large Tent Over 9X12`") {
		t.Error("the backticked spelling must be the same value as the clean one")
	}
	if !EqualValue("  TENT ONLY   NONELECTRIC ", "Tent Only Nonelectric") {
		t.Error("case and spacing must not decide a match")
	}
	if EqualValue("Tent", "Small Tent") {
		t.Error("normalization must not merge distinct values")
	}
}

// TestHookupCoverageIsPerCampgroundAndPerHookup pins the two ways this report
// refuses to average.
//
// Recording is a campground practice, not a site fact, and a campground can
// answer about one utility while saying nothing about another: Lodgepole types
// every site NONELECTRIC — a real answer about electricity — and records a water
// hookup on 1 of its 201 sites.
func TestHookupCoverageIsPerCampgroundAndPerHookup(t *testing.T) {
	mk := func(id, facility string, h Hookups) Availability {
		a := night(id, facility, day(2026, 9, 4))
		a.Site.Facility.Name = "Facility " + facility
		a.Site.Hookups = h
		return a
	}
	sites := []Availability{
		// Says no to electricity, nothing about water.
		mk("1", "public", Hookups{Electric: TriNo}),
		mk("2", "public", Hookups{Electric: TriNo}),
		// Records both.
		mk("3", "resort", Hookups{Electric: TriYes, Water: TriYes}),
	}

	c := AnalyzeHookups(sites, []Hookup{HookupElectric, HookupWater}, ShelterRV)

	if got := c.PerHookup[HookupElectric]; len(got) != 0 {
		t.Errorf("every site answers about electricity; reported %v", got)
	}

	water := c.PerHookup[HookupWater]
	if len(water) != 1 || water[0].FacilityID != "public" || water[0].Silent != 2 {
		t.Fatalf("water silence = %+v, want 2 sites at the public campground", water)
	}

	// No threshold: one recorded site does not make a campground eloquent.
	sites = append(sites, mk("4", "public", Hookups{Electric: TriNo, Water: TriYes}))
	c = AnalyzeHookups(sites, []Hookup{HookupWater}, ShelterRV)
	if got := c.PerHookup[HookupWater]; len(got) != 1 || got[0].Silent != 2 || got[0].TotalSites != 3 {
		t.Errorf("want 2 of 3 silent at the public campground, got %+v", got)
	}

	// A tent camper is served by a shared tap, so that counts as an answer.
	tap := []Availability{mk("5", "public", Hookups{})}
	tap[0].Site.SharedWater = TriYes
	if got := AnalyzeHookups(tap, []Hookup{HookupWater}, ShelterTent).PerHookup[HookupWater]; len(got) != 0 {
		t.Errorf("a shared tap answers a tent camper's water question, got %+v", got)
	}
	if got := AnalyzeHookups(tap, []Hookup{HookupWater}, ShelterRV).PerHookup[HookupWater]; len(got) != 1 {
		t.Error("a shared tap does not answer an RV camper's hookup question")
	}
}
