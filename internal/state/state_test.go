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

	s := state.New()
	s.Selection = "fp1"
	s.Completed["packages"] = true
	s.Current = "tools"
	s.Status = state.StatusInProgress
	s.Error = "none"

	require.NoError(t, store.Save(s))

	loaded, err := store.Load()
	require.NoError(t, err)
	require.Equal(t, s.Selection, loaded.Selection)
	require.True(t, loaded.Completed["packages"])
	require.Equal(t, s.Current, loaded.Current)
	require.Equal(t, s.Status, loaded.Status)
	require.Equal(t, s.Error, loaded.Error)
}

func TestState_resetForSelection(t *testing.T) {
	s := state.New()
	s.Selection = "fp1"
	s.Completed["packages"] = true
	s.Current = "tools"
	s.Status = state.StatusFailed
	s.Error = "boom"

	s.ResetForSelection()
	require.Empty(t, s.Completed)
	require.Empty(t, s.Current)
	require.Empty(t, s.Error)
	require.Equal(t, state.StatusFresh, s.Status)
	require.Equal(t, "fp1", s.Selection)
}

func TestState_loadMissingReturnsFresh(t *testing.T) {
	dir := t.TempDir()
	store := state.NewFileStore(filepath.Join(dir, "missing", "state.json"))

	s, err := store.Load()
	require.NoError(t, err)
	require.NotNil(t, s)
	require.Empty(t, s.Selection)
	require.Empty(t, s.Completed)
	require.Equal(t, state.StatusFresh, s.Status)
}

func TestState_loadCorruptReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	require.NoError(t, os.WriteFile(path, []byte("not json"), 0o600))

	store := state.NewFileStore(path)
	_, err := store.Load()
	require.Error(t, err)
}

func TestState_markCompleteAndFailed(t *testing.T) {
	s := state.New()
	require.False(t, s.IsCompleted("packages"))

	s.MarkComplete("packages")
	require.True(t, s.IsCompleted("packages"))
	require.Equal(t, state.StatusInProgress, s.Status)
	require.Empty(t, s.Current)

	s.MarkFailed("tools", nil)
	require.Equal(t, "tools", s.Current)
	require.Equal(t, state.StatusFailed, s.Status)

	s.MarkCompletePipeline()
	require.Equal(t, state.StatusComplete, s.Status)
	require.Empty(t, s.Current)
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

func TestDefaultPath_usesXDGStateHome(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "")
	p := state.DefaultPath()
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".local", "state", "dotdrift", "state.json")
	require.Equal(t, want, p)
}

func TestDefaultPath_respectsXDGStateHome(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/tmp/xdg-state")
	p := state.DefaultPath()
	require.Equal(t, "/tmp/xdg-state/dotdrift/state.json", p)
}
