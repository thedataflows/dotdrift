// Package profile loads and selects modules from a profile.
package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/BurntSushi/toml"
	"github.com/thedataflows/dotdrift/internal/facts"
)

// Config is the top-level dotdrift.toml configuration.
type Config struct {
	Modules ModulesConfig `toml:"modules"`
}

// ModulesConfig holds the [modules] table.
type ModulesConfig struct {
	Disable []string `toml:"disable"`
}

// Scope values for ModuleConfig.Scope. Scope is module-level: it decides
// whether the module's dotfiles are applied as the invoking user (into ~/)
// or with root privileges (into /etc and other system paths).
const (
	ScopeUser   = "user"
	ScopeSystem = "system"
)

// ModuleConfig is the base module.toml configuration.
type ModuleConfig struct {
	ID       string             `toml:"id"`
	App      string             `toml:"app"`
	Scope    string             `toml:"scope"`
	When     When               `toml:"when"`
	Packages Packages           `toml:"packages"`
	Tools    map[string]string  `toml:"tools"`
	Dotfiles map[string]Dotfile `toml:"dotfiles"`
	Hooks    Hooks              `toml:"hooks"`
}

// ScopeOrDefault returns the module's dotfile scope, defaulting to user when
// the key is omitted. Validity is not checked here — resolve rejects unknown
// values loudly.
func (c ModuleConfig) ScopeOrDefault() string {
	if c.Scope == "" {
		return ScopeUser
	}
	return c.Scope
}

// When filters a module by host, user, os, or gpu.
// Empty fields are ignored; non-empty fields must all match.
type When struct {
	Hosts []string `toml:"hosts"`
	Users []string `toml:"users"`
	OS    []string `toml:"os"`
	GPU   string   `toml:"gpu"`
}

// Packages declares packages a module needs or forbids.
type Packages struct {
	Present []string `toml:"present"`
	Absent  []string `toml:"absent"`
}

// Hooks declares pre/post apply shell commands for a module. Unlike
// packages/tools/dotfiles, hooks are ordered sequences: layers merge by
// appending base → host → user (see internal/resolve).
type Hooks struct {
	Pre  []string `toml:"pre"`
	Post []string `toml:"post"`
}

// Dotfile describes a single managed path.
type Dotfile struct {
	Source string `toml:"source"`
	Mode   string `toml:"mode"`
}

// Module is a discovered module with its resolved identity and path.
type Module struct {
	ID     string
	App    string
	Path   string
	Config ModuleConfig
}

// Skip records a module that was not selected and why.
type Skip struct {
	Module Module
	Reason string
}

// Profile is the loaded set of modules and selection state.
type Profile struct {
	Root     string
	Config   Config
	Modules  []Module
	Selected []Module
	Skipped  []Skip
}

// Load reads a profile directory, unions dotdrift.toml layers, discovers
// modules, and runs selection against the provided facts.
func Load(root string, f *facts.Facts) (*Profile, error) {
	if f == nil {
		f = &facts.Facts{}
	}
	p := &Profile{Root: root}
	if err := p.loadConfig(root, f); err != nil {
		return nil, err
	}
	if err := p.discover(root); err != nil {
		return nil, err
	}
	p.Select(f)
	return p, nil
}

// LoadModuleConfig reads a module.toml from the given directory.
// It returns the parsed config and the resolved module path, or nil if no module.toml exists.
func LoadModuleConfig(dir string) (*ModuleConfig, error) {
	modToml := filepath.Join(dir, "module.toml")
	if _, err := os.Stat(modToml); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var cfg ModuleConfig
	if _, err := toml.DecodeFile(modToml, &cfg); err != nil {
		return nil, fmt.Errorf("decode %s: %w", modToml, err)
	}
	return &cfg, nil
}

// ModuleDir returns the path to a module directory under the given profile root.
func ModuleDir(root, layer, id string) string {
	if layer == "" || layer == "module" {
		return filepath.Join(root, "modules", id)
	}
	return filepath.Join(root, layer, id, "modules", id)
}

// Select re-evaluates which modules are selected or skipped.
func (p *Profile) Select(f *facts.Facts) {
	if f == nil {
		f = &facts.Facts{}
	}
	p.Selected = nil
	p.Skipped = nil
	for _, m := range p.Modules {
		if reason, disabled := p.isDisabled(m); disabled {
			p.Skipped = append(p.Skipped, Skip{Module: m, Reason: reason})
			continue
		}
		if reason, failed := m.Config.When.matches(f); failed {
			p.Skipped = append(p.Skipped, Skip{Module: m, Reason: reason})
			continue
		}
		p.Selected = append(p.Selected, m)
	}
}

func (p *Profile) loadConfig(root string, f *facts.Facts) error {
	base, err := loadDotdriftTOML(filepath.Join(root, "dotdrift.toml"))
	if err != nil {
		return err
	}
	host, err := loadOverlay(filepath.Join(root, "hosts", f.Hostname, "dotdrift.toml"), f.Hostname, "hostname")
	if err != nil {
		return err
	}
	user, err := loadOverlay(filepath.Join(root, "users", f.Username, "dotdrift.toml"), f.Username, "username")
	if err != nil {
		return err
	}
	p.Config = unionConfig(base, host, user)
	return nil
}

// loadOverlay loads a host/user dotdrift.toml layer. An empty fact value
// collapses the overlay path onto the parent directory (e.g.
// hosts/dotdrift.toml); if a file exists at that collapsed path it would be
// silently merged into every configuration, so refuse it loudly. When no file
// exists at the collapsed path the overlay is simply absent.
func loadOverlay(path, value, name string) (Config, error) {
	var cfg Config
	if value != "" {
		return loadDotdriftTOML(path)
	}
	if _, err := os.Stat(path); err == nil {
		return cfg, fmt.Errorf("empty %s: refusing to load collapsed overlay %s", name, path)
	} else if !os.IsNotExist(err) {
		return cfg, err
	}
	return cfg, nil
}

func loadDotdriftTOML(path string) (Config, error) {
	var cfg Config
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return cfg, fmt.Errorf("decode %s: %w", path, err)
	}
	return cfg, nil
}

func unionConfig(base, host, user Config) Config {
	disable := make(map[string]struct{})
	for _, id := range base.Modules.Disable {
		disable[id] = struct{}{}
	}
	for _, id := range host.Modules.Disable {
		disable[id] = struct{}{}
	}
	for _, id := range user.Modules.Disable {
		disable[id] = struct{}{}
	}
	list := make([]string, 0, len(disable))
	for id := range disable {
		list = append(list, id)
	}
	sort.Strings(list)
	return Config{Modules: ModulesConfig{Disable: list}}
}

func (p *Profile) discover(root string) error {
	modulesDir := filepath.Join(root, "modules")
	entries, err := os.ReadDir(modulesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("not a dotdrift profile: %s missing modules/ directory", root)
		}
		return err
	}
	seen := make(map[string]string)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		modPath := filepath.Join(modulesDir, entry.Name())
		mod, err := loadModule(modPath, entry.Name())
		if err != nil {
			return err
		}
		if mod == nil {
			continue
		}
		if prev, dup := seen[mod.ID]; dup {
			return fmt.Errorf("duplicate module id %q: %s and %s", mod.ID, prev, modPath)
		}
		seen[mod.ID] = modPath
		p.Modules = append(p.Modules, *mod)
	}
	sort.Slice(p.Modules, func(i, j int) bool { return p.Modules[i].ID < p.Modules[j].ID })
	return nil
}

func loadModule(path, dirName string) (*Module, error) {
	modToml := filepath.Join(path, "module.toml")
	if _, err := os.Stat(modToml); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var cfg ModuleConfig
	if _, err := toml.DecodeFile(modToml, &cfg); err != nil {
		return nil, fmt.Errorf("decode %s: %w", modToml, err)
	}
	if cfg.ID == "" {
		cfg.ID = dirName
	}
	if cfg.App == "" {
		cfg.App = cfg.ID
	}
	return &Module{ID: cfg.ID, App: cfg.App, Path: path, Config: cfg}, nil
}

func (p *Profile) isDisabled(m Module) (string, bool) {
	for _, id := range p.Config.Modules.Disable {
		if id == m.ID {
			return "disabled", true
		}
	}
	return "", false
}

func (w When) matches(f *facts.Facts) (string, bool) {
	if len(w.Hosts) > 0 && !contains(w.Hosts, f.Hostname) {
		return "when filter", true
	}
	if len(w.Users) > 0 && !contains(w.Users, f.Username) {
		return "when filter", true
	}
	if len(w.OS) > 0 && !contains(w.OS, f.OS) {
		return "when filter", true
	}
	if w.GPU != "" && w.GPU != f.GPU {
		return "when filter", true
	}
	return "", false
}

func contains(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}
