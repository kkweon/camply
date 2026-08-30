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
