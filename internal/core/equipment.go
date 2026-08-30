package core

import "sort"

// FacilityCoverage is how an equipment filter landed at one campground.
type FacilityCoverage struct {
	FacilityID     string
	FacilityName   string
	TotalSites     int
	MissingData    int
	MatchesByName  map[string]int
	ObservedByName map[string]int
}

// Coverage reports how a requested equipment filter actually landed against the
// campsites a provider returned.
//
// It exists because Filter.Apply drops non-matching sites silently, which made
// a meaningless --equipment value indistinguishable from a full campground. A
// filter naming a value the campgrounds never offer returned "0 New Campsites
// Found" for weeks and read as genuine unavailability.
type Coverage struct {
	TotalSites     int
	MissingData    int
	MatchesByName  map[string]int
	ObservedByName map[string]int
	PerFacility    map[string]FacilityCoverage
}

// AnalyzeEquipment measures a filter against raw campsites.
//
// Counting is per distinct campsite, not per availability row: a site free for
// five nights is one site, and counting rows would inflate every number.
func AnalyzeEquipment(availabilities []Availability, requested []Equipment) Coverage {
	c := Coverage{
		MatchesByName:  map[string]int{},
		ObservedByName: map[string]int{},
		PerFacility:    map[string]FacilityCoverage{},
	}
	for _, r := range requested {
		c.MatchesByName[r.EquipmentName] = 0
	}

	seen := map[string]bool{}
	for _, a := range availabilities {
		s := a.Site
		key := s.ID + "|" + s.Facility.ID
		if seen[key] {
			continue
		}
		seen[key] = true

		fac, ok := c.PerFacility[s.Facility.ID]
		if !ok {
			fac = FacilityCoverage{
				FacilityID:     s.Facility.ID,
				FacilityName:   s.Facility.Name,
				MatchesByName:  map[string]int{},
				ObservedByName: map[string]int{},
			}
			for _, r := range requested {
				fac.MatchesByName[r.EquipmentName] = 0
			}
		}

		c.TotalSites++
		fac.TotalSites++

		if len(s.Equipment) == 0 {
			c.MissingData++
			fac.MissingData++
		}
		for _, e := range s.Equipment {
			c.ObservedByName[e.EquipmentName]++
			fac.ObservedByName[e.EquipmentName]++
		}
		for _, r := range requested {
			if equipmentMatches(s, r) {
				c.MatchesByName[r.EquipmentName]++
				fac.MatchesByName[r.EquipmentName]++
			}
		}

		c.PerFacility[s.Facility.ID] = fac
	}

	return c
}

// equipmentMatches mirrors Filter.hasMatchingEquipment for a single term, so
// the diagnosis cannot disagree with the filter it explains.
func equipmentMatches(site *Site, req Equipment) bool {
	for _, have := range site.Equipment {
		if !EqualValue(have.EquipmentName, req.EquipmentName) {
			continue
		}
		if req.MaxLength == 0 || req.MaxLength <= have.MaxLength {
			return true
		}
	}
	return false
}

// UnmatchedNames returns requested names that matched no site anywhere.
func (c Coverage) UnmatchedNames() []string {
	var out []string
	for name, n := range c.MatchesByName {
		if n == 0 {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// FacilitiesWithNoMatch returns facilities where *nothing* requested matched,
// worst first.
//
// The union is what matters, not each name individually. Equipment names differ
// between campgrounds — Meeks Bay uses "Small Tent" where Fallen Leaf uses
// "Tent" — so naming several and having some miss somewhere is the correct way
// to search, not a mistake. A facility only represents lost results when every
// requested name misses it, because then it is dropped from the search
// entirely with nothing to show for it.
func (c Coverage) FacilitiesWithNoMatch() []FacilityCoverage {
	var out []FacilityCoverage
	for _, f := range c.PerFacility {
		matched := false
		for _, n := range f.MatchesByName {
			if n > 0 {
				matched = true
				break
			}
		}
		// MissingData sites survive the filter now, so a facility that has any
		// of them still returns results and must not be reported as emptied.
		// Without this guard a campground whose sites all lack equipment data
		// would fail the search outright while in fact returning every one.
		if !matched && f.MissingData == 0 {
			out = append(out, f)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TotalSites != out[j].TotalSites {
			return out[i].TotalSites > out[j].TotalSites
		}
		return out[i].FacilityID < out[j].FacilityID
	})
	return out
}

// SortedFacilities returns every facility, largest first, for reporting.
func (c Coverage) SortedFacilities() []FacilityCoverage {
	out := make([]FacilityCoverage, 0, len(c.PerFacility))
	for _, f := range c.PerFacility {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TotalSites != out[j].TotalSites {
			return out[i].TotalSites > out[j].TotalSites
		}
		return out[i].FacilityID < out[j].FacilityID
	})
	return out
}

// TopObserved returns the equipment names these campsites actually offer, most
// common first — the list a user needs in order to pick a working value.
func (c Coverage) TopObserved(limit int) []string {
	type kv struct {
		name string
		n    int
	}
	var all []kv
	for name, n := range c.ObservedByName {
		all = append(all, kv{name, n})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].n != all[j].n {
			return all[i].n > all[j].n
		}
		return all[i].name < all[j].name
	})

	var out []string
	for _, e := range all {
		if limit > 0 && len(out) == limit {
			break
		}
		out = append(out, e.name)
	}
	return out
}

// ObservedAt returns one facility's equipment names, most common first.
func (f FacilityCoverage) ObservedAt(limit int) []string {
	c := Coverage{ObservedByName: f.ObservedByName}
	return c.TopObserved(limit)
}
