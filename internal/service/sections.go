package service

import (
	"fmt"
	"sort"
	"strings"
)

// SectionNames are the module.toml plan sections in pipeline order. The
// CLI's negatable flags map onto these (cmd keeps the flag layer; this
// list is the single source of the vocabulary).
var SectionNames = []string{"packages", "tools", "dotfiles", "systemd", "mounts", "smb", "hooks"}

// SectionSet is the resolved selection of sections an apply run executes.
type SectionSet map[string]bool

// Has reports whether the run executes the named section.
func (s SectionSet) Has(name string) bool { return s[name] }

// ResolveSections computes the executed sections from an explicit
// selection (name → true to select, false to subtract; nil or empty
// means everything): any positive entry makes the positives the whole
// selection, negatives subtract, and an empty result is an error — a
// silent no-op apply is never OK. Names outside SectionNames are
// rejected with the valid list (a typo must fail loudly).
func ResolveSections(explicit map[string]bool) (SectionSet, error) {
	all := make(SectionSet, len(SectionNames))
	for _, n := range SectionNames {
		all[n] = true
	}
	var unknown []string
	for name := range explicit {
		if _, ok := all[name]; !ok {
			unknown = append(unknown, name)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return nil, fmt.Errorf("unknown section(s): %s (valid sections: %s)",
			strings.Join(unknown, ", "), strings.Join(SectionNames, ", "))
	}
	selected := make(SectionSet, len(SectionNames))
	for n := range all {
		selected[n] = true
	}
	if len(explicit) > 0 {
		var positives []string
		for name, on := range explicit {
			if on {
				positives = append(positives, name)
			}
		}
		if len(positives) > 0 {
			selected = make(SectionSet, len(positives))
			for _, p := range positives {
				selected[p] = true
			}
		}
		for name, on := range explicit {
			if !on {
				delete(selected, name)
			}
		}
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("no sections selected (valid sections: %s)",
			strings.Join(SectionNames, ", "))
	}
	return selected, nil
}
