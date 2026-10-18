package mise

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/executil"
)

// A mise child streamed to real terminals must stay in the caller's
// process group — the terminal's foreground group (issue 0136). Putting
// it in its own group (SetGroupKill) backgrounds it: sudo inside mise
// then cannot own the terminal, its password read fails with EIO, and
// the typed password echoes in clear. mise's config-side interactive
// marking can only take effect when the child can actually reach the
// foreground. Piped (ForceStream/collector) children keep their own
// group so session cancel still kills the whole tree.
func TestRunOp_terminalChildSharesProcessGroup(t *testing.T) {
	orig := executil.IsTerminal
	executil.IsTerminal = func(io.Writer) bool { return true }
	t.Cleanup(func() { executil.IsTerminal = orig })

	pgidFile := filepath.Join(t.TempDir(), "pgid")
	script := filepath.Join(t.TempDir(), "child.sh")
	require.NoError(t, os.WriteFile(script, []byte(
		"#!/bin/sh\nawk '{print $5}' /proc/self/stat > \"$PGID_OUT\"\n"), 0o755))

	m := &Mise{
		Out: io.Discard, Err: io.Discard,
		Env: []string{"PGID_OUT=" + pgidFile},
	}
	_, err := m.runOp(context.Background(), nil, script)
	require.NoError(t, err)

	b, err := os.ReadFile(pgidFile)
	require.NoError(t, err)
	childPgid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	require.NoError(t, err)
	require.Equal(t, syscall.Getpgrp(), childPgid,
		"a terminal-streamed mise child must share the caller's process group")
}

func TestRunOp_pipedChildKeepsOwnProcessGroup(t *testing.T) {
	pgidFile := filepath.Join(t.TempDir(), "pgid")
	pidFile := filepath.Join(t.TempDir(), "pid")
	script := filepath.Join(t.TempDir(), "child.sh")
	require.NoError(t, os.WriteFile(script, []byte(
		"#!/bin/sh\necho $$ > \"$PID_OUT\"\nawk '{print $5}' /proc/self/stat > \"$PGID_OUT\"\n"), 0o755))

	m := &Mise{
		ForceStream: true, Out: io.Discard, Err: io.Discard,
		Env: []string{"PGID_OUT=" + pgidFile, "PID_OUT=" + pidFile},
	}
	_, err := m.runOp(context.Background(), nil, script)
	require.NoError(t, err)

	pgidB, err := os.ReadFile(pgidFile)
	require.NoError(t, err)
	pidB, err := os.ReadFile(pidFile)
	require.NoError(t, err)
	childPgid, err := strconv.Atoi(strings.TrimSpace(string(pgidB)))
	require.NoError(t, err)
	childPid, err := strconv.Atoi(strings.TrimSpace(string(pidB)))
	require.NoError(t, err)
	require.Equal(t, childPid, childPgid,
		"a piped (collector) mise child keeps its own process group for cancel")
	require.NotEqual(t, syscall.Getpgrp(), childPgid)
}
