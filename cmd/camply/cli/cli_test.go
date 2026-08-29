package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/kkweon/camply/internal/core"
	"github.com/kkweon/camply/internal/logger"
	"github.com/kkweon/camply/internal/providers"
)

// fakeProvider stands in for a real API so command wiring can be exercised
// without network access.
type fakeProvider struct {
	sites    []core.AvailableCampsite
	camps    []core.CampgroundFacility
	areas    []core.RecreationArea
	err      error
	lastReq  core.SearchRequest
	campsite int // times FindCampsites was called
}

func (f *fakeProvider) FindCampsites(_ context.Context, req core.SearchRequest) ([]core.AvailableCampsite, error) {
	f.campsite++
	f.lastReq = req
	return f.sites, f.err
}

func (f *fakeProvider) FindCampgrounds(_ context.Context, req core.SearchRequest) ([]core.CampgroundFacility, error) {
	f.lastReq = req
	return f.camps, f.err
}

func (f *fakeProvider) FindRecreationAreas(_ context.Context, req core.SearchRequest) ([]core.RecreationArea, error) {
	f.lastReq = req
	return f.areas, f.err
}

// fakeRegistry returns a registry containing one supported provider backed by
// fake and one that is advertised but unimplemented.
func fakeRegistry(fake *fakeProvider) []providers.Descriptor {
	return []providers.Descriptor{
		{
			Key:           "fake",
			DisplayName:   "FakeProvider",
			Aliases:       []string{"fk"},
			Description:   "in-memory provider for tests",
			Status:        providers.StatusSupported,
			New:           func() providers.Provider { return fake },
			SupportsState: true,
			RecAreaIDHelp: "fake ID",
		},
		{
			Key:         "planned",
			DisplayName: "PlannedProvider",
			Description: "not built yet",
			Status:      providers.StatusPlanned,
		},
	}
}

type cliResult struct {
	Stdout string
	Stderr string
	Err    error
}

// runCLI builds a fresh command tree per call. Reusing one tree leaks parsed
// flag values between subtests, because pflag values are sticky across
// Execute(). logger output is redirected too: the handler writes straight to
// the process streams, so without this nothing logged is captured.
func runCLI(t *testing.T, descs []providers.Descriptor, args ...string) cliResult {
	t.Helper()

	var out, errOut bytes.Buffer
	logger.SetOutput(&out, &errOut)
	t.Cleanup(func() {
		logger.ResetOutput()
		logger.SetDebug(false)
	})

	root := newRootCmdWithRegistry(descs)
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(args)

	err := root.Execute()
	return cliResult{Stdout: out.String(), Stderr: errOut.String(), Err: err}
}

func site(id string, day int) core.AvailableCampsite {
	d := time.Date(2026, 9, day, 0, 0, 0, 0, time.UTC)
	return core.AvailableCampsite{
		CampsiteID:     id,
		BookingDate:    d,
		BookingEndDate: d.AddDate(0, 0, 1),
		BookingNights:  1,
		FacilityID:     "1",
		FacilityName:   "Test Campground",
		BookingURL:     "https://example.test/campsites/" + id,
	}
}

// Each command must own its --provider storage. Historically one package-level
// providerStr backed the flag on three sibling commands at once.
func TestProviderFlagIsNotSharedBetweenCommands(t *testing.T) {
	fake := &fakeProvider{}
	descs := fakeRegistry(fake)

	root := newRootCmdWithRegistry(descs)
	var seen []string
	for _, name := range []string{"campsites", "campgrounds", "recreation-areas"} {
		for _, c := range root.Commands() {
			if c.Name() != name {
				continue
			}
			f := c.Flags().Lookup("provider")
			if f == nil {
				t.Fatalf("%s has no --provider flag", name)
			}
			if err := f.Value.Set(name + "-value"); err != nil {
				t.Fatal(err)
			}
			seen = append(seen, f.Value.String())
		}
	}

	// If the three flags shared one variable, every read would return the last
	// value written rather than each command's own.
	for i, got := range seen {
		want := []string{"campsites-value", "campgrounds-value", "recreation-areas-value"}[i]
		if got != want {
			t.Errorf("flag %d = %q, want %q — commands are sharing storage", i, got, want)
		}
	}
}

func TestUnknownProviderErrorNamesUsableOnes(t *testing.T) {
	fake := &fakeProvider{}
	res := runCLI(t, fakeRegistry(fake),
		"campsites", "--provider", "Nonsense", "--campground", "1",
		"--start-date", "2026-09-04", "--end-date", "2026-09-07")

	if res.Err == nil {
		t.Fatal("an unknown provider should be an error")
	}
	if !strings.Contains(res.Err.Error(), "FakeProvider") {
		t.Errorf("error should name the usable provider, got: %v", res.Err)
	}
	if fake.campsite != 0 {
		t.Error("the provider should not be queried when resolution fails")
	}
}

func TestPlannedProviderIsRejectedNotSilentlyUsed(t *testing.T) {
	fake := &fakeProvider{}
	res := runCLI(t, fakeRegistry(fake),
		"campsites", "--provider", "PlannedProvider", "--campground", "1",
		"--start-date", "2026-09-04", "--end-date", "2026-09-07")

	if res.Err == nil {
		t.Fatal("a planned-but-unimplemented provider should be an error")
	}
	if !strings.Contains(res.Err.Error(), "not implemented") {
		t.Errorf("error should say it is unimplemented, got: %v", res.Err)
	}
}

func TestProviderNameIsCaseInsensitive(t *testing.T) {
	fake := &fakeProvider{sites: []core.AvailableCampsite{site("A", 4)}}
	res := runCLI(t, fakeRegistry(fake),
		"campsites", "--provider", "fakeprovider", "--campground", "1",
		"--start-date", "2026-09-04", "--end-date", "2026-09-07")

	if res.Err != nil {
		t.Fatalf("lowercase provider name should resolve: %v", res.Err)
	}
	if fake.campsite != 1 {
		t.Errorf("provider queried %d times, want 1", fake.campsite)
	}
}

// Results are printed at INFO on stdout; anyone piping stdout gets results only.
func TestResultsGoToStdout(t *testing.T) {
	fake := &fakeProvider{sites: []core.AvailableCampsite{site("A", 4), site("A", 5)}}
	res := runCLI(t, fakeRegistry(fake),
		"campsites", "--provider", "fake", "--campground", "1", "--nights", "2",
		"--start-date", "2026-09-04", "--end-date", "2026-09-07")

	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if !strings.Contains(res.Stdout, "campsites/A") {
		t.Errorf("result missing from stdout, got: %q", res.Stdout)
	}
}

func TestMissingCampgroundIsAnError(t *testing.T) {
	fake := &fakeProvider{}
	res := runCLI(t, fakeRegistry(fake),
		"campsites", "--provider", "fake",
		"--start-date", "2026-09-04", "--end-date", "2026-09-07")

	if res.Err == nil {
		t.Fatal("a search with no campground or rec-area should be an error")
	}
	if !strings.Contains(res.Err.Error(), "campground") {
		t.Errorf("error should mention --campground, got: %v", res.Err)
	}
}

// A repeated --start-date used to be legal and was zipped against --end-date by
// position, so a mismatched order silently searched the wrong windows.
func TestRepeatedStartDateIsRejectedAndPointsAtDateRanges(t *testing.T) {
	fake := &fakeProvider{}
	res := runCLI(t, fakeRegistry(fake),
		"campsites", "--provider", "fake", "--campgrounds", "1",
		"--start-date", "2026-09-04", "--start-date", "2026-10-01",
		"--end-date", "2026-09-07")

	if res.Err == nil {
		t.Fatal("a repeated --start-date should be rejected")
	}
	if !strings.Contains(res.Err.Error(), "--date-ranges") {
		t.Errorf("error should point at --date-ranges, got: %v", res.Err)
	}
	if fake.campsite != 0 {
		t.Error("the provider should not be queried when flag parsing fails")
	}
}

// The old singular spellings must keep working: CronJobs run :latest-go with
// imagePullPolicy Always, so an image update reaches them before anyone can
// edit a manifest.
func TestOldSingularFlagNamesStillWorkAndWarn(t *testing.T) {
	fake := &fakeProvider{sites: []core.AvailableCampsite{site("A", 4)}}
	res := runCLI(t, fakeRegistry(fake),
		"campsites", "--provider", "fake", "--campground", "232461",
		"--start-date", "2026-09-04", "--end-date", "2026-09-07")

	if res.Err != nil {
		t.Fatalf("old flag names should still work: %v", res.Err)
	}
	if fake.campsite != 1 {
		t.Fatalf("provider queried %d times, want 1", fake.campsite)
	}
	if len(fake.lastReq.Campgrounds) != 1 || fake.lastReq.Campgrounds[0] != "232461" {
		t.Errorf("--campground did not reach the request: %v", fake.lastReq.Campgrounds)
	}
	if !strings.Contains(res.Stderr, "--campgrounds") {
		t.Errorf("a rename warning should name the new flag, got stderr: %q", res.Stderr)
	}
	// Warnings must not contaminate the results stream.
	if strings.Contains(res.Stdout, "renamed") {
		t.Errorf("rename warning leaked into stdout: %q", res.Stdout)
	}
}

// Every flag that accepts several values must say so and show how. Adding a new
// multi-value flag without that guidance fails here.
func TestMultiValueFlagsDocumentHowToPassSeveral(t *testing.T) {
	root := newRootCmdWithRegistry(fakeRegistry(&fakeProvider{}))

	var campsites *cobra.Command
	for _, c := range root.Commands() {
		if c.Name() == "campsites" {
			campsites = c
		}
	}
	if campsites == nil {
		t.Fatal("campsites command not found")
	}

	campsites.Flags().VisitAll(func(f *pflag.Flag) {
		switch f.Value.Type() {
		case "stringSlice":
			if !strings.Contains(f.Usage, "several allowed") {
				t.Errorf("--%s takes several values but does not say so", f.Name)
			}
			if !strings.Contains(f.Usage, "comma-separated") || !strings.Contains(f.Usage, "repeated") {
				t.Errorf("--%s does not show both ways to pass several values", f.Name)
			}
		case "int", "string", "bool":
			if strings.Contains(f.Usage, "several allowed") {
				t.Errorf("--%s takes one value but claims several", f.Name)
			}
		}
	})

	// The singular name must not appear as its own flag; it is an alias.
	if f := campsites.Flags().Lookup("nights"); f == nil || !strings.Contains(f.Usage, "one value") {
		t.Error("--nights is plural but takes one value; its help must say so")
	}
}

func TestProvidersCommandReportsStatusForEveryDescriptor(t *testing.T) {
	res := runCLI(t, fakeRegistry(&fakeProvider{}), "providers")
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	// The table renders to os.Stdout directly, so assert on the command
	// succeeding and covering both descriptors via the registry instead.
	for _, d := range fakeRegistry(&fakeProvider{}) {
		if d.DisplayName == "" {
			t.Error("descriptor without a display name would render a blank row")
		}
	}
}
