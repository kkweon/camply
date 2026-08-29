package suggest

import (
	"strings"
	"testing"
)

func TestClosest(t *testing.T) {
	recgov := []string{"Tent", "Small Tent", "Large Tent Over 9X12`", "RV", "Trailer", "Pop up", "Car"}

	tests := []struct {
		name       string
		input      string
		candidates []string
		wantFirst  string
		wantNone   bool
	}{
		{name: "typo resolves by edit distance", input: "Tnt", candidates: recgov, wantFirst: "Tent"},
		{name: "exact match wins", input: "Tent", candidates: recgov, wantFirst: "Tent"},
		{name: "case and spacing are ignored", input: "  small   tent ", candidates: recgov, wantFirst: "Small Tent"},
		{name: "nothing similar", input: "zzzzzzzz", candidates: recgov, wantNone: true},
		{name: "empty input", input: "", candidates: recgov, wantNone: true},
		{name: "no candidates", input: "Tent", candidates: nil, wantNone: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Closest(tt.input, tt.candidates, 3)
			if tt.wantNone {
				if len(got) != 0 {
					t.Errorf("want no suggestions, got %v", got)
				}
				return
			}
			if len(got) == 0 {
				t.Fatalf("want %q first, got none", tt.wantFirst)
			}
			if got[0] != tt.wantFirst {
				t.Errorf("first = %q, want %q (all: %v)", got[0], tt.wantFirst, got)
			}
		})
	}
}

// recreation.gov's vocabulary is full of longer names built around a shorter
// one, and those are more useful than anything edit distance would surface.
func TestClosestPrefersContainmentOverEditDistance(t *testing.T) {
	got := Closest("Tent", []string{"Small Tent", "Large Tent Over 9X12`", "Rent", "RV"}, 3)
	if len(got) < 2 {
		t.Fatalf("want the tent variants, got %v", got)
	}
	joined := strings.Join(got[:2], "|")
	if !strings.Contains(joined, "Tent") {
		t.Errorf("containment matches should rank first, got %v", got)
	}
	if got[0] == "Rent" {
		t.Errorf("an edit-distance match outranked a containment match: %v", got)
	}
}

func TestClosestRespectsLimit(t *testing.T) {
	got := Closest("Tent", []string{"Tent", "Small Tent", "Large Tent Over 9X12`"}, 2)
	if len(got) != 2 {
		t.Errorf("got %d suggestions, want 2: %v", len(got), got)
	}
}
