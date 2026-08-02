package dotdrift

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/thedataflows/dotdrift/internal/detect"
	"github.com/thedataflows/dotdrift/internal/profile"
)

// ModulesCmd lists selected and skipped modules for a profile.
type ModulesCmd struct {
	Profile string    `help:"Path to profile directory" type:"existingdir" default:"."`
	Modules []string  `arg:"" optional:"" name:"modules" help:"Limit scope to these modules (space or comma separated)"`
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
	if err := p.LimitTo(profile.ParseModuleFilter(c.Modules)); err != nil {
		return err
	}
	out := c.Out
	if out == nil {
		out = os.Stdout
	}
	for _, m := range p.Selected {
		var tags strings.Builder
		if m.Config.Scope == profile.ScopeSystem {
			tags.WriteString(" [system]")
		}
		if m.App != m.ID {
			fmt.Fprintf(&tags, " (app: %s)", m.App)
		}
		fmt.Fprintf(out, "selected %s%s\n", m.ID, tags.String())
	}
	for _, s := range p.Skipped {
		fmt.Fprintf(out, "skipped  %s %s\n", s.Module.ID, s.Reason)
	}
	return nil
}
