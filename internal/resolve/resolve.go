// Package resolve merges profile layers into an execution plan.
package resolve

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/profile"
)

// Plan is the resolved, side-effect-free execution plan for a profile.
type Plan struct {
	Packages PackagesStep
	Tools    ToolsStep
	Dotfiles DotfilesStep
	Hooks    HooksStep
	Mounts   MountsStep
	Smb      SmbStep
}

// PackagesStep lists packages that should be present or removed from the system.
type PackagesStep struct {
	Install []string
	Remove  []string
}

// ToolsStep lists required tool versions.
type ToolsStep struct {
	Versions map[string]string
}

// DotfilesStep lists managed dotfile entries.
type DotfilesStep struct {
	Entries []DotfileEntry
}

// DotfileEntry is a single resolved dotfile target.
type DotfileEntry struct {
	Target string
	Source string
	Mode   string
	Module string
	Layer  string
	Scope  string
}

// HooksStep lists the pre/post apply hook commands aggregated across all
// selected modules. Hooks are ordered sequences: per module the layers
// append base → host → user, and modules aggregate in selection order.
type HooksStep struct {
	Pre  []string
	Post []string
}

type layerConfig struct {
	name string
	path string
	cfg  profile.ModuleConfig
}

type dotfileWinner struct {
	layer string
	path  string
	df    profile.Dotfile
}

// Resolve builds a Plan by merging base, host, and user overlays for each
// selected module. Precedence is user > host > base.
func Resolve(p *profile.Profile, f *facts.Facts) (*Plan, error) {
	if p == nil {
		return nil, fmt.Errorf("profile is nil")
	}
	if f == nil {
		f = &facts.Facts{}
	}

	plan := &Plan{
		Packages: PackagesStep{},
		Tools:    ToolsStep{Versions: make(map[string]string)},
		Dotfiles: DotfilesStep{},
	}

	if len(p.Selected) > 0 {
		if f.Hostname == "" {
			return nil, fmt.Errorf("resolve: hostname fact is empty (required to locate host overlays)")
		}
		if f.Username == "" {
			return nil, fmt.Errorf("resolve: username fact is empty (required to locate user overlays)")
		}
	}

	pkgSet := make(map[string]struct{})
	presentIn := make(map[string][]string)
	absentIn := make(map[string][]string)
	for _, m := range p.Selected {
		// Layer paths derive from the profile root and the module directory
		// name, never from m.Path: for an overlay-only module (ADR-0001) the
		// representative path points into hosts/<h>/modules/<dir> or
		// users/<u>/modules/<dir>, so going up two levels would not reach the
		// profile root. Overlays key on the directory name, not the declared
		// id. An absent layer is an empty layerConfig: loadModuleConfig
		// returns a zero config for a missing module.toml.
		dir := filepath.Base(m.Path)

		// Scope comes from the representative config m.Config (base-preferred
		// by discovery order); overlay scope declarations are ignored. Scope
		// is module-level and validated for every selected module — an
		// unknown value must never pass silently, even for a module with no
		// dotfiles.
		scope := m.Config.ScopeOrDefault()
		if scope != profile.ScopeUser && scope != profile.ScopeSystem {
			return nil, fmt.Errorf("module %s: unknown scope %q (valid: user, system)", m.ID, m.Config.Scope)
		}

		basePath := filepath.Join(p.Root, "modules", dir)
		baseCfg, err := loadModuleConfig(basePath)
		if err != nil {
			return nil, fmt.Errorf("module %s: base layer %s: %w", m.ID, basePath, err)
		}
		base := layerConfig{name: "base", path: basePath, cfg: baseCfg}
		hostPath := filepath.Join(p.Root, "hosts", f.Hostname, "modules", dir)
		hostCfg, err := loadModuleConfig(hostPath)
		if err != nil {
			return nil, fmt.Errorf("module %s: host overlay %s: %w", m.ID, hostPath, err)
		}
		host := layerConfig{name: "host", path: hostPath, cfg: hostCfg}
		userPath := filepath.Join(p.Root, "users", f.Username, "modules", dir)
		userCfg, err := loadModuleConfig(userPath)
		if err != nil {
			return nil, fmt.Errorf("module %s: user overlay %s: %w", m.ID, userPath, err)
		}
		user := layerConfig{name: "user", path: userPath, cfg: userCfg}

		install, remove := mergePackages(base.cfg.Packages, host.cfg.Packages, user.cfg.Packages)
		for _, pkg := range install {
			pkgSet[pkg] = struct{}{}
			presentIn[pkg] = append(presentIn[pkg], m.ID)
		}
		for _, pkg := range remove {
			absentIn[pkg] = append(absentIn[pkg], m.ID)
		}
		plan.Packages.Remove = append(plan.Packages.Remove, remove...)
		sort.Strings(plan.Packages.Remove)

		for k, v := range mergeTools(base.cfg.Tools, host.cfg.Tools, user.cfg.Tools) {
			plan.Tools.Versions[k] = v
		}

		entries, err := mergeDotfiles(base, host, user, scope)
		if err != nil {
			return nil, err
		}
		plan.Dotfiles.Entries = append(plan.Dotfiles.Entries, entries...)

		plan.Hooks.Pre = append(plan.Hooks.Pre, base.cfg.Hooks.Pre...)
		plan.Hooks.Pre = append(plan.Hooks.Pre, host.cfg.Hooks.Pre...)
		plan.Hooks.Pre = append(plan.Hooks.Pre, user.cfg.Hooks.Pre...)
		plan.Hooks.Post = append(plan.Hooks.Post, base.cfg.Hooks.Post...)
		plan.Hooks.Post = append(plan.Hooks.Post, host.cfg.Hooks.Post...)
		plan.Hooks.Post = append(plan.Hooks.Post, user.cfg.Hooks.Post...)

		// Mounts and smb artifacts land in root-owned paths (systemd unit
		// dirs, /etc/samba), so they require system scope; a user-scope
		// module declaring them fails here rather than inside mise.
		if scope != profile.ScopeSystem && (len(base.cfg.Mounts)+len(host.cfg.Mounts)+len(user.cfg.Mounts) > 0 ||
			hasSmbContent(base.cfg.Smb) || hasSmbContent(host.cfg.Smb) || hasSmbContent(user.cfg.Smb)) {
			return nil, fmt.Errorf("module %s: mounts/smb require scope = \"system\" (got %q)", m.ID, scope)
		}

		mounts, err := mergeMounts(base, host, user, dir, scope)
		if err != nil {
			return nil, err
		}
		plan.Mounts.Entries = append(plan.Mounts.Entries, mounts...)

		smb, contributed, err := mergeSmb(base, host, user, dir)
		if err != nil {
			return nil, err
		}
		if contributed {
			plan.Smb.Modules = append(plan.Smb.Modules, SmbModuleSpec{Module: dir, Spec: smb})
		}
	}

	if err := checkPackageConflicts(presentIn, absentIn); err != nil {
		return nil, err
	}

	if err := checkDotfileConflicts(plan.Dotfiles.Entries); err != nil {
		return nil, err
	}

	for pkg := range pkgSet {
		plan.Packages.Install = append(plan.Packages.Install, pkg)
	}
	sort.Strings(plan.Packages.Install)
	sortEntries(plan.Dotfiles.Entries)
	sortMountEntries(plan.Mounts.Entries)
	sortSmbModules(plan.Smb.Modules)

	return plan, nil
}

// Fingerprint returns a stable, human-readable string that identifies the
// current selection and the facts that produced it.
func Fingerprint(p *profile.Profile, f *facts.Facts) string {
	if p == nil {
		return ""
	}
	if f == nil {
		f = &facts.Facts{}
	}

	var b strings.Builder

	ids := make([]string, len(p.Selected))
	for i, m := range p.Selected {
		ids[i] = m.ID
	}
	sort.Strings(ids)
	fmt.Fprintf(&b, "selected=%s\n", strings.Join(ids, ","))

	disable := append([]string{}, p.Config.Modules.Disable...)
	sort.Strings(disable)
	fmt.Fprintf(&b, "disable=%s\n", strings.Join(disable, ","))

	fmt.Fprintf(&b, "hostname=%s\n", f.Hostname)
	fmt.Fprintf(&b, "username=%s\n", f.Username)
	fmt.Fprintf(&b, "os=%s\n", f.OS)
	fmt.Fprintf(&b, "gpu=%s\n", f.GPU)
	fmt.Fprintf(&b, "backend=%s\n", f.Backend)

	return b.String()
}

func loadModuleConfig(modulePath string) (profile.ModuleConfig, error) {
	var cfg profile.ModuleConfig
	path := filepath.Join(modulePath, "module.toml")
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

func mergePackages(base, host, user profile.Packages) (present []string, absent []string) {
	// pkgState records the final declaration for each package: true = present, false = absent.
	// Higher layers win because they are applied last.
	pkgState := make(map[string]bool)
	for _, p := range []profile.Packages{base, host, user} {
		for _, pkg := range p.Present {
			pkgState[pkg] = true
		}
		for _, pkg := range p.Absent {
			pkgState[pkg] = false
		}
	}

	for pkg, isPresent := range pkgState {
		if isPresent {
			present = append(present, pkg)
		} else {
			absent = append(absent, pkg)
		}
	}
	sort.Strings(present)
	sort.Strings(absent)
	return present, absent
}

func mergeTools(base, host, user map[string]string) map[string]string {
	result := make(map[string]string)
	for k, v := range base {
		result[k] = v
	}
	for k, v := range host {
		result[k] = v
	}
	for k, v := range user {
		result[k] = v
	}
	return result
}

// validDotfileModes is dotdrift's dotfile mode vocabulary (see
// docs/product/profile-layout.md) — exactly mise's mode vocabulary, so modes
// pass through to the generated mise.toml unchanged. Unknown or empty modes
// are resolve-time errors: mise silently ignores entries whose mode it does
// not recognize (exit 0, no file created), so anything outside this set would
// no-op without a loud failure.
var validDotfileModes = map[string]bool{
	"symlink":      true,
	"symlink-each": true,
	"copy":         true,
	"template":     true,
}

func mergeDotfiles(base, host, user layerConfig, scope string) ([]DotfileEntry, error) {
	winners := make(map[string]dotfileWinner)
	for target, df := range base.cfg.Dotfiles {
		winners[target] = dotfileWinner{layer: "base", path: base.path, df: df}
	}
	for target, df := range host.cfg.Dotfiles {
		winners[target] = dotfileWinner{layer: "host", path: host.path, df: df}
	}
	for target, df := range user.cfg.Dotfiles {
		winners[target] = dotfileWinner{layer: "user", path: user.path, df: df}
	}

	moduleID := filepath.Base(base.path)
	entries := make([]DotfileEntry, 0, len(winners))
	for target, winner := range winners {
		if !validDotfileModes[winner.df.Mode] {
			return nil, fmt.Errorf("module %s: dotfile %q: unknown mode %q (valid: symlink, symlink-each, copy, template)", moduleID, target, winner.df.Mode)
		}
		source, err := resolveSource(winner, moduleID, base, host, user)
		if err != nil {
			return nil, err
		}
		entries = append(entries, DotfileEntry{
			Target: target,
			Source: source,
			Mode:   winner.df.Mode,
			Module: moduleID,
			Layer:  winner.layer,
			Scope:  scope,
		})
	}
	return entries, nil
}

// resolveSource locates a dotfile source inside the layer directories,
// highest-precedence existing file first. The joined path must stay inside the
// layer root and the file must exist; both violations are resolve-time errors.
func resolveSource(winner dotfileWinner, moduleID string, base, host, user layerConfig) (string, error) {
	rel := winner.df.Source
	for _, layer := range []layerConfig{user, host, base} {
		if layer.path == "" {
			continue
		}
		abs := filepath.Join(layer.path, rel)
		contained, err := filepath.Rel(layer.path, abs)
		if err != nil || contained == ".." || strings.HasPrefix(contained, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("module %s: dotfile source %q escapes the module directory", moduleID, rel)
		}
		if _, err := os.Stat(abs); err == nil {
			return abs, nil
		}
	}
	return "", fmt.Errorf("module %s: dotfile source %q not found in any layer", moduleID, rel)
}

// checkDotfileConflicts rejects a dotfile target claimed by more than one
// module. Two modules writing the same target would emit a duplicate-key
// mise.toml that mise refuses to parse; failing here names both modules and
// the target instead of letting mise surface an opaque parse error.
func checkDotfileConflicts(entries []DotfileEntry) error {
	claimants := make(map[string][]string)
	for _, e := range entries {
		claimants[e.Target] = append(claimants[e.Target], e.Module)
	}
	var conflicts []string
	for target, mods := range claimants {
		if len(mods) < 2 {
			continue
		}
		sort.Strings(mods)
		conflicts = append(conflicts, fmt.Sprintf("%q claimed by modules [%s]",
			target, strings.Join(mods, ", ")))
	}
	if len(conflicts) == 0 {
		return nil
	}
	sort.Strings(conflicts)
	return fmt.Errorf("dotfile target conflict across modules: %s", strings.Join(conflicts, "; "))
}

// checkPackageConflicts rejects packages that are present in at least one
// module and absent in at least one other; install+remove would be ambiguous.
func checkPackageConflicts(presentIn, absentIn map[string][]string) error {
	var conflicts []string
	for pkg, presentMods := range presentIn {
		absentMods, ok := absentIn[pkg]
		if !ok {
			continue
		}
		sort.Strings(presentMods)
		sort.Strings(absentMods)
		conflicts = append(conflicts, fmt.Sprintf("%q present in module(s) [%s] but absent in module(s) [%s]",
			pkg, strings.Join(presentMods, ", "), strings.Join(absentMods, ", ")))
	}
	if len(conflicts) == 0 {
		return nil
	}
	sort.Strings(conflicts)
	return fmt.Errorf("package conflict across modules: %s", strings.Join(conflicts, "; "))
}

func sortEntries(entries []DotfileEntry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Module != entries[j].Module {
			return entries[i].Module < entries[j].Module
		}
		return entries[i].Target < entries[j].Target
	})
}
