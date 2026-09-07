package mise_test

import (
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/mise"
	"github.com/thedataflows/dotdrift/internal/resolve"
)

// [bootstrap.linux.systemd.units] emission (issue 0048): one table per unit,
// sorted by name, TOML types preserved (string/bool/number/array/inline
// table), valid TOML round-trip. Empty input emits nothing.
func TestGenerateBootstrapSystemdUnits_empty(t *testing.T) {
	got, err := mise.GenerateBootstrapSystemdUnits(nil)
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestGenerateBootstrapSystemdUnits_roundTrip(t *testing.T) {
	got, err := mise.GenerateBootstrapSystemdUnits([]resolve.SystemdUnitEntry{
		{Name: "zzz", Kind: "service", Directives: map[string]any{"exec_start": "/bin/sleep 1"}},
		{Name: "my-sync", Kind: "service", Directives: map[string]any{
			"description": "sync files",
			"exec_start":  "~/.local/bin/my-sync --watch",
			"after":       []any{"network-online.target"},
			"environment": map[string]any{"PATH": "/usr/local/bin:/usr/bin"},
			"nice":        int64(10),
			"private_tmp": true,
			"start":       false,
		}},
		{Name: "tick", Kind: "timer", Directives: map[string]any{
			"on_calendar": "daily",
			"persistent":  true,
			"unit":        "svc",
		}},
	})
	require.NoError(t, err)

	// Sorted: my-sync, tick, zzz.
	require.Less(t, indexOfStr(got, "my-sync"), indexOfStr(got, "tick"))
	require.Less(t, indexOfStr(got, "tick"), indexOfStr(got, "zzz"))

	// Valid TOML that decodes into the exact directive tree.
	var decoded struct {
		Bootstrap struct {
			Linux struct {
				Systemd struct {
					Units map[string]map[string]any `toml:"units"`
				} `toml:"systemd"`
			} `toml:"linux"`
		} `toml:"bootstrap"`
	}
	_, err = toml.Decode(got, &decoded)
	require.NoError(t, err, "generated units config must be valid TOML:\n%s", got)
	units := decoded.Bootstrap.Linux.Systemd.Units
	require.Equal(t, "~/.local/bin/my-sync --watch", units["my-sync"]["exec_start"])
	require.Equal(t, []any{"network-online.target"}, units["my-sync"]["after"])
	require.Equal(t, map[string]any{"PATH": "/usr/local/bin:/usr/bin"}, units["my-sync"]["environment"])
	require.Equal(t, int64(10), units["my-sync"]["nice"])
	require.Equal(t, true, units["my-sync"]["private_tmp"])
	require.Equal(t, false, units["my-sync"]["start"])
	require.Equal(t, "daily", units["tick"]["on_calendar"])
}

// Values dotdrift cannot represent in TOML are loud errors, never silent
// drops.
func TestGenerateBootstrapSystemdUnits_unsupportedTypeErrors(t *testing.T) {
	_, err := mise.GenerateBootstrapSystemdUnits([]resolve.SystemdUnitEntry{
		{Name: "bad", Kind: "service", Directives: map[string]any{"exec_start": struct{}{}}},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "bad")
}
