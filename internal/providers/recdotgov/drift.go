package recdotgov

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Drift is what the adapter saw and did not understand.
//
// It separates two Unknowns that are otherwise indistinguishable:
//
//   - the provider said nothing, which is normal, permanent, and already
//     handled by surfacing rather than acting on missing data;
//   - the provider said something this adapter does not recognise, which is a
//     bug report waiting to be filed.
//
// Only the second is drift, and it has to be reported BY VALUE. A count tells a
// reader that something is wrong; the value tells them what to add. Without
// this, a vocabulary that grows on recreation.gov's side decays camply into
// Unknown one campsite at a time, and Unknown is the safe answer nobody
// investigates.
type Drift struct {
	mu     sync.Mutex
	values map[string]map[string]int // field -> raw value -> count
}

var drift = &Drift{}

func (d *Drift) record(field, value string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.values == nil {
		d.values = map[string]map[string]int{}
	}
	if d.values[field] == nil {
		d.values[field] = map[string]int{}
	}
	d.values[field][value]++
}

// TakeDrift returns what this provider could not map since the last call, and
// clears it. One line per field, listing the values with their counts.
func TakeDrift() []string {
	drift.mu.Lock()
	defer drift.mu.Unlock()

	fields := make([]string, 0, len(drift.values))
	for f := range drift.values {
		fields = append(fields, f)
	}
	sort.Strings(fields)

	var out []string
	for _, field := range fields {
		byValue := drift.values[field]
		values := make([]string, 0, len(byValue))
		for v := range byValue {
			values = append(values, v)
		}
		sort.Strings(values)

		parts := make([]string, 0, len(values))
		total := 0
		for _, v := range values {
			parts = append(parts, fmt.Sprintf("%q (%d)", v, byValue[v]))
			total += byValue[v]
		}
		out = append(out, fmt.Sprintf(
			"recdotgov: %d campsites use a %s value camply does not map: %s. "+
				"They are classified Unknown and flagged in results. "+
				"Add them to internal/providers/recdotgov/adapter.go.",
			total, field, strings.Join(parts, ", ")))
	}

	drift.values = nil
	return out
}
