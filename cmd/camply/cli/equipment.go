package cli

import (
	"fmt"
	"strings"

	"github.com/kkweon/camply/internal/core"
	"github.com/kkweon/camply/internal/logger"
	"github.com/kkweon/camply/internal/providers"
	"github.com/kkweon/camply/internal/suggest"
)

// buildEquipmentFilter turns the flag values into filter terms.
//
// The length used to be embedded as "Name,Length", which never worked: the flag
// is a string slice, so pflag split on the comma first and "25" arrived as a
// second equipment name. The constraint is its own flag now.
// A length with nothing to apply it to is silently dropped: with no equipment
// terms, Filter.Apply skips equipment entirely and the constraint never runs.
// That is the same class of quiet failure this change exists to remove.
func validateEquipmentLength(names []string, maxLength int) error {
	if maxLength > 0 && len(names) == 0 {
		return fmt.Errorf(
			"--max-equipment-length has no effect without --%s; it constrains which "+
				"equipment types are acceptable.\n    --%s Tent --max-equipment-length %d",
			providers.FlagEquipmentTypes, providers.FlagEquipmentTypes, maxLength)
	}
	return nil
}

func buildEquipmentFilter(names []string, maxLength int) []core.Equipment {
	out := make([]core.Equipment, 0, len(names))
	for _, n := range names {
		out = append(out, core.Equipment{
			EquipmentName: strings.TrimSpace(n),
			MaxLength:     maxLength,
		})
	}
	return out
}

// describeEquipmentFilter renders the parsed filter for the run log, so the
// operator can see what the flags actually became.
func describeEquipmentFilter(eq []core.Equipment) string {
	if len(eq) == 0 {
		return ""
	}
	names := make([]string, 0, len(eq))
	for _, e := range eq {
		names = append(names, e.EquipmentName)
	}
	desc := strings.Join(names, ", ")
	if eq[0].MaxLength > 0 {
		desc += fmt.Sprintf(" (site must fit %d ft)", eq[0].MaxLength)
	}
	return desc
}

// validateEquipmentTypes checks the requested names against the provider's
// vocabulary before any HTTP call.
//
// This alone does not catch the incident that motivated the work — "Vehicle" is
// a real recreation.gov name, so it passes here and only the post-fetch
// coverage check can catch it. What this does catch is a name belonging to a
// different provider entirely, and plain typos.
func validateVocabularyValues(flag string, names []string, d providers.Descriptor, registry []providers.Descriptor) error {
	vocab, ok := providers.LookupVocabulary(d, flag)
	if !ok {
		return nil
	}

	for _, raw := range names {
		name := strings.TrimSpace(raw)
		if name == "" {
			// An empty term matches nothing and would be reported later as a
			// mysterious miss. Reject it where the cause is still obvious.
			return fmt.Errorf("--%s cannot contain an empty value", flag)
		}
		if containsFold(vocab.Values, name) {
			continue
		}

		// A value that belongs to another provider is the exact shape of the
		// reported bug, so say so rather than offering a generic typo hint.
		if elsewhere := providers.FindElsewhere(registry, name, d.Key); len(elsewhere) > 0 {
			return crossProviderError(flag, name, d, vocab, elsewhere)
		}

		// A comma followed by a space splits into a value with leading
		// whitespace. Report that rather than calling it an unknown name.
		if raw != name {
			return fmt.Errorf(
				"--%s value %q is not valid for %s (note the surrounding whitespace).\n"+
					"When joining values with commas, leave no space after the comma:\n"+
					"    --%s %s",
				providers.FlagEquipmentTypes, raw, d.DisplayName,
				providers.FlagEquipmentTypes, joinQuoted(names))
		}

		if vocab.Closed {
			return unknownValueError(flag, name, d, vocab)
		}
		// Open vocabulary: an unlisted name may still be real, so this only
		// hints. The authoritative check is coverage after the fetch.
		hint := ""
		if h := suggest.Closest(name, vocab.Values, 1); len(h) > 0 {
			hint = fmt.Sprintf(" Did you mean %q?", h[0])
		}
		logger.Warn("%q is not a --%s value camply knows for %s.%s "+
			"Continuing; the search will report how many sites matched.",
			name, flag, d.DisplayName, hint)
	}
	return nil
}

func crossProviderError(flag, name string, d providers.Descriptor, vocab providers.Vocabulary, elsewhere []providers.Term) error {
	var b strings.Builder
	fmt.Fprintf(&b, "--%s=%s is not valid for %s.\n",
		flag, name, d.DisplayName)
	fmt.Fprintf(&b, "%q belongs to %s.\n\n", name, elsewhere[0].Display)

	fmt.Fprintf(&b, "Values %s accepts (%s):\n    %s\n\n",
		d.DisplayName, vocab.Source, strings.Join(vocab.Values, ", "))

	fmt.Fprintf(&b, "Did you mean one of these?\n")
	if hints := suggest.Closest(name, vocab.Values, 3); len(hints) > 0 {
		fmt.Fprintf(&b, "    camply %s campsites --%s %s ...\n",
			d.Key, flag, joinQuoted(hints))
	}
	fmt.Fprintf(&b, "    camply %s campsites --%s %s ...\n",
		elsewhere[0].Provider, flag, shellQuote(elsewhere[0].Value))

	return fmt.Errorf("%s", strings.TrimRight(b.String(), "\n"))
}

func unknownValueError(flag, name string, d providers.Descriptor, vocab providers.Vocabulary) error {
	var b strings.Builder
	fmt.Fprintf(&b, "--%s=%s is not valid for %s.\n\n",
		flag, name, d.DisplayName)
	fmt.Fprintf(&b, "Accepted values (%s):\n    %s\n",
		vocab.Source, strings.Join(vocab.Values, ", "))

	if hints := suggest.Closest(name, vocab.Values, 3); len(hints) > 0 {
		fmt.Fprintf(&b, "\nDid you mean %s?\n", joinQuoted(hints))
		fmt.Fprintf(&b, "    camply %s campsites --%s %s ...\n",
			d.Key, flag, joinQuoted(hints[:1]))
	}
	fmt.Fprintf(&b, "\nSeveral values: --%s %s\n", flag, joinQuoted(firstN(vocab.Values, 2)))

	return fmt.Errorf("%s", strings.TrimRight(b.String(), "\n"))
}

func containsFold(values []string, want string) bool {
	for _, v := range values {
		if strings.EqualFold(strings.TrimSpace(v), want) {
			return true
		}
	}
	return false
}

func joinQuoted(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, v := range values {
		quoted = append(quoted, shellQuote(v))
	}
	return strings.Join(quoted, ",")
}

func firstN(values []string, n int) []string {
	if len(values) < n {
		return values
	}
	return values[:n]
}
