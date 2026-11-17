package resolve

import (
	"fmt"
	"sort"

	"github.com/thedataflows/dotdrift/internal/profile"
)

// ServicesStep lists the services to converge (issue 0159).
type ServicesStep struct {
	Entries []ServiceEntry
}

// ServiceEntry is one resolved service: the spec after layer merge and
// defaulting, the module that declares it, and the service scope, which
// is the module scope.
type ServiceEntry struct {
	Name   string
	Spec   profile.ServiceSpec
	Module string
	Layer  string
	Scope  string // profile.ScopeUser | profile.ScopeSystem
}

// mergeServices unions service declarations whole-entry by name (nearer
// layer wins), validates structure, and applies defaults: state
// "running", enabled true. The module scope is the service scope.
// command defines a program and is user scope only; restart,
// environment, and working_directory configure a definition, so they
// require command (mise rejects them on an existing-unit entry).
func mergeServices(base, host, user layerConfig, moduleID, scope string) ([]ServiceEntry, error) {
	type winner struct {
		layer string
		spec  profile.ServiceSpec
	}
	winners := make(map[string]winner)
	for name, s := range base.cfg.Services {
		winners[name] = winner{layer: "base", spec: s}
	}
	for name, s := range host.cfg.Services {
		winners[name] = winner{layer: "host", spec: s}
	}
	for name, s := range user.cfg.Services {
		winners[name] = winner{layer: "user", spec: s}
	}
	if len(winners) == 0 {
		return nil, nil
	}
	names := make([]string, 0, len(winners))
	for n := range winners {
		names = append(names, n)
	}
	sort.Strings(names)
	entries := make([]ServiceEntry, 0, len(names))
	for _, name := range names {
		w := winners[name]
		spec := w.spec
		if err := validateService(moduleID, name, spec, scope); err != nil {
			return nil, err
		}
		if spec.State == "" {
			spec.State = "running"
		}
		if spec.Enabled == nil {
			enabled := true
			spec.Enabled = &enabled
		}
		entries = append(entries, ServiceEntry{
			Name: name, Spec: spec, Module: moduleID, Layer: w.layer, Scope: scope,
		})
	}
	return entries, nil
}

func validateService(moduleID, name string, s profile.ServiceSpec, scope string) error {
	if name == "" {
		return fmt.Errorf("module %s: service name must not be empty", moduleID)
	}
	for _, r := range name {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') &&
			r != '.' && r != '_' && r != '-' {
			return fmt.Errorf("module %s: service name %q must contain only letters, numbers, '.', '_', or '-'", moduleID, name)
		}
	}
	switch s.State {
	case "", "running", "stopped", "absent":
	default:
		return fmt.Errorf("module %s: service %q: unknown state %q (valid: running, stopped, absent)", moduleID, name, s.State)
	}
	switch s.Restart {
	case "", "on-failure", "always", "never":
	default:
		return fmt.Errorf("module %s: service %q: unknown restart %q (valid: on-failure, always, never)", moduleID, name, s.Restart)
	}
	if s.Command == "" {
		if s.Restart != "" || len(s.Environment) > 0 || s.WorkingDirectory != "" {
			return fmt.Errorf("module %s: service %q: restart, environment, and working_directory require a command (they configure a service definition; without command the entry enables an existing unit)", moduleID, name)
		}
		return nil
	}
	if scope != profile.ScopeUser {
		return fmt.Errorf("module %s: service %q: command requires scope = \"user\" (got %q): mise system services cannot define a program", moduleID, name, scope)
	}
	return nil
}

// checkServiceSystemdConflicts rejects a name declared as both a
// service and a systemd unit anywhere in the plan: both would write
// the same unit definition (mise services reference, issue 0159).
func checkServiceSystemdConflicts(services []ServiceEntry, units []SystemdUnitEntry) error {
	unitNames := make(map[string]string, len(units))
	for _, u := range units {
		unitNames[u.Name] = u.Module
	}
	for _, e := range services {
		if mod, ok := unitNames[e.Name]; ok {
			return fmt.Errorf("service %q (module %s) is also declared as a systemd unit (module %s): declare it in one place", e.Name, e.Module, mod)
		}
	}
	return nil
}

func sortServiceEntries(entries []ServiceEntry) {
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
}
