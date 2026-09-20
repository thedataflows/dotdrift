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
	"strconv"
	"strings"

	"github.com/thedataflows/dotdrift/internal/profile"
)

// mutateStructural applies a committed input to a structural family.
// addPath scopes adds ("" adds the section's entry itself); for edits it
// is empty and key carries the row's machine path. Returns the committed
// row's NEW key for adds ("" for edits — a structural edit never renames).
func mutateStructural(cfg *profile.ModuleConfig, family, addPath, key, input string, add bool) (string, error) {
	switch family {
	case profile.FamilySystemd:
		return mutateSystemd(cfg, addPath, key, input, add)
	case profile.FamilySecrets, profile.FamilyMounts, profile.FamilySmb:
		var err error
		switch family {
		case profile.FamilySecrets:
			err = mutateSecrets(cfg, key, input, add)
		case profile.FamilyMounts:
			err = mutateMounts(cfg, key, input, add)
		case profile.FamilySmb:
			err = mutateSmb(cfg, key, input, add)
		}
		if err != nil {
			return "", err
		}
		if add {
			return structuralEntryKey(family, input), nil
		}
		return "", nil
	case profile.FamilyWhen:
		return mutateWhen(cfg, addPath, key, input, add)
	}
	return "", fmt.Errorf("family %q has no structural editor", family)
}

// structuralEntryKey predicts an entry add's row key from the input
// (secrets `name = ENV`, mounts/shares a bare name).
func structuralEntryKey(family, input string) string {
	if family == profile.FamilySecrets {
		name, _, _ := strings.Cut(input, "=")
		return strings.TrimSpace(name)
	}
	return strings.TrimSpace(input)
}

// mutateSystemd edits the systemd passthrough: units add whole (name
// only, directives follow), directives add as `Name = value` and edit
// value-only (parsed as a TOML value, plain-string fallback). An empty
// value refuses: the encoder drops empty values, so committing one would
// silently delete the directive.
func mutateSystemd(cfg *profile.ModuleConfig, addPath, key, input string, add bool) (string, error) {
	if add {
		if addPath == "" {
			if cfg.Systemd.Units == nil {
				cfg.Systemd.Units = map[string]profile.SystemdUnit{}
			}
			name := strings.TrimSpace(input)
			cfg.Systemd.Units[name] = profile.SystemdUnit{}
			return name, nil
		}
		name, value, ok := strings.Cut(input, "=")
		name = strings.TrimSpace(name)
		if !ok || name == "" {
			return "", errors.New(`add as "Name = value"`)
		}
		unit := cfg.Systemd.Units[addPath]
		if unit == nil {
			unit = profile.SystemdUnit{}
		}
		unit[name] = parseTomlValue(value)
		cfg.Systemd.Units[addPath] = unit
		return pathKey(addPath, name), nil
	}

	parts := splitPath(key)
	if len(parts) != 2 {
		return "", fmt.Errorf("malformed directive row %q", key)
	}
	if input == "" {
		return "", errors.New("directive value must not be empty (d removes the directive)")
	}
	unit := cfg.Systemd.Units[parts[0]]
	if unit == nil {
		return "", fmt.Errorf("unknown unit %q", parts[0])
	}
	unit[parts[1]] = parseTomlValue(input)
	cfg.Systemd.Units[parts[0]] = unit
	return "", nil
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

// mutateWhen edits the when tree: leaves set/clear at any depth (the
// root's plain single-segment keys included), `a` grows an and/or list
// beneath the cursor row's group or sets its not. Returns the new group
// row's key for adds.
func mutateWhen(cfg *profile.ModuleConfig, addPath, key, input string, add bool) (string, error) {
	if add {
		var segs []string
		if addPath != "" {
			segs = splitPath(addPath)
		}
		parent := whenNode(&cfg.When, segs)
		if parent == nil {
			return "", fmt.Errorf("stale when row %q", addPath)
		}
		switch strings.TrimSpace(input) {
		case "and":
			parent.And = append(parent.And, profile.When{})
			return pathKey(append(segs, "and", strconv.Itoa(len(parent.And)-1))...), nil
		case "or":
			parent.Or = append(parent.Or, profile.When{})
			return pathKey(append(segs, "or", strconv.Itoa(len(parent.Or)-1))...), nil
		case "not":
			if parent.Not != nil {
				return "", errors.New("not is already set here")
			}
			parent.Not = &profile.When{}
			return pathKey(append(segs, "not")...), nil
		default:
			return "", errors.New(`add "and", "or", or "not"`)
		}
	}

	segs := splitPath(key)
	field := segs[len(segs)-1]
	var group []string
	if len(segs) > 1 {
		group = segs[:len(segs)-1]
	}
	node := whenNode(&cfg.When, group)
	if node == nil {
		return "", fmt.Errorf("stale when row %q", key)
	}
	switch field {
	case "gpu":
		node.GPU = input
	case "kernel":
		node.Kernel = input
	case "hosts":
		node.Hosts = splitComma(input)
	case "users":
		node.Users = splitComma(input)
	case "os":
		node.OS = splitComma(input)
	case "packages":
		node.Packages = splitComma(input)
	case "tools":
		node.Tools = splitComma(input)
	default:
		return "", fmt.Errorf("unknown when field %q", field)
	}
	return "", nil
}

// whenNode walks a group path (and/or index pairs, not as a singleton)
// from a node, returning the addressed When — nil when the path is stale
// (the rows were rebuilt since).
func whenNode(root *profile.When, segs []string) *profile.When {
	node := root
	for i := 0; i < len(segs); i++ {
		switch segs[i] {
		case "and", "or":
			if i+1 >= len(segs) {
				return nil
			}
			idx, err := strconv.Atoi(segs[i+1])
			if err != nil {
				return nil
			}
			if segs[i] == "and" {
				if idx >= len(node.And) {
					return nil
				}
				node = &node.And[idx]
			} else {
				if idx >= len(node.Or) {
					return nil
				}
				node = &node.Or[idx]
			}
			i++
		case "not":
			if node.Not == nil {
				return nil
			}
			node = node.Not
		default:
			return nil
		}
	}
	return node
}

// checkWhenGroups refuses empty groups — a node with nothing to evaluate
// is a load-time error (resolve refuses the plan); the root itself is
// exempt, an all-empty root is simply no when section.
func checkWhenGroups(prefix string, w profile.When) string {
	for i, sub := range w.And {
		p := fmt.Sprintf("%sand[%d]", prefix, i)
		if !whenHasContent(sub) {
			return fmt.Sprintf("when: %s: empty group (nothing to evaluate)", p)
		}
		if e := checkWhenGroups(p+".", sub); e != "" {
			return e
		}
	}
	for i, sub := range w.Or {
		p := fmt.Sprintf("%sor[%d]", prefix, i)
		if !whenHasContent(sub) {
			return fmt.Sprintf("when: %s: empty group (nothing to evaluate)", p)
		}
		if e := checkWhenGroups(p+".", sub); e != "" {
			return e
		}
	}
	if w.Not != nil {
		if !whenHasContent(*w.Not) {
			return fmt.Sprintf("when: %snot: empty group (nothing to evaluate)", prefix)
		}
		if e := checkWhenGroups(prefix+"not.", *w.Not); e != "" {
			return e
		}
	}
	return ""
}

// whenHasContent reports whether a when node has anything to evaluate.
func whenHasContent(w profile.When) bool {
	return len(w.Hosts) > 0 || len(w.Users) > 0 || len(w.OS) > 0 ||
		w.GPU != "" || w.Kernel != "" ||
		len(w.Packages) > 0 || len(w.Tools) > 0 ||
		len(w.And) > 0 || len(w.Or) > 0 || w.Not != nil
}
