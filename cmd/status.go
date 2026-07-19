package dotdrift

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/thedataflows/dotdrift/internal/state"
)

// StatusCmd shows the resume cursor and last error.
type StatusCmd struct {
	Profile string `help:"Path to profile directory" type:"existingdir" default:"."`
	State   string `help:"Path to state file" type:"path" default:""`
	out     io.Writer
}

// Run loads state and prints the resume cursor.
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

	out := c.out
	if out == nil {
		out = os.Stdout
	}
	fmt.Fprintf(out, "profile: %s\n", c.Profile)
	fmt.Fprintf(out, "state: %s\n", statePath)
	fmt.Fprintf(out, "selection: %s\n", s.Selection)
	fmt.Fprintf(out, "status: %s\n", s.Status)
	if s.Current != "" {
		fmt.Fprintf(out, "current: %s\n", s.Current)
	}
	if s.Error != "" {
		fmt.Fprintf(out, "error: %s\n", s.Error)
	}
	if info, err := os.Stat(statePath); err == nil {
		completed := 0
		for _, step := range pipelineStepNames {
			if s.IsCompleted(step) {
				completed++
			}
		}
		fmt.Fprintf(out, "progress: %d/%d steps\n", completed, len(pipelineStepNames))
		fmt.Fprintf(out, "updated: %s\n", info.ModTime().UTC().Format(time.RFC3339))
		if s.Status == state.StatusFailed || s.Status == state.StatusInProgress {
			if s.Current != "" {
				fmt.Fprintf(out, "next: dotdrift apply  (resumes at %s)\n", s.Current)
			} else {
				fmt.Fprintln(out, "next: dotdrift apply")
			}
		}
	}
	return nil
}
