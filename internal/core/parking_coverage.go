package core

import "sort"

// FacilityParking is how parking landed at one campground.
type FacilityParking struct {
	FacilityID   string
	FacilityName string
	TotalSites   int
	RequiresWalk int
	Unknown      int
}

// ParkingCoverage measures parking across the campsites a provider returned, so
// --exclude-no-vehicle-access can report what it removed instead of removing it
// quietly. It mirrors Coverage, which exists for the same reason after an
// equipment filter silently discarded 423 sites for weeks.
type ParkingCoverage struct {
	TotalSites   int
	RequiresWalk int
	Unknown      int
	PerFacility  map[string]FacilityParking
}

// AnalyzeParking counts per distinct campsite, not per availability row: a site
// free for five nights is one site.
func AnalyzeParking(availabilities []Availability) ParkingCoverage {
	c := ParkingCoverage{PerFacility: map[string]FacilityParking{}}

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
			fac = FacilityParking{FacilityID: s.Facility.ID, FacilityName: s.Facility.Name}
		}

		c.TotalSites++
		fac.TotalSites++
		switch {
		case s.Parking.RequiresWalk():
			c.RequiresWalk++
			fac.RequiresWalk++
		case !s.Parking.ReachableByCar():
			c.Unknown++
			fac.Unknown++
		}

		c.PerFacility[s.Facility.ID] = fac
	}

	return c
}

// FacilitiesFullyDropped returns campgrounds where every site needs a walk from
// the car, largest first — the campgrounds --exclude-no-vehicle-access removes
// from the search entirely, with nothing left to show for them.
func (c ParkingCoverage) FacilitiesFullyDropped() []FacilityParking {
	var out []FacilityParking
	for _, f := range c.PerFacility {
		if f.TotalSites > 0 && f.RequiresWalk == f.TotalSites {
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

// FacilityHookups is what one campground records about one utility.
type FacilityHookups struct {
	FacilityID   string
	FacilityName string
	TotalSites   int
	Silent       int // sites saying nothing about the hookup in question
}

// HookupCoverage measures what campgrounds say about the utilities a search
// asked for.
//
// It is per requested hookup, not per site overall, because a campground can be
// eloquent about one and silent about another: Lodgepole types every site
// NONELECTRIC, which is a real answer about electricity and says nothing at all
// about water.
//
// And it is per campground, because recording is a campground practice rather
// than a site fact — of six Tahoe campgrounds measured, the two resorts record
// water hookups on 68-81% of their sites and the four public campgrounds record
// none. An average over all of them describes neither.
type HookupCoverage struct {
	TotalSites int
	// PerHookup maps a requested hookup to the campgrounds with sites that say
	// nothing about it, most silent first.
	PerHookup map[Hookup][]FacilityHookups
}

// AnalyzeHookups counts per distinct campsite, not per availability row.
func AnalyzeHookups(availabilities []Availability, wanted []Hookup, shelter Shelter) HookupCoverage {
	c := HookupCoverage{PerHookup: map[Hookup][]FacilityHookups{}}
	if len(wanted) == 0 {
		return c
	}

	byHookup := map[Hookup]map[string]FacilityHookups{}
	for _, h := range wanted {
		byHookup[h] = map[string]FacilityHookups{}
	}

	seen := map[string]bool{}
	for _, a := range availabilities {
		s := a.Site
		key := s.ID + "|" + s.Facility.ID
		if seen[key] {
			continue
		}
		seen[key] = true
		c.TotalSites++

		for _, h := range wanted {
			fac, ok := byHookup[h][s.Facility.ID]
			if !ok {
				fac = FacilityHookups{FacilityID: s.Facility.ID, FacilityName: s.Facility.Name}
			}
			fac.TotalSites++
			if s.hookupUnknown(h, shelter) {
				fac.Silent++
			}
			byHookup[h][s.Facility.ID] = fac
		}
	}

	for h, facs := range byHookup {
		// Any silent site is worth counting, with no threshold: Lodgepole
		// records a water hookup on exactly 1 of its 201 sites, and a rule that
		// only names wholly silent campgrounds would leave the other 200
		// unmentioned.
		var silent []FacilityHookups
		for _, f := range facs {
			if f.Silent > 0 {
				silent = append(silent, f)
			}
		}
		sort.Slice(silent, func(i, j int) bool {
			if silent[i].Silent != silent[j].Silent {
				return silent[i].Silent > silent[j].Silent
			}
			return silent[i].FacilityID < silent[j].FacilityID
		})
		if len(silent) > 0 {
			c.PerHookup[h] = silent
		}
	}
	return c
}

// hookupUnknown reports whether the site says nothing about one utility. For
// water under a tent search a shared tap counts as saying something.
func (s *Site) hookupUnknown(h Hookup, shelter Shelter) bool {
	switch h {
	case HookupElectric:
		return s.Hookups.Electric == TriUnknown
	case HookupSewer:
		return s.Hookups.Sewer == TriUnknown
	case HookupWater:
		if shelter == ShelterRV || shelter == ShelterCabin {
			return s.Hookups.Water == TriUnknown
		}
		return s.Hookups.Water == TriUnknown && s.SharedWater == TriUnknown
	}
	return false
}
