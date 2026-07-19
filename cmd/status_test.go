package dotdrift

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/state"
)

func TestStatus_showsState(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	store := state.NewFileStore(statePath)
	s := state.New()
	s.Selection = "abc123"
	s.Status = state.StatusInProgress
	s.Current = "packages"
	s.Error = "network down"
	require.NoError(t, store.Save(s))

	var buf bytes.Buffer
	cmd := &StatusCmd{Profile: dir, State: statePath, out: &buf}
	require.NoError(t, cmd.Run())

	out := buf.String()
	require.Contains(t, out, "profile: "+dir)
	require.Contains(t, out, "state: "+statePath)
	require.Contains(t, out, "selection: abc123")
	require.Contains(t, out, "status: in-progress")
	require.Contains(t, out, "current: packages")
	require.Contains(t, out, "error: network down")
	require.True(t, strings.HasPrefix(out, "profile:"))
}

func TestStatus_defaultsToProfileStatePath(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	cmd := &StatusCmd{Profile: dir, out: &buf}
	require.NoError(t, cmd.Run())
	require.Contains(t, buf.String(), "state: "+state.ProfileStatePath(dir))
}

// Failed run with two completed steps: status reports progress against the
// pipeline step list, the state file's mtime, and a resume hint.
func TestStatus_showsProgressUpdatedAndNext(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	store := state.NewFileStore(statePath)
	s := state.New()
	s.Selection = "abc123"
	s.Status = state.StatusFailed
	s.Current = "dotfiles"
	s.Error = "boom"
	s.Completed["packages"] = true
	s.Completed["tools"] = true
	require.NoError(t, store.Save(s))

	var buf bytes.Buffer
	cmd := &StatusCmd{Profile: dir, State: statePath, out: &buf}
	require.NoError(t, cmd.Run())

	out := buf.String()
	t.Log(out)
	require.Contains(t, out, "progress: 2/3 steps")
	require.Contains(t, out, "updated: ")
	require.Contains(t, out, "next: dotdrift apply  (resumes at dotfiles)")
}

// Complete run: full progress, mtime, and no next-step hint.
func TestStatus_completePrintsFullProgressWithoutNext(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	store := state.NewFileStore(statePath)
	s := state.New()
	s.Selection = "abc123"
	s.Status = state.StatusComplete
	for _, step := range []string{"packages", "tools", "dotfiles", "hooks"} {
		s.Completed[step] = true
	}
	require.NoError(t, store.Save(s))

	var buf bytes.Buffer
	cmd := &StatusCmd{Profile: dir, State: statePath, out: &buf}
	require.NoError(t, cmd.Run())

	out := buf.String()
	t.Log(out)
	require.Contains(t, out, "progress: 3/3 steps")
	require.Contains(t, out, "updated: ")
	require.NotContains(t, out, "next:")
}
