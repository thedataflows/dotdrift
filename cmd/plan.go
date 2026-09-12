package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/packages"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/resolve"
	"github.com/thedataflows/dotdrift/internal/service"
)

// PlanCmd prints the resolved plan without side effects.
type PlanCmd struct {
	Profile   string       `help:"Path to profile directory" type:"existingdir" default:"."`
	JSON      bool         `short:"j" help:"Print the plan as a single JSON object (suppresses the text rendering and warnings)"`
	Deps      bool         `help:"Show dependency tree for packages in the install list"`
	DepsDepth int          `default:"1" help:"Dependency recursion depth (requires --deps)"`
	Modules   []string     `arg:"" optional:"" name:"modules" help:"Limit scope to these modules (space or comma separated)"`
	Facts     *facts.Facts `kong:"-"`
	Out       io.Writer    `kong:"-"`
}

// Run loads the profile and prints the resolved plan through the service
// reads area; --json stays a dumb marshal over the typed plan (0061-D5).
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

	area := service.NewReadsArea(service.ReadsDeps{
		Detect:      detectFacts,
		LoadProfile: profileLoad,
		Resolve:     resolvePlan,
		WarnLoad:    warnLoadNudges,
	})
	r, err := area.Plan(c.Profile, c.Modules, c.Facts)
	if err != nil {
		return err
	}

	out := c.Out
	if out == nil {
		out = os.Stdout
	}

	var deps []packages.PackageDeps
	if c.Deps {
		deps = packages.DepsTree(context.Background(), packagesFor(r.Facts.Backend), r.Plan.Packages.Install, c.DepsDepth)
	}

	if c.JSON {
		return printPlanJSON(out, r.Plan, r.Profile, r.Facts, deps)
	}
	return service.RenderPlanReport(out, r, deps)
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
	Systemd []planJSONSystemdUnit `json:"systemd"`
	Mounts  []planJSONMount       `json:"mounts"`
	Smb     []planJSONSmb         `json:"smb"`
}

// planJSONSystemdUnit is one declarative systemd user unit in the JSON plan.
type planJSONSystemdUnit struct {
	Module string `json:"module"`
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Layer  string `json:"layer"`
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
	doc.Systemd = make([]planJSONSystemdUnit, 0, len(plan.Systemd.Units))
	for _, u := range plan.Systemd.Units {
		doc.Systemd = append(doc.Systemd, planJSONSystemdUnit{
			Module: u.Module, Name: u.Name, Kind: u.Kind, Layer: u.Layer,
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
