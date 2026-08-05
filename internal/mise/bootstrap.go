// Package mise: bootstrap config translation — emits [bootstrap.*] sections
// for mise bootstrap convergence.
package mise

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/thedataflows/dotdrift/internal/resolve"
)

// PrefixedPackages translates bare package names to manager:pkg keys.
// Bare names (no colon) get the detected backend's manager prefix (paru, apt,
// dnf); names already containing a ":" prefix pass through unchanged.
func PrefixedPackages(names []string, backend string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		if strings.Contains(n, ":") {
			out[i] = n
		} else {
			out[i] = backend + ":" + n
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

// PluginDir returns the path where dotdrift writes a named mise package
// plugin (e.g. "paru") under the XDG data home.
func PluginDir(xdgDataHome, name string) string {
	return strings.Join([]string{xdgDataHome, "dotdrift", "mise-plugins", name}, "/")
}

// GenerateBootstrapPlugins emits a [bootstrap.plugins] section registering
// the paru package plugin at the given local path. Returns "" when pluginPath
// is empty.
func GenerateBootstrapPlugins(pluginPath string) string {
	if pluginPath == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("[bootstrap.plugins]\n")
	fmt.Fprintf(&b, "paru = %q\n", pluginPath)
	return b.String()
}

// GenerateBootstrapConfig emits all bootstrap-related sections: packages and
// the paru plugin registration. Returns "" when there is nothing to emit.
func GenerateBootstrapConfig(install []string, backend, pluginPath string) string {
	var out string
	if pkgs := GenerateBootstrapPackages(install, backend); pkgs != "" {
		out += pkgs + "\n"
	}
	if plugins := GenerateBootstrapPlugins(pluginPath); plugins != "" {
		out += plugins + "\n"
	}
	return out
}

// --- System dotfiles → [bootstrap.files] ---

// BootstrapFile is one concrete file target for [bootstrap.files].
type BootstrapFile struct {
	Target   string
	Source   string
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
				})
			}
			continue
		}
		out = append(out, BootstrapFile{
			Target:   target,
			Source:   sourceAbs,
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

// GenerateBootstrapServices emits a [bootstrap.services] section.
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
		enabled := "disabled"
		if s.Enabled {
			enabled = "enabled"
		}
		fmt.Fprintf(&b, "%q = { state = %q, enabled = %q }\n", s.Name, state, enabled)
	}
	return b.String()
}

// --- SMB accounts → [bootstrap.groups] + [bootstrap.users] ---

// GenerateBootstrapAccounts emits [bootstrap.groups] and [bootstrap.users]
// for the samba group and its users.
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
			fmt.Fprintf(&b, "%q = { groups = [%q], state = \"present\" }\n", u, group)
		}
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
