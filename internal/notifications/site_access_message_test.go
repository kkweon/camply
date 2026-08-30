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
	all := []core.SiteAccess{
		core.SiteAccessUnknown, core.SiteAccessDriveIn, core.SiteAccessWalkIn,
		core.SiteAccessHikeIn, core.SiteAccessBoatIn, core.SiteAccessNoVehicle,
	}

	for _, access := range all {
		c := getExampleCampsite()
		c.SiteAccess = access
		c.SiteAccessRaw = access.String()

		_, body := formatMessage(c)
		if !strings.Contains(body, "<b>Site Access:</b>") {
			t.Errorf("%v: message has no Site Access line:\n%s", access, body)
		}
	}
}

func TestFormatMessageTitleWarnsUnlessDriveIn(t *testing.T) {
	tests := []struct {
		access    core.SiteAccess
		raw       string
		wantTitle string
	}{
		{core.SiteAccessDriveIn, "Drive-In", ""},
		{core.SiteAccessWalkIn, "Walk-In", "⚠️ WALK-IN"},
		{core.SiteAccessHikeIn, "Hike-In", "⚠️ HIKE-IN"},
		{core.SiteAccessBoatIn, "Boat-In", "⚠️ BOAT-IN"},
		{core.SiteAccessUnknown, "", "⚠️ UNKNOWN ACCESS"},
	}

	for _, tt := range tests {
		c := getExampleCampsite()
		c.SiteAccess = tt.access
		c.SiteAccessRaw = tt.raw

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
