package profile_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/profile"
)

// writeColorsFile writes content at the absolute path, creating parents.
func writeColorsFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

// [colors] in dotdrift.toml flows into Config.Colors, unioned across
// layers with higher-layer precedence (host beats base; base applies when
// the host is silent). Malformed roles/values fail the load naming the
// file.
func TestProfile_colorsConfig(t *testing.T) {
	t.Run("base override loads", func(t *testing.T) {
		root := t.TempDir()
		writeColorsFile(t, filepath.Join(root, "modules/m/module.toml"), "id = \"m\"\n")
		writeColorsFile(t, filepath.Join(root, "dotdrift.toml"),
			"[colors]\nmissing = \"38;5;75\"\norphan = \"94\"\n")
		p, err := profile.Load(root, &facts.Facts{})
		require.NoError(t, err)
		require.Equal(t, map[string]string{"missing": "38;5;75", "orphan": "94"}, p.Config.Colors)
	})

	t.Run("host layer beats base", func(t *testing.T) {
		root := t.TempDir()
		writeColorsFile(t, filepath.Join(root, "modules/m/module.toml"), "id = \"m\"\n")
		writeColorsFile(t, filepath.Join(root, "dotdrift.toml"), "[colors]\nmissing = \"38;5;75\"\n")
		writeColorsFile(t, filepath.Join(root, "hosts/myhost/dotdrift.toml"), "[colors]\nmissing = \"31\"\n")
		p, err := profile.Load(root, &facts.Facts{Hostname: "myhost"})
		require.NoError(t, err)
		require.Equal(t, map[string]string{"missing": "31"}, p.Config.Colors,
			"host override wins; base value for the same role replaced")
	})

	t.Run("unknown role errors naming the file", func(t *testing.T) {
		root := t.TempDir()
		writeColorsFile(t, filepath.Join(root, "modules/m/module.toml"), "id = \"m\"\n")
		writeColorsFile(t, filepath.Join(root, "dotdrift.toml"), "[colors]\nbogus = \"31\"\n")
		_, err := profile.Load(root, &facts.Facts{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "bogus")
		require.Contains(t, err.Error(), "dotdrift.toml")
	})

	t.Run("malformed value errors", func(t *testing.T) {
		root := t.TempDir()
		writeColorsFile(t, filepath.Join(root, "modules/m/module.toml"), "id = \"m\"\n")
		writeColorsFile(t, filepath.Join(root, "dotdrift.toml"), "[colors]\nmissing = \"red\"\n")
		_, err := profile.Load(root, &facts.Facts{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "missing")
	})
}
