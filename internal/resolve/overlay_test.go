package resolve_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/resolve"
)

// Overlay-only modules (ADR-0001): a module that exists only in a host or
// user layer is selected like a base module, and resolve merges its layers
// with the absent ones treated as empty. Layer paths derive from the
// profile root and the module directory name — never from the
// representative module path, which points into the discovery layer.

// writeOverlayModule writes a module.toml into root/<layer>/<name>/modules/<dir>
// and returns the module directory.
func writeOverlayModule(t *testing.T, root, layer, name, dir, moduleTOML string) string {
	t.Helper()
	d := filepath.Join(root, layer, name, "modules", dir)
	require.NoError(t, os.MkdirAll(d, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(d, "module.toml"), []byte(moduleTOML), 0o644))
	return d
}

// requireEmptyBase creates the base modules/ directory that discovery
// requires even when every module lives in overlays.
func requireEmptyBase(t *testing.T, root string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "modules"), 0o755))
}

// A host-only module resolves its packages and dotfiles against an empty
// base layer; the winning dotfile layer is the host layer.
func TestResolve_overlayOnlyModuleMergesEmptyBase(t *testing.T) {
	root := t.TempDir()
	requireEmptyBase(t, root)
	hostDir := writeOverlayModule(t, root, "hosts", "myhost", "hostonly", `
[packages]
present = ["host-pkg"]

[dotfiles]
"~/.hostrc" = { source = "hostrc", mode = "copy" }
`)
	require.NoError(t, os.WriteFile(filepath.Join(hostDir, "hostrc"), []byte("host"), 0o644))

	plan, err := loadAndResolve(t, root, &facts.Facts{Hostname: "myhost", Username: "cri"})
	require.NoError(t, err)

	require.Contains(t, plan.Packages.Install, "host-pkg",
		"overlay-only module packages must merge against an empty base layer")

	entry := entryByTarget(t, plan.Dotfiles.Entries, "~/.hostrc")
	require.Equal(t, "host", entry.Layer, "the winning layer is the host layer, not a phantom base")
	require.Equal(t, "hostonly", entry.Module)
	require.Equal(t, filepath.Join(hostDir, "hostrc"), entry.Source,
		"dotfile source resolves from the host layer directory")
}

// Scope comes from the representative config m.Config (base-preferred by
// discovery order); for a host-only module that is the host layer's config.
func TestResolve_overlayOnlyModuleScopeFromDiscoveryLayer(t *testing.T) {
	root := t.TempDir()
	requireEmptyBase(t, root)
	hostDir := writeOverlayModule(t, root, "hosts", "myhost", "sysmod", `
scope = "system"

[dotfiles]
"/etc/sysmod.conf" = { source = "sysmod.conf", mode = "copy" }
`)
	require.NoError(t, os.WriteFile(filepath.Join(hostDir, "sysmod.conf"), []byte("x"), 0o644))

	plan, err := loadAndResolve(t, root, &facts.Facts{Hostname: "myhost", Username: "cri"})
	require.NoError(t, err)

	entry := entryByTarget(t, plan.Dotfiles.Entries, "/etc/sysmod.conf")
	require.Equal(t, profile.ScopeSystem, entry.Scope,
		"scope comes from the representative (discovery) layer config")
	require.Equal(t, "host", entry.Layer)
}

// Scope is representative-layer only: an overlay's scope declaration never
// overrides the base layer's scope.
func TestResolve_baseScopePreferredOverOverlay(t *testing.T) {
	root := t.TempDir()
	baseDir := writeModule(t, root, "dual", `
[dotfiles]
"~/.dualrc" = { source = "dualrc", mode = "copy" }
`)
	require.NoError(t, os.WriteFile(filepath.Join(baseDir, "dualrc"), []byte("base"), 0o644))
	writeOverlayModule(t, root, "hosts", "myhost", "dual", `scope = "system"`)

	plan, err := loadAndResolve(t, root, &facts.Facts{Hostname: "myhost", Username: "cri"})
	require.NoError(t, err)

	entry := entryByTarget(t, plan.Dotfiles.Entries, "~/.dualrc")
	require.Equal(t, profile.ScopeUser, entry.Scope,
		"base scope (default user) wins; overlay scope declarations are ignored")
}

// Regression lock: a malformed overlay module.toml for an overlay-only
// module is a loud error naming the module and the overlay path.
func TestResolve_malformedOverlayModuleToml_stillErrors(t *testing.T) {
	root := t.TempDir()
	requireEmptyBase(t, root)
	overlayDir := filepath.Join(root, "hosts", "myhost", "modules", "ghost")
	require.NoError(t, os.MkdirAll(overlayDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(overlayDir, "module.toml"), []byte("not = [valid"), 0o644))

	f := &facts.Facts{Hostname: "myhost", Username: "cri"}
	p, loadErr := profile.Load(root, f)
	if loadErr == nil {
		_, loadErr = resolve.Resolve(p, f)
	}
	require.Error(t, loadErr, "malformed overlay module.toml must propagate for overlay-only modules")
	require.Contains(t, loadErr.Error(), "ghost", "error should name the module")
	require.Contains(t, loadErr.Error(), filepath.Join("hosts", "myhost"), "error should identify the overlay path")
}

// Overlays key on the module directory name, not the declared id: a user
// overlay in users/<u>/modules/<dir> merges onto a host-only module even
// when its module.toml declares a different id.
func TestResolve_moduleIDIsDirNameForOverlayOnly(t *testing.T) {
	root := t.TempDir()
	requireEmptyBase(t, root)
	writeOverlayModule(t, root, "hosts", "myhost", "hostdir", `
id = "declared-id"

[packages]
present = ["host-pkg"]
`)
	writeOverlayModule(t, root, "users", "cri", "hostdir", `
[packages]
present = ["user-pkg"]
`)

	plan, err := loadAndResolve(t, root, &facts.Facts{Hostname: "myhost", Username: "cri"})
	require.NoError(t, err)

	require.Contains(t, plan.Packages.Install, "host-pkg")
	require.Contains(t, plan.Packages.Install, "user-pkg",
		"overlays key on the module directory name, not the declared id")
}
