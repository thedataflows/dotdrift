package apply

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/state"
)

// fakeStep is a Step that records its runs, optionally failing.
type fakeStep struct {
	name string
	err  error
	ran  *[]string
}

func (s *fakeStep) Name() string { return s.name }
func (s *fakeStep) Run(context.Context) error {
	*s.ran = append(*s.ran, s.name)
	return s.err
}

// recordingObserver records the observer callbacks verbatim.
type recordingObserver struct {
	events *[]string
}

func (o *recordingObserver) StepStarted(name string) { *o.events = append(*o.events, "start:"+name) }
func (o *recordingObserver) StepFinished(name string) {
	*o.events = append(*o.events, "finish:"+name)
}
func (o *recordingObserver) StepFailed(name string, err error) {
	*o.events = append(*o.events, "fail:"+name+":"+err.Error())
}

// A pipeline with an observer fires start/finish per completed step and
// start/fail for the failing one; skipped steps never fire it.
func TestPipeline_observerFiresPerExecutedStep(t *testing.T) {
	ran := &[]string{}
	boom := errors.New("boom")
	steps := []Step{
		&fakeStep{name: "a", ran: ran},
		&fakeStep{name: "b", err: boom, ran: ran},
		&fakeStep{name: "c", ran: ran},
	}
	saves := 0
	p := NewPipeline(steps, func(*state.State) error { saves++; return nil })
	evs := &[]string{}
	p.SetObserver(&recordingObserver{events: evs})

	err := p.Run(context.Background())
	require.ErrorIs(t, err, boom)
	require.Equal(t, []string{
		"start:a", "finish:a",
		"start:b", "fail:b:boom",
	}, *evs)
	require.Equal(t, 1, saves, "cursor saved once, after the completed step")
	require.Equal(t, "a", p.State().LastCompleted)
}

// Resumed-through steps are invisible to the observer: a cursor naming a
// starts the stream at the following step.
func TestPipeline_observerSkipsResumedSteps(t *testing.T) {
	ran := &[]string{}
	steps := []Step{
		&fakeStep{name: "a", ran: ran},
		&fakeStep{name: "b", ran: ran},
	}
	p := NewPipeline(steps, func(*state.State) error { return nil })
	p.SetState(&state.State{LastCompleted: "a"})
	evs := &[]string{}
	p.SetObserver(&recordingObserver{events: evs})

	require.NoError(t, p.Run(context.Background()))
	require.Equal(t, []string{"start:b", "finish:b"}, *evs)
	require.Equal(t, []string{"b"}, *ran)
}

// A cursor-persist failure rolls the in-memory cursor back to the last
// step that actually completed: the reported cursor never names a step
// the disk does not (contract 2).
func TestPipeline_saveFailureKeepsCursorHonest(t *testing.T) {
	ran := &[]string{}
	steps := []Step{
		&fakeStep{name: "a", ran: ran},
		&fakeStep{name: "b", ran: ran},
	}
	persistErr := errors.New("disk full")
	p := NewPipeline(steps, func(*state.State) error { return persistErr })
	evs := &[]string{}
	p.SetObserver(&recordingObserver{events: evs})

	err := p.Run(context.Background())
	require.ErrorIs(t, err, persistErr)
	require.Empty(t, p.State().LastCompleted, "cursor must not name a step that was never persisted")
	require.Equal(t, []string{
		"start:a", "fail:a:disk full",
	}, *evs)
}
