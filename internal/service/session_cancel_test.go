package service

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/mise"
)

// Cancel mid-handover reports Cancelled, not Failed (contract 2, 0064-D5):
// the group-killed twin surfaces as "signal: killed", which wraps no
// context error — the session must classify by its own ctx, not by the
// child's last words.
func TestSession_cancelMidHandoverReportsCancelled(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "hook.pid")
	script := filepath.Join(dir, "mise.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\necho $$ > "+pidFile+"\nsleep 30\n"), 0o755))

	deps, _ := stubSessionDeps(t, testFacts())
	deps.NewMise = func() *mise.Mise {
		return &mise.Mise{
			LookPath: func(string) (string, error) { return script, nil },
			Run: func(_ string, args ...string) (string, error) {
				for _, a := range args {
					if a == "--version" {
						return mise.MinMiseVersion + "\n", nil
					}
				}
				return "", nil
			},
			Classify: func(string) mise.InstallKind { return mise.InstallKindUserManaged },
		}
	}

	opts := baseOpts(filepath.Join("..", "..", "testdata", "profiles", "resolve"), filepath.Join(dir, "state.json"))
	opts.Sections = map[string]bool{"hooks": true}
	opts.HandoverAvailable = ptr(true)
	opts.Handover = func(cmd *exec.Cmd) error {
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		return cmd.Run()
	}

	sess, err := NewApplyArea(deps).Start(context.Background(), opts)
	require.NoError(t, err)

	go func() {
		for range sess.Events() {
		}
	}()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(pidFile); err == nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	sess.Cancel()
	res, waitErr := sess.Wait()
	require.Error(t, waitErr, "a cancelled session errors from Wait")
	var sce *SessionCancelledError
	require.ErrorAs(t, waitErr, &sce)
	require.Equal(t, "hooks-pre", sce.StepName)
	require.Equal(t, OutcomeCancelled, res.Outcome)
}
