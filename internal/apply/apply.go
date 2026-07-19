// Package apply orchestrates the dotdrift pipeline with always-resume semantics.
package apply

import (
	"context"
	"fmt"

	"github.com/thedataflows/dotdrift/internal/state"
)

// Step is a single stage in the apply pipeline.
type Step interface {
	Name() string
	Run(ctx context.Context) error
}

// Pipeline runs a list of steps, resuming from the persisted state.
type Pipeline struct {
	steps []Step
	state *state.State
	save  func(*state.State) error
}

// NewPipeline constructs a pipeline with the given steps and a save callback.
func NewPipeline(steps []Step, save func(*state.State) error) *Pipeline {
	return &Pipeline{
		steps: steps,
		state: state.New(),
		save:  save,
	}
}

// SetState sets the initial state. It should be reset for selection changes before calling.
func (p *Pipeline) SetState(s *state.State) {
	p.state = s
}

// Run executes the pipeline from the first incomplete step.
func (p *Pipeline) Run(ctx context.Context) error {
	if p.state.Status == state.StatusComplete {
		// Always rerun full pipeline on success; preserve selection fingerprint.
		selection := p.state.Selection
		p.state = state.New()
		p.state.Selection = selection
	}
	p.state.Status = state.StatusInProgress

	for _, step := range p.steps {
		if p.state.IsCompleted(step.Name()) {
			continue
		}
		p.state.Current = step.Name()
		if err := p.save(p.state); err != nil {
			return fmt.Errorf("persist state before %s: %w", step.Name(), err)
		}
		if err := step.Run(ctx); err != nil {
			p.state.MarkFailed(step.Name(), err)
			_ = p.save(p.state)
			return fmt.Errorf("step %s: %w", step.Name(), err)
		}
		p.state.MarkComplete(step.Name())
		if err := p.save(p.state); err != nil {
			return fmt.Errorf("persist state after %s: %w", step.Name(), err)
		}
	}

	p.state.MarkCompletePipeline()
	if err := p.save(p.state); err != nil {
		return fmt.Errorf("persist final state: %w", err)
	}
	return nil
}

// State returns the current pipeline state.
func (p *Pipeline) State() *state.State {
	return p.state
}
