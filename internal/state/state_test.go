package state_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/state"
)

func TestState_roundTrip(t *testing.T) {
	dir := t.TempDir()
	store := state.NewFileStore(filepath.Join(dir, "state.json"))

	require.NoError(t, store.Save(&state.State{LastCompleted: "tools"}))

	loaded, err := store.Load()
	require.NoError(t, err)
	require.Equal(t, "tools", loaded.LastCompleted)
}

func TestState_loadMissingReturnsFresh(t *testing.T) {
	dir := t.TempDir()
	store := state.NewFileStore(filepath.Join(dir, "missing", "state.json"))

	s, err := store.Load()
	require.NoError(t, err)
	require.NotNil(t, s)
	require.Empty(t, s.LastCompleted)
}

func TestState_loadCorruptReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	require.NoError(t, os.WriteFile(path, []byte("not json"), 0o600))

	store := state.NewFileStore(path)
	_, err := store.Load()
	require.Error(t, err)
	require.Contains(t, err.Error(), "remove or move aside", "corrupt state error should include a recovery hint")
	require.Contains(t, err.Error(), path, "corrupt state error should name the state file")
}

func TestState_removeDeletesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	store := state.NewFileStore(path)
	require.NoError(t, store.Save(&state.State{LastCompleted: "packages"}))

	require.NoError(t, store.Remove())
	_, err := os.Stat(path)
	require.True(t, os.IsNotExist(err), "state file must be removed after Remove")
}

func TestState_removeMissingIsNil(t *testing.T) {
	dir := t.TempDir()
	store := state.NewFileStore(filepath.Join(dir, "never", "state.json"))
	require.NoError(t, store.Remove(), "Remove on a never-saved path must not error")
}

func TestProfileStatePath(t *testing.T) {
	dir := t.TempDir()
	p := state.ProfileStatePath(dir)
	require.True(t, strings.HasSuffix(p, "state.json"), "state path should end with state.json")
	require.True(t, strings.Contains(p, "profiles"), "state path should be under profiles/")
	require.NotEqual(t, p, state.ProfileStatePath(filepath.Join(dir, "other")), "different profiles should have different state paths")
}

func TestProfileStatePath_respectsXDGStateHome(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/tmp/xdg-state")
	dir := t.TempDir()
	p := state.ProfileStatePath(dir)
	require.True(t, strings.HasPrefix(p, "/tmp/xdg-state/dotdrift/"), "state path should respect XDG_STATE_HOME: %s", p)
}

func TestProfileStatePath_defaultsToLocalState(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "")
	dir := t.TempDir()
	p := state.ProfileStatePath(dir)
	require.True(t, strings.HasPrefix(p, filepath.Join(os.Getenv("HOME"), ".local", "state", "dotdrift")), "default state path should be under ~/.local/state/dotdrift: %s", p)
}

func TestProfileStatePath_resolvesSymlinks(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real-profile")
	require.NoError(t, os.Mkdir(real, 0o755))
	link := filepath.Join(dir, "linked-profile")
	require.NoError(t, os.Symlink(real, link))

	require.Equal(t, state.ProfileStatePath(real), state.ProfileStatePath(link),
		"a profile reached via a symlink should share the canonical state path")
}

func TestDefaultPath_usesXDGStateHome(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "")
	p, err := state.DefaultPath()
	require.NoError(t, err)
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".local", "state", "dotdrift", "state.json")
	require.Equal(t, want, p)
}

func TestDefaultPath_respectsXDGStateHome(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/tmp/xdg-state")
	p, err := state.DefaultPath()
	require.NoError(t, err)
	require.Equal(t, "/tmp/xdg-state/dotdrift/state.json", p)
}

func TestDefaultPath_noHomeReturnsError(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", "")
	_, err := state.DefaultPath()
	require.Error(t, err, "DefaultPath should fail explicitly when no state root is available")
	require.Contains(t, err.Error(), "XDG_STATE_HOME", "error should hint at XDG_STATE_HOME")
}
