package core

// The camping domain, as a camper thinks about it rather than as any booking
// API happens to encode it.
//
// It exists because recreation.gov's campsite_type is four orthogonal facts
// crammed into one string, and each campground's operator picks a different one
// to put there. Zephyr Cove spends the slot on shelter, so its 47 hike-in sites
// are typed exactly like the drive-in tent sites beside them. Lodgepole spends
// it on access, so its tent sites lose their shelter. Reading that one field as
// though it consistently meant one thing produced two opposite bugs.
//
// Nothing here knows a provider's vocabulary. The mapping lives in each
// provider's adapter, which is the only place a raw API string appears.

// Tri is a three-valued answer.
//
// Absent is never "no". Acting on missing data is the mistake this package has
// now made twice — an equipment filter that discarded 242 nights of real
// availability at Meeks Bay, and a walk-in site that reached an alert looking
// like a drive-in one.
type Tri int

const (
	TriUnknown Tri = iota
	TriYes
	TriNo
)

// Shelter is one kind of camping: the choice a camper makes before anything
// else. You are going tent camping, RV camping, or cabin camping.
type Shelter int

const (
	ShelterUnknown Shelter = iota
	ShelterTent
	ShelterRV
	ShelterCabin
)

func (s Shelter) String() string {
	switch s {
	case ShelterTent:
		return "tent"
	case ShelterRV:
		return "rv"
	case ShelterCabin:
		return "cabin"
	default:
		return "unknown"
	}
}

// Permitted is the set of shelters a site accepts.
//
// It is a fact about the site, and a different kind of thing from the camper's
// choice — which is why it is a different type. A STANDARD site permits both a
// tent and an RV; nobody goes "standard camping". Measured on 678 live sites,
// 98% of STANDARD sites permit both, and 50 sites named "TENT ONLY" permit RVs,
// so this cannot be collapsed into a single kind without losing results.
type Permitted uint8

const (
	PermitsTent Permitted = 1 << iota
	PermitsRV
	PermitsCabin
)

// PermittedUnknown is the empty set: nothing is known, which is not the same as
// nothing being allowed.
const PermittedUnknown Permitted = 0

// Allows reports set membership: this site is known to take that shelter.
func (p Permitted) Allows(s Shelter) bool {
	switch s {
	case ShelterTent:
		return p&PermitsTent != 0
	case ShelterRV:
		return p&PermitsRV != 0
	case ShelterCabin:
		return p&PermitsCabin != 0
	default:
		return true
	}
}

func (p Permitted) Has(bit Permitted) bool { return p&bit != 0 }

// CouldAllow is the question a shelter filter asks.
//
// It differs from Allows on exactly one input — an unknown set — and the
// difference is the point: missing data is surfaced, never acted on. Both
// meanings are nameable so a caller cannot pick the wrong one by accident,
// which is what happens when one method tries to be both.
func (p Permitted) CouldAllow(s Shelter) bool {
	return p == PermittedUnknown || p.Allows(s)
}

// Parking is how close a car gets to the site.
//
// Three levels rather than a drive-in/not flag, because the flag put Kaspian's
// 30-foot stroll and Zephyr Cove's half-mile haul in the same bucket. Ordered
// by convenience so "at least this good" reads naturally.
type Parking int

const (
	ParkingUnknown Parking = iota
	ParkingAtSite          // a driveway or spur at the site itself
	ParkingWalk            // park nearby, carry gear a short distance
	ParkingNone            // no vehicle reaches it at all
)

func (p Parking) String() string {
	switch p {
	case ParkingAtSite:
		return "at-site"
	case ParkingWalk:
		return "walk"
	case ParkingNone:
		return "none"
	default:
		return "unknown"
	}
}

// ReachableByCar reports whether a car gets all the way to the site. Unknown is
// not a "yes".
func (p Parking) ReachableByCar() bool { return p == ParkingAtSite }

// RequiresWalk reports the cases proven to need carrying gear from the car, or
// to have no car access at all. Unknown is not a "no" either — that split is
// the whole point, and a filter built on !ReachableByCar() would silently drop
// every site whose provider reported nothing.
func (p Parking) RequiresWalk() bool { return p == ParkingWalk || p == ParkingNone }

// Hookups are the utilities delivered to the site itself. A shared spigot in
// the campground is SharedWater on Site, deliberately not folded in here: for
// an RV they are different things.
type Hookups struct{ Electric, Water, Sewer Tri }

// Hookup names one utility a search can require.
type Hookup int

const (
	HookupElectric Hookup = iota
	HookupWater
	HookupSewer
)

func (h Hookup) String() string {
	switch h {
	case HookupWater:
		return "water"
	case HookupSewer:
		return "sewer"
	default:
		return "electric"
	}
}

// Satisfies reports whether a site meets one requested hookup.
//
// Unknown counts as satisfied, for the same reason everywhere else here: a
// provider that said nothing has not said no. Hookups are recorded per
// campground rather than per site — the two Tahoe resorts record them and the
// four public campgrounds record none — so dropping unknowns would silently
// remove whole campgrounds from a search.
//
// Water is the one axis whose meaning depends on the search: for an RV or a
// cabin it is a pipe at the site, and for a tent a shared tap counts too. The
// rule keys off the camper's own --shelter rather than off the site, because a
// site that permits both a tent and an RV has no answer to "which kind of camper
// is this" — only the camper does.
func (s *Site) Satisfies(h Hookup, shelter Shelter) bool {
	switch h {
	case HookupElectric:
		return s.Hookups.Electric != TriNo
	case HookupSewer:
		return s.Hookups.Sewer != TriNo
	case HookupWater:
		if s.Hookups.Water == TriYes {
			return true
		}
		if shelter == ShelterRV || shelter == ShelterCabin {
			return s.Hookups.Water != TriNo
		}
		return s.Hookups.Water != TriNo || s.SharedWater != TriNo
	}
	return true
}

// WaterSource names which fact satisfied a water requirement, so an alert never
// says a bare "water" when what it found was a shared tap.
func (s *Site) WaterSource() string {
	switch {
	case s.Hookups.Water == TriYes:
		return "hookup at site"
	case s.SharedWater == TriYes:
		return "shared source"
	case s.Hookups.Water == TriNo:
		return "no hookup"
	default:
		return "not reported"
	}
}
