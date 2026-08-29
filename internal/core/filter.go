package core

import (
	"sort"
	"strings"
	"time"
)

// Filter handles the fast, native Go filtering that replaces Pandas
type Filter struct{}

// Apply executes all business logic constraints against raw campsites
func (f *Filter) Apply(campsites []AvailableCampsite, req SearchRequest) []AvailableCampsite {
	// 1. Consolidate consecutive nights natively
	consolidated := consolidateNights(campsites, req.Nights)

	var filtered []AvailableCampsite
	for _, site := range consolidated {
		// 2. Check if the site meets the consecutive nights requirement
		if site.BookingNights < req.Nights {
			continue
		}

		// 3. Check if weekends only is requested
		if req.WeekendsOnly && !isWeekend(site.BookingDate) {
			continue
		}

		// 4. Ensure it falls within requested search windows
		if !isInSearchWindow(site, req) {
			continue
		}

		// 5. Check Equipment filtering
		if len(req.Equipment) > 0 && !hasMatchingEquipment(site, req.Equipment) {
			continue
		}

		// 6. Check campsite type. Equipment cannot express this: a WALK TO site
		// still permits a tent, so only the type separates drive-in from
		// walk-in.
		if len(req.CampsiteTypes) > 0 && !hasMatchingCampsiteType(site, req.CampsiteTypes) {
			continue
		}

		filtered = append(filtered, site)
	}

	return filtered
}

// consolidateNights stitches per-night availability slices into bookings of exactly
// requiredNights consecutive nights, mirroring the Python implementation
// (camply/search/base_search.py _consecutive_subseq / _find_consecutive_nights).
//
// For a maximal run of N consecutive free nights it emits every sliding window of
// length requiredNights — i.e. N-requiredNights+1 records, each spanning exactly
// requiredNights nights. Runs shorter than requiredNights emit nothing.
func consolidateNights(campsites []AvailableCampsite, requiredNights int) []AvailableCampsite {
	if requiredNights < 1 {
		requiredNights = 1
	}
	if len(campsites) == 0 {
		return campsites
	}

	// Group by campsite within a facility. The composite key matches Python's
	// (campsite_id, campground_id) grouping and avoids merging units from different
	// facilities that happen to share a numeric ID.
	groups := make(map[string][]AvailableCampsite)
	for _, site := range campsites {
		key := site.CampsiteID + "|" + site.FacilityID
		groups[key] = append(groups[key], site)
	}

	var consolidated []AvailableCampsite

	for _, group := range groups {
		// Sort the group by booking date
		sort.Slice(group, func(i, j int) bool {
			return group[i].BookingDate.Before(group[j].BookingDate)
		})

		// Walk maximal consecutive runs, then emit every sliding window of exactly
		// requiredNights nights within each run.
		runStart := 0
		for i := 1; i <= len(group); i++ {
			isConsecutive := i < len(group) &&
				truncDay(group[i].BookingDate).Equal(truncDay(group[i-1].BookingDate).AddDate(0, 0, 1))
			if isConsecutive {
				continue
			}

			run := group[runStart:i]
			for s := 0; s+requiredNights <= len(run); s++ {
				startSite := run[s]
				endSite := run[s+requiredNights-1]

				mergedSite := startSite
				mergedSite.BookingEndDate = endSite.BookingEndDate
				mergedSite.BookingNights = requiredNights

				consolidated = append(consolidated, mergedSite)
			}
			runStart = i
		}
	}

	return consolidated
}

func truncDay(t time.Time) time.Time {
	return t.Truncate(24 * time.Hour)
}

func hasMatchingEquipment(site AvailableCampsite, requested []Equipment) bool {
	// If the API didn't return any equipment data but we requested some, assume it doesn't match
	if len(site.PermittedEquipment) == 0 {
		return false
	}

	for _, reqEq := range requested {
		for _, siteEq := range site.PermittedEquipment {
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

func isInSearchWindow(site AvailableCampsite, req SearchRequest) bool {
	// If no windows provided, assume valid
	if len(req.StartDates) == 0 || len(req.EndDates) == 0 {
		return true
	}

	siteStart := site.BookingDate.Truncate(24 * time.Hour)
	siteEnd := site.BookingEndDate.Truncate(24 * time.Hour)

	for i := range req.StartDates {
		start := req.StartDates[i].Truncate(24 * time.Hour)
		end := req.EndDates[i].Truncate(24 * time.Hour)

		// Must start on or after requested start, and end on or before requested end
		if (siteStart.Equal(start) || siteStart.After(start)) &&
			(siteEnd.Equal(end) || siteEnd.Before(end)) {
			return true
		}
	}
	return false
}

func hasMatchingCampsiteType(site AvailableCampsite, wanted []string) bool {
	for _, w := range wanted {
		if strings.EqualFold(strings.TrimSpace(w), strings.TrimSpace(site.CampsiteType)) {
			return true
		}
	}
	return false
}
