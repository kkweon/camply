package core

import (
	"fmt"
	"strconv"
	"strings"
)

// This file holds everything a campsite says about itself to a reader — the
// notification body, its title, and the terminal listing all render from here.
//
// The rendering lives together on purpose. Vehicle access and equipment follow
// one rule: missing data is surfaced, never acted on. Two copies of that rule in
// two files drift, and the drift would show up as a warning that quietly stopped
// being emitted.

// SiteAccessSummary is the line an alert always carries.
//
// It is never empty. The incident this field exists for was an alert that said
// nothing at all about vehicle access for a site half a mile from the nearest
// road, so "we don't know" has to be as visible as "you can't drive there" —
// an omitted line reads as reassurance.
func (c AvailableCampsite) SiteAccessSummary() string {
	label := strings.TrimSpace(c.SiteAccessRaw)
	if label == "" {
		label = c.SiteAccess.String()
	}

	switch {
	case c.SiteAccess.NoVehicleAccess():
		return "⚠️ " + strings.ToUpper(label) + " — no vehicle access" + c.maxVehiclesSuffix()
	case c.SiteAccess.HasVehicleAccess():
		return label + c.maxVehiclesSuffix()
	default:
		return "⚠️ UNKNOWN — the provider reports no vehicle access data; " +
			"verify on the booking page before reserving"
	}
}

// SiteAccessAlert is the short marker for a notification title or a terminal
// line. It is empty only for a site confirmed reachable by car, so anything
// that is not a plain drive-in site is flagged wherever it is shown.
func (c AvailableCampsite) SiteAccessAlert() string {
	if c.SiteAccess.HasVehicleAccess() {
		return ""
	}
	if !c.SiteAccess.NoVehicleAccess() {
		return "⚠️ UNKNOWN ACCESS"
	}

	label := strings.TrimSpace(c.SiteAccessRaw)
	if label == "" {
		label = c.SiteAccess.String()
	}
	return "⚠️ " + strings.ToUpper(label)
}

func (c AvailableCampsite) maxVehiclesSuffix() string {
	if c.MaxVehicles == nil {
		return ""
	}
	return " (Max Vehicles: " + strconv.Itoa(*c.MaxVehicles) + ")"
}

// EquipmentSummary is the equipment line an alert always carries.
//
// Like SiteAccessSummary it is never empty. A site that reached the results
// without being shown to match the equipment filter says so here, because it is
// the only place the reader can learn that the match was assumed rather than
// verified.
func (c AvailableCampsite) EquipmentSummary() string {
	if c.EquipmentUnverified {
		return "⚠️ UNKNOWN — the provider reports no equipment data; " +
			"verify on the booking page before reserving"
	}
	if len(c.PermittedEquipment) == 0 {
		return "Not reported"
	}

	parts := make([]string, 0, len(c.PermittedEquipment))
	for _, e := range c.PermittedEquipment {
		if e.MaxLength > 0 {
			parts = append(parts, fmt.Sprintf("%s (max %dft)", e.EquipmentName, e.MaxLength))
			continue
		}
		parts = append(parts, e.EquipmentName)
	}
	return strings.Join(parts, ", ")
}

// Warnings returns every ⚠️ marker a campsite carries, for a notification title
// or a terminal line.
//
// It returns a list rather than one string because a site can be doubtful on
// more than one count at once, and picking a single "worst" marker would hide
// the others. Empty means nothing about this site needs checking.
func (c AvailableCampsite) Warnings() []string {
	var out []string
	if alert := c.SiteAccessAlert(); alert != "" {
		out = append(out, alert)
	}
	if c.EquipmentUnverified {
		out = append(out, "⚠️ NO EQUIPMENT DATA")
	}
	return out
}

// WarningPrefix joins Warnings for a title, empty when there is nothing to flag.
func (c AvailableCampsite) WarningPrefix() string {
	return strings.Join(c.Warnings(), " ")
}
