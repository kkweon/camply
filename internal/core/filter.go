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

		filtered = append(filtered, site)
	}

	return filtered
}

// consolidateNights replaces pandas .groupby().diff() logic with an O(N log N) algorithm
func consolidateNights(campsites []AvailableCampsite, requiredNights int) []AvailableCampsite {
	if len(campsites) == 0 {
		return campsites
	}

	// Group campsites by CampsiteID
	groups := make(map[string][]AvailableCampsite)
	for _, site := range campsites {
		groups[site.CampsiteID] = append(groups[site.CampsiteID], site)
	}

	var consolidated []AvailableCampsite

	for _, group := range groups {
		// Sort the group by booking date
		sort.Slice(group, func(i, j int) bool {
			return group[i].BookingDate.Before(group[j].BookingDate)
		})

		// Sliding window to find consecutive nights
		for i := 0; i < len(group); i++ {
			consecutiveBlocks := []AvailableCampsite{group[i]}

			for j := i + 1; j < len(group); j++ {
				lastBlock := consecutiveBlocks[len(consecutiveBlocks)-1]
				// Is it exactly one day later?
				if group[j].BookingDate.Equal(lastBlock.BookingDate.AddDate(0, 0, 1)) {
					consecutiveBlocks = append(consecutiveBlocks, group[j])
					if len(consecutiveBlocks) >= requiredNights {
						// Found a valid block of N nights!
						startSite := consecutiveBlocks[0]
						endSite := consecutiveBlocks[len(consecutiveBlocks)-1]

						mergedSite := startSite
						mergedSite.BookingEndDate = endSite.BookingEndDate
						mergedSite.BookingNights = len(consecutiveBlocks)

						consolidated = append(consolidated, mergedSite)
					}
				} else {
					// Gap in dates, break the sliding window
					break
				}
			}

			// If requiredNights is 1, just add the single night
			if requiredNights == 1 {
				consolidated = append(consolidated, group[i])
			}
		}
	}

	return consolidated
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
