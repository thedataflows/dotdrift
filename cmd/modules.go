package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/thedataflows/dotdrift/internal/detect"
	"github.com/thedataflows/dotdrift/internal/executil"
	"github.com/thedataflows/dotdrift/internal/profile"
)

// ModulesCmd lists selected and skipped modules for a profile.
type ModulesCmd struct {
	Profile string    `help:"Path to profile directory" type:"existingdir" default:"."`
	Modules []string  `arg:"" optional:"" name:"modules" help:"Limit scope to these modules (space or comma separated)"`
	Out     io.Writer `kong:"-"`
}

// Selection status markers. `+`/`-` are colored (green/red) only on a TTY;
// plain when piped or under --no-color, matching the rest of dotdrift's
// color gating (executil.ColorEnabled).
const (
	ansiReset = "\033[0m"
	ansiGreen = "\033[32m"
	ansiRed   = "\033[31m"
	ansiGrey  = "\033[90m" // bright black — used to dim the description
)

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
	color := executil.ColorEnabled(out)
	selected, skipped := "+", "-"
	if color {
		selected = ansiGreen + "+" + ansiReset
		skipped = ansiRed + "-" + ansiReset
	}
	for _, m := range p.Selected {
		var b strings.Builder
		fmt.Fprintf(&b, "%s %s", selected, m.ID)
		if m.Config.Scope == profile.ScopeSystem {
			b.WriteString(" [system]")
		}
		if m.App != m.ID {
			fmt.Fprintf(&b, " (app: %s)", m.App)
		}
		writeDescription(&b, m.Config.Description, color)
		fmt.Fprintln(out, b.String())
	}
	for _, s := range p.Skipped {
		var b strings.Builder
		reason := s.Reason
		if color {
			reason = ansiRed + reason + ansiReset
		}
		fmt.Fprintf(&b, "%s %s %s", skipped, s.Module.ID, reason)
		writeDescription(&b, s.Module.Config.Description, color)
		fmt.Fprintln(out, b.String())
	}
	return nil
}

// writeDescription appends a module's description after an em-dash separator
// (the same separator the drift report uses for detail) when one is set. On a
// TTY the whole suffix is dimmed grey so the description reads as secondary
// to the colored +/- status marker.
func writeDescription(b *strings.Builder, desc string, color bool) {
	if desc == "" {
		return
	}
	if color {
		fmt.Fprintf(b, " %s— %s%s", ansiGrey, desc, ansiReset)
		return
	}
	fmt.Fprintf(b, " — %s", desc)
}
