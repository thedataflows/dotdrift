package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/thedataflows/dotdrift/internal/drift"
	"github.com/thedataflows/dotdrift/internal/mise"
	"github.com/thedataflows/dotdrift/internal/state"
)

// StatusCmd reports drift between the resolved profile and the live system,
// plus the apply resume cursor. Read-only; exits 0 even when drift is found.
type StatusCmd struct {
	Profile string `help:"Path to profile directory" type:"existingdir" default:"."`
	State   string `help:"Path to state file" type:"path" default:""`
	Verbose bool   `help:"Show each probe as it starts ('checking <section>: <item>') on stderr" short:"v" default:"false"`
	Jobs    int    `help:"Concurrent probe workers (0 = number of CPUs)" short:"j" default:"0"`
	out     io.Writer
	err     io.Writer
}

// Run loads state, resolves the plan, probes the live system for drift, and
// prints the report. Only real errors (detect/load/resolve/corrupt state)
// return non-nil; drift is reported, never an error.
func (c *StatusCmd) Run() error {
	statePath := c.State
	if statePath == "" {
		statePath = state.ProfileStatePath(c.Profile)
	}
	store := state.NewFileStore(statePath)
	s, err := store.Load()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	f, err := detectFacts()
	if err != nil {
		return fmt.Errorf("detect: %w", err)
	}
	p, err := profileLoad(c.Profile, f)
	if err != nil {
		return fmt.Errorf("load profile: %w", err)
	}
	plan, err := resolvePlan(p, f)
	if err != nil {
		return fmt.Errorf("resolve plan: %w", err)
	}

	pr := drift.DefaultProbes()
	pr.IsInstalled = packagesFor(f.Backend).IsInstalled
	pr.ToolCurrent = mise.NewExecMise(defaultMise()).Current
	profileRoot, _ := filepath.Abs(p.Root)
	opts := drift.CheckOptions{Jobs: c.Jobs}
	if c.Verbose {
		errW := c.err
		if errW == nil {
			errW = os.Stderr
		}
		opts.Verbose = errW
	}
	findings := drift.Check(context.Background(), plan, profileRoot, pr, opts)

	out := c.out
	if out == nil {
		out = os.Stdout
	}
	fmt.Fprintf(out, "profile: %s\n", c.Profile)
	fmt.Fprintf(out, "state: %s\n", statePath)
	if s.LastCompleted == "" {
		fmt.Fprintln(out, "resume: clean — next apply starts from the beginning")
	} else {
		fmt.Fprintf(out, "resume: last completed %q — next apply resumes after it\n", s.LastCompleted)
	}
	drift.Render(out, findings)
	return nil
}
