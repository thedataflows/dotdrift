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

func TestStatus_defaultsToDefaultStatePath(t *testing.T) {
	var buf bytes.Buffer
	cmd := &StatusCmd{out: &buf}
	require.NoError(t, cmd.Run())
	require.Contains(t, buf.String(), "state: "+state.DefaultPath())
}
