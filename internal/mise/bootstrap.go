// Package mise: bootstrap config translation — emits [bootstrap.*] sections
// for mise bootstrap convergence.
package mise

import (
	"fmt"
	"sort"
	"strings"
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