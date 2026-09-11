package service

import (
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/resolve"
)

// Event is one item of the apply session's closed vocabulary (0064-D3).
// Consumers type-switch on the concrete types; semantics come from step
// boundaries, hook indexes and exit codes — never from parsing mise
// output.
type Event interface{ event() }

// SessionOutcome is how an apply session ended.
type SessionOutcome string

const (
	// OutcomeCompleted: every selected step ran; the state file was removed.
	OutcomeCompleted SessionOutcome = "completed"
	// OutcomeFailed: a step failed; the cursor names the last completed step.
	OutcomeFailed SessionOutcome = "failed"
	// OutcomeCancelled: Cancel() fired; the cursor names the last completed step.
	OutcomeCancelled SessionOutcome = "cancelled"
)

// PlanResolved opens the event stream: the resolved plan plus the loaded
// resume cursor and whether it is effective for this selection (a stale
// cursor — one naming a step absent from this run — is ignored, contract 2).
// Profile and Facts ride along so the CLI adapter can render the plan
// exactly as printPlan does (0061-D5).
type PlanResolved struct {
	Plan            *resolve.Plan
	Profile         *profile.Profile
	Facts           *facts.Facts
	Cursor          string
	CursorEffective bool
}

func (PlanResolved) event() {}

// BackupTaken reports one module's copy-mode backup generation (0064-D3:
// one event per receiving module directory, when ApplyOpts.Backup is set).
// Count is the number of destinations actually written — backup.Run skips
// nonexistent destinations, so it can be less than len(Files) (the CLI's
// summary line prints this count, byte-parity with the pre-session output).
type BackupTaken struct {
	Dir   string   // the backups/<generation>/ directory written
	Files []string // the copy destinations the run requested
	Count int      // destinations actually written (Run's return)
}

func (BackupTaken) event() {}

// SubStep identifies one hook command inside a hooks step. Sub stays nil
// until real steps surface per-command boundaries (issue 0071) — no
// event lies about granularity that does not exist yet.
type SubStep struct {
	Index, Total int
	Command      string
}

// StepStarted announces a step. NeedsTTY is true when the step will hand
// the terminal to a child; Index is 0-based of Total steps.
type StepStarted struct {
	Name     string
	Index    int
	Total    int
	NeedsTTY bool
	Sub      *SubStep
}

func (StepStarted) event() {}

// StepOutput carries one line of a child's output in event mode (D2:
// ApplyOpts.Output == nil). Colorless by construction — the child is
// piped, so it emits no ANSI.
type StepOutput struct {
	Name  string
	Chunk []byte
}

func (StepOutput) event() {}

// StepFinished marks a step completed and persisted (cursor advanced).
type StepFinished struct {
	Name string
}

func (StepFinished) event() {}

// StepFailed marks a failed step. The session ends Failed right after;
// the cursor names the last completed step.
type StepFailed struct {
	Name string
	Err  *StepError
}

func (StepFailed) event() {}

// SessionEnded closes the event stream. ResumeCursor is "" on Completed
// (the state file was removed); otherwise it names the last completed
// step for the next run.
type SessionEnded struct {
	Outcome      SessionOutcome
	Err          *StepError
	ResumeCursor string
}

func (SessionEnded) event() {}

// StepPreview is one step's up-front TTY classification (0064-D4): what
// the plan-view gate shows before Start's goroutine runs anything.
type StepPreview struct {
	Name     string
	NeedsTTY bool
	Reason   string
}
