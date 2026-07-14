package dotdrift

import (
	"fmt"
	"io"
	"os"

	"github.com/thedataflows/dotdrift/internal/detect"
	"github.com/thedataflows/dotdrift/internal/profile"
)

// ModulesCmd lists selected and skipped modules for a profile.
type ModulesCmd struct {
	Profile string    `help:"Path to profile directory" type:"existingdir" default:"."`
	Out     io.Writer `kong:"-"`
}

// Run loads the profile and prints selection status.
func (c *ModulesCmd) Run() error {
	f, err := detect.Detect()
	if err != nil {
		return fmt.Errorf("detect: %w", err)
	}
	p, err := profile.Load(c.Profile, f)
	if err != nil {
		return fmt.Errorf("load profile: %w", err)
	}
	out := c.Out
	if out == nil {
		out = os.Stdout
	}
	for _, m := range p.Selected {
		fmt.Fprintf(out, "selected %s %s\n", m.ID, m.App)
	}
	for _, s := range p.Skipped {
		fmt.Fprintf(out, "skipped  %s %s\n", s.Module.ID, s.Reason)
	}
	return nil
}
