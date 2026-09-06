package mise_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/mise"
	"github.com/thedataflows/dotdrift/internal/resolve"
)

// The tools step activates the resolved tools globally by writing them into
// mise's conf.d fragment (issue 0037): mise loads ~/.config/mise/conf.d/*.toml
// as global config, so installed tools resolve on PATH — without touching
// config.toml, which a profile may manage as a dotfile.

func TestToolsStep_writesGlobalActivationFragment(t *testing.T) {
	fr := &mise.FakeRunner{}
	plan := &resolve.Plan{Tools: resolve.ToolsStep{Versions: map[string]string{"node": "20"}}}
	frag := filepath.Join(t.TempDir(), "conf.d", "dotdrift.toml")
	step := &mise.ToolsStep{
		Runner: fr, Plan: plan,
		ConfigPath:   filepath.Join(t.TempDir(), "mise.toml"),
		FragmentPath: frag,
	}

	require.NoError(t, step.Run(context.Background()))

	require.True(t, fr.InstallCalled)
	b, err := os.ReadFile(frag)
	require.NoError(t, err, "the fragment must exist after a successful install")
	require.Contains(t, string(b), "[tools]")
	require.Contains(t, string(b), `node = "20"`)
	require.Contains(t, string(b), "dotdrift", "the fragment is marked as generated")
}

func TestToolsStep_emptyPlanRemovesStaleFragment(t *testing.T) {
	frag := filepath.Join(t.TempDir(), "conf.d", "dotdrift.toml")
	require.NoError(t, os.MkdirAll(filepath.Dir(frag), 0o755))
	require.NoError(t, os.WriteFile(frag, []byte("[tools]\nnode = \"20\"\n"), 0o644))
	fr := &mise.FakeRunner{}
	step := &mise.ToolsStep{Runner: fr, Plan: &resolve.Plan{}, FragmentPath: frag}

	require.NoError(t, step.Run(context.Background()))

	_, err := os.Stat(frag)
	require.ErrorIs(t, err, os.ErrNotExist,
		"a stale fragment keeps removed tools active — it must be removed")
	require.False(t, fr.InstallCalled, "nothing to install")
}

func TestToolsStep_noFragmentPathSkipsWrite(t *testing.T) {
	fr := &mise.FakeRunner{}
	plan := &resolve.Plan{Tools: resolve.ToolsStep{Versions: map[string]string{"node": "20"}}}
	step := &mise.ToolsStep{Runner: fr, Plan: plan, ConfigPath: filepath.Join(t.TempDir(), "mise.toml")}

	require.NoError(t, step.Run(context.Background()))
	require.True(t, fr.InstallCalled)
}

func TestToolsFragmentPath_honorsXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	require.Equal(t, "/xdg/mise/conf.d/dotdrift.toml", mise.ToolsFragmentPath())

	t.Setenv("XDG_CONFIG_HOME", "")
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(home, ".config", "mise", "conf.d", "dotdrift.toml"), mise.ToolsFragmentPath())
}
