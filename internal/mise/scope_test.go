package mise_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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

// capturingExec returns an ExecMise whose invocations are recorded as
// "name args..." strings.
func capturingExec(calls *[]string) *mise.ExecMise {
	return mise.NewExecMise(&mise.Mise{
		LookPath: func(string) (string, error) { return "/fake/mise", nil },
		RunContext: func(_ context.Context, name string, args ...string) (string, error) {
			if len(args) > 0 && args[0] == "--version" {
				return mise.MinMiseVersion + "\n", nil
			}
			*calls = append(*calls, name+" "+strings.Join(args, " "))
			return "", nil
		},
	})
}

// The dotfiles-system step writes only the system entries to its own config
// and applies it through the sudo-aware entry point.
func TestDotfilesSystemStep_appliesSystemEntries(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "dotfiles-system", "mise.toml")
	var calls []string
	step := &mise.DotfilesSystemStep{
		Exec:       capturingExec(&calls),
		Entries:    systemEntries,
		ConfigPath: configPath,
		Yes:        true,
	}

	require.Equal(t, "dotfiles-system", step.Name())
	require.NoError(t, step.Run(context.Background()))

	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.Contains(t, string(data), "/etc/demo.conf")
	require.NotContains(t, string(data), "~/.bashrc")

	require.Len(t, calls, 1)
	require.Contains(t, calls[0], "dotfiles apply --cd "+filepath.Dir(configPath))
	require.Contains(t, calls[0], "--yes")
}

// An empty entry list no-ops: no config write, no mise invocation. (cmd
// already skips construction; this is the second line of defense.)
func TestDotfilesSystemStep_emptyEntriesNoop(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "dotfiles-system", "mise.toml")
	var calls []string
	step := &mise.DotfilesSystemStep{
		Exec:       capturingExec(&calls),
		ConfigPath: configPath,
	}

	require.NoError(t, step.Run(context.Background()))
	require.Empty(t, calls)
	_, err := os.Stat(configPath)
	require.True(t, os.IsNotExist(err), "no config must be written for an empty step")
}

// A nil Exec with real entries is a wiring bug and must fail loudly.
func TestDotfilesSystemStep_nilExecErrors(t *testing.T) {
	step := &mise.DotfilesSystemStep{
		Entries:    systemEntries,
		ConfigPath: filepath.Join(t.TempDir(), "mise.toml"),
	}
	require.Error(t, step.Run(context.Background()))
}
