package resolve

import (
	"fmt"
	"sort"

	"github.com/thedataflows/dotdrift/internal/profile"
)

// MountEntry is a single resolved mount: the winning layer's MountSpec plus
// the provenance (module, winning layer, module scope) the apply steps need.
type MountEntry struct {
	Name   string
	Spec   profile.MountSpec
	Module string
	Layer  string
	Scope  string
}

// MountsStep is the plan aggregate of mount entries across all selected
// modules, sorted by (Module, Name) for deterministic output.
type MountsStep struct {
	Entries []MountEntry
}

// SmbModuleSpec is one contributing module's merged smb spec. Modules lists
// which module contributed; the spec itself carries group/users/avahi and
// the merged shares. Sorted by Module for deterministic output.
type SmbModuleSpec struct {
	Module string
	Spec   profile.SmbSpec
}

// SmbStep aggregates each contributing module's merged smb spec. A module
// with no [smb] content in any layer contributes nothing.
type SmbStep struct {
	Modules []SmbModuleSpec
}

type mountWinner struct {
	layer string
	spec  profile.MountSpec
}

// mergeMounts merges [mounts] across layers whole-entry by name — identical
// winner-map semantics to dotfiles per-target: a higher layer's entry fully
// replaces the lower layer's, with no field-level merge. The winning entry
// is structurally validated (the losing entries are not: they never reach
// the plan). Mount type is deliberately NOT validated against any registry.
func mergeMounts(base, host, user layerConfig, moduleID, scope string) ([]MountEntry, error) {
	winners := make(map[string]mountWinner)
	for name, spec := range base.cfg.Mounts {
		winners[name] = mountWinner{layer: "base", spec: spec}
	}
	for name, spec := range host.cfg.Mounts {
		winners[name] = mountWinner{layer: "host", spec: spec}
	}
	for name, spec := range user.cfg.Mounts {
		winners[name] = mountWinner{layer: "user", spec: spec}
	}

	entries := make([]MountEntry, 0, len(winners))
	for name, winner := range winners {
		if err := validateMountSpec(moduleID, name, winner.spec); err != nil {
			return nil, err
		}
		entries = append(entries, MountEntry{
			Name:   name,
			Spec:   winner.spec,
			Module: moduleID,
			Layer:  winner.layer,
			Scope:  scope,
		})
	}
	return entries, nil
}

// validateMountSpec enforces the structural contract of a mount entry:
// source, destination, and type are required; state is "", "enabled", or
// "disabled". Type values are never checked against a registry.
func validateMountSpec(moduleID, name string, spec profile.MountSpec) error {
	if spec.Source == "" {
		return fmt.Errorf("module %s: mount %q: source is required", moduleID, name)
	}
	if spec.Destination == "" {
		return fmt.Errorf("module %s: mount %q: destination is required", moduleID, name)
	}
	if spec.Type == "" {
		return fmt.Errorf("module %s: mount %q: type is required", moduleID, name)
	}
	switch spec.State {
	case "", "enabled", "disabled":
	default:
		return fmt.Errorf("module %s: mount %q: unknown state %q (valid: enabled, disabled)", moduleID, name, spec.State)
	}
	return nil
}

// mergeSmb merges [smb] across layers. Scalar fields replace when the higher
// layer sets them: group replaces when non-empty, users replaces wholesale
// when non-empty (never appended — the deliberate contrast to hooks), avahi
// replaces when the key is present (a *bool, so explicit false is settable).
// Shares merge whole-entry by name like mounts. contributed reports whether
// any layer declared smb content at all.
// hasSmbContent reports whether an smb spec carries any declaration —
// the same "contributed" predicate mergeSmb uses, for the scope guard in
// resolve.go.
func hasSmbContent(s profile.SmbSpec) bool {
	return s.Group != "" || len(s.Users) > 0 || s.Avahi != nil || len(s.Shares) > 0
}

func mergeSmb(base, host, user layerConfig, moduleID string) (spec profile.SmbSpec, contributed bool, err error) {
	for _, layer := range []layerConfig{base, host, user} {
		s := layer.cfg.Smb
		if s.Group != "" {
			spec.Group = s.Group
			contributed = true
		}
		if len(s.Users) > 0 {
			spec.Users = s.Users
			contributed = true
		}
		if s.Avahi != nil {
			spec.Avahi = s.Avahi
			contributed = true
		}
		for name, share := range s.Shares {
			if spec.Shares == nil {
				spec.Shares = make(map[string]profile.ShareSpec)
			}
			spec.Shares[name] = share
			contributed = true
		}
	}

	names := make([]string, 0, len(spec.Shares))
	for name := range spec.Shares {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if spec.Shares[name].Path == "" {
			return spec, contributed, fmt.Errorf("module %s: share %q: path is required", moduleID, name)
		}
	}
	return spec, contributed, nil
}

func sortMountEntries(entries []MountEntry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Module != entries[j].Module {
			return entries[i].Module < entries[j].Module
		}
		return entries[i].Name < entries[j].Name
	})
}

func sortSmbModules(mods []SmbModuleSpec) {
	sort.Slice(mods, func(i, j int) bool {
		return mods[i].Module < mods[j].Module
	})
}
