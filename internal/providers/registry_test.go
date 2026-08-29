package providers

import (
	"strings"
	"testing"
)

func TestLookupIsCaseInsensitiveAcrossKeyNameAndAliases(t *testing.T) {
	tests := []struct {
		input string
		want  Key
	}{
		{"recdotgov", KeyRecDotGov},
		{"RecreationDotGov", KeyRecDotGov},
		{"recreationdotgov", KeyRecDotGov},
		{"  RECDOTGOV  ", KeyRecDotGov},
		{"recgov", KeyRecDotGov},
		{"reservecalifornia", KeyReserveCalifornia},
		{"ReserveCalifornia", KeyReserveCalifornia},
		{"reserve-california", KeyReserveCalifornia},
		{"resca", KeyReserveCalifornia},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			d, ok := Lookup(tt.input)
			if !ok {
				t.Fatalf("Lookup(%q) failed", tt.input)
			}
			if d.Key != tt.want {
				t.Errorf("Lookup(%q) = %q, want %q", tt.input, d.Key, tt.want)
			}
		})
	}
}

func TestLookupRejectsUnknown(t *testing.T) {
	for _, in := range []string{"", "   ", "nope", "recdotgo"} {
		if _, ok := Lookup(in); ok {
			t.Errorf("Lookup(%q) unexpectedly succeeded", in)
		}
	}
}

// A descriptor that claims to be supported but cannot be built would crash at
// use; one that is planned but constructible would contradict `camply providers`.
func TestDescriptorStatusMatchesConstructor(t *testing.T) {
	for _, d := range Descriptors() {
		switch d.Status {
		case StatusSupported:
			if d.New == nil {
				t.Errorf("%s is StatusSupported but has no New", d.Key)
				continue
			}
			if p := d.New(); p == nil {
				t.Errorf("%s New() returned nil", d.Key)
			}
		case StatusPlanned:
			if d.New != nil {
				t.Errorf("%s is StatusPlanned but has a New constructor", d.Key)
			}
		}
		if d.DisplayName == "" || d.Description == "" {
			t.Errorf("%s is missing DisplayName or Description", d.Key)
		}
	}
}

// Every supported provider must say what its --rec-area IDs mean: the two
// namespaces are disjoint, and passing one provider's ID to the other matches
// nothing at all.
func TestSupportedProvidersDocumentRecAreaIDs(t *testing.T) {
	for _, d := range Supported() {
		if strings.TrimSpace(d.RecAreaIDHelp) == "" {
			t.Errorf("%s does not document what --rec-area means", d.Key)
		}
	}
}

func TestNewRejectsPlannedProviderWithActionableMessage(t *testing.T) {
	_, _, err := New("GoingToCamp")
	if err == nil {
		t.Fatal("New(GoingToCamp) should fail; it has no Go implementation")
	}
	// The point of the error is telling the user what to type instead.
	for _, want := range []string{"RecreationDotGov", "ReserveCalifornia"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should list %q as usable, got: %v", want, err)
		}
	}
}

func TestNewRejectsUnknownProviderWithActionableMessage(t *testing.T) {
	_, _, err := New("Yosemite")
	if err == nil {
		t.Fatal("New(Yosemite) should fail")
	}
	if !strings.Contains(err.Error(), "supported providers are") {
		t.Errorf("error should list supported providers, got: %v", err)
	}
}

func TestNewConstructsSupportedProviders(t *testing.T) {
	for _, d := range Supported() {
		p, got, err := New(string(d.Key))
		if err != nil {
			t.Errorf("New(%q) failed: %v", d.Key, err)
			continue
		}
		if p == nil {
			t.Errorf("New(%q) returned a nil provider", d.Key)
		}
		if got.Key != d.Key {
			t.Errorf("New(%q) returned descriptor %q", d.Key, got.Key)
		}
	}
}
