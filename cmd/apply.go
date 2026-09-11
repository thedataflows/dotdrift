package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/alecthomas/kong"
	"github.com/thedataflows/dotdrift/internal/detect"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/mise"
	"github.com/thedataflows/dotdrift/internal/packages"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/resolve"
	"github.com/thedataflows/dotdrift/internal/service"
	"github.com/thedataflows/dotdrift/internal/smb"
)

// Test seams for the shared read preamble (detect/load/resolve), same
// pattern as runGit in init.go. Apply itself runs through the session's
// ApplyDeps (field deps); these stay for plan, status, onboard, restore.
var (
	// detectFacts gathers host facts; swapped out by tests. Shared by apply and onboard.
	detectFacts = detect.Detect
	// profileLoad loads the profile; wrapped by tests to observe call order.
	profileLoad = profile.Load
	// resolvePlan builds the execution plan; wrapped by tests to observe call order.
	resolvePlan = resolve.Resolve
	// defaultMise builds the mise bootstrapper; swapped out by tests.
	defaultMise = mise.DefaultMise
	// packagesFor selects the distro package backend; swapped out by tests.
	packagesFor = packages.For
)

// loadAndResolve runs the detect → load → filter → resolve preamble shared by
// apply, status, and plan. Returns facts, profile, and resolved plan.
func loadAndResolve(profilePath string, modules []string) (*facts.Facts, *profile.Profile, *resolve.Plan, error) {
	f, err := detectFacts()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("detect: %w", err)
	}
	p, plan, err := loadAndResolvePlan(profilePath, modules, f)
	if err != nil {
		return nil, nil, nil, err
	}
	return f, p, plan, nil
}

// loadAndResolvePlan loads the profile and resolves the plan using the given
// facts. Called by loadAndResolve and by PlanCmd (which injects its own facts).
func loadAndResolvePlan(profilePath string, modules []string, f *facts.Facts) (*profile.Profile, *resolve.Plan, error) {
	p, err := profileLoad(profilePath, f)
	if err != nil {
		return nil, nil, fmt.Errorf("load profile: %w", err)
	}
	// Warn before LimitTo: a superuser-overlay module id passed as a filter
	// errors as unknown, and this nudge explains why (issue 0029).
	warnSuperuserOverlays(p)
	warnMisplacedModules(p)
	if err := p.LimitTo(profile.ParseModuleFilter(modules)); err != nil {
		return nil, nil, err
	}
	plan, err := resolvePlan(p, f)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve plan: %w", err)
	}
	return p, plan, nil
}

// loadProfile runs the detect → load → filter preamble shared by modules.
// Returns facts and profile (no plan resolution).
func loadProfile(profilePath string, modules []string) (*facts.Facts, *profile.Profile, error) {
	f, err := detectFacts()
	if err != nil {
		return nil, nil, fmt.Errorf("detect: %w", err)
	}
	p, err := profileLoad(profilePath, f)
	if err != nil {
		return nil, nil, fmt.Errorf("load profile: %w", err)
	}
	if err := p.LimitTo(profile.ParseModuleFilter(modules)); err != nil {
		return nil, nil, err
	}
	return f, p, nil
}

// verboseRunner is the narrow seam concrete runners implement to receive the
// --verbose flag. Interface shapes (packages.Backend, mounts.Runner,
// smb.Runner) stay untouched; fakes without it keep working unchanged.
type verboseRunner interface{ SetVerbose(bool) }

// setVerboseRunner passes the --verbose value to every runner that opts in
// via SetVerbose; others are left as-is.
func setVerboseRunner(v bool, rs ...any) {
	for _, r := range rs {
		if vr, ok := r.(verboseRunner); ok {
			vr.SetVerbose(v)
		}
	}
}

type ApplyCmd struct {
	Profile string    `help:"Path to profile directory" type:"existingdir" default:"."`
	State   string    `help:"Path to state file" type:"path" default:""`
	Yes     bool      `help:"Answer yes to mise prompts" default:"false"`
	Force   bool      `help:"Pass --force to mise dotfiles apply: replace pre-existing files at managed targets (e.g. app-written files where a symlink should go) instead of refusing; copy mode overwrites freely without it" default:"false"`
	Verbose bool      `help:"Stream package manager and mise output live, echoing each command line ('+ argv') to stderr before it runs" short:"v" default:"false"`
	Diff    string    `help:"Show diff for files whose content differs before applying; bare = internal diff, --diff=tool uses the named tool" default:""`
	Backup  bool      `help:"Back up existing copy-mode destinations into the declaring module's backups/<timestamp>/ before applying; copy is the only mode whose apply overwrites destination content" default:"false"`
	Modules []string  `arg:"" optional:"" name:"modules" help:"Limit scope to these modules (space or comma separated)"`
	Out     io.Writer `kong:"-"`

	// Section flags: positives select exactly those sections, negatives
	// (--no-<section>) subtract, no flags runs everything. Each maps to
	// the same-named module.toml plan section. No default:"false" tag —
	// an explicit default marks the flag Set in kong, which would defeat
	// presence detection (positive vs negated vs absent). The group tag
	// renders all six under one "Section flags" heading in --help.
	Packages bool `group:"Section flags" help:"Apply only the packages section" negatable:""`
	Tools    bool `group:"Section flags" help:"Apply only the tools section" negatable:""`
	Dotfiles bool `group:"Section flags" help:"Apply only the dotfiles section (user + system)" negatable:""`
	Systemd  bool `group:"Section flags" help:"Apply only the systemd user units section" negatable:""`
	Mounts   bool `group:"Section flags" help:"Apply only the mounts section (units + services + destination dirs)" negatable:""`
	Smb      bool `group:"Section flags" help:"Apply only the smb section" negatable:""`
	Hooks    bool `group:"Section flags" help:"Run pre/post hook commands" negatable:""`

	// onlySections overrides the flag resolution (programmatic callers,
	// tests); nil = resolve from the parsed flags. kctx is captured by
	// AfterApply so flag presence (Set, positive or negated) is readable.
	// deps is the session seam (tests): nil = the service's real
	// implementations.
	onlySections []string
	kctx         *kong.Context
	deps         *service.ApplyDeps
}

// Run executes the apply pipeline with resume semantics through the
// service layer's apply session (the 0061-D7 adapter, issue 0070):
// display reads up front, then one session whose events render to the
// streams. The session is the only apply orchestration in the tree.
func (c *ApplyCmd) Run() error {
	deps := c.applyDeps()
	// Section resolution errors must surface before any output, so it is
	// validated here even though Start re-resolves (idempotent).
	sections, err := c.resolveSections()
	if err != nil {
		return err
	}

	// Display reads before the session can touch anything: the plan
	// render (same renderer as `dotdrift plan`) and --diff must show the
	// pre-apply state, so they cannot ride the session's event stream —
	// the run goroutine is already writing once PlanResolved is drained.
	f, p, plan, err := displayReads(deps, c.Profile, c.Modules)
	if err != nil {
		return err
	}
	out := c.Out
	if out == nil {
		out = os.Stdout
	}
	profileRoot, err := filepath.Abs(p.Root)
	if err != nil {
		return fmt.Errorf("resolve profile root: %w", err)
	}
	if err := printPlan(out, plan, p, f, nil); err != nil {
		return err
	}
	if c.Diff != "" {
		if err := showDotfileDiffs(plan, profileRoot, c.Diff, out); err != nil {
			return err
		}
	}

	area := service.NewApplyArea(deps)
	sess, err := area.Start(context.Background(), service.ApplyOpts{
		ProfilePath: c.Profile,
		StatePath:   c.State,
		Modules:     c.Modules,
		Sections:    sections,
		Yes:         c.Yes,
		Force:       c.Force,
		Backup:      c.Backup,
		Output:      out, // passthrough: children stream fd-direct (color kept)
		Handover:    handoverToTerminal,
	})
	if err != nil {
		return err
	}
	return renderSession(sess, profileRoot, out)
}

// displayReads runs the detect → load → warn → filter → resolve preamble
// through the session's deps, so tests stub one seam set. Reads are pure;
// Start re-runs them internally for its own goroutine.
func displayReads(deps service.ApplyDeps, profilePath string, modules []string) (*facts.Facts, *profile.Profile, *resolve.Plan, error) {
	f, err := deps.Detect()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("detect: %w", err)
	}
	p, err := deps.LoadProfile(profilePath, f)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("load profile: %w", err)
	}
	// Warn before LimitTo: a superuser-overlay module id passed as a
	// filter errors as unknown, and this nudge explains why (issue 0029).
	// The session does not warn — the nudge is CLI renderer output.
	warnSuperuserOverlays(p)
	warnMisplacedModules(p)
	if err := p.LimitTo(profile.ParseModuleFilter(modules)); err != nil {
		return nil, nil, nil, err
	}
	plan, err := deps.Resolve(p, f)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("resolve plan: %w", err)
	}
	return f, p, plan, nil
}

// applyDeps resolves the session dependency set: the injected seam when
// set (tests), the real implementations otherwise, with --verbose wrapped
// on top — a flag-layer concern (0064-D8). Partial dep sets fall back to
// the real implementations (WithDefaults, the session's own rule), and
// the verbose closures capture the original dep before replacing it
// (never themselves).
func (c *ApplyCmd) applyDeps() service.ApplyDeps {
	deps := service.ApplyDeps{}.WithDefaults()
	if c.deps != nil {
		deps = c.deps.WithDefaults()
	}
	if !c.Verbose {
		return deps
	}
	origMise := deps.NewMise
	deps.NewMise = func() *mise.Mise {
		m := origMise()
		m.Verbose = true
		return m
	}
	origFor := deps.PackagesFor
	deps.PackagesFor = func(backend string) packages.Backend {
		be := origFor(backend)
		setVerboseRunner(true, be)
		return be
	}
	origSmb := deps.NewSmbRunner
	deps.NewSmbRunner = func() smb.Runner {
		sr := origSmb()
		setVerboseRunner(true, sr)
		return sr
	}
	return deps
}

// handoverToTerminal runs one session-built child command on dotdrift's
// real stdio — fd passthrough keeps the child's color and prompt (0071:
// sudo edits and interactive hook tasks are the real handovers). A var so
// tests can stand in for the terminal exec.
var handoverToTerminal = func(cmd *exec.Cmd) error {
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

// renderSession drains the session's events to the streams and returns
// the run's outcome. Passthrough mode emits no StepOutput events (children
// stream fd-direct); the plan was already rendered before Start, so
// PlanResolved only opens the stream here.
func renderSession(sess *service.ApplySession, profileRoot string, out io.Writer) error {
	for ev := range sess.Events() {
		switch e := ev.(type) {
		case service.BackupTaken:
			// Byte-parity with the pre-session line: profile-relative
			// module dir + generation (the event's Dir is absolute).
			moduleDir := filepath.Dir(filepath.Dir(e.Dir))
			fmt.Fprintf(out, "backup: %d path(s) -> %s\n",
				e.Count, filepath.Join(moduleRel(profileRoot, moduleDir), "backups", filepath.Base(e.Dir)))
		}
	}
	res, err := sess.Wait()
	if err != nil {
		return err // cancelled: rerun resumes from the cursor (contract 2)
	}
	if res.Outcome == service.OutcomeFailed {
		return res.StepError
	}
	return nil
}
