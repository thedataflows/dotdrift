package dotdrift

import (
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/thedataflows/dotdrift/internal/detect"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/resolve"
)

// PlanCmd prints the resolved plan without side effects.
type PlanCmd struct {
	Profile string       `help:"Path to profile directory" type:"existingdir" default:"."`
	Facts   *facts.Facts `kong:"-"`
	Out     io.Writer    `kong:"-"`
}

// Run loads the profile and prints the resolved plan.
func (c *PlanCmd) Run() error {
	f := c.Facts
	if f == nil {
		var err error
		f, err = detect.Detect()
		if err != nil {
			return fmt.Errorf("detect: %w", err)
		}
	}

	p, err := profile.Load(c.Profile, f)
	if err != nil {
		return fmt.Errorf("load profile: %w", err)
	}

	plan, err := resolve.Resolve(p, f)
	if err != nil {
		return fmt.Errorf("resolve plan: %w", err)
	}

	out := c.Out
	if out == nil {
		out = os.Stdout
	}

	return printPlan(out, plan, p, f)
}

func printPlan(out io.Writer, plan *resolve.Plan, p *profile.Profile, f *facts.Facts) error {
	fmt.Fprintf(out, "fingerprint:\n%s", resolve.Fingerprint(p, f))
	fmt.Fprintln(out, "packages:")
	for _, pkg := range plan.Packages.Install {
		fmt.Fprintf(out, "  - %s\n", pkg)
	}
	fmt.Fprintln(out, "tools:")
	for _, k := range sortedKeys(plan.Tools.Versions) {
		fmt.Fprintf(out, "  %s: %s\n", k, plan.Tools.Versions[k])
	}
	fmt.Fprintln(out, "dotfiles:")
	for _, e := range plan.Dotfiles.Entries {
		fmt.Fprintf(out, "  %s:\n", e.Target)
		fmt.Fprintf(out, "    source: %s\n", e.Source)
		fmt.Fprintf(out, "    mode: %s\n", e.Mode)
		fmt.Fprintf(out, "    module: %s\n", e.Module)
		fmt.Fprintf(out, "    layer: %s\n", e.Layer)
	}
	fmt.Fprintln(out, "hooks:")
	if len(plan.Hooks.Pre) == 0 && len(plan.Hooks.Post) == 0 {
		fmt.Fprintln(out, "  (none)")
	}
	return nil
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
