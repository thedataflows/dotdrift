package dotdrift

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/state"
)

func scopeFixture(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "testdata", "profiles", "scope")
}

// A profile with system-scope modules gains a dotfiles-system step that runs
// after dotfiles, applies only the system entries from its own config dir,
// and is recorded in resume state.
func TestApply_dotfilesSystemStep(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Backend: "paru"}
	events, _ := stubApplyDeps(t, f)

	cmd := &ApplyCmd{Profile: scopeFixture(t), State: statePath, Yes: true}
	require.NoError(t, cmd.Run())

	userApply := "dotfiles apply --cd " + filepath.Join(dir, "mise", "dotfiles")
	systemApply := "dotfiles apply --cd " + filepath.Join(dir, "mise", "dotfiles-system")
	userIdx, systemIdx := -1, -1
	for i, e := range *events {
		if strings.Contains(e, systemApply) {
			systemIdx = i
		}
		// ".../mise/dotfiles" is a path prefix of ".../mise/dotfiles-system";
		// exclude the system event when locating the user one.
		if strings.Contains(e, userApply) && !strings.Contains(e, "dotfiles-system") {
			userIdx = i
		}
	}
	require.GreaterOrEqual(t, userIdx, 0, "user dotfiles apply missing in %v", *events)
	require.Greater(t, systemIdx, userIdx, "dotfiles-system must run after dotfiles in %v", *events)

	// The per-step configs are partitioned by scope.
	userCfg, err := os.ReadFile(filepath.Join(dir, "mise", "dotfiles", "mise.toml"))
	require.NoError(t, err)
	require.Contains(t, string(userCfg), "~/.bashrc")
	require.NotContains(t, string(userCfg), "/etc/demo.conf")

	sysCfg, err := os.ReadFile(filepath.Join(dir, "mise", "dotfiles-system", "mise.toml"))
	require.NoError(t, err)
	require.Contains(t, string(sysCfg), "/etc/demo.conf")
	require.NotContains(t, string(sysCfg), "~/.bashrc")

	// The pre-pipeline full config (D8a crash snapshot) still contains everything.
	full, err := os.ReadFile(filepath.Join(dir, "mise", "mise.toml"))
	require.NoError(t, err)
	require.Contains(t, string(full), "/etc/demo.conf")
	require.Contains(t, string(full), "~/.bashrc")

	s := loadStateFile(t, statePath)
	require.Equal(t, state.StatusComplete, s.Status)
	require.True(t, s.IsCompleted("dotfiles"), "user dotfiles step must complete")
	require.True(t, s.IsCompleted("dotfiles-system"), "system dotfiles step must complete")
}

// Without system-scope entries there is no dotfiles-system step: no
// invocation, no config dir, no completed state entry.
func TestApply_noSystemEntriesSkipsDotfilesSystem(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Backend: "paru"}
	events, _ := stubApplyDeps(t, f)

	cmd := &ApplyCmd{Profile: resolveFixture(t), State: statePath, Yes: true}
	require.NoError(t, cmd.Run())

	for _, e := range *events {
		require.NotContains(t, e, "dotfiles-system", "no system step must run for a user-only plan")
	}

	s := loadStateFile(t, statePath)
	require.Equal(t, state.StatusComplete, s.Status)
	require.False(t, s.IsCompleted("dotfiles-system"))

	_, err := os.Stat(filepath.Join(dir, "mise", "dotfiles-system"))
	require.True(t, os.IsNotExist(err), "no dotfiles-system config dir must be created")
}
