package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/alecthomas/kong"
)

// sectionNames are the module.toml plan sections, each with a negatable
// apply flag (--packages/--no-packages, ...). Order is the pipeline order.
var sectionNames = []string{"packages", "tools", "dotfiles", "systemd", "mounts", "smb", "hooks"}

// sectionSet is the resolved selection of sections an apply run executes.
type sectionSet map[string]bool

// newSectionSet builds a set from names, rejecting anything outside
// sectionNames (a typo must fail loudly, listing the valid sections).
func newSectionSet(names ...string) (sectionSet, error) {
	valid := map[string]struct{}{}
	for _, n := range sectionNames {
		valid[n] = struct{}{}
	}
	set := make(sectionSet, len(names))
	var unknown []string
	for _, n := range names {
		if _, ok := valid[n]; !ok {
			unknown = append(unknown, n)
			continue
		}
		set[n] = true
	}
	if len(unknown) > 0 {
		return nil, fmt.Errorf("unknown section(s): %s (valid sections: %s)",
			strings.Join(unknown, ", "), strings.Join(sectionNames, ", "))
	}
	return set, nil
}

// resolveSections computes the executed sections from the explicitly set
// flags (name → true for --<section>, false for --no-<section>; absent
// flags omitted): any positive flag makes the positives the whole
// selection, negatives subtract, and an empty result is an error — a
// silent no-op apply is never OK.
func resolveSections(explicit map[string]bool) (sectionSet, error) {
	selected := make(sectionSet, len(sectionNames))
	if len(explicit) > 0 {
		var positives []string
		for name, on := range explicit {
			if on {
				positives = append(positives, name)
			}
		}
		if len(positives) > 0 {
			for _, p := range positives {
				selected[p] = true
			}
		} else {
			for _, n := range sectionNames {
				selected[n] = true
			}
		}
	} else {
		for _, n := range sectionNames {
			selected[n] = true
		}
	}
	for name, on := range explicit {
		if !on {
			delete(selected, name)
		}
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("no sections selected by section flags (valid sections: %s)",
			strings.Join(sectionNames, ", "))
	}
	return selected, nil
}

// AfterApply captures the parse context so Run can read which section
// flags were explicitly set (positive or negated) — a plain bool field
// cannot distinguish `--hooks` from an untouched default.
func (c *ApplyCmd) AfterApply(kctx *kong.Context) error {
	c.kctx = kctx
	return nil
}

// resolveSections resolves the run's executed sections: the programmatic
// onlySections override when set (tests, library callers), otherwise the
// explicitly parsed section flags. DOTDRIFT_NO_HOOKS=1 subtracts hooks on
// top, exactly like --no-hooks (a kill-switch that never resurrects).
func (c *ApplyCmd) resolveSections() (sectionSet, error) {
	if c.onlySections != nil {
		return newSectionSet(c.onlySections...)
	}
	sections := map[string]bool{}
	if c.kctx != nil {
		for _, fl := range c.kctx.Flags() {
			if fl.Flag == nil || !fl.Set {
				continue
			}
			on, ok := fl.Target.Interface().(bool)
			if !ok || !isSectionName(fl.Name) {
				continue
			}
			sections[fl.Name] = on
		}
	}
	if os.Getenv("DOTDRIFT_NO_HOOKS") == "1" {
		sections["hooks"] = false
	}
	return resolveSections(sections)
}

func isSectionName(name string) bool {
	for _, n := range sectionNames {
		if n == name {
			return true
		}
	}
	return false
}

// has reports whether the run executes the named section.
func (s sectionSet) has(name string) bool { return s[name] }
