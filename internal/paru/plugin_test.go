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
