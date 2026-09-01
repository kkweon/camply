package core

import (
	"sort"
	"time"
)

// Filter handles the fast, native Go filtering that replaces Pandas
type Filter struct{}

// FilterStats counts what survived each stage of Apply, in the order the stages
// run. It exists so the log can explain a raw grid full of open nights becoming
// an empty result — without it, "campground is full" and "a filter removed
// everything" print the same nothing.
//
// A stage that was not requested carries the previous stage's count through, so
// every field is always meaningful.
type FilterStats struct {
	RawNights int // availability rows in: one per free site-night
	RawSites  int // distinct sites among them
	Stays     int // stays of exactly req.Nights consecutive nights

	AfterCampsites     int
	AfterWeekends      int
	AfterWindow        int
	AfterEquipment     int
	AfterCampsiteTypes int
	AfterShelter       int
	AfterParking       int
	AfterHookups       int // == len(result)
}

// Apply executes all business logic constraints against raw availabilities.
func (f *Filter) Apply(availabilities []Availability, req SearchRequest) []Availability {
	filtered, _ := f.ApplyWithStats(availabilities, req)
	return filtered
}

// ApplyWithStats is Apply, also reporting how many bookings survived each stage.
func (f *Filter) ApplyWithStats(availabilities []Availability, req SearchRequest) ([]Availability, FilterStats) {
	var stats FilterStats
	stats.RawNights = len(availabilities)
	rawSites := map[string]bool{}
	for _, a := range availabilities {
		rawSites[a.Site.ID+"|"+a.Site.Facility.ID] = true
	}
	stats.RawSites = len(rawSites)

	// 1. Consolidate consecutive nights natively
	consolidated := consolidateNights(availabilities, req.Nights)
	stats.Stays = len(consolidated)

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
		stats.AfterCampsites++

		// 3. Check if the site meets the consecutive nights requirement
		if booking.Nights < req.Nights {
			continue
		}

		// 4. Check if weekends only is requested
		if req.WeekendsOnly && !isWeekend(booking.Start) {
			continue
		}
		stats.AfterWeekends++

		// 5. Ensure it falls within requested search windows
		if !isInSearchWindow(booking, req) {
			continue
		}
		stats.AfterWindow++

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
		stats.AfterEquipment++

		// 7. Check campsite type. This is the coarse cut between tent, RV and
		// cabin. It does NOT separate drive-in from walk-in, though it was once
		// used that way: Zephyr Cove types its hike-in sites TENT ONLY
		// NONELECTRIC and Lodgepole types 3 drive-in sites WALK TO.
		if len(req.CampsiteTypes) > 0 && !hasMatchingCampsiteType(site, req.CampsiteTypes) {
			continue
		}
		stats.AfterCampsiteTypes++

		// 8. The shelter this trip is for: does the site take what I am
		// bringing? CouldAllow, not Allows — a site whose permitted set is
		// unknown is kept and flagged, never dropped.
		if req.Shelter != ShelterUnknown && !site.Permits.CouldAllow(req.Shelter) {
			continue
		}
		stats.AfterShelter++

		// 9. Keep only the parkings asked for.
		//
		// A site whose parking the provider never reported survives: the filter
		// is opt-in and easy to forget, so it is not allowed to be the thing
		// that silently discards a site — that job belongs to the alert, which
		// always says what it knows.
		if len(req.Parking) > 0 && site.Parking != ParkingUnknown {
			if !hasParking(site, req.Parking) {
				continue
			}
			// The user named this proven parking level, so the alert treats
			// it as a confirmation rather than a warning.
			booking.ParkingRequested = true
		}
		stats.AfterParking++

		// 10. Hookups are additive: naming two requires both.
		if !satisfiesHookups(site, req) {
			continue
		}
		stats.AfterHookups++

		filtered = append(filtered, booking)
	}

	return filtered, stats
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
			if EqualValue(siteEq.EquipmentName, reqEq.EquipmentName) {
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

func hasParking(site *Site, wanted []Parking) bool {
	for _, w := range wanted {
		if w == site.Parking {
			return true
		}
	}
	return false
}

func satisfiesHookups(site *Site, req SearchRequest) bool {
	for _, h := range req.Hookups {
		if !site.Satisfies(h, req.Shelter) {
			return false
		}
	}
	return true
}

func hasMatchingCampsiteType(site *Site, wanted []string) bool {
	for _, w := range wanted {
		if EqualValue(w, site.RawType) {
			return true
		}
	}
	return false
}
