package core

import (
	"testing"
	"time"
)

func eqSite(id, facility string, day int, equipment ...string) AvailableCampsite {
	var eq []Equipment
	for _, e := range equipment {
		eq = append(eq, Equipment{EquipmentName: e, MaxLength: 30})
	}
	return AvailableCampsite{
		CampsiteID:         id,
		FacilityID:         facility,
		FacilityName:       "Facility " + facility,
		BookingDate:        time.Date(2026, 9, day, 0, 0, 0, 0, time.UTC),
		PermittedEquipment: eq,
	}
}

// A site free for several nights is one site. Counting availability rows would
// inflate every number in the report.
func TestAnalyzeCountsDistinctSitesNotRows(t *testing.T) {
	sites := []AvailableCampsite{
		eqSite("A", "1", 4, "Tent"),
		eqSite("A", "1", 5, "Tent"),
		eqSite("A", "1", 6, "Tent"),
		eqSite("B", "1", 4, "RV"),
	}
	c := AnalyzeEquipment(sites, []Equipment{{EquipmentName: "Tent"}})

	if c.TotalSites != 2 {
		t.Errorf("TotalSites = %d, want 2", c.TotalSites)
	}
	if c.MatchesByName["Tent"] != 1 {
		t.Errorf("Tent matches = %d, want 1", c.MatchesByName["Tent"])
	}
}

// The reported incident: a valid name that matches nothing at some campgrounds.
func TestAnalyzeFindsPerFacilityMisses(t *testing.T) {
	sites := []AvailableCampsite{
		eqSite("A", "big", 4, "Tent", "RV"),
		eqSite("B", "big", 4, "Tent"),
		eqSite("C", "small", 4, "Small Tent"), // no plain "Tent" here
	}
	c := AnalyzeEquipment(sites, []Equipment{{EquipmentName: "Tent"}})

	if c.MatchesByName["Tent"] != 2 {
		t.Errorf("Tent matches = %d, want 2", c.MatchesByName["Tent"])
	}
	if len(c.UnmatchedNames()) != 0 {
		t.Errorf("Tent matched somewhere, so it is not unmatched: %v", c.UnmatchedNames())
	}

	dropped := c.FacilitiesWithNoMatch()
	if len(dropped) != 1 || dropped[0].FacilityID != "small" {
		t.Fatalf("want the 'small' facility reported as dropped, got %+v", dropped)
	}
	if offers := dropped[0].ObservedAt(2); len(offers) == 0 || offers[0] != "Small Tent" {
		t.Errorf("the report must name what that campground does offer, got %v", offers)
	}
}

// Naming several equipment types so their union covers every campground is the
// documented way to handle per-campground vocabulary differences, and must not
// be reported as a problem. The running CronJobs are configured exactly this way.
func TestUnionCoverageIsNotAMiss(t *testing.T) {
	sites := []AvailableCampsite{
		eqSite("A", "big", 4, "Tent"),
		eqSite("C", "small", 4, "Small Tent"),
	}
	c := AnalyzeEquipment(sites, []Equipment{
		{EquipmentName: "Tent"},
		{EquipmentName: "Small Tent"},
	})

	if dropped := c.FacilitiesWithNoMatch(); len(dropped) != 0 {
		t.Errorf("both campgrounds are covered by the union; none should be dropped: %+v", dropped)
	}
	// Each name individually misses one campground, which is expected and fine.
	if c.MatchesByName["Tent"] != 1 || c.MatchesByName["Small Tent"] != 1 {
		t.Errorf("per-name counts wrong: %v", c.MatchesByName)
	}
}

func TestAnalyzeReportsTotalMiss(t *testing.T) {
	sites := []AvailableCampsite{eqSite("A", "1", 4, "Tent", "RV")}
	c := AnalyzeEquipment(sites, []Equipment{{EquipmentName: "Vehicle"}})

	if got := c.UnmatchedNames(); len(got) != 1 || got[0] != "Vehicle" {
		t.Errorf("UnmatchedNames = %v, want [Vehicle]", got)
	}
	if top := c.TopObserved(2); len(top) == 0 {
		t.Error("the report must list what these sites actually offer")
	}
}

// Filter.hasMatchingEquipment treats absent metadata as no-match, so a filter
// silently drops those sites. Counting them is how that becomes visible.
func TestAnalyzeCountsSitesWithNoEquipmentData(t *testing.T) {
	sites := []AvailableCampsite{
		eqSite("A", "1", 4, "Tent"),
		eqSite("B", "1", 4), // no permitted_equipment
	}
	c := AnalyzeEquipment(sites, []Equipment{{EquipmentName: "Tent"}})

	if c.MissingData != 1 {
		t.Errorf("MissingData = %d, want 1", c.MissingData)
	}
}

// The diagnosis must agree with the filter it explains.
func TestAnalyzeHonoursLengthLikeTheFilter(t *testing.T) {
	sites := []AvailableCampsite{eqSite("A", "1", 4, "RV")} // MaxLength 30
	if c := AnalyzeEquipment(sites, []Equipment{{EquipmentName: "RV", MaxLength: 25}}); c.MatchesByName["RV"] != 1 {
		t.Error("a 25 ft requirement should match a 30 ft site")
	}
	if c := AnalyzeEquipment(sites, []Equipment{{EquipmentName: "RV", MaxLength: 40}}); c.MatchesByName["RV"] != 0 {
		t.Error("a 40 ft requirement should not match a 30 ft site")
	}
}

func TestAnalyzeMatchesCaseInsensitively(t *testing.T) {
	sites := []AvailableCampsite{eqSite("A", "1", 4, "Small Tent")}
	c := AnalyzeEquipment(sites, []Equipment{{EquipmentName: "small tent"}})
	if c.MatchesByName["small tent"] != 1 {
		t.Error("matching should ignore case, as the filter does")
	}
}
