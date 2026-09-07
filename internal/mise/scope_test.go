package mise_test

import (
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/mise"
	"github.com/thedataflows/dotdrift/internal/resolve"
)

var systemEntries = []resolve.DotfileEntry{
	{Target: "/etc/demo.conf", Source: "/mod/demo.conf", Mode: "copy", Module: "demo", Layer: "base", Scope: "system"},
}

// System-scope entries generate a valid standalone mise config (TOML
// round-trip), separate from the user-scope config.
func TestGenerateDotfiles_systemEntriesRoundTrip(t *testing.T) {
	out := mise.GenerateDotfiles(systemEntries)
	require.Contains(t, out, "[dotfiles]")
	require.Contains(t, out, "/etc/demo.conf")

	var decoded struct {
		Dotfiles map[string]struct {
			Source string `toml:"source"`
			Mode   string `toml:"mode"`
		} `toml:"dotfiles"`
	}
	_, err := toml.Decode(out, &decoded)
	require.NoError(t, err, "generated system config must be valid TOML")
	entry, ok := decoded.Dotfiles["/etc/demo.conf"]
	require.True(t, ok, "system target must be present")
	require.Equal(t, "/mod/demo.conf", entry.Source)
	require.Equal(t, "copy", entry.Mode)
}
