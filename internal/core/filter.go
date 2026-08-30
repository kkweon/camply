package core

import (
	"sort"
	"strings"
	"time"
)

// Filter handles the fast, native Go filtering that replaces Pandas
type Filter struct{}

// Apply executes all business logic constraints against raw availabilities.
func (f *Filter) Apply(availabilities []Availability, req SearchRequest) []Availability {
	// 1. Consolidate consecutive nights natively
	consolidated := consolidateNights(availabilities, req.Nights)

	var filtered []Availability
	for _, booking := range consolidated {
		site := booking.Site

		// 2. Restrict to the campsite IDs the user named.
		//
		// Validation that those IDs exist lives in the providers, which hold the
		// campground roster; here they only narrow. A booked-solid campsite is
		// an empty result, not an error.
		if len(req.Campsites) > 0 && !matchesRequestedCampsite(site, req.Campsites) {
			continue
		}

		// 3. Check if the site meets the consecutive nights requirement
		if booking.Nights < req.Nights {
			continue
		}

		// 4. Check if weekends only is requested
		if req.WeekendsOnly && !isWeekend(booking.Start) {
			continue
		}

		// 5. Ensure it falls within requested search windows
		if !isInSearchWindow(booking, req) {
			continue
		}

		// 6. Check Equipment filtering.
		//
		// A site the provider reported no equipment for is kept, not dropped.
		// Deciding on absent evidence is what made this filter discard 242
		// nights of real availability at Meeks Bay, where 17 of 88 campsites
		// carry no equipment data at all. It survives marked instead, and the
		// mark is what the alert shows.
		if len(req.Equipment) > 0 {
			switch {
			case len(site.Equipment) == 0:
				booking.EquipmentUnverified = true
			case !hasMatchingEquipment(site, req.Equipment):
				continue
			}
		}

		// 7. Check campsite type. This is the coarse cut between tent, RV and
		// cabin. It does NOT separate drive-in from walk-in, though it was once
		// used that way: Zephyr Cove types its hike-in sites TENT ONLY
		// NONELECTRIC and Lodgepole types 3 drive-in sites WALK TO.
		if len(req.CampsiteTypes) > 0 && !hasMatchingCampsiteType(site, req.CampsiteTypes) {
			continue
		}

		// 8. Drop sites that need a walk from the car, or have no car access.
		//
		// RequiresWalk, not !ReachableByCar: a site whose access the provider
		// never reported is kept and flagged in the alert instead. The filter is
		// opt-in and easy to forget, so it is not allowed to be the thing that
		// silently discards a site — that job belongs to the alert, which always
		// says what it knows.
		if req.ExcludeNoVehicleAccess && site.Parking.RequiresWalk() {
			continue
		}

		filtered = append(filtered, booking)
	}

	return filtered
}

// consolidateNights stitches per-night availability into bookings of exactly
// requiredNights consecutive nights, mirroring the Python implementation
// (camply/search/base_search.py _consecutive_subseq / _find_consecutive_nights).
//
// For a maximal run of N consecutive free nights it emits every sliding window of
// length requiredNights — i.e. N-requiredNights+1 records, each spanning exactly
// requiredNights nights. Runs shorter than requiredNights emit nothing.
func consolidateNights(availabilities []Availability, requiredNights int) []Availability {
	if requiredNights < 1 {
		requiredNights = 1
	}
	if len(availabilities) == 0 {
		return availabilities
	}

	// Group by campsite within a facility. The composite key matches Python's
	// (campsite_id, campground_id) grouping and avoids merging units from different
	// facilities that happen to share a numeric ID.
	groups := make(map[string][]Availability)
	for _, a := range availabilities {
		key := a.Site.ID + "|" + a.Site.Facility.ID
		groups[key] = append(groups[key], a)
	}

	// Iterate in a stable order. Ranging over the map directly made two
	// identical searches print their results in different orders, which is
	// noise for a human diffing runs and makes the output impossible to pin in
	// a test.
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var consolidated []Availability

	for _, key := range keys {
		group := groups[key]
		// Sort the group by booking date
		sort.Slice(group, func(i, j int) bool {
			return group[i].Start.Before(group[j].Start)
		})

		// Walk maximal consecutive runs, then emit every sliding window of exactly
		// requiredNights nights within each run.
		runStart := 0
		for i := 1; i <= len(group); i++ {
			isConsecutive := i < len(group) &&
				truncDay(group[i].Start).Equal(truncDay(group[i-1].Start).AddDate(0, 0, 1))
			if isConsecutive {
				continue
			}

			run := group[runStart:i]
			for s := 0; s+requiredNights <= len(run); s++ {
				merged := run[s]
				merged.End = run[s+requiredNights-1].End
				merged.Nights = requiredNights

				consolidated = append(consolidated, merged)
			}
			runStart = i
		}
	}

	return consolidated
}

func truncDay(t time.Time) time.Time {
	return t.Truncate(24 * time.Hour)
}

// hasMatchingEquipment reports whether a site is shown to permit one of the
// requested types.
//
// A site with no equipment data is not "no": it is unknown, and the caller must
// handle that before asking. This function only answers where there is evidence.
func hasMatchingEquipment(site *Site, requested []Equipment) bool {
	for _, reqEq := range requested {
		for _, siteEq := range site.Equipment {
			if strings.EqualFold(siteEq.EquipmentName, reqEq.EquipmentName) {
				// If length doesn't matter, or if it fits
				if reqEq.MaxLength == 0 || reqEq.MaxLength <= siteEq.MaxLength {
					return true
				}
			}
		}
	}
	return false
}

func isWeekend(t time.Time) bool {
	day := t.Weekday()
	return day == time.Friday || day == time.Saturday
}

func isInSearchWindow(booking Availability, req SearchRequest) bool {
	// If no windows provided, assume valid
	if len(req.StartDates) == 0 || len(req.EndDates) == 0 {
		return true
	}

	bookingStart := booking.Start.Truncate(24 * time.Hour)
	bookingEnd := booking.End.Truncate(24 * time.Hour)

	for i := range req.StartDates {
		start := req.StartDates[i].Truncate(24 * time.Hour)
		end := req.EndDates[i].Truncate(24 * time.Hour)

		// Must start on or after requested start, and end on or before requested end
		if (bookingStart.Equal(start) || bookingStart.After(start)) &&
			(bookingEnd.Equal(end) || bookingEnd.Before(end)) {
			return true
		}
	}
	return false
}

func hasMatchingCampsiteType(site *Site, wanted []string) bool {
	for _, w := range wanted {
		if strings.EqualFold(strings.TrimSpace(w), strings.TrimSpace(site.RawType)) {
			return true
		}
	}
	return false
}
