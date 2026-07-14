package apply_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/apply"
	"github.com/thedataflows/dotdrift/internal/state"
)

type fakeStep struct {
	name  string
	run   func() error
	runs  int
}

func (s *fakeStep) Name() string { return s.name }
func (s *fakeStep) Run(ctx context.Context) error {
	s.runs++
	if s.run != nil {
		return s.run()
	}
	return nil
}

func TestStepOrder(t *testing.T) {
	var order []string
	steps := []apply.Step{
		&fakeStep{name: "resolve", run: func() error { order = append(order, "resolve"); return nil }},
		&fakeStep{name: "packages", run: func() error { order = append(order, "packages"); return nil }},
		&fakeStep{name: "tools", run: func() error { order = append(order, "tools"); return nil }},
		&fakeStep{name: "dotfiles", run: func() error { order = append(order, "dotfiles"); return nil }},
		&fakeStep{name: "hooks", run: func() error { order = append(order, "hooks"); return nil }},
	}

	var saved *state.State
	pipeline := apply.NewPipeline(steps, func(s *state.State) error { saved = s; return nil })
	pipeline.SetState(state.New())
	require.NoError(t, pipeline.Run(context.Background()))
	require.Equal(t, []string{"resolve", "packages", "tools", "dotfiles", "hooks"}, order)
	require.Equal(t, state.StatusComplete, saved.Status)
}

func TestApply_continuesAfterFailure(t *testing.T) {
	boom := errors.New("boom")
	steps := []apply.Step{
		&fakeStep{name: "packages"},
		&fakeStep{name: "tools", run: func() error { return boom }},
		&fakeStep{name: "dotfiles"},
	}

	s := state.New()
	s.Completed["packages"] = true

	pipeline := apply.NewPipeline(steps, func(*state.State) error { return nil })
	pipeline.SetState(s)
	err := pipeline.Run(context.Background())
	require.Error(t, err)
	require.ErrorIs(t, err, boom)

	final := pipeline.State()
	require.Equal(t, "tools", final.Current)
	require.Equal(t, state.StatusFailed, final.Status)
	require.True(t, final.IsCompleted("packages"))
	require.False(t, final.IsCompleted("tools"))
}

func TestApply_successRerunsFullPipeline(t *testing.T) {
	step := &fakeStep{name: "packages"}
	pipeline := apply.NewPipeline([]apply.Step{step}, func(*state.State) error { return nil })

	pipeline.SetState(state.New())
	require.NoError(t, pipeline.Run(context.Background()))
	require.Equal(t, 1, step.runs)
	require.Equal(t, state.StatusComplete, pipeline.State().Status)

	// After a successful run, a fresh pipeline should rerun the step.
	pipeline2 := apply.NewPipeline([]apply.Step{step}, func(*state.State) error { return nil })
	pipeline2.SetState(pipeline.State())
	require.NoError(t, pipeline2.Run(context.Background()))
	require.Equal(t, 2, step.runs)
}
