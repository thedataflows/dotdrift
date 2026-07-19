package dotdrift

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/apply"
	"github.com/thedataflows/dotdrift/internal/state"
)

// blockingStep blocks mid-pipeline until released, simulating a slow apply.
type blockingStep struct {
	entered chan struct{}
	release chan struct{}
}

func (s *blockingStep) Name() string { return "packages" }
func (s *blockingStep) Run(ctx context.Context) error {
	close(s.entered)
	<-s.release
	return nil
}

// Two applies on the same state path must serialize: while the first apply
// holds the sidecar lock mid-pipeline, the second apply's Lock blocks until
// the first releases.
func TestApply_secondApplyBlocksOnSidecarLock(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")

	// First apply: lock, load, run pipeline (mirrors ApplyCmd.Run).
	first := state.NewFileStore(statePath)
	require.NoError(t, first.Lock())
	s, err := first.Load()
	require.NoError(t, err)

	step := &blockingStep{entered: make(chan struct{}), release: make(chan struct{})}
	pipeline := apply.NewPipeline([]apply.Step{step}, first.Save)
	pipeline.SetState(s)
	done := make(chan error, 1)
	go func() { done <- pipeline.Run(context.Background()) }()

	<-step.entered // first apply is now mid-pipeline with the lock held

	// Second apply on the same state path must not acquire the lock.
	second := state.NewFileStore(statePath)
	acquired := make(chan error, 1)
	go func() { acquired <- second.Lock() }()
	select {
	case err := <-acquired:
		t.Fatalf("second apply acquired sidecar lock while first apply held it: %v", err)
	case <-time.After(200 * time.Millisecond):
		// still blocked: correct
	}

	// First apply finishes and releases; the second apply may proceed.
	close(step.release)
	require.NoError(t, <-done)
	require.NoError(t, first.Unlock())

	select {
	case err := <-acquired:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("second apply did not acquire the lock after the first released it")
	}
	require.NoError(t, second.Unlock())
}
