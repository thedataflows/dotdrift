// Package onboard materializes live paths into a module.
package onboard

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/thedataflows/dotdrift/internal/mise"
	"github.com/thedataflows/dotdrift/internal/resolve"
	"github.com/thedataflows/dotdrift/internal/state"
)

// Options configures the onboard operation.
type Options struct {
	// Ctx carries cancellation to mise subprocesses; nil means Background.
	Ctx         context.Context
	ProfileRoot string
	Paths       []string
	App         string
	Mode        string
	Packages    []string
	Tools       []string
	Host        bool
	DryRun      bool
	Yes         bool
	Home        string
	Hostname    string
}

// Onboard materializes live paths into a module and applies them.
type Onboard struct {
	Mise mise.Runner
}

// dotfileEntry matches the TOML shape for a single dotfile.
type dotfileEntry struct {
	Source string `toml:"source"`
	Mode   string `toml:"mode"`
}

// moduleConfig is the TOML shape written to module.toml.
type moduleConfig struct {
	ID       string                  `toml:"id,omitempty"`
	App      string                  `toml:"app,omitempty"`
	Packages packagesConfig          `toml:"packages,omitempty"`
	Tools    map[string]string       `toml:"tools,omitempty"`
	Dotfiles map[string]dotfileEntry `toml:"dotfiles,omitempty"`
}

type packagesConfig struct {
	Present []string `toml:"present,omitempty"`
}

// Run copies the live paths into the module, writes module.toml, and applies.
func (o *Onboard) Run(opts Options) error {
	if len(opts.Paths) == 0 {
		return fmt.Errorf("no paths provided")
	}

	home := opts.Home
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("home dir: %w", err)
		}
	}

	expanded := make([]string, len(opts.Paths))
	for i, p := range opts.Paths {
		expanded[i] = expandPath(p, home)
	}

	app := opts.App
	if app == "" {
		app = inferApp(expanded, home)
	}
	if app == "" {
		return fmt.Errorf("could not infer app from paths")
	}

	mode := opts.Mode
	if mode == "" {
		mode = "link"
	}

	moduleDir := filepath.Join(opts.ProfileRoot, "modules", app)
	if opts.Host {
		if opts.Hostname == "" {
			return fmt.Errorf("hostname required for host overlay")
		}
		moduleDir = filepath.Join(opts.ProfileRoot, "hosts", opts.Hostname, "modules", app)
	}

	entries := make(map[string]dotfileEntry)
	for _, p := range expanded {
		target, source, err := mapPath(p, home, moduleDir)
		if err != nil {
			return err
		}
		if !opts.DryRun {
			if _, err := os.Stat(source); err == nil {
				return fmt.Errorf("conflict: %s already exists in module", source)
			}
			if err := copyPath(p, source); err != nil {
				return fmt.Errorf("copy %s: %w", p, err)
			}
		}
		relSource, _ := filepath.Rel(moduleDir, source)
		entries[target] = dotfileEntry{Source: filepath.ToSlash(relSource), Mode: mode}
	}

	if opts.DryRun {
		return nil
	}

	cfg := moduleConfig{
		ID:       app,
		App:      app,
		Packages: packagesConfig{Present: opts.Packages},
		Tools:    toolsMap(opts.Tools),
		Dotfiles: entries,
	}
	if err := writeModuleTOML(moduleDir, cfg); err != nil {
		return err
	}

	if o.Mise == nil {
		return fmt.Errorf("no mise runner configured")
	}

	configPath, err := writeMiseConfig(opts.ProfileRoot, moduleDir, entries)
	if err != nil {
		return err
	}
	ctx := opts.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if err := o.Mise.EnsureAndInstall(ctx, configPath); err != nil {
		return fmt.Errorf("mise install: %w", err)
	}
	if err := o.Mise.DotfilesApply(ctx, configPath, opts.Yes); err != nil {
		return fmt.Errorf("mise dotfiles apply: %w", err)
	}
	return nil
}

func expandPath(p, home string) string {
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(home, p[2:])
	}
	return p
}

func inferApp(paths []string, home string) string {
	for _, p := range paths {
		rel, err := filepath.Rel(filepath.Join(home, ".config"), p)
		if err == nil && !strings.HasPrefix(rel, "..") {
			parts := strings.Split(rel, string(os.PathSeparator))
			if parts[0] != "" && parts[0] != "." {
				return parts[0]
			}
		}
	}
	base := filepath.Base(paths[0])
	return strings.TrimPrefix(base, ".")
}

func mapPath(p, home, moduleDir string) (target, source string, err error) {
	rel, err := filepath.Rel(home, p)
	if err == nil && !strings.HasPrefix(rel, "..") {
		target = "~" + string(os.PathSeparator) + rel
		source = filepath.Join(moduleDir, "home", rel)
		return target, source, nil
	}
	if !filepath.IsAbs(p) {
		p, err = filepath.Abs(p)
		if err != nil {
			return "", "", err
		}
	}
	target = p
	source = filepath.Join(moduleDir, "system", strings.TrimPrefix(p, string(os.PathSeparator)))
	return target, source, nil
}

// copyPath copies src to dst preserving file modes. Ownership is not
// preserved: copied files are owned by the current user.
func copyPath(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return copyDir(src, dst, info.Mode())
	}
	return copyFile(src, dst, info.Mode())
}

func copyDir(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(dst, mode); err != nil {
		return err
	}
	// MkdirAll applies mode only to newly created dirs; enforce it.
	if err := os.Chmod(dst, mode); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		s := filepath.Join(src, e.Name())
		d := filepath.Join(dst, e.Name())
		if e.Type()&os.ModeSymlink != 0 {
			target, err := os.Readlink(s)
			if err != nil {
				return err
			}
			if err := os.Symlink(target, d); err != nil {
				return err
			}
			continue
		}
		info, err := e.Info()
		if err != nil {
			return err
		}
		if info.IsDir() {
			if err := copyDir(s, d, info.Mode()); err != nil {
				return err
			}
			continue
		}
		if err := copyFile(s, d, info.Mode()); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	// umask may have stripped bits at creation; enforce the source mode.
	return os.Chmod(dst, mode)
}

func writeModuleTOML(moduleDir string, cfg moduleConfig) error {
	if err := os.MkdirAll(moduleDir, 0o755); err != nil {
		return fmt.Errorf("create module dir: %w", err)
	}
	data, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("encode module.toml: %w", err)
	}
	path := filepath.Join(moduleDir, "module.toml")
	return os.WriteFile(path, data, 0o644)
}

// writeMiseConfig generates the onboard mise config under the profile's XDG
// state directory (alongside apply's config) so the profile directory stays
// free of runtime state. Dotfile sources are absolute because mise resolves
// them against the config's directory.
func writeMiseConfig(profileRoot, moduleDir string, entries map[string]dotfileEntry) (string, error) {
	absModule, err := filepath.Abs(moduleDir)
	if err != nil {
		absModule = moduleDir
	}

	targets := make([]string, 0, len(entries))
	for target := range entries {
		targets = append(targets, target)
	}
	sort.Strings(targets)

	plan := &resolve.Plan{
		Dotfiles: resolve.DotfilesStep{Entries: make([]resolve.DotfileEntry, 0, len(entries))},
	}
	for _, target := range targets {
		e := entries[target]
		plan.Dotfiles.Entries = append(plan.Dotfiles.Entries, resolve.DotfileEntry{
			Target: target,
			Source: filepath.Join(absModule, e.Source),
			Mode:   e.Mode,
		})
	}
	cfg := mise.GenerateConfig(plan)

	configPath := filepath.Join(filepath.Dir(state.ProfileStatePath(profileRoot)), "onboard", "mise.toml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(configPath, []byte(cfg), 0o644); err != nil {
		return "", err
	}
	return configPath, nil
}

func toolsMap(tools []string) map[string]string {
	if len(tools) == 0 {
		return nil
	}
	out := make(map[string]string, len(tools))
	for _, t := range tools {
		parts := strings.SplitN(t, "=", 2)
		if len(parts) == 2 {
			out[parts[0]] = parts[1]
		} else {
			out[t] = "latest"
		}
	}
	return out
}

