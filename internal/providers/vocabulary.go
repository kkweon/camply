package providers

import (
	"github.com/kkweon/camply/internal/suggest"
)

// Vocabulary is the set of values one provider accepts for one flag.
type Vocabulary struct {
	Flag string

	// Closed reports whether Values is exhaustive. UseDirect synthesizes its
	// equipment names, so nothing outside the list can ever be returned.
	// Recreation.gov forwards whatever a campground configured, so its list is
	// a curated sample and an unlisted name may still be legitimate.
	Closed bool

	Values []string

	// Source says where the list came from, for error messages.
	Source string
}

// Term locates one value: which provider's which flag accepts it.
type Term struct {
	Provider Key
	Display  string
	Flag     string
	Value    string
}

// VocabulariesFor returns the value sets a provider accepts.
func VocabulariesFor(d Descriptor) []Vocabulary {
	if d.Vocabularies == nil {
		return nil
	}
	return d.Vocabularies()
}

// LookupVocabulary finds one provider's list for one flag.
func LookupVocabulary(d Descriptor, flag string) (Vocabulary, bool) {
	for _, v := range VocabulariesFor(d) {
		if v.Flag == flag {
			return v, true
		}
	}
	return Vocabulary{}, false
}

// FindElsewhere reports which other providers accept this value.
//
// This is what lets an error say "Vehicle is a ReserveCalifornia name" instead
// of only "Vehicle did not match anything" — the difference between a message
// the user can act on and one that leaves them guessing. It needs every
// provider's vocabulary at once, not just the active one.
func FindElsewhere(descs []Descriptor, value string, exclude Key) []Term {
	want := suggest.Normalize(value)
	if want == "" {
		return nil
	}

	var found []Term
	for _, d := range descs {
		if d.Key == exclude || d.Status != StatusSupported {
			continue
		}
		for _, v := range VocabulariesFor(d) {
			for _, candidate := range v.Values {
				if suggest.Normalize(candidate) == want {
					found = append(found, Term{
						Provider: d.Key,
						Display:  d.DisplayName,
						Flag:     v.Flag,
						Value:    candidate,
					})
				}
			}
		}
	}
	return found
}
