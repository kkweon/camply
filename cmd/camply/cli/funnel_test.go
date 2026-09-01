package cli

import (
	"testing"
	"time"

	"github.com/kkweon/camply/internal/core"
)

// sept is shorthand for a date in the month every fixture here uses.
func sept(d int) time.Time { return day(2026, time.September, d) }

// The Search line is the reproduction record: every requested constraint must
// appear, spelled as the flag that requests it, and nothing unrequested may.
func TestDescribeSearchEchoesEveryRequestedConstraint(t *testing.T) {
	req := core.SearchRequest{
		StartDates:       []time.Time{sept(4), sept(11)},
		EndDates:         []time.Time{sept(7), sept(14)},
		Nights:           2,
		WeekendsOnly:     true,
		Campsites:        []string{"2433", "2437"},
		MinVehicleLength: 25,
		Shelter:          core.ShelterTent,
		Parking:          []core.Parking{core.ParkingAtSite},
		Hookups:          []core.Hookup{core.HookupElectric, core.HookupWater},
	}

	got := describeSearch(req)
	want := "2026-09-04 → 2026-09-07; 2026-09-11 → 2026-09-14, --nights 2, --weekends, " +
		"--campsites 2433,2437, --min-vehicle-length 25, --shelter tent, " +
		"--parking at-site, --hookups electric,water"
	if got != want {
		t.Errorf("describeSearch:\n got: %s\nwant: %s", got, want)
	}
}

func TestDescribeSearchOmitsWhatWasNotAsked(t *testing.T) {
	req := core.SearchRequest{
		StartDates: []time.Time{sept(4)},
		EndDates:   []time.Time{sept(7)},
		Nights:     1,
	}
	got := describeSearch(req)
	want := "2026-09-04 → 2026-09-07, --nights 1"
	if got != want {
		t.Errorf("describeSearch:\n got: %s\nwant: %s", got, want)
	}
}

// The funnel must name each requested stage with its survivor count, in the
// order the stages run, and stay silent about stages nobody requested — an
// inactive stage removed nothing and would only pad the line.
func TestDescribeFunnelNarratesOnlyActiveStages(t *testing.T) {
	stats := core.FilterStats{
		RawNights:      195,
		RawSites:       15,
		Stays:          136,
		AfterCampsites: 40,
		AfterWeekends:  12,
		AfterWindow:    1,
		AfterParking:   0,
	}
	req := core.SearchRequest{
		Nights:       2,
		WeekendsOnly: true,
		Campsites:    []string{"2433"},
		Parking:      []core.Parking{core.ParkingAtSite},
	}

	got := describeFunnel(stats, req)
	want := "15 sites with 195 open nights → 136 stays of 2 consecutive nights → " +
		"40 at --campsites 2433 → 12 starting Fri/Sat → 1 inside the requested dates → " +
		"0 after --parking at-site"
	if got != want {
		t.Errorf("describeFunnel:\n got: %s\nwant: %s", got, want)
	}
}

// One night needs no consolidation stage: "N stays of 1 consecutive nights"
// restates the raw count in different words.
func TestDescribeFunnelSkipsStaysForSingleNights(t *testing.T) {
	stats := core.FilterStats{
		RawNights:    250,
		RawSites:     19,
		Stays:        250,
		AfterWindow:  234,
		AfterHookups: 200,
	}
	req := core.SearchRequest{
		Nights:  1,
		Hookups: []core.Hookup{core.HookupElectric},
	}

	got := describeFunnel(stats, req)
	want := "19 sites with 250 open nights → 234 inside the requested dates → 200 after --hookups"
	if got != want {
		t.Errorf("describeFunnel:\n got: %s\nwant: %s", got, want)
	}
}
