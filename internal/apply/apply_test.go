package apply_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/apply"
	"github.com/thedataflows/dotdrift/internal/state"
)

type fakeStep struct {
	name string
	run  func() error
	runs int
}

func (s *fakeStep) Name() string { return s.name }
func (s *fakeStep) Run(ctx context.Context) error {
	s.runs++
	if s.run != nil {
		return s.run()
	}
	return nil
}

func threeSteps(order *[]string) []apply.Step {
	return []apply.Step{
		&fakeStep{name: "packages", run: func() error { *order = append(*order, "packages"); return nil }},
		&fakeStep{name: "tools", run: func() error { *order = append(*order, "tools"); return nil }},
		&fakeStep{name: "dotfiles", run: func() error { *order = append(*order, "dotfiles"); return nil }},
	}
}

func TestStepOrder_packagesToolsDotfiles(t *testing.T) {
	var order []string
	var saved *state.State
	pipeline := apply.NewPipeline(threeSteps(&order), func(s *state.State) error { saved = s; return nil })
	pipeline.SetState(state.New())
	require.NoError(t, pipeline.Run(context.Background()))
	require.Equal(t, []string{"packages", "tools", "dotfiles"}, order)
	require.Equal(t, "dotfiles", saved.LastCompleted)
}

func TestApply_resumesAfterCursor(t *testing.T) {
	var order []string
	pipeline := apply.NewPipeline(threeSteps(&order), func(*state.State) error { return nil })
	pipeline.SetState(&state.State{LastCompleted: "packages"})
	require.NoError(t, pipeline.Run(context.Background()))
	// packages is skipped (it is the cursor); tools and dotfiles run.
	require.Equal(t, []string{"tools", "dotfiles"}, order)
}

func TestApply_unknownCursorRunsAllSteps(t *testing.T) {
	var order []string
	pipeline := apply.NewPipeline(threeSteps(&order), func(*state.State) error { return nil })
	pipeline.SetState(&state.State{LastCompleted: "bogus"})
	require.NoError(t, pipeline.Run(context.Background()))
	require.Equal(t, []string{"packages", "tools", "dotfiles"}, order)
}

func TestApply_failureKeepsCursorOnDisk(t *testing.T) {
	boom := errors.New("boom")
	steps := []apply.Step{
		&fakeStep{name: "packages"},
		&fakeStep{name: "tools", run: func() error { return boom }},
	}

	store := state.NewFileStore(filepath.Join(t.TempDir(), "state.json"))
	pipeline := apply.NewPipeline(steps, store.Save)
	pipeline.SetState(state.New())
	err := pipeline.Run(context.Background())
	require.ErrorIs(t, err, boom)

	onDisk, err := store.Load()
	require.NoError(t, err)
	require.Equal(t, "packages", onDisk.LastCompleted, "cursor must name the last successful step")
}

func TestApply_stateAccessor(t *testing.T) {
	var order []string
	pipeline := apply.NewPipeline(threeSteps(&order), func(*state.State) error { return nil })
	pipeline.SetState(state.New())
	require.Empty(t, pipeline.State().LastCompleted)
	require.NoError(t, pipeline.Run(context.Background()))
	require.Equal(t, "dotfiles", pipeline.State().LastCompleted)
}
