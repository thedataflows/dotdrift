package service

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/thedataflows/dotdrift/internal/backup"
	"github.com/thedataflows/dotdrift/internal/detect"
	"github.com/thedataflows/dotdrift/internal/drift"
	"github.com/thedataflows/dotdrift/internal/executil"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/generate"
	"github.com/thedataflows/dotdrift/internal/mise"
	"github.com/thedataflows/dotdrift/internal/onboard"
	"github.com/thedataflows/dotdrift/internal/profile"
)

// The writes area: onboard, restore, and generate orchestration in one
// place, so the CLI translates flags and prints while the TUI speaks
// these types only (ADR-0008's doorway; T-tui-writes). Output goes where
// the caller points it — the CLI's bytes are the area's contract.

// WritesDeps carries the seams the write flows run through: fact
// detection, profile loading (restore), the mise runner factory
// (onboard), the user's home (restore targets), and the volume lister
// (generate's mounts flow). WithDefaults wires the real implementations.
type WritesDeps struct {
	Detect      func() (*facts.Facts, error)
	LoadProfile func(root string, f *facts.Facts) (*profile.Profile, error)
	NewMise     func(verbose bool) mise.Runner
	HomeDir     func() (string, error)
	ListVolumes func(ctx context.Context, reg *generate.Registry, sources []string) ([]generate.Volume, error)
}

// WithDefaults fills unset seams with the real implementations.
func (d WritesDeps) WithDefaults() WritesDeps {
	if d.Detect == nil {
		d.Detect = detect.Detect
	}
	if d.LoadProfile == nil {
		d.LoadProfile = profile.Load
	}
	if d.NewMise == nil {
		d.NewMise = func(verbose bool) mise.Runner {
			m := mise.DefaultMise()
			m.Verbose = verbose
			return mise.NewExecMise(m)
		}
	}
	if d.HomeDir == nil {
		d.HomeDir = os.UserHomeDir
	}
	if d.ListVolumes == nil {
		d.ListVolumes = generate.Volumes
	}
	return d
}

// WritesArea is the constructor-held writes surface.
type WritesArea struct {
	deps WritesDeps
}

// NewWritesArea builds the writes area over the given seams.
func NewWritesArea(deps WritesDeps) *WritesArea {
	return &WritesArea{deps: deps.WithDefaults()}
}

// OnboardOpts is the onboard flow's input: raw flag values (packages
// still unparsed), the overlay choice as set-plus-owner, and where the
// report goes. HostSet/UserSet with an empty owner mean "the detected
// account" (the CLI's bare-flag form); an explicit owner wins.
type OnboardOpts struct {
	ProfileRoot string
	Paths       []string
	App         string
	Mode        string
	Packages    []string
	Tools       []string
	HostSet     bool
	Hostname    string
	UserSet     bool
	Username    string
	DryRun      bool
	Yes         bool
	Verbose     bool
	Out         io.Writer // nil discards; the CLI passes its stdout
}

// Onboard adopts live paths into a module and applies it: detect fills
// the overlay owners, packages parse at the boundary, and the onboard
// flow runs exactly as the CLI always ran it (mise included).
func (a *WritesArea) Onboard(opts OnboardOpts) error {
	f, err := a.deps.Detect()
	if err != nil {
		return fmt.Errorf("detect: %w", err)
	}
	pkgs, err := onboard.ParsePackages(opts.Packages)
	if err != nil {
		return fmt.Errorf("parse packages: %w", err)
	}
	out := opts.Out
	if out == nil {
		out = io.Discard
	}
	o := &onboard.Onboard{Mise: a.deps.NewMise(opts.Verbose), Out: out}
	return o.Run(onboard.Options{
		ProfileRoot: opts.ProfileRoot,
		Paths:       opts.Paths,
		App:         opts.App,
		Mode:        opts.Mode,
		Packages:    pkgs,
		Tools:       opts.Tools,
		Host:        opts.HostSet,
		Hostname:    overlayOwner(opts.HostSet, opts.Hostname, f.Hostname),
		User:        opts.UserSet,
		Username:    overlayOwner(opts.UserSet, opts.Username, f.Username),
		DryRun:      opts.DryRun,
		Yes:         opts.Yes,
	})
}

// overlayOwner resolves an overlay owner: an explicit value wins, a bare
// flag falls back to the detected fact.
func overlayOwner(set bool, given, detected string) string {
	if set && given != "" {
		return given
	}
	if set {
		return detected
	}
	return ""
}

// RestoreHit is one backed-up file: generation Gen of ModuleDir holds the
// copy at Path (absolute) for the target it mirrors.
type RestoreHit struct {
	ModuleDir string
	Gen       string
	Path      string
}

// RestoreOpts is the restore flow's input. Handover runs the privileged
// children (sudo install / sudo rm) on the consumer's terminal — the
// session Handover contract (0064-D9); the TUI never runs sudo itself.
type RestoreOpts struct {
	ProfilePath string
	Targets     []string
	Gen         string
	DryRun      bool
	Out         io.Writer // nil discards; the CLI passes its stdout
	Handover    func(*exec.Cmd) error
}

// RestorePlanItem is one resolved restore: the absolute target, its
// backup location as a profile-relative label, the generation, and
// whether the copy needs the consumer's terminal.
type RestorePlanItem struct {
	Target   string
	Label    string
	Gen      string
	Elevated bool
}

// RestorePlan resolves targets to their backups without touching
// anything: the newest generation holding each target, or the pinned
// generation, with the same refusals Restore enforces.
func (a *WritesArea) RestorePlan(opts RestoreOpts) ([]RestorePlanItem, error) {
	resolved, err := a.resolveRestore(opts)
	if err != nil {
		return nil, err
	}
	plan := make([]RestorePlanItem, 0, len(resolved))
	for _, r := range resolved {
		plan = append(plan, RestorePlanItem{
			Target:   r.target,
			Label:    r.label,
			Gen:      r.hit.Gen,
			Elevated: !executil.PathUserWritable(r.target),
		})
	}
	return plan, nil
}

// Restore copies backed-up files back to their live targets (the inverse
// of apply --backup), reporting each copy as the CLI always printed it.
func (a *WritesArea) Restore(opts RestoreOpts) error {
	resolved, err := a.resolveRestore(opts)
	if err != nil {
		return err
	}
	out := opts.Out
	if out == nil {
		out = io.Discard
	}
	restored := 0
	for _, r := range resolved {
		if opts.DryRun {
			fmt.Fprintf(out, "would restore: %s (%s)\n", r.target, r.label)
			restored++
			continue
		}
		if executil.PathUserWritable(r.target) {
			if err := backup.RestoreItem(filepath.Join(r.hit.ModuleDir, "backups", r.hit.Gen), r.target); err != nil {
				return err
			}
		} else {
			info, err := os.Stat(r.hit.Path)
			if err != nil {
				return fmt.Errorf("restore %s: read backup: %w", r.target, err)
			}
			if err := elevateRestore(opts.Handover, r.hit.Path, r.target, info.Mode().Perm()); err != nil {
				return fmt.Errorf("restore %s (elevated): %w", r.target, err)
			}
		}
		fmt.Fprintf(out, "restored: %s (%s)\n", r.target, r.label)
		restored++
	}
	if restored > 0 {
		fmt.Fprintln(out, "note: restored targets may drift from the profile; dotdrift apply overwrites them again - re-onboard a path to keep its restored content")
	}
	return nil
}

// resolvedRestore is one target's resolution: the absolute target, its
// newest (or pinned) hit, and the report label.
type resolvedRestore struct {
	target string
	label  string
	hit    RestoreHit
}

// resolveRestore carries the shared resolution: profile load, target
// normalization, index lookup, generation pinning, and the refusals —
// word for word the CLI's restore errors.
func (a *WritesArea) resolveRestore(opts RestoreOpts) ([]resolvedRestore, error) {
	if len(opts.Targets) == 0 {
		return nil, fmt.Errorf("restore: specify at least one target path (--list browses generations)")
	}
	f, err := a.deps.Detect()
	if err != nil {
		return nil, fmt.Errorf("detect: %w", err)
	}
	p, err := a.deps.LoadProfile(opts.ProfilePath, f)
	if err != nil {
		return nil, fmt.Errorf("load profile: %w", err)
	}
	home, err := a.deps.HomeDir()
	if err != nil {
		return nil, fmt.Errorf("home dir: %w", err)
	}
	hits := IndexBackups(ModuleLayers(p))

	var resolved []resolvedRestore
	for _, spec := range opts.Targets {
		target, err := NormalizeRestoreTarget(spec, home)
		if err != nil {
			return nil, err
		}
		byModule := hits[target]
		if len(byModule) == 0 {
			return nil, fmt.Errorf("restore: no backup of %s (try --list)", target)
		}
		if opts.Gen != "" {
			filtered := map[string][]RestoreHit{}
			for dir, list := range byModule {
				for _, h := range list {
					if h.Gen == opts.Gen {
						filtered[dir] = append(filtered[dir], h)
					}
				}
			}
			if len(filtered) == 0 {
				var have []string
				for _, list := range byModule {
					for _, h := range list {
						have = append(have, h.Gen)
					}
				}
				sort.Strings(have)
				return nil, fmt.Errorf("restore: generation %s holds no backup of %s (have: %s)",
					opts.Gen, target, strings.Join(have, ", "))
			}
			byModule = filtered
		}
		if len(byModule) > 1 {
			var dirs []string
			for dir := range byModule {
				dirs = append(dirs, dir)
			}
			sort.Strings(dirs)
			return nil, fmt.Errorf("restore: %s is backed up by multiple modules (%s); remove the stale backup",
				target, strings.Join(dirs, ", "))
		}
		// One module: hits are newest-first (per module).
		var hit RestoreHit
		for _, list := range byModule {
			hit = list[0]
		}
		resolved = append(resolved, resolvedRestore{
			target: target,
			label:  fmt.Sprintf("%s/backups/%s", ModuleRel(p.Root, hit.ModuleDir), hit.Gen),
			hit:    hit,
		})
	}
	return resolved, nil
}

// elevateRestore copies one backed-up file to a target the current user
// cannot write (system files) through the consumer's Handover:
// `sudo install -D -m <mode>` creates leading directories and lands the
// file root-owned — cp -p would stamp the backup's user owner onto /etc.
// A symlink at the target is removed first (install would follow it like
// cp). sudo's timestamp cache means at most one password prompt per run.
func elevateRestore(handover func(*exec.Cmd) error, src, dst string, mode os.FileMode) error {
	if handover == nil {
		return fmt.Errorf("restore: elevated copy needs a terminal handover")
	}
	if fi, err := os.Lstat(dst); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		if err := handover(exec.Command("sudo", "rm", "-f", dst)); err != nil {
			return fmt.Errorf("remove symlink (elevated): %w", err)
		}
	}
	return handover(exec.Command("sudo", "install", "-D", "-m", fmt.Sprintf("%04o", mode.Perm()), src, dst))
}

// IndexBackups walks every module layer's backup generations and maps
// each backed-up file to its live target: the mirrored path under a
// generation directory IS the absolute target (minus the leading
// separator). Hits per module are newest-first (generations iterate
// newest-first).
func IndexBackups(layers []drift.ModuleLayer) map[string]map[string][]RestoreHit {
	hits := map[string]map[string][]RestoreHit{}
	for _, ml := range layers {
		if ml.Path == "" {
			continue
		}
		gens, err := backup.Generations(ml.Path)
		if err != nil {
			continue // unreadable backups dir: nothing to restore from
		}
		for _, gen := range gens { // newest-first
			genDir := filepath.Join(ml.Path, "backups", gen)
			_ = filepath.WalkDir(genDir, func(path string, d os.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return nil
				}
				rel, relErr := filepath.Rel(genDir, path)
				if relErr != nil {
					return nil
				}
				target := filepath.Clean(string(filepath.Separator) + filepath.ToSlash(rel))
				if hits[target] == nil {
					hits[target] = map[string][]RestoreHit{}
				}
				hits[target][ml.Path] = append(hits[target][ml.Path], RestoreHit{
					ModuleDir: ml.Path, Gen: gen, Path: path,
				})
				return nil
			})
		}
	}
	return hits
}

// NormalizeRestoreTarget expands ~ and requires an absolute path: the
// mirrored backup layout keys on absolute targets.
func NormalizeRestoreTarget(spec, home string) (string, error) {
	t := spec
	if t == "~" || strings.HasPrefix(t, "~/") {
		t = filepath.Join(home, strings.TrimPrefix(t, "~"))
	}
	if !filepath.IsAbs(t) {
		return "", fmt.Errorf("restore: target %q must be absolute or start with ~", spec)
	}
	return filepath.Clean(t), nil
}

// ModuleRel renders a module directory as a profile-relative label
// (modules/<m>, hosts/<h>/modules/<m>, ...).
func ModuleRel(root, moduleDir string) string {
	rel, err := filepath.Rel(root, moduleDir)
	if err != nil {
		return moduleDir
	}
	return rel
}

// GenerateSelection resolves the target selection, filling hostname/
// username from detected facts when the layer needs them and the
// selection lacks them.
func (a *WritesArea) GenerateSelection(sel generate.Selection) (generate.Selection, error) {
	wantHost := sel.Layer == generate.LayerHost && sel.Hostname == ""
	wantUser := sel.Layer == generate.LayerUser && sel.Username == ""
	if !wantHost && !wantUser {
		return sel, nil
	}
	f, err := a.deps.Detect()
	if err != nil {
		return sel, fmt.Errorf("detect: %w", err)
	}
	if wantHost {
		sel.Hostname = f.Hostname
	}
	if wantUser {
		sel.Username = f.Username
	}
	return sel, nil
}

// Volumes lists the volumes the mounts flow classifies, annotated
// MANAGED against the sources the target module already declares (an
// absent module is tolerated: nothing managed).
func (a *WritesArea) Volumes(root string, sel generate.Selection) ([]generate.Volume, error) {
	sources, err := generate.ExistingMountSources(root, sel)
	if err != nil {
		return nil, err
	}
	reg, err := generate.Load()
	if err != nil {
		return nil, err
	}
	return a.deps.ListVolumes(context.Background(), reg, sources)
}

// WriteGenerate materializes an assembled generate input (built by the
// caller through the shared builders — the one assembly path) and prints
// the summary the CLI shows.
func (a *WritesArea) WriteGenerate(root string, sel generate.Selection, input generate.Input, out io.Writer) error {
	if err := generate.WriteModule(root, sel, input); err != nil {
		return err
	}
	return generate.PrintSummary(out, root, sel)
}
