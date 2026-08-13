package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"sort"
	"strings"

	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/packages"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/resolve"
	"github.com/thedataflows/dotdrift/internal/smb"
)

// PlanCmd prints the resolved plan without side effects.
type PlanCmd struct {
	Profile   string       `help:"Path to profile directory" type:"existingdir" default:"."`
	JSON      bool         `help:"Print the plan as a single JSON object (suppresses the text rendering and warnings)"`
	Deps      bool         `help:"Show dependency tree for packages in the install list"`
	DepsDepth int          `default:"1" help:"Dependency recursion depth (requires --deps)"`
	Modules   []string     `arg:"" optional:"" name:"modules" help:"Limit scope to these modules (space or comma separated)"`
	Facts     *facts.Facts `kong:"-"`
	Out       io.Writer    `kong:"-"`
}

// Run loads the profile and prints the resolved plan.
func (c *PlanCmd) Run() error {
	// Programmatic construction leaves DepsDepth at its zero value (kong's
	// default only applies to CLI parsing), so 0 without --deps means unset.
	if c.Deps && c.DepsDepth < 1 {
		return fmt.Errorf("--deps-depth must be >= 1")
	}
	// kong cannot distinguish "flag absent" from the default 1, so
	// `--deps-depth 1` alone is a harmless no-op; any other explicit value
	// without --deps is a user error and fails loudly.
	if !c.Deps && c.DepsDepth != 0 && c.DepsDepth != 1 {
		return fmt.Errorf("--deps-depth requires --deps")
	}
	if c.DepsDepth == 0 {
		c.DepsDepth = 1
	}

	f := c.Facts
	if f == nil {
		var err error
		f, err = detectFacts()
		if err != nil {
			return fmt.Errorf("detect: %w", err)
		}
	}

	p, plan, err := loadAndResolvePlan(c.Profile, c.Modules, f)
	if err != nil {
		return err
	}

	out := c.Out
	if out == nil {
		out = os.Stdout
	}

	var deps []packages.PackageDeps
	if c.Deps {
		deps = packages.DepsTree(context.Background(), packagesFor(f.Backend), plan.Packages.Install, c.DepsDepth)
	}

	if c.JSON {
		return printPlanJSON(out, plan, p, f, deps)
	}
	return printPlan(out, plan, p, f, deps)
}

type planJSONDotfile struct {
	Target string `json:"target"`
	Source string `json:"source"`
	Mode   string `json:"mode"`
	// Edit-entry fields (omitted for whole-file entries via omitempty).
	Line     string `json:"line,omitempty"`
	Block    string `json:"block,omitempty"`
	Comment  string `json:"comment,omitempty"`
	Template string `json:"template,omitempty"`
	Module   string `json:"module"`
	Layer    string `json:"layer"`
	Scope    string `json:"scope"`
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

type planJSONDep struct {
	Name    string        `json:"name"`
	Deps    []planJSONDep `json:"deps,omitempty"`
	Unknown bool          `json:"unknown,omitempty"`
}

type planJSONHook struct {
	Command  string `json:"command"`
	Optional bool   `json:"optional,omitempty"`
}

type planJSONDoc struct {
	Fingerprint string   `json:"fingerprint"`
	Modules     []string `json:"modules"`
	Packages    struct {
		Install []string      `json:"install"`
		Remove  []string      `json:"remove"`
		Deps    []planJSONDep `json:"deps,omitempty"`
	} `json:"packages"`
	Tools    map[string]string `json:"tools"`
	Dotfiles []planJSONDotfile `json:"dotfiles"`
	Hooks    struct {
		Pre  []planJSONHook `json:"pre"`
		Post []planJSONHook `json:"post"`
	} `json:"hooks"`
	Mounts []planJSONMount `json:"mounts"`
	Smb    []planJSONSmb   `json:"smb"`
}

// printPlanJSON renders the plan as one JSON object. The no-modules warning is
// intentionally omitted so stdout stays parseable by machine consumers.
func printPlanJSON(out io.Writer, plan *resolve.Plan, p *profile.Profile, f *facts.Facts, deps []packages.PackageDeps) error {
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
	doc.Packages.Deps = toPlanJSONDeps(deps)
	doc.Hooks.Pre = toPlanJSONHooks(plan.Hooks.Pre)
	doc.Hooks.Post = toPlanJSONHooks(plan.Hooks.Post)
	for _, e := range plan.Dotfiles.Entries {
		doc.Dotfiles = append(doc.Dotfiles, planJSONDotfile{
			Target:   e.Target,
			Source:   e.Source,
			Mode:     e.Mode,
			Line:     e.Line,
			Block:    e.Block,
			Comment:  e.Comment,
			Template: e.Template,
			Module:   e.Module,
			Layer:    e.Layer,
			Scope:    e.Scope,
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

func toPlanJSONDeps(nodes []packages.PackageDeps) []planJSONDep {
	if nodes == nil {
		return nil
	}
	out := make([]planJSONDep, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, planJSONDep{Name: n.Name, Unknown: n.Unknown, Deps: toPlanJSONDeps(n.Deps)})
	}
	return out
}

// toPlanJSONHooks maps resolved hook commands to their JSON form, preserving
// the Optional flag so consumers can see which hooks are best-effort.
func toPlanJSONHooks(hooks []profile.HookCommand) []planJSONHook {
	if hooks == nil {
		return nil
	}
	out := make([]planJSONHook, 0, len(hooks))
	for _, h := range hooks {
		out = append(out, planJSONHook{Command: h.Command, Optional: h.Optional})
	}
	return out
}

// optionalMarker renders the text-plan suffix for a hook: " (optional)" when
// the hook is best-effort, "" when required.
func optionalMarker(optional bool) string {
	if optional {
		return " (optional)"
	}
	return ""
}

func printPlan(out io.Writer, plan *resolve.Plan, p *profile.Profile, f *facts.Facts, deps []packages.PackageDeps) error {
	if len(p.Selected) == 0 {
		fmt.Fprintln(out, "warning: no modules selected")
	}
	fmt.Fprintf(out, "fingerprint:\n%s", resolve.Fingerprint(p, f))
	fmt.Fprintln(out, "packages:")
	if deps != nil {
		for _, node := range deps {
			printDepTree(out, node, "  ")
		}
	} else {
		for _, pkg := range plan.Packages.Install {
			fmt.Fprintf(out, "  - %s\n", pkg)
		}
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
		switch {
		case e.Line != "":
			fmt.Fprintf(out, "    line: %q\n", e.Line)
		case e.Block != "":
			fmt.Fprintf(out, "    block: %q\n", e.Block)
			if e.Comment != "" {
				fmt.Fprintf(out, "    comment: %q\n", e.Comment)
			}
		case e.Template != "":
			fmt.Fprintf(out, "    source: %s\n", e.Source)
			fmt.Fprintf(out, "    template: %s\n", e.Template)
		default:
			fmt.Fprintf(out, "    source: %s\n", e.Source)
			fmt.Fprintf(out, "    mode: %s\n", e.Mode)
		}
		// System-scope entries are marked on the module line; user scope is
		// the default and stays unmarked. (Edit entries are always user-scope.)
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
		fmt.Fprintf(out, "    - %s%s\n", c.Command, optionalMarker(c.Optional))
	}
	fmt.Fprintln(out, "  post:")
	for _, c := range plan.Hooks.Post {
		fmt.Fprintf(out, "    - %s%s\n", c.Command, optionalMarker(c.Optional))
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

// printDepTree renders one dependency node at indent: `- name` (marked
// `(deps unknown)` when the query failed), then a `deps:` block one level
// deeper when children exist.
func printDepTree(out io.Writer, node packages.PackageDeps, indent string) {
	if node.Unknown {
		fmt.Fprintf(out, "%s- %s (deps unknown)\n", indent, node.Name)
	} else {
		fmt.Fprintf(out, "%s- %s\n", indent, node.Name)
	}
	if len(node.Deps) > 0 {
		child := indent + "  "
		fmt.Fprintf(out, "%sdeps:\n", child)
		for _, d := range node.Deps {
			printDepTree(out, d, child)
		}
	}
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
