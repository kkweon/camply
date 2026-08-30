package recdotgov

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/kkweon/camply/internal/core"
)

// This file is the only place recreation.gov's vocabulary appears. Everything
// upstream of it works in camply's own terms.
//
// The mapping was derived by measuring 678 live campsites across six
// campgrounds, not by reading the API docs, because the fields do not mean what
// their names suggest.

// Attribute names the adapter reads.
//
// attrMaxVehicleLength is a different attribute from attrMaxNumVehicles, and
// only its POSITIVE value is evidence: it reads 0 on 27 genuine drive-in sites
// at Manzanita Lake and on all 9 Kaspian sites. Treating that 0 as "no vehicle"
// would delete valid results.
const (
	attrSiteAccess       = "Site Access"
	attrMaxNumVehicles   = "Max Num of Vehicles"
	attrMaxVehicleLength = "Max Vehicle Length"
	attrDrivewayLength   = "Driveway Length"
	attrDrivewaySurface  = "Driveway Surface"
	attrDrivewayEntry    = "Driveway Entry"
)

// hikeDistanceAttrs are the spellings of one concept. Attribute names are an
// alias set, never a substring match — "Water" as a substring would sweep
// drinking water and lakefront location in with a hookup.
var hikeDistanceAttrs = []string{"Hike in Distance", "Hike In Distance to Site"}

// siteAccessLabels maps the Site Access vocabulary. It is open: an unlisted
// label is honoured as non-drivable rather than guessed into a known mode.
var siteAccessLabels = map[string]core.Parking{
	"drive-in": core.ParkingAtSite,
	"drive in": core.ParkingAtSite,
	"walk-in":  core.ParkingWalk,
	"walk in":  core.ParkingWalk,
	"hike-in":  core.ParkingWalk,
	"hike in":  core.ParkingWalk,
	"boat-in":  core.ParkingNone,
	"boat in":  core.ParkingNone,
}

// noVehicleCampsiteTypes name their own arrival mode, so the fallback can say
// which one rather than a bare "no vehicle".
var noVehicleCampsiteTypes = map[string]core.Parking{
	"WALK TO":       core.ParkingWalk,
	"HIKE TO":       core.ParkingWalk,
	"GROUP HIKE TO": core.ParkingWalk,
	"BOAT IN":       core.ParkingNone,
}

// classifyParking decides how close a car gets.
//
// The signals are ranked by how they held up against the live API, not by
// convenience:
//
//  1. Max Num of Vehicles == 0, an explicit no-vehicle notice, or a BOAT IN type
//     is conclusive: no car reaches it.
//  2. Site Access, or a type that names an arrival mode, or a recorded hike
//     distance means park-and-walk. Site Access outranks the vehicle count,
//     because Lodgepole reports "Max Num of Vehicles: 1" on all 54 of its
//     Hike-In and Walk-In sites — that is trailhead parking, not access.
//  3. A driveway attribute, or a positive Max Vehicle Length, means the car
//     stops at the site. Driveway Length/Surface/Entry are populated on 69-83%
//     of sites and are exactly what "a driveway at the site" means; camply never
//     read them.
//  4. Otherwise Unknown — a real answer here, reported rather than guessed.
func classifyParking(a attributes, campsiteType, notices string) (core.Parking, string) {
	upperType := strings.ToUpper(strings.TrimSpace(campsiteType))

	if mv := a.number(attrMaxNumVehicles); mv != nil && *mv == 0 {
		return core.ParkingNone, attrMaxNumVehicles + "=0"
	}
	if strings.Contains(strings.ToUpper(notices), "NO VEHICLE ACCESS") {
		return core.ParkingNone, "notice: NO VEHICLE ACCESS"
	}
	if upperType == "BOAT IN" {
		return core.ParkingNone, "campsite_type=BOAT IN"
	}

	// Meeks Bay returns an empty string where it has no answer, which is
	// absence, not a label.
	if raw := strings.TrimSpace(a.text(attrSiteAccess)); raw != "" {
		if known, ok := siteAccessLabels[strings.ToLower(raw)]; ok {
			return known, attrSiteAccess + "=" + raw
		}
		// A label outside the known set still means "not Drive-In".
		return core.ParkingWalk, attrSiteAccess + "=" + raw
	}
	if p, ok := noVehicleCampsiteTypes[upperType]; ok {
		return p, "campsite_type=" + upperType
	}
	for _, name := range hikeDistanceAttrs {
		if a.text(name) != "" {
			return core.ParkingWalk, name + "=" + a.text(name)
		}
	}

	if a.text(attrDrivewayLength) != "" || a.text(attrDrivewaySurface) != "" || a.text(attrDrivewayEntry) != "" {
		return core.ParkingAtSite, "driveway attributes"
	}
	if l := a.number(attrMaxVehicleLength); l != nil && *l > 0 {
		return core.ParkingAtSite, attrMaxVehicleLength + "=" + strconv.Itoa(*l)
	}

	return core.ParkingUnknown, ""
}

// tentEquipment and rvEquipment partition recreation.gov's equipment vocabulary
// by what a camper sleeps in.
var (
	tentEquipment = map[string]bool{
		"tent": true, "small tent": true,
		"large tent over 9x12": true, "large tent over 9x12`": true,
		"hammock": true,
	}
	rvEquipment = map[string]bool{
		"rv": true, "rv/motorhome": true, "pop up": true, "fifth wheel": true,
		"caravan/camper van": true, "trailer": true, "pickup camper": true,
	}
)

// classifyPermits decides what the site accepts.
//
// permitted_equipment is the authority and the type token only the fallback,
// which is the reverse of how it reads: 98% of STANDARD sites permit both a tent
// and an RV, and 50 sites typed TENT ONLY permit RVs. A cabin is the exception —
// it is not something you bring, so 80% of CABIN sites list no equipment at all,
// and that silence is expected rather than missing data.
func classifyPermits(equipment []core.Equipment, campsiteType string) (core.Permitted, string) {
	upperType := strings.ToUpper(strings.TrimSpace(campsiteType))

	var permits core.Permitted
	if strings.Contains(upperType, "CABIN") {
		permits |= core.PermitsCabin
	}
	for _, e := range equipment {
		name := strings.ToLower(strings.TrimSpace(e.EquipmentName))
		if tentEquipment[name] {
			permits |= core.PermitsTent
		}
		if rvEquipment[name] {
			permits |= core.PermitsRV
		}
	}
	if permits != core.PermittedUnknown {
		if len(equipment) > 0 {
			return permits, "permitted_equipment"
		}
		return permits, "campsite_type=" + upperType
	}

	switch {
	case strings.Contains(upperType, "TENT"):
		return core.PermitsTent, "campsite_type=" + upperType
	case strings.Contains(upperType, "RV"):
		return core.PermitsRV, "campsite_type=" + upperType
	case strings.Contains(upperType, "STANDARD"):
		return core.PermitsTent | core.PermitsRV, "campsite_type=" + upperType
	}
	return core.PermittedUnknown, ""
}

// hookupAttrs are alias sets, one concept per line.
var (
	electricHookupAttrs = []string{"Electricity Hookup", "Electric Hookup", "Electric Hookups"}
	waterHookupAttrs    = []string{"Water Hookup", "Water Hookups"}
	sewerHookupAttrs    = []string{"Sewer Hookup"}
	// sharedWaterAttrs are a tap somewhere in the campground, not a hookup at
	// the site. For a tent camper they are the water that matters.
	sharedWaterAttrs = []string{
		"Drinking Water", "Water Spigot", "Drinking Water (hand pump)",
		"Accessible Drinking Water",
	}
)

// classifyHookups reads electricity from the campsite type token and water and
// sewer from attributes.
//
// The token is the better source where both exist: 23 of 137 ELECTRIC sites
// carry no electricity attribute. It speaks only to electricity though — 61
// sites typed NONELECTRIC have a water hookup — so the three must not be derived
// from one another.
func classifyHookups(a attributes, campsiteType string) core.Hookups {
	upperType := strings.ToUpper(campsiteType)

	h := core.Hookups{}
	switch {
	case strings.Contains(upperType, "NONELECTRIC"):
		h.Electric = core.TriNo
	case strings.Contains(upperType, "ELECTRIC"):
		h.Electric = core.TriYes
	case a.anyPresent(electricHookupAttrs):
		h.Electric = core.TriYes
	}
	h.Water = a.yesNo(waterHookupAttrs)
	h.Sewer = a.yesNo(sewerHookupAttrs)
	return h
}

var leadingDigits = regexp.MustCompile(`\d+`)

// attributes is the flat, per-campground attribute list keyed by name.
type attributes map[string]string

func newAttributes(list []recdotgovAttribute) attributes {
	a := make(attributes, len(list))
	for _, attr := range list {
		a[attr.AttributeName] = attr.AttributeValue
	}
	return a
}

func (a attributes) text(name string) string { return strings.TrimSpace(a[name]) }

// number returns nil for an absent or unparseable value, keeping "not reported"
// distinct from zero — only the first of those means anything.
func (a attributes) number(name string) *int {
	raw, ok := a[name]
	if !ok {
		return nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return nil
	}
	return &n
}

// firstNumber reads the first alias of a concept that carries a number. Values
// are written inconsistently even within one campground ("39 feet", "30 Feet",
// "20"), so it takes the leading digits.
func (a attributes) firstNumber(names []string) *int {
	for _, n := range names {
		raw, ok := a[n]
		if !ok {
			continue
		}
		if m := leadingDigits.FindString(raw); m != "" {
			v, err := strconv.Atoi(m)
			if err != nil {
				continue
			}
			return &v
		}
	}
	return nil
}

func (a attributes) anyPresent(names []string) bool {
	for _, n := range names {
		if _, ok := a[n]; ok {
			return true
		}
	}
	return false
}

// yesNo reads an attribute that carries an explicit Yes/No, which is a real
// TriNo rather than an absence. Anything else present counts as Yes, since these
// attributes are also used as bare presence flags.
func (a attributes) yesNo(names []string) core.Tri {
	for _, n := range names {
		raw, ok := a[n]
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case "no", "n/a", "":
			return core.TriNo
		default:
			return core.TriYes
		}
	}
	return core.TriUnknown
}
