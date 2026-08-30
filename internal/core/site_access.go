package core

import "sort"

// SiteAccess is how a camper reaches a campsite.
//
// It exists because a walk-in site can carry a drive-in campsite_type. Zephyr
// Cove types its 47 hike-in sites TENT ONLY NONELECTRIC, identical to its
// drive-in tent sites, so a --campsite-types filter cannot tell them apart and
// an alert for one read exactly like an alert for the other.
//
// The arrival mode is kept rather than collapsed into a single "walk-in" flag:
// a boat-in site is not a walk-in site, and the difference is what a reader
// needs in order to judge an alert. Filtering asks NoVehicleAccess instead.
type SiteAccess int

const (
	// SiteAccessUnknown is the zero value on purpose: a provider that does not
	// populate this field must never be mistaken for a drive-in site.
	SiteAccessUnknown SiteAccess = iota
	SiteAccessDriveIn
	SiteAccessWalkIn
	SiteAccessHikeIn
	SiteAccessBoatIn
	// SiteAccessNoVehicle is "no vehicle reaches it, arrival mode unrecognised".
	//
	// recreation.gov's Site Access vocabulary is open, so a label outside the
	// known set must still be honoured as non-drivable rather than guessed into
	// one of the named modes or quietly downgraded to Unknown.
	SiteAccessNoVehicle
)

// HasVehicleAccess reports whether a car reaches the site.
//
// Unknown is not a "yes": only a positive signal from the provider counts.
func (a SiteAccess) HasVehicleAccess() bool {
	return a == SiteAccessDriveIn
}

// NoVehicleAccess reports the cases proven unreachable by car.
//
// Unknown is not a "no" either, which is the whole point of the split. A filter
// built on !HasVehicleAccess() would silently drop every site whose provider
// reported nothing — the failure this package already learned from with
// equipment data. Missing data is surfaced, never acted on.
func (a SiteAccess) NoVehicleAccess() bool {
	switch a {
	case SiteAccessWalkIn, SiteAccessHikeIn, SiteAccessBoatIn, SiteAccessNoVehicle:
		return true
	default:
		return false
	}
}

// String is the display label, used when a provider reported no raw label.
func (a SiteAccess) String() string {
	switch a {
	case SiteAccessDriveIn:
		return "Drive-In"
	case SiteAccessWalkIn:
		return "Walk-In"
	case SiteAccessHikeIn:
		return "Hike-In"
	case SiteAccessBoatIn:
		return "Boat-In"
	case SiteAccessNoVehicle:
		return "No Vehicle Access"
	default:
		return "Unknown"
	}
}

// FacilitySiteAccess is how vehicle access landed at one campground.
type FacilitySiteAccess struct {
	FacilityID   string
	FacilityName string
	TotalSites   int
	NoVehicle    int
	Unknown      int
}

// SiteAccessCoverage measures vehicle access across the campsites a provider
// returned, so --exclude-no-vehicle-access can report what it removed instead
// of removing it quietly. It mirrors Coverage, which exists for the same reason
// after an equipment filter silently discarded 423 sites for weeks.
type SiteAccessCoverage struct {
	TotalSites  int
	NoVehicle   int
	Unknown     int
	PerFacility map[string]FacilitySiteAccess
}

// AnalyzeSiteAccess counts per distinct campsite, not per availability row: a
// site free for five nights is one site.
func AnalyzeSiteAccess(sites []AvailableCampsite) SiteAccessCoverage {
	c := SiteAccessCoverage{PerFacility: map[string]FacilitySiteAccess{}}

	seen := map[string]bool{}
	for _, s := range sites {
		key := s.CampsiteID + "|" + s.FacilityID
		if seen[key] {
			continue
		}
		seen[key] = true

		fac, ok := c.PerFacility[s.FacilityID]
		if !ok {
			fac = FacilitySiteAccess{FacilityID: s.FacilityID, FacilityName: s.FacilityName}
		}

		c.TotalSites++
		fac.TotalSites++
		switch {
		case s.SiteAccess.NoVehicleAccess():
			c.NoVehicle++
			fac.NoVehicle++
		case !s.SiteAccess.HasVehicleAccess():
			c.Unknown++
			fac.Unknown++
		}

		c.PerFacility[s.FacilityID] = fac
	}

	return c
}

// FacilitiesFullyDropped returns campgrounds where every site is proven
// unreachable by car, largest first — the campgrounds --exclude-no-vehicle-access
// removes from the search entirely, with nothing left to show for them.
func (c SiteAccessCoverage) FacilitiesFullyDropped() []FacilitySiteAccess {
	var out []FacilitySiteAccess
	for _, f := range c.PerFacility {
		if f.TotalSites > 0 && f.NoVehicle == f.TotalSites {
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
