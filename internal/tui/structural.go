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
	case profile.FamilySecrets:
		return mutateSecrets(cfg, key, input, add)
	case profile.FamilyMounts:
		return mutateMounts(cfg, key, input, add)
	case profile.FamilySmb:
		return mutateSmb(cfg, key, input, add)
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

// mutateSecrets: entries add as "name = ENV" (the file's short form);
// env edits as required text, description freely, allow_empty as a bool.
func mutateSecrets(cfg *profile.ModuleConfig, key, input string, add bool) error {
	if add {
		name, env, _ := strings.Cut(input, "=")
		name, env = strings.TrimSpace(name), strings.TrimSpace(env)
		if name == "" || env == "" {
			return errors.New(`add as "name = ENV"`)
		}
		if cfg.Secrets == nil {
			cfg.Secrets = map[string]profile.Secret{}
		}
		cfg.Secrets[name] = profile.Secret{Env: env}
		return nil
	}
	parts := splitPath(key)
	if len(parts) != 2 {
		return fmt.Errorf("malformed secret field row %q", key)
	}
	s := cfg.Secrets[parts[0]]
	switch parts[1] {
	case "env":
		if input == "" {
			return errors.New("env must not be empty")
		}
		s.Env = input
	case "description":
		s.Description = input
	case "allow_empty":
		v, err := parseBoolInput(input)
		if err != nil {
			return err
		}
		s.AllowEmpty = v
	default:
		return fmt.Errorf("unknown secret field %q", parts[1])
	}
	cfg.Secrets[parts[0]] = s
	return nil
}

// mutateMounts: entries add bare (the required fields grow through their
// rows); source/destination/type refuse to empty, options is a comma
// list, state is ""/enabled/disabled.
func mutateMounts(cfg *profile.ModuleConfig, key, input string, add bool) error {
	if add {
		if cfg.Mounts == nil {
			cfg.Mounts = map[string]profile.MountSpec{}
		}
		cfg.Mounts[strings.TrimSpace(input)] = profile.MountSpec{}
		return nil
	}
	parts := splitPath(key)
	if len(parts) != 2 {
		return fmt.Errorf("malformed mount field row %q", key)
	}
	m := cfg.Mounts[parts[0]]
	switch parts[1] {
	case "source":
		if input == "" {
			return errors.New("source is required")
		}
		m.Source = input
	case "destination":
		if input == "" {
			return errors.New("destination is required")
		}
		m.Destination = input
	case "type":
		if input == "" {
			return errors.New("type is required")
		}
		m.Type = input
	case "options":
		m.Options = splitComma(input)
	case "startat":
		m.StartAt = input
	case "state":
		switch input {
		case "", "enabled", "disabled":
			m.State = input
		default:
			return errors.New(`state must be "", "enabled", or "disabled"`)
		}
	default:
		return fmt.Errorf("unknown mount field %q", parts[1])
	}
	cfg.Mounts[parts[0]] = m
	return nil
}

// mutateSmb: shares add bare; the [smb] scalars edit as single-segment
// keys (group free text, users a comma list, avahi tri-state), share
// fields as entry-scoped ones with path refusing to empty.
func mutateSmb(cfg *profile.ModuleConfig, key, input string, add bool) error {
	if add {
		if cfg.Smb.Shares == nil {
			cfg.Smb.Shares = map[string]profile.ShareSpec{}
		}
		cfg.Smb.Shares[strings.TrimSpace(input)] = profile.ShareSpec{}
		return nil
	}
	parts := splitPath(key)
	switch len(parts) {
	case 1:
		switch parts[0] {
		case "group":
			cfg.Smb.Group = input
		case "users":
			cfg.Smb.Users = splitComma(input)
		case "avahi":
			v, set, err := parseAvahiInput(input)
			if err != nil {
				return err
			}
			if set {
				cfg.Smb.Avahi = &v
			} else {
				cfg.Smb.Avahi = nil
			}
		default:
			return fmt.Errorf("unknown smb field %q", parts[0])
		}
	case 2:
		sh := cfg.Smb.Shares[parts[0]]
		switch parts[1] {
		case "path":
			if input == "" {
				return errors.New("path is required")
			}
			sh.Path = input
		case "comment":
			sh.Comment = input
		case "valid_users":
			sh.ValidUsers = input
		case "writable":
			v, err := parseBoolInput(input)
			if err != nil {
				return err
			}
			sh.Writable = v
		case "public":
			v, err := parseBoolInput(input)
			if err != nil {
				return err
			}
			sh.Public = v
		default:
			return fmt.Errorf("unknown share field %q", parts[1])
		}
		cfg.Smb.Shares[parts[0]] = sh
	default:
		return fmt.Errorf("malformed smb row %q", key)
	}
	return nil
}
