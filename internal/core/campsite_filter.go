package core

import (
	"fmt"
	"sort"
	"strings"
)

// KnownCampsite is one campsite that exists at a campground being searched,
// whether or not it currently has availability.
type KnownCampsite struct {
	CampsiteID   string
	SiteName     string
	FacilityName string
}

// matchesRequestedCampsite reports whether a site is one of the IDs --campsites
// named.
func matchesRequestedCampsite(site *Site, requested []string) bool {
	for _, want := range requested {
		if strings.TrimSpace(want) == strings.TrimSpace(site.ID) {
			return true
		}
	}
	return false
}

// ValidateRequestedCampsites rejects a --campsites value naming a campsite that
// exists at none of the campgrounds searched.
//
// It checks against the campground's full roster, never against availability. A
// real campsite that is simply booked solid has to stay an ordinary empty
// result: validating against availability would make every CronJob watching one
// site start failing the moment that site filled up, which is the opposite of
// what a watch is for. Only an ID that is not there at all is a typo, and a typo
// gets an error with the IDs that would have worked — never a silent, plausible
// zero-result run.
func ValidateRequestedCampsites(requested []string, roster []KnownCampsite) error {
	if len(requested) == 0 || len(roster) == 0 {
		return nil
	}

	known := make(map[string]bool, len(roster))
	for _, c := range roster {
		known[c.CampsiteID] = true
	}

	var unknown []string
	for _, want := range requested {
		if want = strings.TrimSpace(want); want != "" && !known[want] {
			unknown = append(unknown, want)
		}
	}
	if len(unknown) == 0 {
		return nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "--campsites %s names no campsite at the campground(s) searched.\n",
		strings.Join(unknown, ", "))
	// Deliberately not a recreation.gov URL: this runs for every provider, and
	// "Campsite ID" is the label camply itself prints in results and alerts.
	fmt.Fprintf(&b, "  Use the \"Campsite ID\" camply reports, not the site name printed on the post.\n")
	for _, line := range describeRoster(roster) {
		b.WriteString("  " + line + "\n")
	}
	return fmt.Errorf("%s", strings.TrimRight(b.String(), "\n"))
}

// describeRoster lists a few real IDs per campground, so the error carries a
// value the user can paste rather than only telling them they were wrong.
func describeRoster(roster []KnownCampsite) []string {
	byFacility := map[string][]KnownCampsite{}
	for _, c := range roster {
		byFacility[c.FacilityName] = append(byFacility[c.FacilityName], c)
	}

	facilities := make([]string, 0, len(byFacility))
	for name := range byFacility {
		facilities = append(facilities, name)
	}
	sort.Strings(facilities)

	const samples = 3
	var out []string
	for _, name := range facilities {
		sites := byFacility[name]
		sort.Slice(sites, func(i, j int) bool { return sites[i].CampsiteID < sites[j].CampsiteID })

		var examples []string
		for _, c := range sites {
			if len(examples) == samples {
				break
			}
			if c.SiteName != "" {
				examples = append(examples, fmt.Sprintf("%s (site %s)", c.CampsiteID, c.SiteName))
			} else {
				examples = append(examples, c.CampsiteID)
			}
		}

		label := name
		if label == "" {
			label = "campground"
		}
		out = append(out, fmt.Sprintf("%s has %d campsites, e.g. %s",
			label, len(sites), strings.Join(examples, ", ")))
	}
	return out
}
