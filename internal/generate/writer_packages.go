package generate

import (
	"maps"
	"slices"
)

// unionAbsent merges registry-required package removals (e.g. the ntfs3
// preset's ntfs-3g removal, mirroring the legacy script's kernel >= 7.1
// cleanup) into packages.absent. A removal candidate already in the
// unioned present list is skipped: packages are union-only, so a module
// that ever required the package keeps it installed (removing a mount
// never removes its packages either).
func unionAbsent(existing []string, presets map[string]Entry, present []string) []string {
	skip := make(map[string]struct{}, len(present))
	for _, p := range present {
		skip[p] = struct{}{}
	}
	set := make(map[string]struct{}, len(existing)+1)
	for _, p := range existing {
		set[p] = struct{}{}
	}
	for _, preset := range presets {
		for _, p := range preset.Absent {
			if _, conflict := skip[p]; conflict {
				continue
			}
			set[p] = struct{}{}
		}
	}
	if len(set) == 0 {
		return nil
	}
	return slices.Sorted(maps.Keys(set))
}

// unionPackages returns the sorted, deduped union of the existing present
// list, the registry-required packages of every used mount type, and the
// samba/avahi packages when smb is managed (avahi only when enabled).
func unionPackages(existing []string, input Input, presets map[string]Entry) []string {
	set := make(map[string]struct{}, len(existing)+4)
	for _, p := range existing {
		set[p] = struct{}{}
	}
	for _, preset := range presets {
		for _, p := range preset.Packages {
			set[p] = struct{}{}
		}
	}
	if input.Smb != nil {
		set[sambaPackage] = struct{}{}
		if input.Smb.Avahi == nil || *input.Smb.Avahi {
			set[avahiPackage] = struct{}{}
		}
	}
	return slices.Sorted(maps.Keys(set))
}
