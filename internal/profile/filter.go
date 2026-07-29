package profile

import (
	"fmt"
	"strings"
)

// FilterReason is the Skip.Reason stamped on modules excluded by the
// positional module filter.
const FilterReason = "module filter"

// ParseModuleFilter normalizes positional module-filter args: each arg is
// split on commas, whitespace is trimmed, empties are dropped, and duplicates
// collapse keeping first-seen order. Empty input returns nil.
func ParseModuleFilter(args []string) []string {
	seen := make(map[string]struct{})
	var ids []string
	for _, arg := range args {
		for _, part := range strings.Split(arg, ",") {
			id := strings.TrimSpace(part)
			if id == "" {
				continue
			}
			if _, dup := seen[id]; dup {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	return ids
}

// LimitTo restricts the selection to the listed module ids. An empty list is
// a no-op. Every id must name a discovered module, and the filter never
// resurrects modules skipped for their own reason (disabled, when filter):
// naming one is an error carrying that reason. Selected modules not in ids
// move to Skipped with reason "module filter", preserving order.
func (p *Profile) LimitTo(ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	known := make(map[string]struct{}, len(p.Modules))
	valid := make([]string, 0, len(p.Modules))
	for _, m := range p.Modules {
		known[m.ID] = struct{}{}
		valid = append(valid, m.ID)
	}
	var unknown []string
	for _, id := range ids {
		if _, ok := known[id]; !ok {
			unknown = append(unknown, id)
		}
	}
	if len(unknown) > 0 {
		return fmt.Errorf("unknown module(s): %s (valid modules: %s)",
			strings.Join(unknown, ", "), strings.Join(valid, ", "))
	}

	skipReasons := make(map[string]string, len(p.Skipped))
	for _, s := range p.Skipped {
		skipReasons[s.Module.ID] = s.Reason
	}
	var notSelected []string
	for _, id := range ids {
		if reason, skipped := skipReasons[id]; skipped {
			notSelected = append(notSelected, fmt.Sprintf("%s (%s)", id, reason))
		}
	}
	if len(notSelected) > 0 {
		return fmt.Errorf("module(s) not selected: %s", strings.Join(notSelected, ", "))
	}

	want := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		want[id] = struct{}{}
	}
	var selected []Module
	for _, m := range p.Selected {
		if _, ok := want[m.ID]; ok {
			selected = append(selected, m)
		} else {
			p.Skipped = append(p.Skipped, Skip{Module: m, Reason: FilterReason})
		}
	}
	p.Selected = selected
	return nil
}
