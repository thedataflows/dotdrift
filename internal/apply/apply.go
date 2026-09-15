// Package apply orchestrates the dotdrift pipeline with always-resume semantics.
package apply

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/thedataflows/dotdrift/internal/state"
)

// Step is a single stage in the apply pipeline.
type Step interface {
	Name() string
	Run(ctx context.Context) error
}

// OverwriteStep is implemented by steps that can name the existing
// destinations they will overwrite (M15's destructive-apply confirm).
// Reads are pure predicates (stat), matching RequiresTTY's
// classification-time contract.
type OverwriteStep interface {
	Step
	OverwriteTargets() []string
}

// Observer receives pipeline step transitions. Callbacks fire on the
// pipeline's run goroutine, in order, only for steps that actually ran —
// steps skipped through the resume cursor never fire it. The hook
// callbacks carry per-command boundaries inside a hook step (issue 0071);
// a hook failure is announced there before the step-level StepFailed.
type Observer interface {
	StepStarted(name string)
	StepFinished(name string)
	StepFailed(name string, err error)
	HookStarted(step string, sub SubStep)
	HookFailed(step string, sub SubStep, err error)
}

// SubStep identifies one command inside a hook step's command sequence.
type SubStep struct {
	Index, Total int
	Command      string
}

// observerSetter is the optional injection a Step may implement to receive
// the pipeline's observer (only the hook step needs it today).
type observerSetter interface {
	SetObserver(Observer)
}

// HandoverFunc runs one child command under the consumer's real terminal
// (0064-D9): the consumer wires the cmd's stdio, runs it synchronously —
// return = the step is over — and reports the exec outcome. The TUI
// bridges to tea.ExecProcess; the CLI wires os.Stdin/out/err and runs.
type HandoverFunc func(*exec.Cmd) error

// HandoverStep is a Step whose work includes a child command that must
// reach the controlling terminal — a sudo prompt, an interactive hook.
// The session injects its HandoverFunc before Run; the cmd the step
// builds has Path/Args/Env/Dir set, and the session's wrapper enforces
// Setpgid plus nil stdio (the consumer wires it). A handover error fails
// the step like any other: resumable, cursor untouched.
type HandoverStep interface {
	Step
	// RequiresTTY returns a non-empty reason when the step must run a
	// child under the consumer's terminal (the up-front classification,
	// 0064-D4).
	RequiresTTY() string
	// SetHandover injects the consumer's handover callback before Run.
	SetHandover(HandoverFunc)
}

// Pipeline runs a list of steps, resuming from the persisted state.
type Pipeline struct {
	steps []Step
	state *state.State
	save  func(*state.State) error
	obs   Observer
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

// SetObserver attaches a step-transition observer (nil = none).
func (p *Pipeline) SetObserver(o Observer) {
	p.obs = o
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
		if p.obs != nil {
			p.obs.StepStarted(step.Name())
			// Steps that report their own sub-boundaries (the hook step)
			// get the observer injected before they run.
			if obsSetter, ok := step.(observerSetter); ok {
				obsSetter.SetObserver(p.obs)
			}
		}
		prev := p.state.LastCompleted
		if err := step.Run(ctx); err != nil {
			if p.obs != nil {
				p.obs.StepFailed(step.Name(), err)
			}
			return fmt.Errorf("step %s: %w", step.Name(), err)
		}
		p.state.LastCompleted = step.Name()
		if err := p.save(p.state); err != nil {
			// The cursor on disk still names the previous step — carry and
			// report exactly that (contract 2: it names the last step that
			// COMPLETED, and a persist failure means this one did not).
			p.state.LastCompleted = prev
			if p.obs != nil {
				p.obs.StepFailed(step.Name(), err)
			}
			return fmt.Errorf("persist cursor after %s: %w", step.Name(), err)
		}
		if p.obs != nil {
			p.obs.StepFinished(step.Name())
		}
	}
	return nil
}

// State returns the current pipeline state.
func (p *Pipeline) State() *state.State {
	return p.state
}
