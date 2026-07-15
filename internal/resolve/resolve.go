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
}

// HooksStep is a placeholder for pre/post hooks in v0.1.
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
		Hooks:    HooksStep{},
	}

	pkgSet := make(map[string]struct{})
	for _, m := range p.Selected {
		root, err := rootFromModule(m)
		if err != nil {
			return nil, err
		}

		base := layerConfig{name: "base", path: m.Path, cfg: m.Config}
		hostPath := filepath.Join(root, "hosts", f.Hostname, "modules", filepath.Base(m.Path))
		hostCfg, _ := loadModuleConfig(hostPath)
		host := layerConfig{name: "host", path: hostPath, cfg: hostCfg}
		userPath := filepath.Join(root, "users", f.Username, "modules", filepath.Base(m.Path))
		userCfg, _ := loadModuleConfig(userPath)
		user := layerConfig{name: "user", path: userPath, cfg: userCfg}

		install, remove := mergePackages(base.cfg.Packages, host.cfg.Packages, user.cfg.Packages)
		for _, pkg := range install {
			pkgSet[pkg] = struct{}{}
		}
		plan.Packages.Remove = append(plan.Packages.Remove, remove...)
		sort.Strings(plan.Packages.Remove)

		for k, v := range mergeTools(base.cfg.Tools, host.cfg.Tools, user.cfg.Tools) {
			plan.Tools.Versions[k] = v
		}

		plan.Dotfiles.Entries = append(plan.Dotfiles.Entries, mergeDotfiles(base, host, user)...)
	}

	for pkg := range pkgSet {
		plan.Packages.Install = append(plan.Packages.Install, pkg)
	}
	sort.Strings(plan.Packages.Install)
	sortEntries(plan.Dotfiles.Entries)

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

func rootFromModule(m profile.Module) (string, error) {
	// m.Path is root/modules/<id>; go up two levels to reach the profile root.
	modulesDir := filepath.Dir(m.Path)
	root := filepath.Dir(modulesDir)
	return root, nil
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

func mergeDotfiles(base, host, user layerConfig) []DotfileEntry {
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
		entries = append(entries, DotfileEntry{
			Target: target,
			Source: resolveSource(winner, base, host, user),
			Mode:   winner.df.Mode,
			Module: moduleID,
			Layer:  winner.layer,
		})
	}
	return entries
}

func resolveSource(winner dotfileWinner, base, host, user layerConfig) string {
	rel := winner.df.Source
	for _, layer := range []layerConfig{user, host, base} {
		if layer.path == "" {
			continue
		}
		abs := filepath.Join(layer.path, rel)
		if _, err := os.Stat(abs); err == nil {
			return abs
		}
	}
	return filepath.Join(winner.path, rel)
}

func sortEntries(entries []DotfileEntry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Module != entries[j].Module {
			return entries[i].Module < entries[j].Module
		}
		return entries[i].Target < entries[j].Target
	})
}
