package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/spf13/pflag"
)

func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// The pairing of a start and an end must live inside one value. When it was
// spread across two parallel slices, a mismatched order passed validation and
// searched the wrong windows.
func TestParseDateWindows(t *testing.T) {
	tests := []struct {
		name       string
		ranges     []string
		start, end string
		want       []dateWindow
		wantErr    string
	}{
		{
			name:   "single range",
			ranges: []string{"2026-09-04:2026-09-07"},
			want:   []dateWindow{{day(2026, 9, 4), day(2026, 9, 7)}},
		},
		{
			name:   "comma separated ranges",
			ranges: []string{"2026-09-04:2026-09-07", "2026-10-01:2026-10-05"},
			want: []dateWindow{
				{day(2026, 9, 4), day(2026, 9, 7)},
				{day(2026, 10, 1), day(2026, 10, 5)},
			},
		},
		{
			name:   "order of ranges does not change the pairing",
			ranges: []string{"2026-10-01:2026-10-05", "2026-09-04:2026-09-07"},
			want: []dateWindow{
				{day(2026, 10, 1), day(2026, 10, 5)},
				{day(2026, 9, 4), day(2026, 9, 7)},
			},
		},
		{
			name:  "start and end pair",
			start: "2026-09-04", end: "2026-09-07",
			want: []dateWindow{{day(2026, 9, 4), day(2026, 9, 7)}},
		},
		{
			name:    "reversed window",
			ranges:  []string{"2026-09-07:2026-09-04"},
			wantErr: "ends before it starts",
		},
		{
			name:    "missing separator",
			ranges:  []string{"2026-09-04"},
			wantErr: "missing the \":\" separator",
		},
		{
			name:    "unparseable date",
			ranges:  []string{"09/04/2026:2026-09-07"},
			wantErr: "expected YYYY-MM-DD",
		},
		{
			name:    "start without end",
			start:   "2026-09-04",
			wantErr: "needs both",
		},
		{
			name:    "end without start",
			end:     "2026-09-07",
			wantErr: "needs both",
		},
		{
			name:   "ranges combined with the pair",
			ranges: []string{"2026-10-01:2026-10-05"},
			start:  "2026-09-04", end: "2026-09-07",
			wantErr: "cannot be combined",
		},
		{
			name:    "nothing given",
			wantErr: "no search window given",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDateWindows(tt.ranges, tt.start, tt.end)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("want error containing %q, got none", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %q, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d windows, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if !got[i].Start.Equal(tt.want[i].Start) || !got[i].End.Equal(tt.want[i].End) {
					t.Errorf("window %d = %v..%v, want %v..%v",
						i, got[i].Start, got[i].End, tt.want[i].Start, tt.want[i].End)
				}
			}
		})
	}
}

// A suggestion the user cannot paste back is worse than none.
func TestCombinedFlagsErrorSuggestsAValidCommand(t *testing.T) {
	_, err := parseDateWindows([]string{"2026-10-01:2026-10-05"}, "2026-09-04", "")
	if err == nil {
		t.Fatal("want an error")
	}
	// Only start was given, so no half-built "2026-09-04:" may appear.
	if strings.Contains(err.Error(), "2026-09-04:") {
		t.Errorf("error suggests an incomplete window: %v", err)
	}

	_, err = parseDateWindows([]string{"2026-10-01:2026-10-05"}, "2026-09-04", "2026-09-07")
	if err == nil {
		t.Fatal("want an error")
	}
	if !strings.Contains(err.Error(), "2026-10-01:2026-10-05,2026-09-04:2026-09-07") {
		t.Errorf("error should merge both windows into one suggestion: %v", err)
	}
}

func TestSingleValueRejectsRepetition(t *testing.T) {
	v := singleValue{name: "start-date", hint: multiWindowHint}

	if err := v.Set("2026-09-04"); err != nil {
		t.Fatalf("first Set failed: %v", err)
	}
	err := v.Set("2026-10-01")
	if err == nil {
		t.Fatal("a second --start-date should be rejected")
	}
	if !strings.Contains(err.Error(), "--date-ranges") {
		t.Errorf("error should point at --date-ranges, got: %v", err)
	}
	if v.value != "2026-09-04" {
		t.Errorf("value = %q, want the first one retained", v.value)
	}
}

func TestAliasNormalizerRewritesAndRecords(t *testing.T) {
	var used map[string]string
	fs := pflag.NewFlagSet("t", pflag.ContinueOnError)
	fs.SetNormalizeFunc(aliasNormalizer(&used))

	var campgrounds, equipment []string
	fs.StringSliceVar(&campgrounds, "campgrounds", nil, "")
	fs.StringSliceVar(&equipment, "equipment-types", nil, "")

	if err := fs.Parse([]string{"--campground", "232461", "--equipment", "Tent"}); err != nil {
		t.Fatalf("old singular names should still parse: %v", err)
	}

	if len(campgrounds) != 1 || campgrounds[0] != "232461" {
		t.Errorf("--campground did not reach --campgrounds: %v", campgrounds)
	}
	if len(equipment) != 1 || equipment[0] != "Tent" {
		t.Errorf("--equipment did not reach --equipment-types: %v", equipment)
	}

	warnings := renamedFlagWarnings(used)
	if len(warnings) != 2 {
		t.Fatalf("got %d warnings, want 2: %v", len(warnings), warnings)
	}
	joined := strings.Join(warnings, "\n")
	for _, want := range []string{"--campground", "--campgrounds", "--equipment", "--equipment-types"} {
		if !strings.Contains(joined, want) {
			t.Errorf("warnings should mention %s, got: %s", want, joined)
		}
	}
}

func TestNoWarningsWhenNewNamesAreUsed(t *testing.T) {
	var used map[string]string
	fs := pflag.NewFlagSet("t", pflag.ContinueOnError)
	fs.SetNormalizeFunc(aliasNormalizer(&used))

	var campgrounds []string
	fs.StringSliceVar(&campgrounds, "campgrounds", nil, "")

	if err := fs.Parse([]string{"--campgrounds", "232461,234039"}); err != nil {
		t.Fatal(err)
	}
	if w := renamedFlagWarnings(used); len(w) != 0 {
		t.Errorf("new names should not warn, got: %v", w)
	}
}
