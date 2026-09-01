package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kkweon/camply/internal/providers"
	"github.com/kkweon/camply/internal/providers/recdotgov"
	"github.com/kkweon/camply/internal/providers/recdotgov/recdotgovtest"
)

// updateOutput regenerates testdata/output instead of asserting against it:
//
//	go test ./cmd/camply/cli -run TestGolden -update
//
// Regenerate deliberately and read the diff. These files are the record of what
// camply prints today; a refactor that changes them has changed behaviour.
//
// The two halves of this test are kept apart on purpose. testdata/output is the
// expectation, and it moves whenever camply's behaviour moves. The inputs live
// under internal/providers/recdotgov/testdata/input and are the live API's own
// bytes: they move only when recreation.gov's responses move. Never edit an
// input to make an assertion pass — that is how a fixture starts describing a
// campground the provider cannot actually produce.
var updateOutput = flag.Bool("update", false, "rewrite testdata/output/*.txt")

// replayRegistry is the real provider registry with recdotgov pointed at the
// fixture server, so help text, vocabularies and flag wiring are exercised
// exactly as in production.
func replayRegistry(addr string) []providers.Descriptor {
	descs := providers.Descriptors()
	for i := range descs {
		if descs[i].Key == providers.KeyRecDotGov {
			descs[i].New = func() providers.Provider { return recdotgov.NewProviderAt("http", addr) }
		}
	}
	return descs
}

// TestGolden pins what camply prints today, end to end through the real
// provider, filter, coverage report and table.
//
// It is the gate for the domain-model refactor: that work must reproduce these
// files byte for byte, because its whole promise is that nothing user-visible
// changes. A diff here is a bug, not an improvement.
func TestGolden(t *testing.T) {
	srv := recdotgovtest.NewServer(t)
	registry := replayRegistry(srv.Listener.Addr().String())

	cases := []struct {
		name string
		args []string
	}{
		{
			// The reported incident: campsite 10300345 is hike-in yet typed
			// exactly like the drive-in tent sites beside it.
			name: "zephyr_unfiltered",
			args: []string{
				"recdotgov", "campsites", "--campgrounds", "10300216",
				"--date-ranges", "2026-09-04:2026-09-07", "--nights", "2",
			},
		},
		{
			// The live CronJob's arguments.
			name: "zephyr_cronjob",
			args: []string{
				"recdotgov", "campsites", "--campgrounds", "10300216",
				"--campsite-types", "STANDARD NONELECTRIC", "--campsite-types", "TENT ONLY NONELECTRIC",
				"--parking", "at-site",
				"--date-ranges", "2026-09-04:2026-09-07", "--nights", "2",
			},
		},
		{
			// The same search on the deprecated flag, which the CronJobs still
			// pass. The result set must match zephyr_cronjob above: mapping the
			// old flag to at-site,walk would silently start including walk-in
			// sites, and that is the user's call, not an upgrade's.
			name: "zephyr_cronjob_deprecated_flag",
			args: []string{
				"recdotgov", "campsites", "--campgrounds", "10300216",
				"--campsite-types", "STANDARD NONELECTRIC", "--campsite-types", "TENT ONLY NONELECTRIC",
				"--exclude-no-vehicle-access",
				"--date-ranges", "2026-09-04:2026-09-07", "--nights", "2",
			},
		},
		{
			// The shelter axis. A STANDARD site takes a tent, so a tent search
			// must return it; an empty result means the camper's choice has been
			// confused with the site's permitted set.
			name: "zephyr_shelter_tent",
			args: []string{
				"recdotgov", "campsites", "--campgrounds", "10300216",
				"--shelter", "tent",
				"--date-ranges", "2026-09-01:2026-09-30", "--nights", "1",
			},
		},
		{
			// Hookups, at one of the two campgrounds that record them.
			name: "meeksbay_hookups",
			args: []string{
				"recdotgov", "campsites", "--campgrounds", "10220612",
				"--shelter", "rv", "--hookups", "electric,water",
				"--date-ranges", "2026-09-01:2026-09-30", "--nights", "1",
			},
		},
		{
			// Leaks in both directions: hike-in sites typed TENT ONLY, and
			// drive-in sites typed WALK TO.
			name: "lodgepole_cronjob",
			args: []string{
				"recdotgov", "campsites", "--campgrounds", "232461",
				"--campsite-types", "STANDARD NONELECTRIC", "--campsite-types", "TENT ONLY NONELECTRIC",
				"--parking", "at-site",
				"--date-ranges", "2026-09-04:2026-09-07", "--nights", "2",
			},
		},
		{
			// Every site is walk-in, so the flag empties the campground: the
			// fatal path.
			name: "kaspian_emptied",
			args: []string{
				"recdotgov", "campsites", "--campgrounds", "232875",
				"--parking", "at-site",
				"--date-ranges", "2026-09-04:2026-09-07", "--nights", "1",
			},
		},
		{
			// Missing equipment data, kept and flagged. A month-wide window so
			// the kept sites actually reach the table — a scenario that prints
			// nothing pins nothing.
			name: "meeksbay_equipment",
			args: []string{
				"recdotgov", "campsites", "--campgrounds", "10220612",
				"--equipment-types", "Tent",
				"--date-ranges", "2026-09-01:2026-09-30", "--nights", "1",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := runCLI(t, registry, tc.args...)

			var b strings.Builder
			fmt.Fprintf(&b, "$ camply %s\n\n", strings.Join(tc.args, " "))
			fmt.Fprintf(&b, "--- stdout ---\n%s\n--- stderr ---\n%s", res.Stdout, res.Stderr)
			if res.Err != nil {
				fmt.Fprintf(&b, "\n--- error ---\n%v\n", res.Err)
			}
			got := scrubTimestamps(b.String())

			path := filepath.Join("testdata", "output", tc.name+".txt")
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
				t.Errorf("output changed.\n--- want ---\n%s\n--- got ---\n%s", want, got)
			}
		})
	}
}

// scrubTimestamps removes the wall clock from log lines so the goldens compare
// on content alone.
func scrubTimestamps(s string) string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if i := strings.Index(line, "] "); strings.HasPrefix(line, "[") && i > 0 {
			line = "[TIME" + line[i:]
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}
