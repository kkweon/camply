package cli

import (
	"fmt"
	"strings"

	"github.com/kkweon/camply/internal/core"
	"github.com/kkweon/camply/internal/logger"
	"github.com/kkweon/camply/internal/providers"
)

// flaggedInResults is the one claim a coverage message may safely make.
//
// The Site Access and Equipment lines and their ⚠️ markers are unconditional, so
// a site that survives every filter is always labelled. What no coverage message
// may claim is that a site WILL survive them: each message measures one filter,
// and another can remove the same site after this one has counted it. Saying
// otherwise produced two lines of output that contradicted each other — one
// reporting sites as "included", the next reporting them excluded.
const flaggedInResults = "any that reach the results are flagged ⚠️"

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
		logger.Warn("%d of the %d campsites searched report no equipment data. --%s does not "+
			"exclude them; %s.",
			c.MissingData, c.TotalSites, providers.FlagEquipmentTypes, flaggedInResults)
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

// reportSiteAccessCoverage narrates what vehicle access did to the results.
//
// It follows reportEquipmentCoverage's contract: counts are always reported, and
// only one condition is fatal — a campground where every site is unreachable by
// car, so --exclude-no-vehicle-access dropped it entirely.
//
// It also speaks when the flag is off. A search that returns walk-in sites and
// says nothing about them is exactly the state that let a hike-in site half a
// mile from the road arrive looking like a drive-in one.
func reportSiteAccessCoverage(c core.SiteAccessCoverage, excludeNoVehicle, allowPartial bool) error {
	if c.TotalSites == 0 {
		return nil
	}

	if !excludeNoVehicle {
		if c.NoVehicle > 0 {
			logger.Info("%d of the %d campsites searched have no vehicle access. --%s would "+
				"exclude them; %s.",
				c.NoVehicle, c.TotalSites, flagExcludeNoVehicleAccess, flaggedInResults)
		}
		return nil
	}

	logger.Info("--%s excluded %d campsites with no vehicle access (of %d searched).",
		flagExcludeNoVehicleAccess, c.NoVehicle, c.TotalSites)
	if c.Unknown > 0 {
		// Stated positively and every run: these are kept on purpose. Silently
		// dropping them would be the same silent-loss bug this flag exists to
		// fix, one level down.
		logger.Info("%d campsites searched report no vehicle access data. --%s does not exclude "+
			"them; %s UNKNOWN — verify those on the booking page.",
			c.Unknown, flagExcludeNoVehicleAccess, flaggedInResults)
	}

	dropped := c.FacilitiesFullyDropped()
	if len(dropped) == 0 {
		return nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "--%s emptied %d of %d campground(s), so they were dropped from the search:\n",
		flagExcludeNoVehicleAccess, len(dropped), len(c.PerFacility))
	for _, f := range dropped {
		label := f.FacilityName
		if label == "" {
			label = "campground"
		}
		fmt.Fprintf(&b, "    %s (#%s): all %d sites are unreachable by car\n", label, f.FacilityID, f.TotalSites)
	}
	report := strings.TrimRight(b.String(), "\n")

	if allowPartial {
		logger.Warn("%s", report)
		return nil
	}
	return fmt.Errorf("%s\n\nTo search these campgrounds anyway, pass --allow-partial-match", report)
}
