package resolve

import (
	"fmt"
	"sort"

	"github.com/thedataflows/dotdrift/internal/profile"
)

// SystemdStep lists the systemd user units to converge (issue 0048).
type SystemdStep struct {
	Units []SystemdUnitEntry
}

// SystemdUnitEntry is one resolved user unit: name, derived kind
// (service|timer), and the passthrough directive table.
type SystemdUnitEntry struct {
	Name       string
	Kind       string // "service" | "timer"
	Directives map[string]any
	Module     string
	Layer      string
}

// systemdTriggerKeys make an entry a timer when present (mirrors mise's
// SystemdTomlConfig timer detection: any timer key, and at least one of the
// first four is required).
var systemdTriggerKeys = []string{
	"on_boot_sec", "on_unit_active_sec", "on_unit_inactive_sec", "on_calendar",
}

var systemdTimerKeys = append(append([]string{}, systemdTriggerKeys...),
	"randomized_delay_sec", "accuracy_sec", "persistent", "unit")

// systemdServiceOnlyKeys are rejected on timer entries (mirrors mise's
// service-only directive rejection).
var systemdServiceOnlyKeys = []string{
	"exec_start", "type", "remain_after_exit", "exec_stop", "timeout_start_sec",
	"timeout_stop_sec", "no_new_privileges", "private_tmp", "environment",
	"environment_file", "nice", "umask", "working_directory", "restart",
	"restart_sec", "standard_output", "standard_error",
}

// mergeSystemd unions unit declarations whole-entry by name (nearer layer
// wins) and validates structure. Units are user-scope only: a system-scope
// module declaring them is an error — mise skips user units under sudo, so
// such a declaration would silently no-op (issue 0048).
func mergeSystemd(base, host, user layerConfig, moduleID, scope string) ([]SystemdUnitEntry, error) {
	type winner struct {
		layer string
		unit  profile.SystemdUnit
	}
	winners := make(map[string]winner)
	for name, u := range base.cfg.Systemd.Units {
		winners[name] = winner{layer: "base", unit: u}
	}
	for name, u := range host.cfg.Systemd.Units {
		winners[name] = winner{layer: "host", unit: u}
	}
	for name, u := range user.cfg.Systemd.Units {
		winners[name] = winner{layer: "user", unit: u}
	}
	if len(winners) == 0 {
		return nil, nil
	}
	if scope != profile.ScopeUser {
		return nil, fmt.Errorf("module %s: systemd user units require scope = \"user\" (got %q): mise skips user units under sudo, a system-scope declaration would silently no-op", moduleID, scope)
	}

	names := make([]string, 0, len(winners))
	for n := range winners {
		names = append(names, n)
	}
	sort.Strings(names)
	entries := make([]SystemdUnitEntry, 0, len(names))
	for _, name := range names {
		w := winners[name]
		kind, err := validateSystemdUnit(moduleID, name, w.unit)
		if err != nil {
			return nil, err
		}
		entries = append(entries, SystemdUnitEntry{
			Name: name, Kind: kind, Directives: w.unit,
			Module: moduleID, Layer: w.layer,
		})
	}
	return entries, nil
}

// validateSystemdUnit enforces the structural contract (mise owns directive
// semantics): name charset, kind derivation, required keys per kind, and the
// service-only-key rejection on timers. Returns the derived kind.
func validateSystemdUnit(moduleID, name string, unit profile.SystemdUnit) (string, error) {
	if name == "" {
		return "", fmt.Errorf("module %s: systemd unit name must not be empty", moduleID)
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' ||
			r == '.' || r == '_' || r == '-' || r == '@') {
			return "", fmt.Errorf("module %s: systemd unit name %q must contain only letters, numbers, '.', '_', '-', or '@'", moduleID, name)
		}
	}

	isTimer := false
	for _, k := range systemdTimerKeys {
		if _, ok := unit[k]; ok {
			isTimer = true
			break
		}
	}

	if !isTimer {
		es, _ := unit["exec_start"].(string)
		if es == "" {
			return "", fmt.Errorf("module %s: systemd service unit %q must set a non-empty exec_start", moduleID, name)
		}
		return "service", nil
	}

	for _, k := range systemdServiceOnlyKeys {
		if _, ok := unit[k]; ok {
			return "", fmt.Errorf("module %s: systemd timer unit %q cannot set service-only directive %q", moduleID, name, k)
		}
	}
	hasTrigger := false
	for _, k := range systemdTriggerKeys {
		if _, ok := unit[k]; ok {
			hasTrigger = true
			break
		}
	}
	if !hasTrigger {
		return "", fmt.Errorf("module %s: systemd timer unit %q must set at least one of on_boot_sec, on_unit_active_sec, on_unit_inactive_sec, or on_calendar", moduleID, name)
	}
	return "timer", nil
}

// sortSystemdUnits orders plan units by name (deterministic plan + emission).
func sortSystemdUnits(units []SystemdUnitEntry) {
	sort.Slice(units, func(i, j int) bool { return units[i].Name < units[j].Name })
}
