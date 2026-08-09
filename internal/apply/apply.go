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

// SetState sets the initial state (the loaded resume cursor).
func (p *Pipeline) SetState(s *state.State) {
	p.state = s
}

// Run executes the pipeline, skipping steps through the persisted cursor.
// A cursor naming a step absent from this run (stale or from a different
// selection) is ignored: all steps run.
func (p *Pipeline) Run(ctx context.Context) error {
	skipThrough := p.state.LastCompleted
	if skipThrough != "" {
		known := false
		for _, st := range p.steps {
			if st.Name() == skipThrough {
				known = true
				break
			}
		}
		if !known {
			skipThrough = ""
		}
	}

	for _, step := range p.steps {
		if skipThrough != "" {
			if step.Name() == skipThrough {
				skipThrough = ""
			}
			continue
		}
		if err := step.Run(ctx); err != nil {
			return fmt.Errorf("step %s: %w", step.Name(), err)
		}
		p.state.LastCompleted = step.Name()
		if err := p.save(p.state); err != nil {
			return fmt.Errorf("persist cursor after %s: %w", step.Name(), err)
		}
	}
	return nil
}

// State returns the current pipeline state.
func (p *Pipeline) State() *state.State {
	return p.state
}
