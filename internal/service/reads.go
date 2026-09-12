package service

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/thedataflows/dotdrift/internal/detect"
	"github.com/thedataflows/dotdrift/internal/drift"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/mise"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/resolve"
	"github.com/thedataflows/dotdrift/internal/state"
)

// ReadsDeps carries the reads area's construction seams: the same
// detect/load/resolve seams the apply session holds, plus the load-time
// warn hook and the other-accounts listing the CLI adapter wires to its
// pinned test seams. Zero-value fields are replaced by the real
// implementations by WithDefaults.
type ReadsDeps struct {
	Detect        func() (*facts.Facts, error)
	LoadProfile   func(root string, f *facts.Facts) (*profile.Profile, error)
	Resolve       func(p *profile.Profile, f *facts.Facts) (*resolve.Plan, error)
	OtherAccounts func(root string, f *facts.Facts) ([]profile.Account, error)
	// WarnLoad renders the load-time skip nudges (superuser overlays,
	// misplaced dirs — issues 0029/0033) after load, before the filter.
	// It is the CLI renderer's zerolog output; nil = quiet (the modules
	// read never warns — its listing is the canonical surfacing).
	WarnLoad func(p *profile.Profile)
}

// WithDefaults returns the dep set with zero-value fields replaced by the
// real implementations (the session's own rule). WarnLoad stays nil when
// unset: the nudges are a consumer's renderer output, not a default.
func (d ReadsDeps) WithDefaults() ReadsDeps {
	if d.Detect == nil {
		d.Detect = detect.Detect
	}
	if d.LoadProfile == nil {
		d.LoadProfile = profile.Load
	}
	if d.Resolve == nil {
		d.Resolve = resolve.Resolve
	}
	if d.OtherAccounts == nil {
		d.OtherAccounts = profile.OtherAccounts
	}
	return d
}

// ReadsArea is the lock-free reads half of the service layer (0061-D2/D6):
// typed models over the domain packages plus the canonical renderers.
// Reads never touch the state lock (contract 11) — only the apply session
// writes convergence state.
type ReadsArea struct {
	deps ReadsDeps
}

// NewReadsArea builds the reads area.
func NewReadsArea(deps ReadsDeps) *ReadsArea {
	return &ReadsArea{deps: deps.WithDefaults()}
}

// ModulesRead is the modules listing model: facts and the filtered profile.
type ModulesRead struct {
	Facts   *facts.Facts
	Profile *profile.Profile
}

// Modules runs the detect → load → filter preamble (no plan resolution,
// no warns — parity with the old cmd loadProfile).
func (r *ReadsArea) Modules(profilePath string, modules []string) (*ModulesRead, error) {
	f, err := r.deps.Detect()
	if err != nil {
		return nil, fmt.Errorf("detect: %w", err)
	}
	p, err := r.deps.LoadProfile(profilePath, f)
	if err != nil {
		return nil, fmt.Errorf("load profile: %w", schemaError(err))
	}
	if err := p.LimitTo(profile.ParseModuleFilter(modules)); err != nil {
		return nil, err
	}
	return &ModulesRead{Facts: f, Profile: p}, nil
}

// PlanRead is the plan model: facts, profile, and the resolved plan.
type PlanRead struct {
	Facts   *facts.Facts
	Profile *profile.Profile
	Plan    *resolve.Plan
}

// Plan runs the detect (when f is nil) → load → warn → filter → resolve
// preamble (parity with the old cmd loadAndResolvePlan).
func (r *ReadsArea) Plan(profilePath string, modules []string, f *facts.Facts) (*PlanRead, error) {
	if f == nil {
		var err error
		f, err = r.deps.Detect()
		if err != nil {
			return nil, fmt.Errorf("detect: %w", err)
		}
	}
	p, err := r.deps.LoadProfile(profilePath, f)
	if err != nil {
		return nil, fmt.Errorf("load profile: %w", schemaError(err))
	}
	// Warn before LimitTo: a superuser-overlay module id passed as a filter
	// errors as unknown, and this nudge explains why (issue 0029).
	if r.deps.WarnLoad != nil {
		r.deps.WarnLoad(p)
	}
	if err := p.LimitTo(profile.ParseModuleFilter(modules)); err != nil {
		return nil, err
	}
	plan, err := r.deps.Resolve(p, f)
	if err != nil {
		return nil, fmt.Errorf("resolve plan: %w", err)
	}
	return &PlanRead{Facts: f, Profile: p, Plan: plan}, nil
}

// StatusOpts carries a status read's inputs. ProbesFor is called with the
// detected facts once the preamble has run — the package backend and mise
// seams are the caller's, and building the probes needs the backend from
// the facts. Verbose receives the per-probe progress lines when non-nil.
type StatusOpts struct {
	ProfilePath string
	StatePath   string // "" = the profile's default state path
	Modules     []string
	Jobs        int
	Verbose     io.Writer
	ProbesFor   func(f *facts.Facts) drift.Probes
}

// StatusRead is the status model: state, resolved plan, live-system drift
// findings, and the other configured accounts (ADR-0006's notice inputs).
// ProfilePath is the path as given; ProfileRoot is absolute.
type StatusRead struct {
	ProfilePath string
	StatePath   string
	State       *state.State
	Facts       *facts.Facts
	Profile     *profile.Profile
	Plan        *resolve.Plan
	ProfileRoot string
	Findings    []drift.Finding
	Others      []profile.Account
}

// Status loads state, resolves the plan, and probes the live system for
// drift (plus profile-content orphans). Read-only and lock-free; drift is
// reported in the model, never as an error.
func (r *ReadsArea) Status(ctx context.Context, opts StatusOpts) (*StatusRead, error) {
	statePath := opts.StatePath
	if statePath == "" {
		statePath = state.ProfileStatePath(opts.ProfilePath)
	}
	s, err := state.NewFileStore(statePath).Load()
	if err != nil {
		return nil, fmt.Errorf("load state: %w", err)
	}

	pr, err := r.Plan(opts.ProfilePath, opts.Modules, nil)
	if err != nil {
		return nil, err
	}
	profileRoot, err := filepath.Abs(pr.Profile.Root)
	if err != nil {
		return nil, fmt.Errorf("resolve profile root: %w", err)
	}

	findings := drift.Check(ctx, pr.Plan, profileRoot, opts.ProbesFor(pr.Facts), drift.CheckOptions{
		Jobs:    opts.Jobs,
		Verbose: opts.Verbose,
	})
	findings = append(findings, drift.CheckOrphans(ModuleLayers(pr.Profile))...)

	others, err := r.deps.OtherAccounts(pr.Profile.Root, pr.Facts)
	if err != nil {
		return nil, fmt.Errorf("list other accounts: %w", err)
	}

	return &StatusRead{
		ProfilePath: opts.ProfilePath,
		StatePath:   statePath,
		State:       s,
		Facts:       pr.Facts,
		Profile:     pr.Profile,
		Plan:        pr.Plan,
		ProfileRoot: profileRoot,
		Findings:    findings,
		Others:      others,
	}, nil
}

// DiffEntry is one copy-mode dotfile whose live target differs from the
// profile source.
type DiffEntry struct {
	Module        string
	Target        string // live path
	Source        string // resolved path inside the profile
	TargetContent string
	SourceContent string
}

// Diff collects the differing copy-mode dotfiles of a resolved plan.
// Unreadable sources and missing targets are skipped (nothing to diff) —
// parity with the pre-move cmd renderer walk.
func (r *ReadsArea) Diff(plan *resolve.Plan, profileRoot string) ([]DiffEntry, error) {
	home, _ := os.UserHomeDir()
	var out []DiffEntry
	for _, e := range plan.Dotfiles.Entries {
		if e.Mode != "copy" {
			continue
		}
		files, err := mise.ResolveBootstrapFiles([]resolve.DotfileEntry{e}, profileRoot, home)
		if err != nil {
			continue
		}
		for _, f := range files {
			src, err := os.ReadFile(f.Source)
			if err != nil {
				continue
			}
			tgt, err := os.ReadFile(f.Target)
			if err != nil {
				continue // missing target — nothing to diff
			}
			if string(src) == string(tgt) {
				continue
			}
			out = append(out, DiffEntry{
				Module:        e.Module,
				Target:        f.Target,
				Source:        f.Source,
				TargetContent: string(tgt),
				SourceContent: string(src),
			})
		}
	}
	return out, nil
}

// Detect gathers host facts.
func (r *ReadsArea) Detect() (*facts.Facts, error) {
	f, err := r.deps.Detect()
	if err != nil {
		return nil, fmt.Errorf("detect: %w", err)
	}
	return f, nil
}

// ModuleLayers lists EVERY module layer directory in the profile:
// modules/*, hosts/*/modules/*, users/*/modules/*. Orphans are
// profile-content drift, not live-system drift — a leftover under
// another host's or user's overlay is visible from any machine, and a
// module not selected here (when-filter) is still scanned. Reference
// semantics live in drift.ReferencedPaths (layer declarations, all
// views), so no plan or facts are needed here.
func ModuleLayers(p *profile.Profile) []drift.ModuleLayer {
	if p == nil || p.Root == "" {
		return nil
	}
	var layers []drift.ModuleLayer
	add := func(layer, owner, moduleDir string) {
		layers = append(layers, drift.ModuleLayer{
			Dir: filepath.Base(moduleDir), Layer: layer, Owner: owner, Path: moduleDir,
		})
	}
	// moduleDirs lists the subdirectories of a modules/ root.
	moduleDirs := func(root string) []string {
		entries, err := os.ReadDir(root)
		if err != nil {
			return nil
		}
		var dirs []string
		for _, e := range entries {
			if e.IsDir() {
				dirs = append(dirs, filepath.Join(root, e.Name()))
			}
		}
		return dirs
	}
	for _, d := range moduleDirs(filepath.Join(p.Root, "modules")) {
		add("base", "", d)
	}
	owners := func(kind string) []string {
		entries, err := os.ReadDir(filepath.Join(p.Root, kind))
		if err != nil {
			return nil
		}
		var names []string
		for _, e := range entries {
			if e.IsDir() {
				names = append(names, e.Name())
			}
		}
		return names
	}
	for _, h := range owners("hosts") {
		for _, d := range moduleDirs(filepath.Join(p.Root, "hosts", h, "modules")) {
			add("host", h, d)
		}
	}
	for _, u := range owners("users") {
		for _, d := range moduleDirs(filepath.Join(p.Root, "users", u, "modules")) {
			add("user", u, d)
		}
	}
	return layers
}

// ModuleConfigAt loads one layer's module.toml (a tree origin's raw
// declaration, T-tui-shell) through the doorway. Missing declarations are
// (nil, nil) — the profile package's contract; broken files surface as
// *SchemaError so front ends read Path/Line instead of parsing text.
func (r *ReadsArea) ModuleConfigAt(dir string) (*profile.ModuleConfig, error) {
	cfg, err := profile.LoadModuleConfig(dir)
	if err != nil {
		return nil, schemaError(err)
	}
	return cfg, nil
}

// Schema-error translation at the boundary (0061-D3): profile load errors
// are wrapped strings today; the ones naming a file (and, when the
// reporter could localize it, a line) become *SchemaError so front ends
// match with errors.As. Unrecognized errors pass through unchanged.

var (
	// schemaLineRe matches "…: line N" (BurntSushi's format, optionally
	// behind the profile loader's "decode <path>: " wrap).
	schemaLineRe = regexp.MustCompile(`^(?:decode )?(.+?):(?: toml:)? line (\d+)`)
	// schemaLocRe matches the strict decoder's "path:line: msg" and
	// "path:line:col: msg" bar (first line of possibly multi-line output).
	schemaLocRe = regexp.MustCompile(`^(.+?):(\d+)(?::\d+)?: `)
	// schemaPathRe matches a file-naming error without a line
	// ("path: msg", e.g. palette validation inside dotdrift.toml).
	schemaPathRe = regexp.MustCompile(`^(?:decode )?([^:\s]+\.toml): `)
)

// schemaError translates a load error to *SchemaError when it names a
// schema file; otherwise it returns err unchanged. Error() keeps the
// original string, so CLI output is byte-identical across the migration.
func schemaError(err error) error {
	if err == nil {
		return nil
	}
	first, _, _ := strings.Cut(err.Error(), "\n")
	if m := schemaLineRe.FindStringSubmatch(first); m != nil {
		line, _ := strconv.Atoi(m[2])
		return &SchemaError{Path: m[1], Line: line, Err: err}
	}
	if m := schemaLocRe.FindStringSubmatch(first); m != nil {
		line, _ := strconv.Atoi(m[2])
		return &SchemaError{Path: m[1], Line: line, Err: err}
	}
	if m := schemaPathRe.FindStringSubmatch(first); m != nil {
		return &SchemaError{Path: m[1], Err: err}
	}
	return err
}
