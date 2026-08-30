package notifications

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kkweon/camply/internal/core"
)

var updateGolden = flag.Bool("update", false, "rewrite testdata/message.golden")

// TestMessageGolden pins the exact text of a notification.
//
// The terminal goldens in cmd/camply/cli cover the table; this covers the body,
// which is where the safety guarantee actually lives — the incident was an alert
// that said nothing about vehicle access. The domain-model refactor must
// reproduce this file byte for byte.
func TestMessageGolden(t *testing.T) {
	vehicles := func(n int) *int { return &n }

	base := func() core.Availability {
		return core.Availability{
			Site: &core.Site{
				ID:   "10300345",
				Name: "36",
				Loop: "4",
				Facility: core.Facility{
					ID:               "10300216",
					Name:             "Zephyr Cove RV & Campground",
					RecreationArea:   "Lake Tahoe",
					RecreationAreaID: "20",
				},
				RawType:      "TENT ONLY NONELECTRIC",
				UseType:      "Overnight",
				MinOccupancy: 1,
				MaxOccupancy: 6,
				BookingURL:   "https://www.recreation.gov/camping/campsites/10300345",
			},
			Start:  time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC),
			End:    time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC),
			Nights: 2,
			Status: "Available",
		}
	}

	cases := []struct {
		name string
		mut  func(*core.Availability)
	}{
		{"the reported incident: hike-in, no vehicle access", func(a *core.Availability) {
			// As the adapter classifies campsite 10300345: Site Access Hike-In,
			// no vehicles, tents only, and NONELECTRIC is an explicit no.
			a.Site.Parking, a.Site.AccessLabel, a.Site.MaxVehicles = core.ParkingNone, "Hike-In", vehicles(0)
			a.Site.Equipment = []core.Equipment{{EquipmentName: "Tent"}, {EquipmentName: "Small Tent"}}
			a.Site.Permits = core.PermitsTent
			a.Site.Hookups = core.Hookups{Electric: core.TriNo}
		}},
		{"drive-in RV site with hookups", func(a *core.Availability) {
			amps := 50
			a.Site.Parking, a.Site.AccessLabel, a.Site.MaxVehicles = core.ParkingAtSite, "Drive-In", vehicles(2)
			a.Site.Equipment = []core.Equipment{{EquipmentName: "Tent"}, {EquipmentName: "RV", MaxLength: 30}}
			a.Site.Permits = core.PermitsTent | core.PermitsRV
			a.Site.Hookups = core.Hookups{Electric: core.TriYes, Water: core.TriYes, Sewer: core.TriNo}
			a.Site.Amps = &amps
		}},
		{"access unknown", func(a *core.Availability) {}},
		{"walk-in tent site with a shared tap and a recorded distance", func(a *core.Availability) {
			feet := 39
			a.Site.Parking, a.Site.AccessLabel = core.ParkingWalk, "Walk-In"
			a.Site.Permits = core.PermitsTent
			a.Site.SharedWater = core.TriYes
			a.Site.WalkFeet = &feet
		}},
		{"boat-in", func(a *core.Availability) {
			a.Site.Parking, a.Site.AccessLabel = core.ParkingNone, "Boat-In"
		}},
		{"non-drivable with an unrecognised label", func(a *core.Availability) {
			a.Site.Parking, a.Site.AccessLabel = core.ParkingWalk, "Ferry-In"
		}},
		{"equipment unverified", func(a *core.Availability) {
			a.Site.Parking, a.Site.AccessLabel, a.Site.MaxVehicles = core.ParkingAtSite, "Drive-In", vehicles(1)
			a.EquipmentUnverified = true
		}},
	}

	var b strings.Builder
	for _, tc := range cases {
		c := base()
		tc.mut(&c)
		title, body := formatMessage(c)
		fmt.Fprintf(&b, "### %s\nTITLE: %s\n%s", tc.name, title, body)
	}
	got := b.String()

	path := filepath.Join("testdata", "message.golden")
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v\nrun with -update to create it", err)
	}
	if got != string(want) {
		t.Errorf("notification text changed.\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}
