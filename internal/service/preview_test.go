package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// Preview classifies a would-be session's steps WITHOUT starting one
// (the TUI plan gate's data, 0064-D4): the same buildSteps/RequiresTTY
// path Start uses — so the classification cannot drift from behavior —
// but no lock, no run goroutine, and no writes. Start afterwards still
// works and classifies identically.
func TestApplyArea_preview_classifiesWithoutStarting(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	deps, events := stubSessionDeps(t, testFacts())
	area := NewApplyArea(deps)

	opts := baseOpts(resolveFixture(t), statePath)

	previews, err := area.Preview(opts)
	require.NoError(t, err)
	require.NotEmpty(t, previews)

	// No side effects before Start: no lock file, no run-goroutine work
	// (mise ensure), nothing.
	_, lockErr := os.Stat(statePath + ".lock")
	require.Error(t, lockErr, "Preview must not take the sidecar lock")
	require.Empty(t, *events, "Preview must not run any orchestration")

	// Classification matches a real Start exactly — names, order, needs,
	// reasons (the hooks steps carry the interactive-hook reason).
	sess, err := area.Start(context.Background(), opts)
	require.NoError(t, err)
	require.Equal(t, sess.Preview(), previews, "Preview must equal the started session's classification")
	require.True(t, func() bool {
		for _, p := range previews {
			if p.Name == "hooks-pre" {
				return p.NeedsTTY
			}
		}
		return false
	}(), "hooks-pre classifies NeedsTTY: hook tasks are interactive everywhere (0104)")

	_, _ = sess.Wait()
}
