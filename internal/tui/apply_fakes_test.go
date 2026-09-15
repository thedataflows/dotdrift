package tui

// Apply fakes for the compositor's apply-driver tests (moved from the
// deleted M14 apply_test.go in T-tui-cleanup).

import (
	"context"

	"github.com/thedataflows/dotdrift/internal/service"
)

// fakeLauncher is the consumer-side doorway, faked: previews are canned,
// Start counts calls and hands back the canned run or error.
type fakeLauncher struct {
	previews    []service.StepPreview
	previewErr  error
	startErr    error
	run         ApplyRun
	started     int
	startedOpts []service.ApplyOpts
}

func (f *fakeLauncher) Preview(service.ApplyOpts) ([]service.StepPreview, error) {
	return f.previews, f.previewErr
}

func (f *fakeLauncher) Start(_ context.Context, opts service.ApplyOpts) (ApplyRun, error) {
	f.started++
	f.startedOpts = append(f.startedOpts, opts)
	if f.startErr != nil {
		return nil, f.startErr
	}
	return f.run, nil
}

// testRun is an ApplyRun fake: a controllable (buffered) event channel
// and result.
type testRun struct {
	previews  []service.StepPreview
	events    chan service.Event
	result    *service.SessionResult
	err       error
	cancelled int
}

func newTestRun() *testRun {
	return &testRun{events: make(chan service.Event, 64)}
}

func (r *testRun) Events() <-chan service.Event          { return r.events }
func (r *testRun) Preview() []service.StepPreview        { return r.previews }
func (r *testRun) Cancel()                               { r.cancelled++ }
func (r *testRun) Wait() (*service.SessionResult, error) { return r.result, r.err }
