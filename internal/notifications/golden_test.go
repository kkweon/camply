package notifications

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kkweon/camply/internal/core"
	"github.com/kkweon/camply/internal/providers/recdotgov"
	"github.com/kkweon/camply/internal/providers/recdotgov/recdotgovtest"
)

// replayedIncidentSite returns campsite 10300345 -- the hike-in site the whole
// safety guarantee was written for -- as the provider actually builds it from
// recreation.gov's recorded bytes.
//
// Every case below starts here rather than from a core.Availability literal.
// The literal this replaces declared the recreation area "Lake Tahoe" on a
// recreation.gov site, and the adapter had never set that field: the golden
// pinned a notification that could not occur, so a title rendering as a bare
// campground name and an empty leading field passed review for months. A test
// cannot invent a campground it never names.
func replayedIncidentSite(t *testing.T) core.Availability {
	t.Helper()

	srv := recdotgovtest.NewServer(t)
	p := recdotgov.NewProviderAt("http", srv.Listener.Addr().String())

	const (
		campground = "10300216"
		campsite   = "10300345"
	)
	start := time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC)

	req := core.SearchRequest{
		Campgrounds: []string{campground},
		StartDates:  []time.Time{start},
		EndDates:    []time.Time{time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC)},
		Nights:      2,
	}

	raw, err := p.FindCampsites(context.Background(), req)
	if err != nil {
		t.Fatalf("replaying the recorded corpus: %v", err)
	}

	// The filter, too: a notifier is handed what Apply produced, never the
	// provider's raw output. Consecutive nights are stitched into a booking
	// there, so skipping it would pin a one-night alert for a two-night search.
	var filter core.Filter
	found := filter.Apply(raw, req)

	for _, a := range found {
		if a.Site != nil && a.Site.ID == campsite && a.Start.Equal(start) {
			return a
		}
	}
	t.Fatalf("campsite %s starting %s is not in the recording", campsite, start.Format("2006-01-02"))
	return core.Availability{}
}

var updateOutput = flag.Bool("update", false, "rewrite testdata/output/message.txt")

// TestMessageGolden pins the exact text of a notification.
//
// The terminal goldens in cmd/camply/cli cover the table; this covers the body,
// which is where the safety guarantee actually lives — the incident was an alert
// that said nothing about vehicle access. The domain-model refactor must
// reproduce this file byte for byte.
func TestMessageGolden(t *testing.T) {
	vehicles := func(n int) *int { return &n }

	// Each case gets its own copy of the site, so a mutator testing one axis
	// cannot leak into the next case through the shared pointer.
	recorded := replayedIncidentSite(t)
	base := func() core.Availability {
		c := recorded
		site := *recorded.Site
		c.Site = &site
		return c
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
		{"access unknown", func(a *core.Availability) {
			// A provider that reported nothing: no access, no equipment, no
			// hookups. Cleared field by field because the replayed base is a
			// fully described site, where the literal this replaced happened to
			// be zero-valued and so tested this by accident.
			a.Site.Parking, a.Site.AccessLabel, a.Site.MaxVehicles = core.ParkingUnknown, "", nil
			a.Site.Permits = 0
			a.Site.Equipment = nil
			a.Site.Hookups = core.Hookups{}
		}},
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
		{"walk-in the user asked for", func(a *core.Availability) {
			// Same site as the walk-in case, but --parking walk named it:
			// the access facts stay, the ⚠️ and the alarm caps go.
			feet := 39
			a.Site.Parking, a.Site.AccessLabel, a.Site.MaxVehicles = core.ParkingWalk, "Walk-In", vehicles(0)
			a.Site.Permits = core.PermitsTent
			a.Site.WalkFeet = &feet
			a.ParkingRequested = true
		}},
		{"equipment unverified", func(a *core.Availability) {
			a.Site.Parking, a.Site.AccessLabel, a.Site.MaxVehicles = core.ParkingAtSite, "Drive-In", vehicles(1)
			// The filter only raises this flag for a site with no equipment at
			// all (filter.go), so the equipment has to go with it. Leaving the
			// replayed site's real equipment in place would pin a state camply
			// cannot reach: a warning that nothing was reported, above a list
			// of what was.
			a.Site.Equipment = nil
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

	path := filepath.Join("testdata", "output", "message.txt")
	if *updateOutput {
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

// TestMessageDegradesWhenTheProviderLocatesNothing covers the shape the golden
// corpus does not contain.
//
// All four recorded campgrounds report both a recreation area and a town, so
// nothing else here exercises a provider that reports neither — and both
// providers do. In a 475-campground sample of RIDB, 54 carry no address at all
// and 29 give a state with no city; on UseDirect, 14 of 299 places have neither,
// and its places fetch is best-effort, so a bad response empties the lot at once.
//
// Absent data must read as absent: no title opening on " | ", and no 📍 line
// with nothing after it.
func TestMessageDegradesWhenTheProviderLocatesNothing(t *testing.T) {
	a := replayedIncidentSite(t)
	a.Site.Facility.RecreationArea = ""
	a.Site.Facility.Location = ""

	title, body := formatMessage(a)

	for _, segment := range strings.Split(title, " | ") {
		if strings.TrimSpace(segment) == "" {
			t.Errorf("title carries an empty field: %q", title)
		}
	}
	if !strings.Contains(title, a.Site.Facility.Name) {
		t.Errorf("title %q does not name the campground", title)
	}
	if strings.Contains(body, "📍") {
		t.Errorf("body carries a location line with no location:\n%s", body)
	}
}
