package cli

import (
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kkweon/camply/internal/providers"
	"github.com/kkweon/camply/internal/providers/recdotgov"
)

// updateGolden regenerates the .golden files instead of asserting against them:
//
//	go test ./cmd/camply/cli -run TestGolden -update
//
// Regenerate deliberately and read the diff. These files are the record of what
// camply prints today; a refactor that changes them has changed behaviour.
var updateGolden = flag.Bool("update", false, "rewrite testdata/golden/*.golden")

// goldenFixtures maps a scenario to the recorded responses it replays. Each
// directory holds one campground's trimmed metadata and availability, cut from
// the live API.
var goldenFixtures = map[string]string{
	"10300216": "zephyr",
	"232461":   "lodgepole",
	"232875":   "kaspian",
	"10220612": "meeksbay",
}

// goldenServer replays recorded recreation.gov responses.
//
// Routing mirrors the two endpoints the provider calls: the campsite search
// (metadata, keyed by asset_id) and the month availability (keyed by campground
// in the path).
func goldenServer(t *testing.T) *httptest.Server {
	t.Helper()

	root := filepath.Join("..", "..", "..", "internal", "providers", "recdotgov", "testdata", "golden")
	serve := func(w http.ResponseWriter, campground, file string) {
		dir, ok := goldenFixtures[campground]
		if !ok {
			http.Error(w, "no fixture for campground "+campground, http.StatusNotFound)
			return
		}
		data, err := os.ReadFile(filepath.Join(root, dir, file))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/search/campsites":
			// fq=asset_id:232461
			fq := r.URL.Query().Get("fq")
			serve(w, strings.TrimPrefix(fq, "asset_id:"), "metadata.json")
		case strings.HasPrefix(r.URL.Path, "/api/camps/availability/campground/"):
			rest := strings.TrimPrefix(r.URL.Path, "/api/camps/availability/campground/")
			serve(w, strings.TrimSuffix(rest, "/month"), "availability.json")
		default:
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
		}
	}))
}

// goldenRegistry is the real provider registry with recdotgov pointed at the
// fixture server, so help text, vocabularies and flag wiring are exercised
// exactly as in production.
func goldenRegistry(addr string) []providers.Descriptor {
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
	srv := goldenServer(t)
	defer srv.Close()
	registry := goldenRegistry(srv.Listener.Addr().String())

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
				"--exclude-no-vehicle-access",
				"--date-ranges", "2026-09-04:2026-09-07", "--nights", "2",
			},
		},
		{
			// Leaks in both directions: hike-in sites typed TENT ONLY, and
			// drive-in sites typed WALK TO.
			name: "lodgepole_cronjob",
			args: []string{
				"recdotgov", "campsites", "--campgrounds", "232461",
				"--campsite-types", "STANDARD NONELECTRIC", "--campsite-types", "TENT ONLY NONELECTRIC",
				"--exclude-no-vehicle-access",
				"--date-ranges", "2026-09-04:2026-09-07", "--nights", "2",
			},
		},
		{
			// Every site is walk-in, so the flag empties the campground: the
			// fatal path.
			name: "kaspian_emptied",
			args: []string{
				"recdotgov", "campsites", "--campgrounds", "232875",
				"--exclude-no-vehicle-access",
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

			path := filepath.Join("testdata", "golden", tc.name+".golden")
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
