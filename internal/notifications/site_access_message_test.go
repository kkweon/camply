package notifications

import (
	"strings"
	"testing"

	"github.com/kkweon/camply/internal/core"
)

// TestFormatMessageAlwaysReportsSiteAccess is the regression test for the
// incident itself.
//
// The alert for Zephyr Cove campsite 10300345 contained Campsite Type,
// Occupancy, Use Type and a booking link, and nothing at all about the site
// being half a mile from the nearest road. Everything else here is filtering
// convenience; this line is the guarantee, and it has to hold for every value
// including the one that means "we don't know".
func TestFormatMessageAlwaysReportsSiteAccess(t *testing.T) {
	all := []core.Parking{
		core.ParkingUnknown, core.ParkingAtSite, core.ParkingWalk, core.ParkingNone,
	}

	for _, access := range all {
		for _, requested := range []bool{false, true} {
			c := getExampleCampsite()
			c.Site.Parking = access
			c.Site.AccessLabel = access.String()
			c.ParkingRequested = requested

			// Exactly once: either as a warning above the fold or inline on
			// the site line — never omitted, and never both.
			_, body := formatMessage(c)
			if got := strings.Count(body, c.SiteAccessSummary()); got != 1 {
				t.Errorf("%v/requested=%v: access summary appears %d times, want 1:\n%s",
					access, requested, got, body)
			}
		}
	}
}

func TestFormatMessageTitleWarnsUnlessDriveIn(t *testing.T) {
	tests := []struct {
		access    core.Parking
		raw       string
		requested bool
		wantTitle string
	}{
		{core.ParkingAtSite, "Drive-In", false, ""},
		{core.ParkingWalk, "Walk-In", false, "⚠️ WALK-IN"},
		{core.ParkingWalk, "Hike-In", false, "⚠️ HIKE-IN"},
		{core.ParkingNone, "Boat-In", false, "⚠️ BOAT-IN"},
		{core.ParkingUnknown, "", false, "⚠️ UNKNOWN ACCESS"},
		// A walk the user's --parking filter named is a confirmation, not a
		// warning — but not knowing stays a warning no matter what was asked.
		{core.ParkingWalk, "Walk-In", true, ""},
		{core.ParkingNone, "Hike-In", true, ""},
		{core.ParkingUnknown, "", true, "⚠️ UNKNOWN ACCESS"},
	}

	for _, tt := range tests {
		c := getExampleCampsite()
		c.Site.Parking = tt.access
		c.Site.AccessLabel = tt.raw
		c.ParkingRequested = tt.requested

		title, _ := formatMessage(c)
		if tt.wantTitle == "" {
			if strings.Contains(title, "⚠️") {
				t.Errorf("%v: title carries a warning it should not: %q", tt.access, title)
			}
			continue
		}
		// The title is what a lock screen shows, and often all that is read
		// before the booking link is tapped.
		if !strings.HasPrefix(title, tt.wantTitle) {
			t.Errorf("%v: title = %q, want it to start with %q", tt.access, title, tt.wantTitle)
		}
	}
}

// TestExampleCampsiteExercisesTheWarning keeps `camply test-notifications`
// honest: a drive-in sample would exercise the one case with nothing to warn
// about, so the path most worth verifying by hand would go untested.
func TestExampleCampsiteExercisesTheWarning(t *testing.T) {
	title, body := formatMessage(getExampleCampsite())
	if !strings.Contains(title, "⚠️") {
		t.Errorf("example title raises no warning: %q", title)
	}
	if !strings.Contains(body, "no vehicle access") {
		t.Errorf("example body does not exercise the no-vehicle wording:\n%s", body)
	}
}

// TestFormatMessageAlwaysReportsEquipment mirrors the Site Access guarantee.
//
// An equipment filter can let a site through without ever matching it, and the
// body is the only place that can be admitted.
func TestFormatMessageAlwaysReportsEquipment(t *testing.T) {
	for _, unverified := range []bool{false, true} {
		c := getExampleCampsite()
		c.EquipmentUnverified = unverified

		title, body := formatMessage(c)
		if !strings.Contains(body, "<b>Equipment:</b>") {
			t.Errorf("unverified=%v: message has no Equipment line:\n%s", unverified, body)
		}
		if unverified && !strings.Contains(title, "⚠️ NO EQUIPMENT DATA") {
			t.Errorf("an unverified equipment match must reach the title, got %q", title)
		}
	}
}
