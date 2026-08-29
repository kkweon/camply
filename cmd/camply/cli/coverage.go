package cli

import (
	"fmt"
	"strings"

	"github.com/kkweon/camply/internal/core"
	"github.com/kkweon/camply/internal/logger"
	"github.com/kkweon/camply/internal/providers"
)

// reportEquipmentCoverage turns the measured coverage into an actionable error,
// or warnings under --allow-partial-match.
//
// This is the check that catches the reported incident. A value can be
// perfectly valid for a provider and still match nothing at the campgrounds
// being searched; only counting after the fetch can tell. Failing loudly is the
// point: a job that exits non-zero is visible, weeks of "0 New Campsites Found"
// are not.
//
// Only one condition is fatal: a campground where nothing requested matched, so
// it was dropped from the search entirely. Equipment names differ between
// campgrounds, so naming several and having some miss somewhere is the correct
// way to search and must not fail — that is how the running CronJobs are
// configured, deliberately.
func reportEquipmentCoverage(c core.Coverage, requested []core.Equipment, d providers.Descriptor,
	registry []providers.Descriptor, allowPartial bool,
) error {
	if len(requested) == 0 || c.TotalSites == 0 {
		return nil
	}

	// Informative, not fatal: results are still complete.
	for _, name := range c.UnmatchedNames() {
		msg := fmt.Sprintf("--%s=%s matched none of the %d campsites searched.",
			providers.FlagEquipmentTypes, name, c.TotalSites)
		if elsewhere := providers.FindElsewhere(registry, name, d.Key); len(elsewhere) > 0 {
			msg += fmt.Sprintf(" %q is a %s value.", name, elsewhere[0].Display)
		}
		logger.Warn("%s", msg)
	}
	if c.MissingData > 0 {
		logger.Warn("%d of %d campsites report no equipment data, so --%s excluded them. "+
			"Searching without the filter would include them.",
			c.MissingData, c.TotalSites, providers.FlagEquipmentTypes)
	}

	dropped := c.FacilitiesWithNoMatch()
	if len(dropped) == 0 {
		return nil
	}

	report := describeDroppedFacilities(c, requested, dropped, d, registry)
	if allowPartial {
		logger.Warn("%s", report)
		return nil
	}
	return fmt.Errorf("%s\n\nTo search these campgrounds anyway, pass --allow-partial-match", report)
}

func describeDroppedFacilities(c core.Coverage, requested []core.Equipment,
	dropped []core.FacilityCoverage, d providers.Descriptor, registry []providers.Descriptor,
) string {
	names := make([]string, 0, len(requested))
	for _, r := range requested {
		names = append(names, r.EquipmentName)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "--%s %s matched nothing at %d of %d campground(s), so they were dropped from the search:\n",
		providers.FlagEquipmentTypes, joinQuoted(names), len(dropped), len(c.PerFacility))

	var suggestions []string
	for _, f := range dropped {
		label := f.FacilityName
		if label == "" {
			label = "campground"
		}
		offers := f.ObservedAt(5)
		fmt.Fprintf(&b, "    %s (#%s): 0 of %d sites", label, f.FacilityID, f.TotalSites)
		if len(offers) > 0 {
			fmt.Fprintf(&b, " — offers %s", strings.Join(offers, ", "))
			suggestions = append(suggestions, offers[0])
		}
		b.WriteString("\n")
	}

	if len(suggestions) > 0 {
		fmt.Fprintf(&b, "  Equipment names differ between campgrounds. Add the ones they use:\n")
		fmt.Fprintf(&b, "    --%s %s\n",
			providers.FlagEquipmentTypes, joinQuoted(dedupe(append(names, suggestions...))))
	}

	// The reported incident exactly: a value that is real, but real elsewhere.
	for _, n := range names {
		if elsewhere := providers.FindElsewhere(registry, n, d.Key); len(elsewhere) > 0 {
			fmt.Fprintf(&b, "  Note: %q is also a %s value.\n", n, elsewhere[0].Display)
			break
		}
	}

	return strings.TrimRight(b.String(), "\n")
}

func dedupe(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range values {
		if seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}
