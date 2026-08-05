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

// --- Bootstrap plugins config ---

func TestGenerateBootstrapPlugins_emitsSection(t *testing.T) {
	got := mise.GenerateBootstrapPlugins("/home/user/.local/share/dotdrift/mise-plugins/paru")
	require.Contains(t, got, "[bootstrap.plugins]")
	require.Contains(t, got, "paru")
	require.Contains(t, got, "/home/user/.local/share/dotdrift/mise-plugins/paru")
}

func TestGenerateBootstrapPlugins_emptyWhenNoPath(t *testing.T) {
	require.Empty(t, mise.GenerateBootstrapPlugins(""))
}

// --- Plugin directory ---

func TestPluginDir_underXDGDataHome(t *testing.T) {
	dir := mise.PluginDir("/custom/xdg", "paru")
	require.Equal(t, filepath.Join("/custom/xdg", "dotdrift", "mise-plugins", "paru"), dir)
}

// --- Full bootstrap config emitter ---

func TestGenerateBootstrapConfig_packagesAndPlugins(t *testing.T) {
	got := mise.GenerateBootstrapConfig(
		[]string{"neovim"}, // install
		"paru",             // backend
		"/path/to/plugin",  // plugin path
	)
	require.Contains(t, got, "[bootstrap.packages]")
	require.Contains(t, got, `"paru:neovim" = "latest"`)
	require.Contains(t, got, "[bootstrap.plugins]")
	require.Contains(t, got, "paru")
}
