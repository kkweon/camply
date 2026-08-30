package core

import (
	"fmt"
	"strconv"
	"strings"
)

// Everything a campsite says about itself to a reader: the notification body,
// its title, and the terminal listing all render from here.
//
// The rendering lives together on purpose. Parking and equipment follow one
// rule — missing data is surfaced, never acted on — and two copies of that rule
// in two files drift, with the drift showing up as a warning that quietly
// stopped being emitted.

// accessLabel is the provider's own word for how the site is reached, falling
// back to camply's own when the provider offered none.
func (a Availability) accessLabel() string {
	if label := strings.TrimSpace(a.Site.AccessLabel); label != "" {
		return label
	}
	switch a.Site.Parking {
	case ParkingAtSite:
		return "Drive-In"
	case ParkingWalk:
		return "Walk-In"
	case ParkingNone:
		return "No Vehicle Access"
	default:
		return "Unknown"
	}
}

// SiteAccessSummary is the line an alert always carries.
//
// It is never empty. The incident this exists for was an alert that said
// nothing at all about vehicle access for a site half a mile from the nearest
// road, so "we don't know" has to be as visible as "you can't drive there" — an
// omitted line reads as reassurance.
func (a Availability) SiteAccessSummary() string {
	switch {
	case a.Site.Parking.RequiresWalk():
		return "⚠️ " + strings.ToUpper(a.accessLabel()) + " — no vehicle access" +
			a.walkSuffix() + a.maxVehiclesSuffix()
	case a.Site.Parking.ReachableByCar():
		return a.accessLabel() + a.maxVehiclesSuffix()
	default:
		return "⚠️ UNKNOWN — the provider reports no vehicle access data; " +
			"verify on the booking page before reserving"
	}
}

// SiteAccessAlert is the short marker for a notification title or a terminal
// line. It is empty only for a site a car reaches, so anything else is flagged
// wherever it is shown.
func (a Availability) SiteAccessAlert() string {
	if a.Site.Parking.ReachableByCar() {
		return ""
	}
	if !a.Site.Parking.RequiresWalk() {
		return "⚠️ UNKNOWN ACCESS"
	}
	return "⚠️ " + strings.ToUpper(a.accessLabel())
}

// walkSuffix reports how far the gear is carried, where the provider says.
//
// Only 5% of sites record it, and it is absent exactly where it matters most —
// Zephyr Cove's half-mile is prose in a notice — so it is shown when known and
// never filtered on. Filtering would make 95% of sites unknown and recreate the
// silent loss this codebase spent a day removing.
func (a Availability) walkSuffix() string {
	if a.Site.WalkFeet == nil {
		return ""
	}
	return fmt.Sprintf(", %dft from parking", *a.Site.WalkFeet)
}

func (a Availability) maxVehiclesSuffix() string {
	if a.Site.MaxVehicles == nil {
		return ""
	}
	return " (Max Vehicles: " + strconv.Itoa(*a.Site.MaxVehicles) + ")"
}

// EquipmentSummary is the equipment line an alert always carries.
//
// Like SiteAccessSummary it is never empty. A site that reached the results
// without being shown to match the equipment filter says so here, because it is
// the only place the reader can learn the match was assumed rather than
// verified.
func (a Availability) EquipmentSummary() string {
	if a.EquipmentUnverified {
		return "⚠️ UNKNOWN — the provider reports no equipment data; " +
			"verify on the booking page before reserving"
	}
	if len(a.Site.Equipment) == 0 {
		return "Not reported"
	}

	parts := make([]string, 0, len(a.Site.Equipment))
	for _, e := range a.Site.Equipment {
		if e.MaxLength > 0 {
			parts = append(parts, fmt.Sprintf("%s (max %dft)", e.EquipmentName, e.MaxLength))
			continue
		}
		parts = append(parts, e.EquipmentName)
	}
	return strings.Join(parts, ", ")
}

// Warnings returns every ⚠️ marker this booking carries.
//
// A list rather than one string because a site can be doubtful on more than one
// count at once, and picking a single "worst" marker would hide the others.
func (a Availability) Warnings() []string {
	var out []string
	if alert := a.SiteAccessAlert(); alert != "" {
		out = append(out, alert)
	}
	if a.EquipmentUnverified {
		out = append(out, "⚠️ NO EQUIPMENT DATA")
	}
	return out
}

// WarningPrefix joins Warnings for a title, empty when there is nothing to flag.
func (a Availability) WarningPrefix() string {
	return strings.Join(a.Warnings(), " ")
}

// PermitsSummary is what the site accepts, in the camper's terms.
//
// It answers "will this take what I am bringing", which is a different question
// from the campsite type the provider prints — a STANDARD site takes a tent and
// an RV both, and its type says neither.
func (a Availability) PermitsSummary() string {
	var parts []string
	for _, p := range []struct {
		bit   Permitted
		label string
	}{
		{PermitsTent, "tent"},
		{PermitsRV, "RV"},
		{PermitsCabin, "cabin"},
	} {
		if a.Site.Permits.Has(p.bit) {
			parts = append(parts, p.label)
		}
	}
	if len(parts) == 0 {
		return "⚠️ UNKNOWN — the provider does not say what this site accepts"
	}
	return strings.Join(parts, ", ")
}

// HookupsSummary reports the utilities, naming which fact was found for water so
// a shared tap is never reported as a hookup at the site.
func (a Availability) HookupsSummary() string {
	tri := func(t Tri) string {
		switch t {
		case TriYes:
			return "yes"
		case TriNo:
			return "no"
		default:
			return "not reported"
		}
	}

	parts := []string{"electric: " + tri(a.Site.Hookups.Electric)}
	if a.Site.Amps != nil {
		parts[0] += fmt.Sprintf(" (%dA)", *a.Site.Amps)
	}
	parts = append(parts,
		"water: "+a.Site.WaterSource(),
		"sewer: "+tri(a.Site.Hookups.Sewer))
	return strings.Join(parts, ", ")
}
