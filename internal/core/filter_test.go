package core

import (
	"testing"
	"time"
)

// night builds a single-night availability for a campsite on the given day.
func night(campsiteID, facilityID string, day time.Time) Availability {
	return Availability{
		Site: &Site{
			ID:       campsiteID,
			Facility: Facility{ID: facilityID},
		},
		Start:  day,
		End:    day.AddDate(0, 0, 1),
		Nights: 1,
	}
}

func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// run returns single-night slices for `count` consecutive days starting at `start`.
func run(campsiteID, facilityID string, start time.Time, count int) []Availability {
	out := make([]Availability, 0, count)
	for i := 0; i < count; i++ {
		out = append(out, night(campsiteID, facilityID, start.AddDate(0, 0, i)))
	}
	return out
}

func TestConsolidateNights(t *testing.T) {
	d1 := day(2026, 6, 1)

	tests := []struct {
		name     string
		input    []Availability
		nights   int
		wantLen  int
		wantEach int // expected BookingNights on every record (0 = don't check)
	}{
		{
			name:     "single night over a 2-night run yields one record per night",
			input:    run("100", "50", d1, 2),
			nights:   1,
			wantLen:  2,
			wantEach: 1,
		},
		{
			name:     "two nights over a 3-night run yields two sliding windows",
			input:    run("100", "50", d1, 3),
			nights:   2,
			wantLen:  2,
			wantEach: 2,
		},
		{
			name:     "run shorter than required nights yields nothing",
			input:    run("100", "50", d1, 1),
			nights:   2,
			wantLen:  0,
			wantEach: 0,
		},
		{
			name:     "run of exactly required nights yields one record",
			input:    run("100", "50", d1, 3),
			nights:   3,
			wantLen:  1,
			wantEach: 3,
		},
		{
			name:     "distinct campsites are grouped separately",
			input:    append(run("100", "50", d1, 1), run("101", "50", d1, 1)...),
			nights:   1,
			wantLen:  2,
			wantEach: 1,
		},
		{
			name: "same campsite id under different facilities is not merged",
			input: append(
				run("100", "50", d1, 1),
				run("100", "51", d1.AddDate(0, 0, 1), 1)..., // would look "consecutive" if merged
			),
			nights:   2,
			wantLen:  0, // neither facility has a 2-night run on its own
			wantEach: 0,
		},
		{
			name:     "gap breaks a run (single nights)",
			input:    append(run("100", "50", d1, 1), night("100", "50", d1.AddDate(0, 0, 2))),
			nights:   1,
			wantLen:  2,
			wantEach: 1,
		},
		{
			name:     "gap breaks a run (no two-night stay possible)",
			input:    append(run("100", "50", d1, 1), night("100", "50", d1.AddDate(0, 0, 2))),
			nights:   2,
			wantLen:  0,
			wantEach: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := consolidateNights(tt.input, tt.nights)
			if len(got) != tt.wantLen {
				t.Fatalf("got %d records, want %d: %+v", len(got), tt.wantLen, got)
			}
			if tt.wantEach > 0 {
				for _, c := range got {
					if c.Nights != tt.wantEach {
						t.Errorf("got BookingNights=%d, want %d for %+v", c.Nights, tt.wantEach, c)
					}
					// The span must equal the recorded nights.
					if spanDays := int(c.End.Sub(c.Start).Hours() / 24); spanDays != c.Nights {
						t.Errorf("span of %d days != BookingNights %d for %+v", spanDays, c.Nights, c)
					}
				}
			}
		})
	}
}

// TestConsolidateNightsWindowsAreExact guards against the original bug where a 3-night
// run with nights=2 also produced a phantom 3-night block.
func TestConsolidateNightsWindowsAreExact(t *testing.T) {
	d1 := day(2026, 6, 1)
	got := consolidateNights(run("100", "50", d1, 3), 2)

	if len(got) != 2 {
		t.Fatalf("got %d records, want 2: %+v", len(got), got)
	}
	for _, c := range got {
		if c.Nights != 2 {
			t.Errorf("expected only 2-night windows, got %d-night block %+v", c.Nights, c)
		}
	}
}

// TestFilterApplyConsolidates exercises the full Apply path end-to-end with a wide
// search window so consolidation is actually observable (a narrow window would mask
// over-long phantom blocks via the window filter).
func TestFilterApplyConsolidates(t *testing.T) {
	d1 := day(2026, 6, 1)
	req := SearchRequest{
		StartDates: []time.Time{d1},
		EndDates:   []time.Time{day(2026, 6, 10)},
		Nights:     1,
	}

	got := (&Filter{}).Apply(run("100", "50", d1, 2), req)
	if len(got) != 2 {
		t.Fatalf("Apply returned %d records, want 2 (one per single night): %+v", len(got), got)
	}
}

// Equipment cannot express drive-in: a WALK TO site still permits a tent, so
// only campsite_type separates sites reachable by car from those that are not.
func TestFilterByCampsiteType(t *testing.T) {
	sites := []Availability{
		{Site: &Site{ID: "1", RawType: "STANDARD NONELECTRIC"}, Start: day(2026, 9, 4), End: day(2026, 9, 5)},
		{Site: &Site{ID: "2", RawType: "TENT ONLY NONELECTRIC"}, Start: day(2026, 9, 4), End: day(2026, 9, 5)},
		{Site: &Site{ID: "3", RawType: "WALK TO"}, Start: day(2026, 9, 4), End: day(2026, 9, 5)},
		{Site: &Site{ID: "4", RawType: "RV NONELECTRIC"}, Start: day(2026, 9, 4), End: day(2026, 9, 5)},
	}
	req := SearchRequest{
		Nights:        1,
		StartDates:    []time.Time{day(2026, 9, 1)},
		EndDates:      []time.Time{day(2026, 9, 30)},
		CampsiteTypes: []string{"STANDARD NONELECTRIC", "TENT ONLY NONELECTRIC"},
	}

	got := (&Filter{}).Apply(sites, req)
	if len(got) != 2 {
		t.Fatalf("got %d sites, want 2 (WALK TO and RV NONELECTRIC excluded)", len(got))
	}
	for _, s := range got {
		if s.Site.RawType == "WALK TO" || s.Site.RawType == "RV NONELECTRIC" {
			t.Errorf("%s should have been filtered out", s.Site.RawType)
		}
	}
}

func TestCampsiteTypeMatchIgnoresCaseAndPadding(t *testing.T) {
	sites := []Availability{
		{Site: &Site{ID: "1", RawType: "WALK TO"}, Start: day(2026, 9, 4), End: day(2026, 9, 5)},
	}
	req := SearchRequest{
		Nights:        1,
		StartDates:    []time.Time{day(2026, 9, 1)},
		EndDates:      []time.Time{day(2026, 9, 30)},
		CampsiteTypes: []string{"  walk to  "},
	}
	if got := (&Filter{}).Apply(sites, req); len(got) != 1 {
		t.Errorf("got %d sites, want 1 — matching should tolerate case and padding", len(got))
	}
}

// No campsite-type filter must mean no filtering, not "match nothing".
func TestNoCampsiteTypeFilterKeepsEverything(t *testing.T) {
	sites := []Availability{
		{Site: &Site{ID: "1", RawType: "WALK TO"}, Start: day(2026, 9, 4), End: day(2026, 9, 5)},
		{Site: &Site{ID: "2"}, Start: day(2026, 9, 4), End: day(2026, 9, 5)},
	}
	req := SearchRequest{Nights: 1, StartDates: []time.Time{day(2026, 9, 1)}, EndDates: []time.Time{day(2026, 9, 30)}}
	if got := (&Filter{}).Apply(sites, req); len(got) != 2 {
		t.Errorf("got %d sites, want 2", len(got))
	}
}
