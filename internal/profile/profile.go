// Package profile loads and selects modules from a profile.
package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

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
	ID          string               `toml:"id"`
	App         string               `toml:"app"`
	Description string               `toml:"description"`
	Scope       string               `toml:"scope"`
	When        When                 `toml:"when"`
	Packages    Packages             `toml:"packages"`
	Tools       map[string]string    `toml:"tools"`
	Dotfiles    map[string]Dotfile   `toml:"dotfiles"`
	Hooks       Hooks                `toml:"hooks"`
	Mounts      map[string]MountSpec `toml:"mounts"`
	Smb         SmbSpec              `toml:"smb"`
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

// When filters a module by host, user, os, gpu, or kernel.
// Empty fields are ignored; non-empty fields must all match.
// Kernel holds one "<op> <version>" constraint ("<", "<=", ">", ">=",
// "==", "!=") compared numerically per dotted segment against the running
// kernel release; an empty kernel fact never matches a non-empty
// constraint.
type When struct {
	Hosts  []string `toml:"hosts"`
	Users  []string `toml:"users"`
	OS     []string `toml:"os"`
	GPU    string   `toml:"gpu"`
	Kernel string   `toml:"kernel"`
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
	Pre  []HookCommand `toml:"pre"`
	Post []HookCommand `toml:"post"`
}

// HookCommand is a single pre/post hook shell command. It runs from the
// profile root with the DOTDRIFT_* facts in the environment. Optional = true
// makes a non-zero exit non-fatal: the failure is logged at warn and the
// apply step continues, so a flaky/best-effort hook cannot abort the run.
//
// Two TOML spellings decode into the same value: the legacy string array
// (`pre = ["echo hi"]`, all required) and the structured table array
// (`[[hooks.pre]] command = "..." optional = true`). UnmarshalText makes the
// string form work without a custom Hooks decoder.
type HookCommand struct {
	Command  string `toml:"command"`
	Optional bool   `toml:"optional"`
}

// UnmarshalTOML accepts both spellings. The legacy form feeds each array
// element as a string; the structured form feeds a map with command/optional.
func (h *HookCommand) UnmarshalTOML(v any) error {
	switch val := v.(type) {
	case string:
		h.Command = val
	case map[string]any:
		if cmd, ok := val["command"].(string); ok {
			h.Command = cmd
		}
		if opt, ok := val["optional"].(bool); ok {
			h.Optional = opt
		}
	default:
		return fmt.Errorf("hook entry must be a command string or a table with command/optional")
	}
	if h.Command == "" {
		return fmt.Errorf("hook entry missing command")
	}
	return nil
}

// Dotfile describes a single managed path. Whole-file entries use
// Source+Mode (symlink, symlink-each, copy, template). Edit entries are
// partial edits to a file something else owns, keyed by "<file-path>/<edit-id>"
// in the [dotfiles] map: a `line` ensures an exact line exists, a `block`
// wraps content in mise marker delimiters, and a `source`+`template` renders
// a block via an engine (e.g. "tera"). The syntax mirrors mise's edit-entry
// vocabulary exactly (see https://mise.jdx.dev/dotfiles.html, "Edit entries").
type Dotfile struct {
	Source   string `toml:"source"`
	Mode     string `toml:"mode"`
	Line     string `toml:"line"`     // edit: ensure exact line exists
	Block    string `toml:"block"`    // edit: marker-delimited block
	Comment  string `toml:"comment"`  // edit: comment prefix for a block (mise default: #)
	Template string `toml:"template"` // edit: engine name (e.g. "tera"); requires source
}

// IsEdit reports whether the entry is a partial edit (line/block/template, or
// mode = "edit" with a source file) rather than a whole-file entry. An empty
// line/block/template and a mode other than "edit" means the entry is
// whole-file (an ensure-empty-line edit is nonsense; mise is the backstop).
func (d Dotfile) IsEdit() bool {
	return d.Line != "" || d.Block != "" || d.Template != "" || d.Mode == "edit"
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
	if err := p.discover(root, f); err != nil {
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
	id := cfg.ID
	if id == "" {
		id = filepath.Base(dir)
	}
	if err := validateWhenKernel(id, cfg.When.Kernel); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// validateWhenKernel rejects a malformed when.kernel constraint at load
// time, naming the module — a typo must fail loudly, never silently skip
// the module.
func validateWhenKernel(id, expr string) error {
	if expr == "" {
		return nil
	}
	if err := facts.CheckKernelConstraint(expr); err != nil {
		return fmt.Errorf("module %s: %w", id, err)
	}
	return nil
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
	if w.Kernel != "" {
		// Validated at load (LoadModuleConfig), so the expression is
		// well-formed here; a compare error means an unparseable running
		// release, which never matches.
		fields := strings.Fields(w.Kernel)
		if len(fields) != 2 {
			return "when filter", true
		}
		if ok, err := facts.CompareKernel(f.Kernel, fields[0], fields[1]); err != nil || !ok {
			return "when filter", true
		}
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
