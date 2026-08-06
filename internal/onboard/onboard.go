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
	Packages    []PackageEntry
	Tools       []string
	Host        bool
	DryRun      bool
	Yes         bool
	Force       bool
	Home        string
	Hostname    string
}

// Onboard materializes live paths into a module and applies them.
type Onboard struct {
	Mise mise.Runner
}

// PackageEntry is one declared distro package. Description, when set, is
// rendered as a trailing "# <desc>" comment next to the package in
// module.toml — it is cosmetic only; resolve reads back just the bare
// names (the TOML parser ignores comments), so descriptions do not enter
// the data model.
type PackageEntry struct {
	Name        string
	Description string
}

// dotfileEntry is one managed path rendered as an inline TOML table.
type dotfileEntry struct {
	Source string
	Mode   string
}

// moduleConfig is the shape written to module.toml. It is rendered by
// hand (encodeModuleTOML) rather than a TOML encoder so dotfiles become
// inline tables and packages can carry description comments.
type moduleConfig struct {
	Packages packagesConfig
	Tools    map[string]string
	Dotfiles map[string]dotfileEntry
}

type packagesConfig struct {
	Present []PackageEntry
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
		mode = "symlink"
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
				if !opts.Force {
					return fmt.Errorf("conflict: %s already exists in module (remove it or onboard into a different module id)", source)
				}
				if err := os.RemoveAll(source); err != nil {
					return fmt.Errorf("force replace %s: %w", source, err)
				}
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
	// Onboard just copied the live paths into the module, so takeover is
	// lossless and forced: mise otherwise refuses to overwrite the
	// still-present live files.
	if err := o.Mise.DotfilesApply(ctx, configPath, opts.Yes, true); err != nil {
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
	path := filepath.Join(moduleDir, "module.toml")
	return os.WriteFile(path, []byte(encodeModuleTOML(cfg)), 0o644)
}

// encodeModuleTOML renders the module config as TOML by hand. BurntSushi
// cannot emit inline tables (it expands every map/struct into a sub-table),
// so this produces the lighter style hand-authored modules use — dotfiles
// as "target" = { source = ..., mode = ... } and packages as an array whose
// entries may carry a trailing "# description" comment. Map keys are sorted
// for deterministic output.
func encodeModuleTOML(cfg moduleConfig) string {
	var b strings.Builder
	if len(cfg.Packages.Present) > 0 {
		b.WriteString("[packages]\npresent = [\n")
		for _, p := range cfg.Packages.Present {
			if p.Description != "" {
				b.WriteString("  " + tomlBasicString(p.Name) + ", # " + p.Description + "\n")
			} else {
				b.WriteString("  " + tomlBasicString(p.Name) + ",\n")
			}
		}
		b.WriteString("]\n\n")
	}
	if len(cfg.Tools) > 0 {
		b.WriteString("[tools]\n")
		keys := make([]string, 0, len(cfg.Tools))
		for k := range cfg.Tools {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			b.WriteString(tomlKey(k) + " = " + tomlBasicString(cfg.Tools[k]) + "\n")
		}
		b.WriteString("\n")
	}
	if len(cfg.Dotfiles) > 0 {
		b.WriteString("[dotfiles]\n")
		keys := make([]string, 0, len(cfg.Dotfiles))
		for k := range cfg.Dotfiles {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			e := cfg.Dotfiles[k]
			b.WriteString(tomlBasicString(k) + " = { source = " + tomlBasicString(e.Source) + ", mode = " + tomlBasicString(e.Mode) + " }\n")
		}
	}
	return b.String()
}

// tomlBasicString renders s as a TOML basic string with the minimal
// escaping the TOML spec requires (control chars, quote, backslash).
func tomlBasicString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		case '\r':
			b.WriteString(`\r`)
		default:
			if r < 0x20 {
				fmt.Fprintf(&b, `\u%04x`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

// tomlKey renders a key bare when it matches the simple-key charset
// ([A-Za-z0-9_-]+), quoted otherwise.
func tomlKey(k string) string {
	if isBareKey(k) {
		return k
	}
	return tomlBasicString(k)
}

func isBareKey(k string) bool {
	if k == "" {
		return false
	}
	for _, r := range k {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_' || r == '-':
		default:
			return false
		}
	}
	return true
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

// ParsePackages parses --packages values into entries. kong splits the flag
// value on commas (its default), so each element is already one entry by
// the time it reaches here; an entry is either a bare package name
// ("ripgrep", "aur/foo") or name="description". Because kong splits on
// commas unconditionally, a description must not itself contain a comma —
// use a comma-free wording. (Hand-authored module.toml files are not so
// constrained, since their descriptions sit inside the quoted array entry.)
func ParsePackages(values []string) ([]PackageEntry, error) {
	var out []PackageEntry
	for _, v := range values {
		entry, ok, err := parsePackageEntry(v)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, entry)
		}
	}
	return out, nil
}

func parsePackageEntry(s string) (PackageEntry, bool, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return PackageEntry{}, false, nil
	}
	name, rest, hasDesc := strings.Cut(s, "=")
	name = strings.TrimSpace(name)
	if name == "" {
		return PackageEntry{}, false, fmt.Errorf("empty package name in %q", s)
	}
	if !hasDesc {
		return PackageEntry{Name: name}, true, nil
	}
	desc := strings.TrimSpace(rest)
	if len(desc) >= 2 && desc[0] == '"' && desc[len(desc)-1] == '"' {
		desc = desc[1 : len(desc)-1]
	}
	return PackageEntry{Name: name, Description: desc}, true, nil
}
