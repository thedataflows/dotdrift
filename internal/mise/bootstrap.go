// Package mise: bootstrap config translation — emits [bootstrap.*] sections
// for mise bootstrap convergence.
package mise

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/resolve"
)

// PrefixedPackages translates package specs into mise `manager:pkg` keys.
//
// Rules (issues 0003, 0054): an explicit `manager:pkg` spec (anything
// containing a colon) passes through unchanged — mise's built-in managers
// (pacman, apt, dnf) and other plugins are respected. An `aur/<pkg>` spec is
// dotdrift's AUR marker; it maps to the paru plugin (the only AUR-capable
// manager until mise ships its built-in aur manager — see issue 0045), with
// the marker stripped so pacman -Q / paru -S see the real package name. A
// bare name (no colon, no aur/) gets the detected backend's manager prefix —
// on Arch that is `paru:` (issue 0054, reversing 0043's pacman mapping):
// paru covers repo and AUR packages in one manager and prompts for sudo
// itself when it needs to elevate. apt/dnf bare names keep their backend
// prefix.
func PrefixedPackages(names []string, backend string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		switch {
		case strings.Contains(n, ":"):
			out[i] = n // explicit manager prefix — pass through
		case strings.HasPrefix(n, "aur/"):
			out[i] = "paru:" + strings.TrimPrefix(n, "aur/") // AUR marker → paru plugin (until 0045)
		default:
			out[i] = backend + ":" + n // bare name → detected backend
		}
	}
	return out
}

// GenerateBootstrapPackages emits a [bootstrap.packages] section. Bare names
// get the backend prefix; explicit prefixes pass through. Every entry is
// pinned to "latest" (mise's package-plugin API does not support version pins
// for AUR). Returns "" when the list is empty.
func GenerateBootstrapPackages(install []string, backend string) string {
	prefixed := PrefixedPackages(install, backend)
	if len(prefixed) == 0 {
		return ""
	}
	sort.Strings(prefixed)
	var b strings.Builder
	b.WriteString("[bootstrap.packages]\n")
	for _, k := range prefixed {
		fmt.Fprintf(&b, "%q = \"latest\"\n", k)
	}
	return b.String()
}

// MisePluginsDir returns mise's package-plugin registry directory
// ($XDG_DATA_HOME/mise/plugins), where mise discovers installed plugins.
// dotdrift copies its embedded paru plugin here as real files (see
// paru.EnsureInstalled), the same on-disk shape as any other mise plugin.
func MisePluginsDir(xdgDataHome string) string {
	return strings.Join([]string{xdgDataHome, "mise", "plugins"}, "/")
}

// ToolsFragmentPath returns the path of dotdrift's global tools activation
// fragment: $XDG_CONFIG_HOME/mise/conf.d/dotdrift.toml (default
// ~/.config/mise/conf.d/dotdrift.toml). mise loads conf.d/*.toml as
// additional global config, so the resolved [tools] written there activate
// on PATH — without touching config.toml, which a profile may manage as a
// dotfile (issue 0037).
func ToolsFragmentPath() string {
	xdgConfig := os.Getenv("XDG_CONFIG_HOME")
	if xdgConfig == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		xdgConfig = filepath.Join(home, ".config")
	}
	return filepath.Join(xdgConfig, "mise", "conf.d", "dotdrift.toml")
}

// PluginsDirFromEnv resolves the mise plugin registry directory from the
// environment ($XDG_DATA_HOME/mise/plugins, falling back to
// ~/.local/share/mise/plugins). Returns the empty string when no home
// directory can be determined.
func PluginsDirFromEnv() string {
	xdgData := os.Getenv("XDG_DATA_HOME")
	if xdgData == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		xdgData = filepath.Join(home, ".local", "share")
	}
	return MisePluginsDir(xdgData)
}

// --- System dotfiles → [bootstrap.files] ---

// BootstrapFile is one concrete file target for [bootstrap.files].
type BootstrapFile struct {
	Target string
	Source string
	Mode   string // copy | symlink | symlink-each | template (informative; GenerateBootstrapFiles ignores it)

	Template bool
}

// ResolveBootstrapFiles expands system-scope dotfile entries into individual
// file targets. symlink-each entries are expanded by listing the source
// directory. sourceRoot must be an absolute path to the profile root.
func ResolveBootstrapFiles(entries []resolve.DotfileEntry, sourceRoot, homeDir string) ([]BootstrapFile, error) {
	var out []BootstrapFile
	for _, e := range entries {
		target := expandHome(e.Target, homeDir)
		sourceAbs := e.Source
		if !filepath.IsAbs(sourceAbs) {
			sourceAbs = filepath.Join(sourceRoot, sourceAbs)
		}
		if e.Mode == "symlink-each" {
			files, err := os.ReadDir(sourceAbs)
			if err != nil {
				return nil, fmt.Errorf("bootstrap files: read symlink-each source %s: %w", sourceAbs, err)
			}
			for _, f := range files {
				if f.IsDir() {
					continue
				}
				out = append(out, BootstrapFile{
					Target: filepath.Join(target, f.Name()),
					Source: filepath.Join(sourceAbs, f.Name()),
					Mode:   "symlink",
				})
			}
			continue
		}
		out = append(out, BootstrapFile{
			Target:   target,
			Source:   sourceAbs,
			Mode:     e.Mode,
			Template: e.Mode == "template",
		})
	}
	return out, nil
}

// GenerateBootstrapFiles emits a [bootstrap.files] section. copy/symlink modes
// become content copies (symlink→copy is a deliberate improvement for system
// files); template mode adds template = true.
func GenerateBootstrapFiles(files []BootstrapFile) string {
	if len(files) == 0 {
		return ""
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Target < files[j].Target })
	var b strings.Builder
	b.WriteString("[bootstrap.files]\n")
	for _, f := range files {
		fmt.Fprintf(&b, "%q = { source = %q", f.Target, f.Source)
		if f.Template {
			b.WriteString(", template = true")
		}
		b.WriteString(" }\n")
	}
	return b.String()
}

// --- Mount destinations → [bootstrap.directories] ---

// GenerateBootstrapDirectories emits a [bootstrap.directories] section so
// mise creates mount-point directories before units are enabled.
func GenerateBootstrapDirectories(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	seen := make(map[string]bool)
	var unique []string
	for _, p := range paths {
		if !seen[p] {
			seen[p] = true
			unique = append(unique, p)
		}
	}
	sort.Strings(unique)
	var b strings.Builder
	b.WriteString("[bootstrap.directories]\n")
	for _, p := range unique {
		fmt.Fprintf(&b, "%q = { state = \"present\" }\n", p)
	}
	return b.String()
}

// --- Mount/smb units → [bootstrap.services] ---

// BootstrapService is one systemd unit to converge via [bootstrap.services].
type BootstrapService struct {
	Name    string // unit name, e.g. "mnt-data.mount"
	Enabled bool
	Running bool
}

// GenerateBootstrapServices emits a [bootstrap.services] section. mise's
// schema types `enabled` as a boolean (true/false) and `state` as a
// "running"/"stopped" string.
func GenerateBootstrapServices(services []BootstrapService) string {
	if len(services) == 0 {
		return ""
	}
	sort.Slice(services, func(i, j int) bool { return services[i].Name < services[j].Name })
	var b strings.Builder
	b.WriteString("[bootstrap.services]\n")
	for _, s := range services {
		state := "stopped"
		if s.Running {
			state = "running"
		}
		fmt.Fprintf(&b, "%q = { state = %q, enabled = %t }\n", s.Name, state, s.Enabled)
	}
	return b.String()
}

// --- SMB accounts → [bootstrap.groups] + [bootstrap.users] ---

// GenerateBootstrapAccounts emits [bootstrap.groups] and [bootstrap.users]
// for the samba group and its users. mise requires an explicit primary
// `group` on every present user (issue 0040) — the smb group serves as both
// primary and supplementary group for these accounts.
func GenerateBootstrapAccounts(group string, users []string) string {
	if group == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("[bootstrap.groups]\n")
	fmt.Fprintf(&b, "%q = { state = \"present\" }\n", group)
	if len(users) > 0 {
		sortedUsers := make([]string, len(users))
		copy(sortedUsers, users)
		sort.Strings(sortedUsers)
		b.WriteString("\n[bootstrap.users]\n")
		for _, u := range sortedUsers {
			fmt.Fprintf(&b, "%q = { group = %q, groups = [%q], state = \"present\" }\n", u, group, group)
		}
	}
	return b.String()
}

// --- systemd user units → [bootstrap.linux.systemd.units] ---

// GenerateBootstrapSystemdUnits emits one
// [bootstrap.linux.systemd.units.<name>] table per unit (issue 0048).
// Directive tables pass through with their TOML types; units render sorted
// by name for deterministic configs. A value dotdrift cannot represent in
// TOML is an error naming the unit, never a silent drop.
func GenerateBootstrapSystemdUnits(units []resolve.SystemdUnitEntry) (string, error) {
	if len(units) == 0 {
		return "", nil
	}
	sorted := make([]resolve.SystemdUnitEntry, len(units))
	copy(sorted, units)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	var b strings.Builder
	for _, u := range sorted {
		fmt.Fprintf(&b, "[bootstrap.linux.systemd.units.%s]\n", tomlKey(u.Name))
		keys := make([]string, 0, len(u.Directives))
		for k := range u.Directives {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v, err := tomlSystemdValue(u.Name, k, u.Directives[k])
			if err != nil {
				return "", err
			}
			fmt.Fprintf(&b, "%s = %s\n", tomlKey(k), v)
		}
		b.WriteString("\n")
	}
	return b.String(), nil
}

// tomlSystemdValue renders one directive value as TOML, preserving the
// decoded type: strings quoted+escaped, bools/numbers bare, arrays and
// inline tables recursive (sorted keys).
func tomlSystemdValue(unit, key string, v any) (string, error) {
	switch val := v.(type) {
	case string:
		return `"` + tomlEscape(val) + `"`, nil
	case bool:
		return fmt.Sprintf("%t", val), nil
	case int64:
		return fmt.Sprintf("%d", val), nil
	case float64:
		return fmt.Sprintf("%g", val), nil
	case []any:
		parts := make([]string, len(val))
		for i, item := range val {
			s, err := tomlSystemdValue(unit, key, item)
			if err != nil {
				return "", err
			}
			parts[i] = s
		}
		return "[" + strings.Join(parts, ", ") + "]", nil
	case map[string]any:
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, len(keys))
		for i, k := range keys {
			s, err := tomlSystemdValue(unit, key, val[k])
			if err != nil {
				return "", err
			}
			parts[i] = tomlKey(k) + " = " + s
		}
		return "{ " + strings.Join(parts, ", ") + " }", nil
	default:
		return "", fmt.Errorf("systemd unit %q directive %q: unsupported value type %T", unit, key, v)
	}
}

// --- Secrets → [bootstrap.secrets] ---
// GenerateBootstrapSecrets emits a [bootstrap.secrets] section (issue 0044).
// Entries with only an env var use the short form; description or
// allow_empty force the table form. Sorted by logical name. Returns "" when
// no secrets are declared.
func GenerateBootstrapSecrets(secrets map[string]profile.Secret) string {
	if len(secrets) == 0 {
		return ""
	}
	names := make([]string, 0, len(secrets))
	for n := range secrets {
		names = append(names, n)
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteString("[bootstrap.secrets]\n")
	for _, n := range names {
		s := secrets[n]
		if s.Description == "" && !s.AllowEmpty {
			fmt.Fprintf(&b, "%s = %q\n", tomlKey(n), s.Env)
			continue
		}
		fmt.Fprintf(&b, "%s = { env = %q", tomlKey(n), s.Env)
		if s.Description != "" {
			fmt.Fprintf(&b, ", description = %q", s.Description)
		}
		if s.AllowEmpty {
			b.WriteString(", allow_empty = true")
		}
		b.WriteString(" }\n")
	}
	return b.String()
}

// expandHome replaces a leading ~ with homeDir.
func expandHome(path, homeDir string) string {
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(homeDir, path[2:])
	}
	if path == "~" {
		return homeDir
	}
	return path
}
