package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/rs/zerolog/log"
	"github.com/thedataflows/dotdrift/internal/apply"
	"github.com/thedataflows/dotdrift/internal/detect"
	"github.com/thedataflows/dotdrift/internal/executil"
	"github.com/thedataflows/dotdrift/internal/generate"
	"github.com/thedataflows/dotdrift/internal/mise"
	"github.com/thedataflows/dotdrift/internal/packages"
	"github.com/thedataflows/dotdrift/internal/paru"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/resolve"
	"github.com/thedataflows/dotdrift/internal/smb"
	"github.com/thedataflows/dotdrift/internal/state"
)

// Test seams (package-level vars, same pattern as runGit in init.go):
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
	// newSmbRunner builds the smb step runner; swapped out by tests.
	newSmbRunner = func() smb.Runner { return &smb.ExecRunner{} }
	// stdinIsTerminal reports whether dotdrift's stdin is a TTY, deciding
	// whether generated hook tasks opt into mise interactive mode so an
	// interactive hook command (e.g. sudo) reaches a controlling terminal.
	// Swapped out by tests.
	stdinIsTerminal = executil.IsStdinTerminal
)

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

// pipelineStepNames is the single source of truth for the ordered pipeline
// step names: apply builds its steps in this order and status reports
// progress against it. Update this list when adding or removing a step.
// hooks-pre/hooks-post, dotfiles-system, mounts, and smb are conditional:
// they only run when the plan has hook commands / system-scope dotfile
// entries / mount entries / smb modules, so a completed apply may
// legitimately show fewer completed steps than the denominator.
var pipelineStepNames = []string{"hooks-pre", "packages", "tools", "dotfiles", "dotfiles-system", "mounts", "smb", "hooks-post"}

// packagesStep is the apply pipeline step for packages. It delegates install
// to mise bootstrap (which converges [bootstrap.packages] via the paru plugin
// or built-in managers) while keeping removal inline — mise's package-plugin
// v1 does not support uninstall (packages.absent handling, issue 0002).
type packagesStep struct {
	runner         mise.Runner
	backend        packages.Backend // for Absent only
	plan           *resolve.Plan
	backendStr     string // detected backend for prefix translation
	configPath     string // bootstrap mise.toml path
	misePluginsDir string // mise plugin registry dir ($XDG_DATA_HOME/mise/plugins); empty = non-Arch
}

var _ apply.Step = (*packagesStep)(nil)

func (s *packagesStep) Name() string { return "packages" }

func (s *packagesStep) Run(ctx context.Context) error {
	// Maintain the paru package plugin (Arch backends): copy the embedded plugin
	// into mise's registry as real files, but only when its content hash differs
	// from what is installed (or it is missing/a stale symlink) — no writes on
	// the common up-to-date path. Runs even with nothing to install.
	if s.misePluginsDir != "" {
		if updated, err := paru.EnsureInstalled(s.misePluginsDir, "paru"); err != nil {
			return fmt.Errorf("maintain paru plugin: %w", err)
		} else if updated {
			log.Info().Msg("paru mise plugin installed/updated")
		}
	}
	// Removal is best-effort (warn, don't fail) — same contract as before.
	if len(s.plan.Packages.Remove) > 0 {
		if err := s.backend.Absent(ctx, s.plan.Packages.Remove); err != nil {
			log.Warn().Err(err).Msg("remove packages failed; continuing")
		}
	}
	if len(s.plan.Packages.Install) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.configPath), 0o755); err != nil {
		return fmt.Errorf("create packages config dir: %w", err)
	}
	content := mise.GenerateBootstrapPackages(s.plan.Packages.Install, s.backendStr)
	if err := os.WriteFile(s.configPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write packages config: %w", err)
	}
	// dotdrift copies the paru plugin into mise's registry itself (EnsureInstalled
	// above), so there is no [bootstrap.plugins] declaration and the plugins
	// phase is not run — mise discovers it as a normal installed plugin.
	return s.runner.Bootstrap(ctx, s.configPath, true, "packages")
}

// systemFilesStep replaces DotfilesSystemStep: it emits [bootstrap.files] from
// system-scope dotfile entries and [bootstrap.directories] from mount
// destinations, then converges via `mise bootstrap --only files`. The hidden
// sudo helpers in mise bootstrap are strictly safer than the old
// `sudo -E mise dotfiles apply` path (never leak content in argv/logs).
type systemFilesStep struct {
	runner     mise.Runner
	entries    []resolve.DotfileEntry
	sourceRoot string
	homeDir    string
	dirs       []string // mount destinations for [bootstrap.directories]
	configPath string
}

var _ apply.Step = (*systemFilesStep)(nil)

func (s *systemFilesStep) Name() string { return "dotfiles-system" }

func (s *systemFilesStep) Run(ctx context.Context) error {
	files, err := mise.ResolveBootstrapFiles(s.entries, s.sourceRoot, s.homeDir)
	if err != nil {
		return err
	}
	content := mise.GenerateBootstrapFiles(files)
	if d := mise.GenerateBootstrapDirectories(s.dirs); d != "" {
		content += "\n" + d
	}
	if content == "" {
		return nil
	}
	if err := writeBootstrapConfig(s.configPath, content); err != nil {
		return err
	}
	return s.runner.Bootstrap(ctx, s.configPath, true, "files")
}

// mountsServicesStep replaces mounts.Step: it emits [bootstrap.services] for
// each mount unit (+ timer if startat) and converges via
// `mise bootstrap --only services`. Directory creation moved to systemFilesStep.
type mountsServicesStep struct {
	runner     mise.Runner
	entries    []resolve.MountEntry
	configPath string
}

var _ apply.Step = (*mountsServicesStep)(nil)

func (s *mountsServicesStep) Name() string { return "mounts" }

func (s *mountsServicesStep) Run(ctx context.Context) error {
	if len(s.entries) == 0 {
		return nil
	}
	var svcs []mise.BootstrapService
	for _, e := range s.entries {
		escaped := generate.EscapePath(e.Spec.Destination)
		enabled := e.Spec.State != "disabled"
		svcs = append(svcs, mise.BootstrapService{
			Name: escaped + ".mount", Enabled: enabled, Running: enabled,
		})
		if e.Spec.StartAt != "" {
			svcs = append(svcs, mise.BootstrapService{
				Name: escaped + ".timer", Enabled: enabled, Running: enabled,
			})
		}
	}
	content := mise.GenerateBootstrapServices(svcs)
	if err := writeBootstrapConfig(s.configPath, content); err != nil {
		return err
	}
	return s.runner.Bootstrap(ctx, s.configPath, true, "services")
}

// smbBootstrapStep replaces smb.Step: it emits [bootstrap.groups]/[users]/
// [services] for the declarative parts and converges via
// `mise bootstrap --only accounts,services`. The interactive smbpasswd/testparm
// logic stays as a post-action via the existing smb.Runner.
type smbBootstrapStep struct {
	runner     mise.Runner
	modules    []resolve.SmbModuleSpec
	configPath string
	smbRunner  smb.Runner // for smbpasswd/testparm post-actions
	out        io.Writer
}

var _ apply.Step = (*smbBootstrapStep)(nil)

func (s *smbBootstrapStep) Name() string { return "smb" }

func (s *smbBootstrapStep) Run(ctx context.Context) error {
	if len(s.modules) == 0 {
		return nil
	}
	// Aggregate group/users/services across modules.
	group := "smb"
	var users []string
	var svcs []mise.BootstrapService
	avahiOn := false
	for _, m := range s.modules {
		if m.Spec.Group != "" {
			group = m.Spec.Group
		}
		if len(m.Spec.Users) > 0 {
			users = m.Spec.Users
		}
		if m.Spec.Avahi == nil || *m.Spec.Avahi {
			avahiOn = true
		}
	}
	svcs = append(svcs, mise.BootstrapService{Name: "smb", Enabled: true, Running: true})
	if avahiOn {
		svcs = append(svcs, mise.BootstrapService{Name: "avahi-daemon", Enabled: true, Running: true})
	}

	content := mise.GenerateBootstrapAccounts(group, users) + "\n" + mise.GenerateBootstrapServices(svcs)
	if err := writeBootstrapConfig(s.configPath, content); err != nil {
		return err
	}
	if err := s.runner.Bootstrap(ctx, s.configPath, true, "accounts", "services"); err != nil {
		return err
	}
	// Post-actions: testparm validation + interactive smbpasswd (kept inline;
	// mise has no declarative equivalent for these).
	return smb.PostBootstrap(ctx, s.smbRunner, s.modules, s.out)
}

// writeBootstrapConfig writes content to configPath, creating parent dirs.
func writeBootstrapConfig(configPath, content string) error {
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	return os.WriteFile(configPath, []byte(content), 0o644)
}

// ApplyCmd runs the full pipeline and always resumes.
type ApplyCmd struct {
	Profile string    `help:"Path to profile directory" type:"existingdir" default:"."`
	State   string    `help:"Path to state file" type:"path" default:""`
	Yes     bool      `help:"Answer yes to mise prompts" default:"false"`
	NoHooks bool      `help:"Skip pre/post hook commands (also DOTDRIFT_NO_HOOKS=1)" default:"false"`
	Verbose bool      `help:"Stream package manager and mise output live, echoing each command line ('+ argv') to stderr before it runs" short:"v" default:"false"`
	Diff    string    `help:"Show diff for files whose content differs before applying; bare = internal diff, --diff=tool uses the named tool" default:""`
	Modules []string  `arg:"" optional:"" name:"modules" help:"Limit scope to these modules (space or comma separated)"`
	Out     io.Writer `kong:"-"`
}

// Run executes the apply pipeline with resume semantics.
func (c *ApplyCmd) Run() error {
	f, err := detectFacts()
	if err != nil {
		return fmt.Errorf("detect: %w", err)
	}

	p, err := profileLoad(c.Profile, f)
	if err != nil {
		return fmt.Errorf("load profile: %w", err)
	}
	if err := p.LimitTo(profile.ParseModuleFilter(c.Modules)); err != nil {
		return err
	}

	plan, err := resolvePlan(p, f)
	if err != nil {
		return fmt.Errorf("resolve plan: %w", err)
	}

	statePath := c.State
	if statePath == "" {
		statePath = state.ProfileStatePath(c.Profile)
	}
	store := state.NewFileStore(statePath)
	// Serialize concurrent applies: the sidecar lock is held from before Load
	// until the pipeline's final save/removal, so two applies can never
	// interleave load→pipeline→save on the same state file.
	if err := store.Lock(); err != nil {
		return fmt.Errorf("lock state: %w", err)
	}
	defer func() { _ = store.Unlock() }()
	s, err := store.Load()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
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
		showDotfileDiffs(plan, profileRoot, c.Diff, out)
	}

	m := defaultMise()
	m.Verbose = c.Verbose
	path, err := m.Ensure()
	if err != nil {
		return fmt.Errorf("ensure mise: %w", err)
	}
	_ = path
	runner := mise.NewExecMise(m)

	// Decision D8a (keep + test): write the FULL mise config ([tools] +
	// [dotfiles] + [tasks]) before the pipeline starts. The tools/dotfiles steps later
	// rewrite this file section-by-section, so if apply crashes or fails
	// before them, the on-disk config still mirrors the whole resolved plan
	// for crash recovery and manual mise runs.
	configDir := filepath.Join(filepath.Dir(statePath), "mise")
	configPath := filepath.Join(configDir, "mise.toml")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return fmt.Errorf("create mise config dir: %w", err)
	}
	if err := os.WriteFile(configPath, []byte(mise.GenerateApplyConfig(plan, profileRoot, f, stdinIsTerminal())), 0o644); err != nil {
		return fmt.Errorf("write mise config: %w", err)
	}

	// The tools/dotfiles steps rewrite their config file with a single
	// section each. Giving each step its own config — in its own directory
	// so `mise --cd` still discovers it as mise.toml — keeps the shared
	// full config (and its [tasks] hook definitions) intact for the hooks
	// steps; otherwise hooks-post would run `mise run` against a config
	// with no tasks.
	toolsConfigPath := filepath.Join(configDir, "tools", "mise.toml")
	dotfilesConfigPath := filepath.Join(configDir, "dotfiles", "mise.toml")
	packagesConfigPath := filepath.Join(configDir, "packages", "mise.toml")
	systemConfigPath := filepath.Join(configDir, "system", "mise.toml")
	mountsConfigPath := filepath.Join(configDir, "mounts", "mise.toml")
	smbConfigPath := filepath.Join(configDir, "smb", "mise.toml")

	// Arch backends install through the dotdrift paru mise plugin (mise's pacman
	// built-in has no AUR support, issue 0003). dotdrift copies it into mise's
	// plugin registry (hash-gated) — real files, no symlink, no declaration.
	var misePluginsDir string
	if f.Backend == "paru" {
		xdgData := os.Getenv("XDG_DATA_HOME")
		if xdgData == "" {
			home, _ := os.UserHomeDir()
			xdgData = filepath.Join(home, ".local", "share")
		}
		misePluginsDir = mise.MisePluginsDir(xdgData)
	}

	// Hooks steps are skipped at construction when their command list is
	// empty or when the user opted out via --no-hooks / DOTDRIFT_NO_HOOKS=1
	// (HooksStep.Run also no-ops on an empty list as a second line of
	// defense). hooks-pre runs before packages so a pre-hook failure aborts
	// before any side effect; hooks-post runs last.
	hooksDisabled := c.NoHooks || os.Getenv("DOTDRIFT_NO_HOOKS") == "1"
	backend := packagesFor(f.Backend)
	setVerboseRunner(c.Verbose, backend)

	// The dotfiles portion splits by scope: user entries apply as today via
	// the DotfilesStep (against a scope-filtered plan copy), system entries
	// get their own step applied with root privileges. The dotfiles-system
	// step is appended only when at least one system-scope entry exists.
	userPlan := *plan
	var userEntries, systemEntries []resolve.DotfileEntry
	for _, e := range plan.Dotfiles.Entries {
		if e.Scope == profile.ScopeSystem {
			systemEntries = append(systemEntries, e)
		} else {
			userEntries = append(userEntries, e)
		}
	}
	userPlan.Dotfiles.Entries = userEntries

	var steps []apply.Step
	if !hooksDisabled && len(plan.Hooks.Pre) > 0 {
		steps = append(steps, &mise.HooksStep{
			Exec: runner, Commands: plan.Hooks.Pre, ConfigPath: configPath,
			Task: "hooks-pre", StepName: "hooks-pre",
		})
	}
	steps = append(steps,
		&packagesStep{runner: runner, backend: backend, plan: plan, backendStr: f.Backend, configPath: packagesConfigPath, misePluginsDir: misePluginsDir},
		&mise.ToolsStep{Runner: runner, Plan: plan, ConfigPath: toolsConfigPath},
		&mise.DotfilesStep{Runner: runner, Plan: &userPlan, ConfigPath: dotfilesConfigPath, Yes: c.Yes},
	)
	// System files + mount directories → mise bootstrap --only files.
	// Runs when there are system-scope dotfiles OR mount destinations (mkdir).
	if len(systemEntries) > 0 || len(plan.Mounts.Entries) > 0 {
		var mountDests []string
		for _, e := range plan.Mounts.Entries {
			mountDests = append(mountDests, e.Spec.Destination)
		}
		homeDir, _ := os.UserHomeDir()
		steps = append(steps, &systemFilesStep{
			runner: runner, entries: systemEntries, sourceRoot: profileRoot,
			homeDir: homeDir, dirs: mountDests, configPath: systemConfigPath,
		})
	}
	// Mount unit services → mise bootstrap --only services.
	if len(plan.Mounts.Entries) > 0 {
		steps = append(steps, &mountsServicesStep{
			runner: runner, entries: plan.Mounts.Entries, configPath: mountsConfigPath,
		})
	}
	// SMB accounts + services → mise bootstrap --only accounts,services,
	// then interactive smbpasswd/testparm post-actions.
	if len(plan.Smb.Modules) > 0 {
		sr := newSmbRunner()
		setVerboseRunner(c.Verbose, sr)
		steps = append(steps, &smbBootstrapStep{
			runner: runner, modules: plan.Smb.Modules, configPath: smbConfigPath,
			smbRunner: sr, out: out,
		})
	}
	if !hooksDisabled && len(plan.Hooks.Post) > 0 {
		steps = append(steps, &mise.HooksStep{
			Exec: runner, Commands: plan.Hooks.Post, ConfigPath: configPath,
			Task: "hooks-post", StepName: "hooks-post",
		})
	}

	pipeline := apply.NewPipeline(steps, store.Save)
	pipeline.SetState(s)
	if err := pipeline.Run(context.Background()); err != nil {
		return fmt.Errorf("apply: %w", err)
	}
	if err := store.Remove(); err != nil {
		return fmt.Errorf("remove state file: %w", err)
	}
	return nil
}
