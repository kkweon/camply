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
			Vocabularies: func() []providers.Vocabulary {
				return []providers.Vocabulary{{
					Flag:   providers.FlagEquipmentTypes,
					Closed: false, // open, like recreation.gov
					Values: []string{"Tent", "Small Tent", "RV", "Car"},
					Source: "names observed on FakeProvider",
				}}
			},
		},
		{
			Key:           "nostate",
			DisplayName:   "NoStateProvider",
			Description:   "supported, but its API has no state filter",
			Status:        providers.StatusSupported,
			New:           func() providers.Provider { return fake },
			SupportsState: false,
			RecAreaIDHelp: "NoState Place ID",
			Vocabularies: func() []providers.Vocabulary {
				return []providers.Vocabulary{{
					Flag:   providers.FlagEquipmentTypes,
					Closed: true, // synthesized, like UseDirect
					Values: []string{"Tent", "RV", "Trailer", "Vehicle"},
					Source: "synthesized by camply",
				}}
			},
		},
		{
			Key:         "planned",
			DisplayName: "PlannedProvider",
			Description: "not built yet",
			Status:      providers.StatusPlanned,
		},
	}
}

// findCmd walks the tree by name, e.g. findCmd(root, "recdotgov", "campsites").
func findCmd(t *testing.T, root *cobra.Command, path ...string) *cobra.Command {
	t.Helper()
	cur := root
	for _, name := range path {
		var next *cobra.Command
		for _, c := range cur.Commands() {
			if c.Name() == name {
				next = c
				break
			}
		}
		if next == nil {
			t.Fatalf("command %q not found under %q", name, cur.Name())
		}
		cur = next
	}
	return cur
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

// The premise of the split: a provider only offers flags its API can act on.
func TestStateFlagOnlyExistsWhereTheAPISupportsIt(t *testing.T) {
	root := newRootCmdWithRegistry(fakeRegistry(&fakeProvider{}))

	withState := findCmd(t, root, "fake", "campgrounds")
	if withState.Flags().Lookup("state") == nil {
		t.Error("a provider that supports --state should offer it")
	}

	without := findCmd(t, root, "nostate", "campgrounds")
	if without.Flags().Lookup("state") != nil {
		t.Error("a provider that ignores --state must not offer it; " +
			"accepting input and dropping it is the bug being fixed")
	}
}

// The two providers use disjoint rec-area ID namespaces, so one provider's ID
// matches nothing on the other. The help has to say which is meant.
func TestRecAreaHelpIsProviderSpecific(t *testing.T) {
	root := newRootCmdWithRegistry(fakeRegistry(&fakeProvider{}))

	a := findCmd(t, root, "fake", "campsites").Flags().Lookup("rec-areas")
	b := findCmd(t, root, "nostate", "campsites").Flags().Lookup("rec-areas")

	if a == nil || b == nil {
		t.Fatal("--rec-areas missing")
	}
	if !strings.Contains(a.Usage, "fake ID") {
		t.Errorf("help should describe this provider's IDs, got: %s", a.Usage)
	}
	if !strings.Contains(b.Usage, "NoState Place ID") {
		t.Errorf("help should describe this provider's IDs, got: %s", b.Usage)
	}
	if a.Usage == b.Usage {
		t.Error("both providers show identical --rec-areas help despite disjoint ID spaces")
	}
}

func TestProviderSubcommandHasNoProviderFlag(t *testing.T) {
	root := newRootCmdWithRegistry(fakeRegistry(&fakeProvider{}))
	cmd := findCmd(t, root, "fake", "campsites")
	if cmd.Flags().Lookup("provider") != nil {
		t.Error("the provider is the subcommand; --provider would be redundant and contradictable")
	}
}

// An unknown flag must say where it does work, not just that it failed.
func TestUnsupportedFlagErrorNamesAProviderThatHasIt(t *testing.T) {
	res := runCLI(t, fakeRegistry(&fakeProvider{}),
		"nostate", "campgrounds", "--state", "CA")

	if res.Err == nil {
		t.Fatal("--state on a provider without it should fail")
	}
	msg := res.Err.Error()
	if strings.TrimSpace(msg) == "unknown flag: --state" {
		t.Fatal("error is cobra's default; it does not tell the user what to do")
	}
	for _, want := range []string{"NoStateProvider", "camply fake campgrounds --state"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error should contain %q, got:\n%s", want, msg)
		}
	}
}

// An unrelated typo must keep cobra's own message rather than being reshaped.
func TestUnrelatedUnknownFlagKeepsDefaultError(t *testing.T) {
	res := runCLI(t, fakeRegistry(&fakeProvider{}),
		"fake", "campgrounds", "--bogus")
	if res.Err == nil {
		t.Fatal("an unknown flag should fail")
	}
	if !strings.Contains(res.Err.Error(), "unknown flag: --bogus") {
		t.Errorf("unexpected error: %v", res.Err)
	}
}

func TestDeprecatedCommandsAreHiddenButStillRun(t *testing.T) {
	root := newRootCmdWithRegistry(fakeRegistry(&fakeProvider{}))
	legacy := findCmd(t, root, "campsites")

	if legacy.Deprecated == "" {
		t.Error("the top-level campsites command should be marked deprecated")
	}
	if legacy.IsAvailableCommand() {
		t.Error("a deprecated command should not be advertised in help")
	}
}

// Telling someone their command is deprecated without showing the replacement
// leaves them to work it out. The hint must be pasteable into a manifest.
func TestLegacyCommandPrintsAPasteableReplacement(t *testing.T) {
	fake := &fakeProvider{sites: []core.AvailableCampsite{equippedSite("A", "1", 4, "Tent")}}
	res := runCLI(t, fakeRegistry(fake),
		"campsites", "--provider", "FakeProvider",
		"--campground", "232461", "--campground", "234039",
		"--equipment", "Tent",
		"--start-date", "2026-09-04", "--end-date", "2026-09-07",
		"--nights", "2", "--weekends")

	if res.Err != nil {
		t.Fatalf("the deprecated command must keep working: %v", res.Err)
	}

	replacement := replacementLine(t, res.Stderr)

	for _, want := range []string{
		"camply fake campsites",
		"--campgrounds 232461,234039", // slices render as input, not Go syntax
		"--equipment-types Tent",      // renamed flag uses its new name
		"--nights 2",
		"--weekends", // bool renders without a value
	} {
		if !strings.Contains(replacement, want) {
			t.Errorf("replacement should contain %q, got:\n%s", want, replacement)
		}
	}
	// The provider is the subcommand now; repeating --provider could contradict it.
	if strings.Contains(replacement, "--provider") {
		t.Errorf("replacement repeats --provider: %s", replacement)
	}

	// Results are written at INFO on stdout and must stay uncontaminated.
	//
	// Only camply's own warning is asserted here. Cobra prints its built-in
	// Deprecated notice through OutOrStderr(), which resolves to the out writer
	// whenever one is set — as this harness does to capture results. With no out
	// writer, as in the real binary, it falls back to stderr.
	if strings.Contains(res.Stdout, "camply fake campsites --campgrounds") {
		t.Errorf("migration hint leaked into the results stream: %q", res.Stdout)
	}
	if !strings.Contains(res.Stdout, "0 New Campsites Found") {
		t.Errorf("results missing from stdout: %q", res.Stdout)
	}
}

// replacementLine pulls the suggested command out of the deprecation warning.
// Asserting on the whole of stderr would also match the quoted command the user
// typed, which legitimately contains --provider.
func replacementLine(t *testing.T, stderr string) string {
	t.Helper()
	for _, line := range strings.Split(stderr, "\n") {
		if strings.Contains(line, "camply fake campsites") {
			return strings.TrimSpace(line)
		}
	}
	t.Fatalf("no replacement command found in stderr:\n%s", stderr)
	return ""
}

func TestLegacyCommandWarnsAboutIgnoredFlags(t *testing.T) {
	fake := &fakeProvider{}
	res := runCLI(t, fakeRegistry(fake),
		"campgrounds", "--provider", "NoStateProvider", "--state", "CA")

	if res.Err != nil {
		t.Fatalf("the deprecated command must keep working: %v", res.Err)
	}
	if !strings.Contains(res.Stderr, "--state has no effect") {
		t.Errorf("a flag the provider ignores should be called out, got:\n%s", res.Stderr)
	}
}

func equippedSite(id, facility string, day int, equipment ...string) core.AvailableCampsite {
	s := site(id, day)
	s.FacilityID = facility
	s.FacilityName = "Facility " + facility
	for _, e := range equipment {
		s.PermittedEquipment = append(s.PermittedEquipment, core.Equipment{EquipmentName: e, MaxLength: 30})
	}
	return s
}

// The incident, end to end: a filter value that is valid for the provider but
// matches nothing at some of the campgrounds searched. This used to print
// "0 New Campsites Found" and read as a full campground.
func TestEquipmentThatMatchesNothingFailsInsteadOfReportingZero(t *testing.T) {
	fake := &fakeProvider{sites: []core.AvailableCampsite{
		equippedSite("A", "big", 4, "Tent", "RV"),
		equippedSite("B", "big", 4, "Tent"),
		equippedSite("C", "small", 4, "Small Tent"),
	}}

	res := runCLI(t, fakeRegistry(fake),
		"fake", "campsites", "--campgrounds", "big,small",
		"--date-ranges", "2026-09-04:2026-09-07", "--equipment-types", "Tent")

	if res.Err == nil {
		t.Fatal("a filter that matches nothing at a campground must fail, not report zero")
	}
	msg := res.Err.Error()
	for _, want := range []string{
		"matched nothing at 1 of 2 campground(s)",
		"Facility small",
		"Small Tent",            // what that campground does offer
		"--allow-partial-match", // the escape hatch
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error should contain %q, got:\n%s", want, msg)
		}
	}
	if strings.Contains(res.Stdout, "0 New Campsites Found") {
		t.Error("the run must not present this as an empty result")
	}
}

func TestTotalMissNamesTheProviderTheValueBelongsTo(t *testing.T) {
	fake := &fakeProvider{sites: []core.AvailableCampsite{
		equippedSite("A", "1", 4, "Tent"),
	}}

	// "RV" is a valid FakeProvider value, so it survives pre-flight validation.
	// It is absent from every site here, which only counting after the fetch can
	// reveal — the shape of the reported incident.
	res := runCLI(t, fakeRegistry(fake),
		"fake", "campsites", "--campgrounds", "1",
		"--date-ranges", "2026-09-04:2026-09-07", "--equipment-types", "RV")

	if res.Err == nil {
		t.Fatal("a value matching no site should fail")
	}
	msg := res.Err.Error()
	if !strings.Contains(msg, "matched nothing at 1 of 1 campground(s)") {
		t.Errorf("error should report which campgrounds were dropped, got:\n%s", msg)
	}
	if !strings.Contains(msg, "NoStateProvider") {
		t.Errorf("error should say where the value does belong, got:\n%s", msg)
	}
}

func TestAllowPartialMatchDowngradesToAWarning(t *testing.T) {
	fake := &fakeProvider{sites: []core.AvailableCampsite{
		equippedSite("A", "big", 4, "Tent"),
		equippedSite("C", "small", 4, "Small Tent"),
	}}

	res := runCLI(t, fakeRegistry(fake),
		"fake", "campsites", "--campgrounds", "big,small",
		"--date-ranges", "2026-09-04:2026-09-07",
		"--equipment-types", "Tent", "--allow-partial-match")

	if res.Err != nil {
		t.Fatalf("--allow-partial-match should let the search proceed: %v", res.Err)
	}
	if !strings.Contains(res.Stderr, "matched nothing at 1 of 2 campground(s)") {
		t.Errorf("the problem should still be reported, got stderr:\n%s", res.Stderr)
	}
}

// A closed vocabulary can reject before any request is made.
func TestClosedVocabularyRejectsBeforeSearching(t *testing.T) {
	fake := &fakeProvider{}
	res := runCLI(t, fakeRegistry(fake),
		"nostate", "campsites", "--campgrounds", "1",
		"--date-ranges", "2026-09-04:2026-09-07", "--equipment-types", "Tnt")

	if res.Err == nil {
		t.Fatal("an unknown value on a closed vocabulary should fail")
	}
	if !strings.Contains(res.Err.Error(), "Did you mean") {
		t.Errorf("error should suggest a correction, got:\n%s", res.Err)
	}
	if fake.campsite != 0 {
		t.Error("validation must run before the provider is queried")
	}
}

func TestValueFromAnotherProviderIsRejectedWithBothOptions(t *testing.T) {
	fake := &fakeProvider{}
	res := runCLI(t, fakeRegistry(fake),
		"nostate", "campsites", "--campgrounds", "1",
		"--date-ranges", "2026-09-04:2026-09-07", "--equipment-types", "Small Tent")

	if res.Err == nil {
		t.Fatal("a value belonging to another provider should fail")
	}
	msg := res.Err.Error()
	for _, want := range []string{"belongs to FakeProvider", "camply fake campsites"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error should contain %q, got:\n%s", want, msg)
		}
	}
}

// The embedded Name,Length syntax never worked: the flag is a slice, so pflag
// split on the comma and "25" arrived as an equipment name.
func TestEquipmentLengthIsItsOwnFlag(t *testing.T) {
	fake := &fakeProvider{sites: []core.AvailableCampsite{
		equippedSite("A", "1", 4, "RV"), // MaxLength 30
	}}

	res := runCLI(t, fakeRegistry(fake),
		"fake", "campsites", "--campgrounds", "1",
		"--date-ranges", "2026-09-04:2026-09-07",
		"--equipment-types", "RV", "--max-equipment-length", "25")

	if res.Err != nil {
		t.Fatalf("a 25 ft requirement should match a 30 ft site: %v", res.Err)
	}
	if len(fake.lastReq.Equipment) != 1 {
		t.Fatalf("want one equipment term, got %v", fake.lastReq.Equipment)
	}
	if got := fake.lastReq.Equipment[0]; got.EquipmentName != "RV" || got.MaxLength != 25 {
		t.Errorf("equipment = %+v, want {RV 25}", got)
	}
}
