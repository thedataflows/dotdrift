package tui

// Issue 0074 structural families: the row grammar's mutation half. Rows
// carry machine paths (pathKey); these mutators translate a committed
// input + path into config writes. Every commit still re-encodes the
// whole family through the profile encoders and splices it into the
// draft's working raw — the strict-decode round-trip in applyEdit is the
// correctness proof, these functions only place values. Refusing returns
// an error: the field stays open with the reason rendered at it.

import (
	"errors"
	"fmt"
	"strings"

	"github.com/thedataflows/dotdrift/internal/profile"
)

// mutateStructural applies a committed input to a structural family.
// addPath scopes adds ("" adds the section's entry itself); for edits it
// is empty and key carries the row's machine path.
func mutateStructural(cfg *profile.ModuleConfig, family, addPath, key, input string, add bool) error {
	switch family {
	case profile.FamilySystemd:
		return mutateSystemd(cfg, addPath, key, input, add)
	}
	return fmt.Errorf("family %q has no structural editor", family)
}

// mutateSystemd edits the systemd passthrough: units add whole (name
// only, directives follow), directives add as `Name = value` and edit
// value-only (parsed as a TOML value, plain-string fallback). An empty
// value refuses: the encoder drops empty values, so committing one would
// silently delete the directive.
func mutateSystemd(cfg *profile.ModuleConfig, addPath, key, input string, add bool) error {
	if add {
		if addPath == "" {
			if cfg.Systemd.Units == nil {
				cfg.Systemd.Units = map[string]profile.SystemdUnit{}
			}
			cfg.Systemd.Units[strings.TrimSpace(input)] = profile.SystemdUnit{}
			return nil
		}
		name, value, ok := strings.Cut(input, "=")
		name = strings.TrimSpace(name)
		if !ok || name == "" {
			return errors.New(`add as "Name = value"`)
		}
		unit := cfg.Systemd.Units[addPath]
		if unit == nil {
			unit = profile.SystemdUnit{}
		}
		unit[name] = parseTomlValue(value)
		cfg.Systemd.Units[addPath] = unit
		return nil
	}

	parts := splitPath(key)
	if len(parts) != 2 {
		return fmt.Errorf("malformed directive row %q", key)
	}
	if input == "" {
		return errors.New("directive value must not be empty (d removes the directive)")
	}
	unit := cfg.Systemd.Units[parts[0]]
	if unit == nil {
		return fmt.Errorf("unknown unit %q", parts[0])
	}
	unit[parts[1]] = parseTomlValue(input)
	cfg.Systemd.Units[parts[0]] = unit
	return nil
}
