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

// TestEnsureInstalled_copiesWhenMissing: a missing plugin is copied into mise's
// registry as real files (not a symlink) and reports that it wrote.
func TestEnsureInstalled_copiesWhenMissing(t *testing.T) {
	registry := t.TempDir()
	target := filepath.Join(registry, "paru")

	updated, err := paru.EnsureInstalled(registry, "paru")
	require.NoError(t, err)
	require.True(t, updated)

	fi, err := os.Lstat(target)
	require.NoError(t, err)
	require.True(t, fi.IsDir(), "plugin must be a real directory, not a symlink")
	require.Zero(t, fi.Mode()&os.ModeSymlink)
	for _, rel := range []string{"metadata.lua", "mise.plugin.toml", "hooks/package_installed.lua", "hooks/package_install.lua"} {
		_, err := os.Stat(filepath.Join(target, rel))
		require.NoError(t, err, "expected %s", rel)
	}
}

// TestEnsureInstalled_noWriteWhenUpToDate: when the installed plugin's hash
// matches the embedded one, EnsureInstalled writes nothing and reports no
// change — the common path (no useless writes on every apply).
func TestEnsureInstalled_noWriteWhenUpToDate(t *testing.T) {
	registry := t.TempDir()
	target := filepath.Join(registry, "paru")

	_, err := paru.EnsureInstalled(registry, "paru")
	require.NoError(t, err)
	before, err := os.Stat(filepath.Join(target, "metadata.lua"))
	require.NoError(t, err)

	updated, err := paru.EnsureInstalled(registry, "paru")
	require.NoError(t, err)
	require.False(t, updated, "must not rewrite an up-to-date plugin")

	after, err := os.Stat(filepath.Join(target, "metadata.lua"))
	require.NoError(t, err)
	require.Equal(t, before.Size(), after.Size())
}

// TestEnsureInstalled_recopiesOnContentDrift: a tampered file changes the
// installed hash, so EnsureInstalled recopies and restores correct content.
func TestEnsureInstalled_recopiesOnContentDrift(t *testing.T) {
	registry := t.TempDir()
	target := filepath.Join(registry, "paru")
	_, err := paru.EnsureInstalled(registry, "paru")
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(target, "metadata.lua"), []byte("tampered"), 0o644))

	updated, err := paru.EnsureInstalled(registry, "paru")
	require.NoError(t, err)
	require.True(t, updated, "content drift must trigger a recopy")

	got, err := os.ReadFile(filepath.Join(target, "metadata.lua"))
	require.NoError(t, err)
	require.Contains(t, string(got), "PLUGIN = {")
}

// TestEnsureInstalled_replacesSymlinkWithCopy: a symlink at the target (stale
// shape left by an older dotdrift) is replaced with a real directory copy.
func TestEnsureInstalled_replacesSymlinkWithCopy(t *testing.T) {
	registry := t.TempDir()
	target := filepath.Join(registry, "paru")
	require.NoError(t, os.Symlink(t.TempDir(), target))

	updated, err := paru.EnsureInstalled(registry, "paru")
	require.NoError(t, err)
	require.True(t, updated)

	fi, err := os.Lstat(target)
	require.NoError(t, err)
	require.True(t, fi.IsDir(), "symlink must be replaced with a real directory")
	require.Zero(t, fi.Mode()&os.ModeSymlink)
}
