package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/thedataflows/dotdrift/internal/executil"
	"github.com/thedataflows/dotdrift/internal/palette"
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
// color gating (executil.ColorEnabled). Hues come from the palette
// (internal/palette), so dotdrift.toml [colors] overrides apply here too.

// Run loads the profile and prints selection status.
func (c *ModulesCmd) Run() error {
	_, p, err := loadProfile(c.Profile, c.Modules)
	if err != nil {
		return err
	}
	out := c.Out
	if out == nil {
		out = os.Stdout
	}
	pal, err := palette.FromConfig(p.Config.Colors)
	if err != nil {
		return err // already validated at load; unreachable double-check
	}
	color := executil.ColorEnabled(out)
	selected, skipped := "+", "-"
	if color {
		selected = pal.Wrap(palette.OK, "+")
		skipped = pal.Wrap(palette.Error, "-")
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
		writeDescription(&b, m.Config.Description, color, pal)
		fmt.Fprintln(out, b.String())
	}
	for _, s := range p.Skipped {
		var b strings.Builder
		reason := s.Reason
		if color {
			reason = pal.Wrap(palette.Error, reason)
		}
		fmt.Fprintf(&b, "%s %s %s", skipped, s.Module.ID, reason)
		writeDescription(&b, s.Module.Config.Description, color, pal)
		fmt.Fprintln(out, b.String())
	}
	return nil
}

// writeDescription appends a module's description after an em-dash separator
// (the same separator the drift report uses for detail) when one is set. On a
// TTY the whole suffix is dimmed grey so the description reads as secondary
// to the colored +/- status marker.
func writeDescription(b *strings.Builder, desc string, color bool, pal *palette.Palette) {
	if desc == "" {
		return
	}
	if color {
		fmt.Fprintf(b, " %s", pal.Wrap(palette.Dim, "- "+desc))
		return
	}
	fmt.Fprintf(b, " - %s", desc)
}
