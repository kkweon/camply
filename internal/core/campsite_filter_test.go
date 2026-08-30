package core

import (
	"strings"
	"testing"
)

// TestFilterRestrictsToRequestedCampsites is the regression test for a flag that
// was declared, accepted, documented — and read by nothing. Passing an ID that
// matched no site left the result set completely unchanged.
func TestFilterRestrictsToRequestedCampsites(t *testing.T) {
	sites := []AvailableCampsite{
		night("10300345", "10300216", day(2026, 9, 4)),
		night("10300346", "10300216", day(2026, 9, 4)),
		night("10300347", "10300216", day(2026, 9, 4)),
	}
	f := Filter{}

	if got := f.Apply(sites, SearchRequest{Nights: 1}); len(got) != 3 {
		t.Fatalf("no --campsites: got %d results, want 3", len(got))
	}

	got := f.Apply(sites, SearchRequest{Nights: 1, Campsites: []string{"10300345", "10300347"}})
	if len(got) != 2 {
		t.Fatalf("got %d results, want 2", len(got))
	}
	for _, c := range got {
		if c.CampsiteID == "10300346" {
			t.Errorf("campsite %s was not requested but survived the filter", c.CampsiteID)
		}
	}

	// The case that proved the flag was dead: an ID matching nothing must empty
	// the results, not pass everything through.
	if got := f.Apply(sites, SearchRequest{Nights: 1, Campsites: []string{"99999999"}}); len(got) != 0 {
		t.Errorf("an unmatched campsite ID returned %d results, want 0", len(got))
	}
}

func TestValidateRequestedCampsites(t *testing.T) {
	roster := []KnownCampsite{
		{CampsiteID: "10300345", SiteName: "36", FacilityName: "Zephyr Cove RV & Campground"},
		{CampsiteID: "10300346", SiteName: "37", FacilityName: "Zephyr Cove RV & Campground"},
		{CampsiteID: "10300347", SiteName: "38", FacilityName: "Zephyr Cove RV & Campground"},
		{CampsiteID: "64413", SiteName: "005", FacilityName: "FALLEN LEAF CAMPGROUND"},
	}

	t.Run("a real campsite passes even with no availability", func(t *testing.T) {
		// The roster is the campground's full inventory, so a site booked solid
		// for the whole window still validates. Erroring here would break every
		// CronJob watching a single popular site.
		if err := ValidateRequestedCampsites([]string{"10300345"}, roster); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("IDs across different campgrounds all pass", func(t *testing.T) {
		if err := ValidateRequestedCampsites([]string{"10300345", "64413"}, roster); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("an unknown ID errors with usable IDs", func(t *testing.T) {
		err := ValidateRequestedCampsites([]string{"99999999"}, roster)
		if err == nil {
			t.Fatal("expected an error for an ID at no campground searched")
		}
		msg := err.Error()
		for _, want := range []string{"99999999", "10300345 (site 36)", "Zephyr Cove RV & Campground"} {
			if !strings.Contains(msg, want) {
				t.Errorf("error message is missing %q:\n%s", want, msg)
			}
		}
	})

	t.Run("a site name is a common mix-up and the error says so", func(t *testing.T) {
		// "36" is what is printed on the post at the site; 10300345 is the ID.
		err := ValidateRequestedCampsites([]string{"36"}, roster)
		if err == nil {
			t.Fatal("expected an error for a site name passed as an ID")
		}
		if !strings.Contains(err.Error(), "not the site name") {
			t.Errorf("error does not explain the ID/name mix-up:\n%s", err)
		}
	})

	t.Run("no request and no roster are both no-ops", func(t *testing.T) {
		if err := ValidateRequestedCampsites(nil, roster); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if err := ValidateRequestedCampsites([]string{"99999999"}, nil); err != nil {
			t.Errorf("an empty roster must not manufacture an error: %v", err)
		}
	})
}
