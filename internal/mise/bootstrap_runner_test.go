package mise_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/mise"
)

// --- Bootstrap runner ---

func TestExecMise_bootstrapInvokesMise(t *testing.T) {
	var gotName string
	var gotArgs []string
	m := &mise.Mise{
		LookPath: func(string) (string, error) { return "/fake/mise", nil },
		RunContext: func(_ context.Context, name string, args ...string) (string, error) {
			if len(args) == 1 && args[0] == "--version" {
				return mise.MinMiseVersion + "\n", nil
			}
			gotName = name
			gotArgs = args
			return "", nil
		},
		Classify: func(string) mise.InstallKind { return mise.InstallKindUserManaged },
	}
	exec := mise.NewExecMise(m)
	err := exec.Bootstrap(context.Background(), "/tmp/cfg/mise.toml", true)
	require.NoError(t, err)
	require.Equal(t, "/fake/mise", gotName)
	require.Contains(t, gotArgs, "bootstrap")
	require.Contains(t, gotArgs, "--yes")
}

func TestFakeRunner_bootstrap(t *testing.T) {
	f := &mise.FakeRunner{}
	require.NoError(t, f.Bootstrap(context.Background(), "/x.toml", true))
	require.True(t, f.BootstrapCalled)
}

// --- Plugin registry ---

func TestMisePluginsDir_underXDGDataHome(t *testing.T) {
	dir := mise.MisePluginsDir("/custom/xdg")
	require.Equal(t, filepath.Join("/custom/xdg", "mise", "plugins"), dir)
}
