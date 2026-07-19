package dotdrift

import (
	"encoding/json"
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
	JSON    bool         `help:"Print the plan as a single JSON object (suppresses the text rendering and warnings)"`
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

	if c.JSON {
		return printPlanJSON(out, plan, p, f)
	}
	return printPlan(out, plan, p, f)
}

type planJSONDotfile struct {
	Target string `json:"target"`
	Source string `json:"source"`
	Mode   string `json:"mode"`
	Module string `json:"module"`
	Layer  string `json:"layer"`
}

type planJSONDoc struct {
	Fingerprint string   `json:"fingerprint"`
	Modules     []string `json:"modules"`
	Packages    struct {
		Install []string `json:"install"`
		Remove  []string `json:"remove"`
	} `json:"packages"`
	Tools    map[string]string `json:"tools"`
	Dotfiles []planJSONDotfile `json:"dotfiles"`
	Hooks    struct {
		Pre  []string `json:"pre"`
		Post []string `json:"post"`
	} `json:"hooks"`
}

// printPlanJSON renders the plan as one JSON object. The no-modules warning is
// intentionally omitted so stdout stays parseable by machine consumers.
func printPlanJSON(out io.Writer, plan *resolve.Plan, p *profile.Profile, f *facts.Facts) error {
	doc := planJSONDoc{
		Fingerprint: resolve.Fingerprint(p, f),
		Modules:     make([]string, 0, len(p.Selected)),
		Tools:       plan.Tools.Versions,
		Dotfiles:    make([]planJSONDotfile, 0, len(plan.Dotfiles.Entries)),
	}
	for _, m := range p.Selected {
		doc.Modules = append(doc.Modules, m.ID)
	}
	sort.Strings(doc.Modules)
	doc.Packages.Install = plan.Packages.Install
	doc.Packages.Remove = plan.Packages.Remove
	doc.Hooks.Pre = append([]string{}, plan.Hooks.Pre...)
	doc.Hooks.Post = append([]string{}, plan.Hooks.Post...)
	for _, e := range plan.Dotfiles.Entries {
		doc.Dotfiles = append(doc.Dotfiles, planJSONDotfile{
			Target: e.Target,
			Source: e.Source,
			Mode:   e.Mode,
			Module: e.Module,
			Layer:  e.Layer,
		})
	}

	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

func printPlan(out io.Writer, plan *resolve.Plan, p *profile.Profile, f *facts.Facts) error {
	if len(p.Selected) == 0 {
		fmt.Fprintln(out, "warning: no modules selected")
	}
	fmt.Fprintf(out, "fingerprint:\n%s", resolve.Fingerprint(p, f))
	fmt.Fprintln(out, "packages:")
	for _, pkg := range plan.Packages.Install {
		fmt.Fprintf(out, "  - %s\n", pkg)
	}
	fmt.Fprintln(out, "remove:")
	for _, pkg := range plan.Packages.Remove {
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
	fmt.Fprintln(out, "  pre:")
	for _, c := range plan.Hooks.Pre {
		fmt.Fprintf(out, "    - %s\n", c)
	}
	fmt.Fprintln(out, "  post:")
	for _, c := range plan.Hooks.Post {
		fmt.Fprintf(out, "    - %s\n", c)
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
