package dotdrift

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"sort"
	"strings"

	"github.com/thedataflows/dotdrift/internal/detect"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/resolve"
	"github.com/thedataflows/dotdrift/internal/smb"
)

// PlanCmd prints the resolved plan without side effects.
type PlanCmd struct {
	Profile string       `help:"Path to profile directory" type:"existingdir" default:"."`
	JSON    bool         `help:"Print the plan as a single JSON object (suppresses the text rendering and warnings)"`
	Modules []string     `arg:"" optional:"" name:"modules" help:"Limit scope to these modules (space or comma separated)"`
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
	if err := p.LimitTo(profile.ParseModuleFilter(c.Modules)); err != nil {
		return err
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
	Scope  string `json:"scope"`
}

type planJSONMount struct {
	Module      string   `json:"module"`
	Name        string   `json:"name"`
	Source      string   `json:"source"`
	Destination string   `json:"destination"`
	Type        string   `json:"type"`
	Options     []string `json:"options"`
	StartAt     string   `json:"startat"`
	State       string   `json:"state"`
	Layer       string   `json:"layer"`
	Scope       string   `json:"scope"`
}

type planJSONShare struct {
	Path       string `json:"path"`
	Comment    string `json:"comment"`
	ValidUsers string `json:"valid_users"`
	Writable   bool   `json:"writable"`
	Public     bool   `json:"public"`
}

type planJSONSmb struct {
	Module string                   `json:"module"`
	Group  string                   `json:"group"`
	Users  []string                 `json:"users"`
	Avahi  *bool                    `json:"avahi"`
	Shares map[string]planJSONShare `json:"shares"`
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
	Mounts []planJSONMount `json:"mounts"`
	Smb    []planJSONSmb   `json:"smb"`
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
			Scope:  e.Scope,
		})
	}
	doc.Mounts = make([]planJSONMount, 0, len(plan.Mounts.Entries))
	for _, e := range plan.Mounts.Entries {
		doc.Mounts = append(doc.Mounts, planJSONMount{
			Module:      e.Module,
			Name:        e.Name,
			Source:      e.Spec.Source,
			Destination: e.Spec.Destination,
			Type:        e.Spec.Type,
			Options:     append([]string{}, e.Spec.Options...),
			StartAt:     e.Spec.StartAt,
			State:       e.Spec.State,
			Layer:       e.Layer,
			Scope:       e.Scope,
		})
	}
	doc.Smb = make([]planJSONSmb, 0, len(plan.Smb.Modules))
	for _, m := range plan.Smb.Modules {
		jm := planJSONSmb{
			Module: m.Module,
			Group:  m.Spec.Group,
			Users:  append([]string{}, m.Spec.Users...),
			Avahi:  m.Spec.Avahi,
			Shares: make(map[string]planJSONShare, len(m.Spec.Shares)),
		}
		for name, share := range m.Spec.Shares {
			jm.Shares[name] = planJSONShare{
				Path:       share.Path,
				Comment:    share.Comment,
				ValidUsers: share.ValidUsers,
				Writable:   share.Writable,
				Public:     share.Public,
			}
		}
		doc.Smb = append(doc.Smb, jm)
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
		// System-scope entries are marked on the module line; user scope is
		// the default and stays unmarked.
		marker := ""
		if e.Scope == profile.ScopeSystem {
			marker = " [system]"
		}
		fmt.Fprintf(out, "    module: %s%s\n", e.Module, marker)
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
	// Mounts and smb sections render last and are omitted entirely when the
	// profile declares neither, keeping output for other profiles stable.
	if len(plan.Mounts.Entries) > 0 {
		fmt.Fprintln(out, "mounts:")
		for _, e := range plan.Mounts.Entries {
			fmt.Fprintf(out, "  %s: %s %s %s -> %s [%s][%s]",
				e.Module, e.Name, e.Spec.Type, e.Spec.Source, e.Spec.Destination, e.Layer, e.Scope)
			if e.Spec.StartAt != "" {
				fmt.Fprintf(out, " startat=%s", e.Spec.StartAt)
			}
			if e.Spec.State != "" {
				fmt.Fprintf(out, " state=%s", e.Spec.State)
			}
			fmt.Fprintln(out)
		}
	}
	if len(plan.Smb.Modules) > 0 {
		fmt.Fprintln(out, "smb:")
		for _, m := range plan.Smb.Modules {
			fmt.Fprintf(out, "  %s:\n", m.Module)
			// Group and avahi render the effective activation values, not the
			// raw spec: an unset group activates as smb.DefaultGroup and an
			// unset avahi defaults to enabled.
			group := m.Spec.Group
			if group == "" {
				group = smb.DefaultGroup
			}
			fmt.Fprintf(out, "    group: %s\n", group)
			fmt.Fprintf(out, "    users: %s\n", strings.Join(m.Spec.Users, ", "))
			fmt.Fprintf(out, "    avahi: %t\n", m.Spec.Avahi == nil || *m.Spec.Avahi)
			if len(m.Spec.Shares) > 0 {
				fmt.Fprintln(out, "    shares:")
				for _, name := range slices.Sorted(maps.Keys(m.Spec.Shares)) {
					fmt.Fprintf(out, "      %s -> %s\n", name, m.Spec.Shares[name].Path)
				}
			}
		}
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
