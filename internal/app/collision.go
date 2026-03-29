package app

import (
	"strings"

	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/ui"
)

// wellKnownGroups contains Kubernetes API groups that don't follow the
// *.k8s.io naming convention but are still built-in.
var wellKnownGroups = map[string]bool{
	"apps":        true,
	"batch":       true,
	"autoscaling": true,
	"policy":      true,
}

// isBuiltInGroup returns true if the API group is a well-known Kubernetes
// built-in group: the core group (empty string), any group ending with
// ".k8s.io", or a well-known short group name (apps, batch, etc.).
func isBuiltInGroup(group string) bool {
	return group == "" || strings.HasSuffix(group, ".k8s.io") || wellKnownGroups[group]
}

// markCollisions detects name collisions among PluginEntries and sets
// Qualified=true on entries that need disambiguation.
//
// Rules:
//   - If multiple entries share the same Name, they collide
//   - In a collision, entries from built-in groups stay Qualified=false
//   - Only non-built-in entries get Qualified=true
//   - If ALL colliding entries are built-in or ALL are non-built-in,
//     ALL get Qualified=true
func markCollisions(entries []ui.PluginEntry) []ui.PluginEntry {
	// Group entry indices by name.
	byName := make(map[string][]int)
	for i, e := range entries {
		byName[e.Name] = append(byName[e.Name], i)
	}

	remove := make(map[int]bool)

	for _, indices := range byName {
		if len(indices) < 2 {
			continue
		}

		// Determine if we have a mix of built-in and non-built-in.
		hasBuiltIn := false
		hasNonBuiltIn := false
		for _, idx := range indices {
			if isBuiltInGroup(entries[idx].GVR.Group) {
				hasBuiltIn = true
			} else {
				hasNonBuiltIn = true
			}
		}

		if hasBuiltIn && hasNonBuiltIn {
			// Mixed: only non-built-in entries get qualified.
			for _, idx := range indices {
				if !isBuiltInGroup(entries[idx].GVR.Group) {
					entries[idx].Qualified = true
				}
			}
		} else if hasBuiltIn {
			// All built-in (e.g. events in core/v1 and events.k8s.io/v1):
			// keep only the first-registered (primary) entry, remove duplicates.
			for _, idx := range indices[1:] {
				remove[idx] = true
			}
		} else {
			// All non-built-in: qualify all.
			for _, idx := range indices {
				entries[idx].Qualified = true
			}
		}
	}

	if len(remove) > 0 {
		filtered := make([]ui.PluginEntry, 0, len(entries)-len(remove))
		for i, e := range entries {
			if !remove[i] {
				filtered = append(filtered, e)
			}
		}
		return filtered
	}

	return entries
}

// buildPickerEntries constructs PluginEntry values from all registered plugins
// and runs collision detection to set Qualified on entries that need
// disambiguation.
func buildPickerEntries() []ui.PluginEntry {
	allPlugins := plugin.All()
	entries := make([]ui.PluginEntry, len(allPlugins))
	for i, p := range allPlugins {
		entries[i] = ui.PluginEntry{
			Name:      p.Name(),
			ShortName: p.ShortName(),
			GVR:       p.GVR(),
		}
	}
	return markCollisions(entries)
}
