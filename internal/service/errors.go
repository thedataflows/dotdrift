package service

import (
	"fmt"
)

// StepError is a pipeline step failure: the step that failed and why.
// Always resumable — the resume cursor names the last completed step
// (contract 2), so a rerun continues after it.
type StepError struct {
	Step string
	Err  error
}

func (e *StepError) Error() string { return fmt.Sprintf("step %s: %v", e.Step, e.Err) }
func (e *StepError) Unwrap() error { return e.Err }

// AlreadyRunningError is returned by Start when another apply holds the
// sidecar lock for the state file (contract 11). Reads stay lock-free.
type AlreadyRunningError struct {
	StatePath string
}

func (e *AlreadyRunningError) Error() string {
	return fmt.Sprintf("another apply is already running (state file %s)", e.StatePath)
}

// SessionCancelledError is the error return of Wait when the session was
// cancelled: the named step was interrupted, the cursor still names the
// last completed step, and a rerun resumes from there.
type SessionCancelledError struct {
	StepName string
}

func (e *SessionCancelledError) Error() string {
	if e.StepName == "" {
		return "apply cancelled"
	}
	return fmt.Sprintf("apply cancelled during step %s (rerun to resume)", e.StepName)
}

// SchemaError is a strict-schema load failure (a profile file that does not
// decode against its schema): the file, its 1-based line (0 when the
// failure is not localizable), and the underlying error. Error() renders
// the original wrapped string so CLI output stays byte-identical; front
// ends match with errors.As to render structure (the TUI opens broken
// files read-only, 0065-D8).
type SchemaError struct {
	Path string
	Line int
	Err  error
}

func (e *SchemaError) Error() string {
	if e.Err == nil {
		return e.Path
	}
	return e.Err.Error()
}

func (e *SchemaError) Unwrap() error { return e.Err }
