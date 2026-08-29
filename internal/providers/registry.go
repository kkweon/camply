package providers

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kkweon/camply/internal/providers/recdotgov"
	"github.com/kkweon/camply/internal/providers/usedirect"
)

// Compile-time proof that both providers satisfy the shared interface.
//
// These assertions live here rather than in each provider package because this
// file imports those packages; asserting from inside them would be an import
// cycle.
var (
	_ Provider = (*recdotgov.Provider)(nil)
	_ Provider = (*usedirect.Provider)(nil)
)

// Key identifies a provider in command names and flag values.
type Key string

const (
	KeyRecDotGov         Key = "recdotgov"
	KeyReserveCalifornia Key = "reservecalifornia"
	KeyGoingToCamp       Key = "goingtocamp"
	KeyYellowstone       Key = "yellowstone"
)

// Status records whether a provider is usable today.
type Status int

const (
	// StatusSupported has a Go implementation and can be constructed.
	StatusSupported Status = iota
	// StatusPlanned is advertised by `camply providers` but has no Go
	// implementation yet. New is nil.
	StatusPlanned
)

// Descriptor carries everything the CLI needs to build a provider's commands:
// how to construct it, and which flags are meaningful for it.
//
// The capability fields exist because the providers genuinely differ. Treating
// them as interchangeable is what let a ReserveCalifornia equipment name reach
// RecreationDotGov and silently discard 423 of 667 campsites.
type Descriptor struct {
	Key         Key
	DisplayName string   // legacy --provider value, e.g. "RecreationDotGov"
	Aliases     []string // additional accepted spellings
	Description string
	Status      Status

	// New constructs the provider. Nil when Status is StatusPlanned.
	New func() Provider

	// SupportsState reports whether --state does anything. UseDirect ignores it.
	SupportsState bool

	// RecAreaIDHelp describes what a --rec-area value means here. The two
	// providers use disjoint ID namespaces, so one provider's ID silently
	// matches nothing on the other.
	RecAreaIDHelp string

	// Vocabularies returns the value sets this provider accepts, so a bad value
	// can be rejected with the valid ones listed rather than matching nothing.
	Vocabularies func() []Vocabulary
}

// Flags whose accepted values the vocabularies describe.
const (
	FlagEquipmentTypes = "equipment-types"
	FlagCampsiteTypes  = "campsite-types"
)

func descriptors() []Descriptor {
	return []Descriptor{
		{
			Key:           KeyRecDotGov,
			DisplayName:   "RecreationDotGov",
			Aliases:       []string{"recreation-dot-gov", "recgov"},
			Description:   "Recreation.gov API (US Federal)",
			Status:        StatusSupported,
			New:           func() Provider { return recdotgov.NewProvider() },
			SupportsState: true,
			RecAreaIDHelp: "RIDB Recreation Area ID (see 'camply recdotgov recreation-areas')",
			Vocabularies: func() []Vocabulary {
				return []Vocabulary{{
					Flag: FlagEquipmentTypes,
					// Open: recreation.gov returns whatever a campground
					// configured, and it differs between campgrounds in one
					// search. An unlisted name may still be real.
					Closed: false,
					Values: recdotgov.KnownEquipment,
					Source: "names observed on recreation.gov",
				}, {
					Flag:   FlagCampsiteTypes,
					Closed: false,
					Values: recdotgov.KnownCampsiteTypes,
					Source: "types observed on recreation.gov",
				}}
			},
		},
		{
			Key:           KeyReserveCalifornia,
			DisplayName:   usedirect.ReserveCaliforniaName,
			Aliases:       []string{"reserve-california", "resca"},
			Description:   "Reserve California API",
			Status:        StatusSupported,
			New:           func() Provider { return usedirect.NewReserveCalifornia() },
			SupportsState: false,
			RecAreaIDHelp: "ReserveCalifornia Place ID, an integer (see 'camply reservecalifornia recreation-areas')",
			Vocabularies: func() []Vocabulary {
				return []Vocabulary{{
					Flag: FlagEquipmentTypes,
					// Closed: these names are synthesized by camply, so nothing
					// else can be returned.
					Closed: true,
					Values: usedirect.SynthesizedEquipment,
					Source: "synthesized by camply from unit category and vehicle length",
				}, {
					Flag: FlagCampsiteTypes,
					// Open: the categories come from the live /rdr/search/filters
					// response, so camply cannot enumerate them ahead of time.
					Closed: false,
					Values: usedirect.KnownUnitCategories,
					Source: "unit categories from ReserveCalifornia's filters endpoint",
				}}
			},
		},
		{
			Key:         KeyGoingToCamp,
			DisplayName: "GoingToCamp",
			Description: "GoingToCamp.com API (Canada & US)",
			Status:      StatusPlanned,
		},
		{
			Key:         KeyYellowstone,
			DisplayName: "Yellowstone",
			Description: "Yellowstone National Park Lodges API",
			Status:      StatusPlanned,
		},
	}
}

// Descriptors returns every known provider, supported or not.
func Descriptors() []Descriptor { return descriptors() }

// Supported returns only the providers that can actually be constructed.
func Supported() []Descriptor {
	var out []Descriptor
	for _, d := range descriptors() {
		if d.Status == StatusSupported {
			out = append(out, d)
		}
	}
	return out
}

// SupportedNames returns the display names of constructible providers, sorted,
// for use in error messages.
func SupportedNames() []string {
	var names []string
	for _, d := range Supported() {
		names = append(names, d.DisplayName)
	}
	sort.Strings(names)
	return names
}

func normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// LookupIn resolves a name against the given descriptors' Key, DisplayName and
// Aliases, ignoring case. Commands take their registry as a parameter so tests
// can supply fakes.
func LookupIn(descs []Descriptor, name string) (Descriptor, bool) {
	n := normalize(name)
	if n == "" {
		return Descriptor{}, false
	}
	for _, d := range descs {
		if n == normalize(string(d.Key)) || n == normalize(d.DisplayName) {
			return d, true
		}
		for _, a := range d.Aliases {
			if n == normalize(a) {
				return d, true
			}
		}
	}
	return Descriptor{}, false
}

// Lookup resolves a name against the real registry.
func Lookup(name string) (Descriptor, bool) { return LookupIn(descriptors(), name) }

func supportedNamesIn(descs []Descriptor) []string {
	var names []string
	for _, d := range descs {
		if d.Status == StatusSupported {
			names = append(names, d.DisplayName)
		}
	}
	sort.Strings(names)
	return names
}

// NewFrom resolves and constructs a provider from the given registry.
//
// An unknown or unimplemented name is an error that names the usable providers,
// so the caller learns what to type instead of only what failed.
func NewFrom(descs []Descriptor, name string) (Provider, Descriptor, error) {
	usable := strings.Join(supportedNamesIn(descs), ", ")

	d, ok := LookupIn(descs, name)
	if !ok {
		return nil, Descriptor{}, fmt.Errorf(
			"unknown provider %q; supported providers are: %s", name, usable)
	}
	if d.Status != StatusSupported || d.New == nil {
		return nil, d, fmt.Errorf(
			"provider %q is not implemented in the Go rewrite yet; supported providers are: %s",
			d.DisplayName, usable)
	}
	return d.New(), d, nil
}

// New resolves a provider by name against the real registry.
func New(name string) (Provider, Descriptor, error) {
	return NewFrom(descriptors(), name)
}
