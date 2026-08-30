package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/pflag"
)

const (
	dateLayout   = "2006-01-02"
	dateRangeSep = ":"
)

// Flags that accept several values are named in the plural and take each value
// standalone, so no two flags have to be lined up by position to mean anything.
//
// The old singular spellings keep working through flagAliases; see
// aliasNormalizer.
var flagAliases = map[string]string{
	"campground": "campgrounds",
	"rec-area":   "rec-areas",
	"campsite":   "campsites",
	"equipment":  "equipment-types",
}

// aliasNormalizer maps the old singular flag names onto their plural
// replacements and records which ones were used, so the command can tell the
// user what to rename without breaking their existing invocation.
//
// pflag also runs this when flags are defined, but definitions already use the
// new names, so only user input lands in `used`.
func aliasNormalizer(used *map[string]string) func(*pflag.FlagSet, string) pflag.NormalizedName {
	return func(_ *pflag.FlagSet, name string) pflag.NormalizedName {
		if to, ok := flagAliases[name]; ok {
			if *used == nil {
				*used = map[string]string{}
			}
			(*used)[name] = to
			return pflag.NormalizedName(to)
		}
		return pflag.NormalizedName(name)
	}
}

// renamedFlagWarnings renders one line per deprecated spelling the user typed.
func renamedFlagWarnings(used map[string]string) []string {
	if len(used) == 0 {
		return nil
	}
	var from []string
	for old := range used {
		from = append(from, old)
	}
	sort.Strings(from)

	var out []string
	for _, old := range from {
		out = append(out, fmt.Sprintf(
			"--%s was renamed to --%s (it accepts several values). Both work for now.",
			old, used[old]))
	}
	return out
}

// singleValue is a string flag that refuses to be given twice.
//
// --start-date and --end-date used to be slices zipped together by position, so
// `--start-date A --start-date B --end-date D --end-date C` passed validation
// and silently searched the wrong windows. Rejecting the second occurrence
// makes that unrepresentable, and points at the flag that does express several
// windows.
type singleValue struct {
	name  string
	hint  string
	value string
	set   bool
}

func (s *singleValue) String() string { return s.value }
func (s *singleValue) Type() string   { return "string" }

func (s *singleValue) Set(v string) error {
	if s.set {
		return fmt.Errorf("--%s was given more than once. %s", s.name, s.hint)
	}
	s.value, s.set = v, true
	return nil
}

// dateWindow is one search window. Start and end travel together so their
// pairing cannot be scrambled by flag order.
type dateWindow struct {
	Start time.Time
	End   time.Time
}

const multiWindowHint = "To search several windows, use --date-ranges " +
	"START:END, e.g. --date-ranges 2026-09-04:2026-09-07,2026-10-01:2026-10-05"

// parseDateWindows resolves --date-ranges and the --start-date/--end-date pair
// into search windows.
func parseDateWindows(ranges []string, start, end string) ([]dateWindow, error) {
	hasPair := start != "" || end != ""

	switch {
	case len(ranges) > 0 && hasPair:
		// Only suggest a merged value when both halves are present; half a
		// window pasted back in would just fail again.
		if start != "" && end != "" {
			return nil, fmt.Errorf(
				"--date-ranges cannot be combined with --start-date/--end-date. Use one or the other:\n"+
					"    --date-ranges %s,%s%s%s",
				strings.Join(ranges, ","), start, dateRangeSep, end)
		}
		return nil, fmt.Errorf(
			"--date-ranges cannot be combined with --start-date/--end-date. Use one or the other:\n"+
				"    --date-ranges %s", strings.Join(ranges, ","))

	case len(ranges) > 0:
		return parseRangeValues(ranges)

	case start != "" && end != "":
		w, err := newDateWindow(start, end)
		if err != nil {
			return nil, err
		}
		return []dateWindow{w}, nil

	case start != "":
		return nil, fmt.Errorf("--start-date given without --end-date; a search window needs both")

	case end != "":
		return nil, fmt.Errorf("--end-date given without --start-date; a search window needs both")

	default:
		return nil, fmt.Errorf(
			"no search window given. Use --start-date and --end-date, or --date-ranges START%sEND",
			dateRangeSep)
	}
}

func parseRangeValues(ranges []string) ([]dateWindow, error) {
	var out []dateWindow
	for _, raw := range ranges {
		value := strings.TrimSpace(raw)
		startStr, endStr, found := strings.Cut(value, dateRangeSep)
		if !found {
			return nil, fmt.Errorf(
				"--date-ranges %q is missing the %q separator. Each value is one window:\n"+
					"    --date-ranges 2026-09-04%s2026-09-07",
				value, dateRangeSep, dateRangeSep)
		}
		w, err := newDateWindow(strings.TrimSpace(startStr), strings.TrimSpace(endStr))
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, nil
}

func newDateWindow(startStr, endStr string) (dateWindow, error) {
	start, err := time.Parse(dateLayout, startStr)
	if err != nil {
		return dateWindow{}, fmt.Errorf("invalid start date %q; expected YYYY-MM-DD", startStr)
	}
	end, err := time.Parse(dateLayout, endStr)
	if err != nil {
		return dateWindow{}, fmt.Errorf("invalid end date %q; expected YYYY-MM-DD", endStr)
	}
	if end.Before(start) {
		return dateWindow{}, fmt.Errorf(
			"search window %s%s%s ends before it starts", startStr, dateRangeSep, endStr)
	}
	return dateWindow{Start: start, End: end}, nil
}

// splitWindows returns the parallel slices core.SearchRequest still expects.
func splitWindows(windows []dateWindow) (starts, ends []time.Time) {
	for _, w := range windows {
		starts = append(starts, w.Start)
		ends = append(ends, w.End)
	}
	return starts, ends
}

// multiHelp spells out both accepted ways of passing several values, so the
// help text answers "how do I give more than one?" without a trip to the docs.
//
// csv is a comma-joined example; repeatA and repeatB show the same thing via a
// repeated flag.
func multiHelp(desc, csv, repeatA, repeatB string) string {
	return fmt.Sprintf("%s (several allowed).\ncomma-separated: --%s\nor repeated:     --%s --%s",
		desc, csv, repeatA, repeatB)
}

// flagExcludeNoVehicleAccess is named once so the flag registration and every
// message that tells a user to pass it cannot drift apart.
const flagExcludeNoVehicleAccess = "exclude-no-vehicle-access"
