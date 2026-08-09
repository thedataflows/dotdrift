package paru_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/paru"
)

func TestWritePlugin_createsAllFiles(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, paru.WritePlugin(dir))

	for _, rel := range []string{
		"metadata.lua",
		"mise.plugin.toml",
		"hooks/package_installed.lua",
		"hooks/package_install.lua",
	} {
		info, err := os.Stat(filepath.Join(dir, rel))
		require.NoError(t, err, "expected file %s", rel)
		require.NotZero(t, info.Size(), "file %s must not be empty", rel)
	}
}

func TestWritePlugin_tomlRequiresParu(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, paru.WritePlugin(dir))
	content, err := os.ReadFile(filepath.Join(dir, "mise.plugin.toml"))
	require.NoError(t, err)
	require.Contains(t, string(content), `[package-manager]`)
	require.Contains(t, string(content), `requires = ["paru"]`)
	require.Contains(t, string(content), `os = ["linux"]`)
}
// TestWritePlugin_metadataLuaAssignsPluginGlobal guards the root cause of the
// "declared in [bootstrap.plugins] but not installed" warnings: mise's vfox
// loader runs `require "metadata"; return PLUGIN`, so metadata.lua must assign
// the global PLUGIN (not `return` a table) and must carry the required `name`.
func TestWritePlugin_metadataLuaAssignsPluginGlobal(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, paru.WritePlugin(dir))
	content, err := os.ReadFile(filepath.Join(dir, "metadata.lua"))
	require.NoError(t, err)
	s := string(content)
	require.Contains(t, s, "PLUGIN = {")
	require.NotContains(t, s, "return {", "metadata.lua must assign PLUGIN, not return a table")
	require.Contains(t, s, `name = "paru"`)
}

func TestWritePlugin_installedLuaDelegatesToDotdrift(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, paru.WritePlugin(dir))
	content, err := os.ReadFile(filepath.Join(dir, "hooks/package_installed.lua"))
	require.NoError(t, err)
	s := string(content)
	require.Contains(t, s, "function PLUGIN:PackageInstalled(ctx)")
	require.Contains(t, s, "dotdrift paru installed")
}

func TestWritePlugin_installLuaDelegatesToDotdrift(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, paru.WritePlugin(dir))
	content, err := os.ReadFile(filepath.Join(dir, "hooks/package_install.lua"))
	require.NoError(t, err)
	s := string(content)
	require.Contains(t, s, "function PLUGIN:PackageInstall(ctx)")
	require.Contains(t, s, "dotdrift paru install")
}

func TestWritePlugin_idempotent(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, paru.WritePlugin(dir))
	first, _ := os.ReadFile(filepath.Join(dir, "hooks/package_installed.lua"))
	require.NoError(t, paru.WritePlugin(dir))
	second, _ := os.ReadFile(filepath.Join(dir, "hooks/package_installed.lua"))
	require.Equal(t, first, second)
}

// TestEnsureInstalled_createsAndRepairsSymlink covers the registration
// invariant dotdrift owns: the mise plugin registry must symlink the source,
// and a stale/mis-pointing link is repaired on every apply (mise's own
// bootstrap linking skips an existing link, so dotdrift cannot rely on it).
func TestEnsureInstalled_createsAndRepairsSymlink(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "dotdrift", "mise-plugins", "paru")
	registry := filepath.Join(root, "mise", "plugins")
	link := filepath.Join(registry, "paru")

	// missing → created, pointing at the source
	require.NoError(t, paru.EnsureInstalled(source, registry))
	got, err := os.Readlink(link)
	require.NoError(t, err)
	require.Equal(t, source, got)

	// mis-pointing → repaired back to the source
	require.NoError(t, os.Remove(link))
	require.NoError(t, os.Symlink("/nonexistent/wrong", link))
	require.NoError(t, paru.EnsureInstalled(source, registry))
	got, err = os.Readlink(link)
	require.NoError(t, err)
	require.Equal(t, source, got)

	// already correct → left untouched (still valid)
	require.NoError(t, paru.EnsureInstalled(source, registry))
	got, err = os.Readlink(link)
	require.NoError(t, err)
	require.Equal(t, source, got)
}

// TestEnsureInstalled_exactMirrorRemovesStaleFiles: the on-disk plugin must be
// an exact mirror of the embedded one — a file an older dotdrift wrote but the
// current one does not must not linger.
func TestEnsureInstalled_exactMirrorRemovesStaleFiles(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "dotdrift", "mise-plugins", "paru")
	registry := filepath.Join(root, "mise", "plugins")

	// simulate a leftover from an older dotdrift version
	require.NoError(t, os.MkdirAll(filepath.Join(source, "hooks"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(source, "hooks", "package_upgrade.lua"), []byte("stale"), 0o644))

	require.NoError(t, paru.EnsureInstalled(source, registry))

	_, err := os.Stat(filepath.Join(source, "hooks", "package_upgrade.lua"))
	require.True(t, os.IsNotExist(err), "stale file from an older dotdrift must be removed")
	for _, rel := range []string{"metadata.lua", "mise.plugin.toml", "hooks/package_installed.lua", "hooks/package_install.lua"} {
		_, err := os.Stat(filepath.Join(source, rel))
		require.NoError(t, err, "expected current plugin file %s", rel)
	}
}

// TestEnsureInstalled_leavesNonSymlinkEntryAlone: a real directory at the link
// path (e.g. a git-cloned plugin installed outside dotdrift) is not dotdrift's
// to delete.
func TestEnsureInstalled_leavesNonSymlinkEntryAlone(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "dotdrift", "mise-plugins", "paru")
	registry := filepath.Join(root, "mise", "plugins")
	link := filepath.Join(registry, "paru")

	require.NoError(t, os.MkdirAll(link, 0o755)) // real dir, not a symlink
	require.NoError(t, paru.EnsureInstalled(source, registry))

	fi, err := os.Lstat(link)
	require.NoError(t, err)
	require.Zero(t, fi.Mode()&os.ModeSymlink, "non-symlink entry must be left in place")
	// source is still written
	_, err = os.Stat(filepath.Join(source, "metadata.lua"))
	require.NoError(t, err)
}
