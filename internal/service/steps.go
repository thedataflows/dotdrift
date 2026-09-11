package service

// The apply pipeline's step implementations, absorbed from cmd/apply.go
// (0064-D6, issue 0069). This is the moved copy; the cmd original is
// deleted by 0070 when ApplyCmd migrates onto the session. Test seams
// arrived as ApplyDeps instead of cmd's package vars.

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/thedataflows/dotdrift/internal/apply"
	"github.com/thedataflows/dotdrift/internal/backup"
	"github.com/thedataflows/dotdrift/internal/executil"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/generate"
	"github.com/thedataflows/dotdrift/internal/mise"
	"github.com/thedataflows/dotdrift/internal/packages"
	"github.com/thedataflows/dotdrift/internal/paru"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/resolve"
	"github.com/thedataflows/dotdrift/internal/smb"
)

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
			log.Info().Str("version", paru.PluginVersion).Msg("paru mise plugin installed/updated")
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
	content := mise.GenerateBootstrapPackages(s.plan.Packages.Install, s.backendStr)
	if err := writeBootstrapConfig(s.configPath, content); err != nil {
		return fmt.Errorf("write packages config: %w", err)
	}
	// dotdrift copies the paru plugin into mise's registry itself (EnsureInstalled
	// above), so there is no [bootstrap.plugins] declaration and the plugins
	// phase is not run — mise discovers it as a normal installed plugin.
	return s.runner.Bootstrap(ctx, s.configPath, true, "packages")
}

// systemFilesStep applies system-scope dotfile entries and creates mount
// destination directories. Whole-file entries (symlink-each pre-expanded) and
// mount dirs converge through mise's native [bootstrap.files] +
// [bootstrap.directories] via `mise bootstrap --only files` (issue 0042):
// mise attempts changes as the current user and retries the remainder in one
// privileged batch, writes atomically, and fails loud with the exact command
// when elevation is impossible — dotdrift runs no sudo for whole files.
// symlink→copy is inherent: bootstrap.files manages content, not links.
// Edit entries have no bootstrap.files equivalent (contract #18): they keep
// the [dotfiles] path, elevated via DotfilesApplySudo when an edit target is
// not user-writable.
type systemFilesStep struct {
	exec       *mise.ExecMise
	entries    []resolve.DotfileEntry
	sourceRoot string
	homeDir    string
	dirs       []string // mount destinations → [bootstrap.directories]
	configPath string   // [bootstrap.files] + [bootstrap.directories] + [bootstrap.secrets]
	editsPath  string   // [dotfiles] for edit entries
	yes        bool
	force      bool                      // apply --force → --force on the edits dotfiles apply (issue 0046)
	secrets    map[string]profile.Secret // declared secret inputs → [bootstrap.secrets]
}

var _ apply.Step = (*systemFilesStep)(nil)

func (s *systemFilesStep) Name() string { return "dotfiles-system" }

func (s *systemFilesStep) Run(ctx context.Context) error {
	var whole, edit []resolve.DotfileEntry
	for _, e := range s.entries {
		if e.IsEdit() {
			edit = append(edit, e)
		} else {
			whole = append(whole, e)
		}
	}

	// Whole-file entries + mount destination directories → bootstrap files.
	if len(whole) > 0 || len(s.dirs) > 0 {
		if s.exec == nil {
			return fmt.Errorf("system files require an exec mise runner")
		}
		var content string
		if len(whole) > 0 {
			files, err := mise.ResolveBootstrapFiles(whole, s.sourceRoot, s.homeDir)
			if err != nil {
				return fmt.Errorf("resolve system files: %w", err)
			}
			content = mise.GenerateBootstrapFiles(files)
		}
		content += mise.GenerateBootstrapDirectories(s.dirs)
		// Declared secret inputs ride the same config: the system files
		// template path is the only one where mise resolves secret()
		// ([dotfiles] templates have no secret function — verified against
		// mise 2026.9.1, issue 0044).
		content += mise.GenerateBootstrapSecrets(s.secrets)
		if err := writeBootstrapConfig(s.configPath, content); err != nil {
			return fmt.Errorf("write system files config: %w", err)
		}
		if err := s.exec.Bootstrap(ctx, s.configPath, s.yes, "files"); err != nil {
			return fmt.Errorf("system files: %w", err)
		}
	}

	// Edit entries → elevated [dotfiles] path (contract #18).
	if len(edit) > 0 {
		if s.exec == nil {
			return fmt.Errorf("system edits require an exec mise runner")
		}
		if err := writeBootstrapConfig(s.editsPath, mise.GenerateDotfiles(edit)); err != nil {
			return fmt.Errorf("write system edits config: %w", err)
		}
		// Decide elevation up front from the edit targets' writability: runOp
		// streams the child's fds straight to the terminal (preserving mise's
		// color), so a "Permission denied" can't be read back from stderr.
		if os.Geteuid() != 0 && !systemTargetsUserWritable(edit, s.homeDir) {
			if err := s.exec.DotfilesApplySudo(ctx, s.editsPath, s.yes, s.force); err != nil {
				return fmt.Errorf("system edits (elevated): %w", err)
			}
		} else {
			if err := s.exec.DotfilesApply(ctx, s.editsPath, s.yes, s.force); err != nil {
				return fmt.Errorf("system edits: %w", err)
			}
		}
	}
	return nil
}

// systemTargetsUserWritable reports whether the current user can write every
// edit target. Any non-writable target ⇒ the edits batch must converge
// elevated (sudo) in one pass. Used to decide sudo up front instead of
// inspecting mise's stderr (which runOp no longer captures — it streams
// straight to the terminal to keep mise's color).
func systemTargetsUserWritable(entries []resolve.DotfileEntry, homeDir string) bool {
	for _, e := range entries {
		p := e.Target
		if strings.HasPrefix(p, "~/") {
			p = filepath.Join(homeDir, p[2:])
		}
		if !executil.PathUserWritable(p) {
			return false
		}
	}
	return true
}

// systemdUnitsStep converges declarative systemd USER units (services and
// timers) via [bootstrap.linux.systemd.units] and
// `mise bootstrap --only linux-systemd-units` (issue 0048). mise writes the
// dev.mise.<name> unit files, daemon-reloads, enables per wanted_by, and
// starts/stops per start; under sudo mise skips user units with a warning
// (wrong user manager) — no dotdrift-side euid branching.
type systemdUnitsStep struct {
	runner     mise.Runner
	units      []resolve.SystemdUnitEntry
	configPath string
	yes        bool
}

var _ apply.Step = (*systemdUnitsStep)(nil)

func (s *systemdUnitsStep) Name() string { return "systemd" }

func (s *systemdUnitsStep) Run(ctx context.Context) error {
	if len(s.units) == 0 {
		return nil
	}
	content, err := mise.GenerateBootstrapSystemdUnits(s.units)
	if err != nil {
		return fmt.Errorf("generate systemd units config: %w", err)
	}
	if err := writeBootstrapConfig(s.configPath, content); err != nil {
		return fmt.Errorf("write systemd units config: %w", err)
	}
	return s.runner.Bootstrap(ctx, s.configPath, s.yes, "linux-systemd-units")
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

// configPaths are the generated mise.toml locations for one run, keyed by
// role (issue 0053: step configs are siblings of shared/, never nested
// under it — mise --cd config discovery walks UP and would merge the full
// plan into every per-step config otherwise).
type configPaths struct {
	tools, systemd, dotfiles, packages string
	system, systemEdits                string
	mounts, smb, shared                string
}

func newConfigPaths(stateDir string) configPaths {
	configDir := filepath.Join(stateDir, "mise")
	join := func(dir string) string { return filepath.Join(configDir, dir, "mise.toml") }
	return configPaths{
		tools:       join("tools"),
		systemd:     join("systemd"),
		dotfiles:    join("dotfiles"),
		packages:    join("packages"),
		system:      join("system"),
		systemEdits: join("system-edits"),
		mounts:      join("mounts"),
		smb:         join("smb"),
		shared:      join("shared"),
	}
}

// backupCopyTargets snapshots every existing copy-mode destination of the
// resolved plan into the declaring module's backups/<generation>/ tree
// (issue 0025). Copy is the only mode whose apply overwrites destination
// content (symlinks are recreated as links, edits are marker-scoped). The
// declaring layer names the receiving module directory — base entries back
// up under modules/<m>, host winners under hosts/<h>/modules/<m>, user
// winners under users/<u>/modules/<m> — so each backup sits next to the
// profile files that replace it. One generation (timestamp) is shared by
// every module in the run. Returns one BackupTaken per receiving module
// directory that actually received a backup; the session emits them as
// events instead of printing lines (the CLI adapter renders them).
func backupCopyTargets(plan *resolve.Plan, profileRoot string, f *facts.Facts) ([]BackupTaken, error) {
	home, _ := os.UserHomeDir()
	gen := time.Now().Format("20060102-150405")

	filesByDir := map[string][]backup.File{}
	var dirs []string
	for _, e := range plan.Dotfiles.Entries {
		if e.IsEdit() || e.Mode != "copy" {
			continue
		}
		target := e.Target
		if strings.HasPrefix(target, "~/") {
			target = filepath.Join(home, target[2:])
		}
		dir := moduleLayerDir(profileRoot, e, f)
		if _, seen := filesByDir[dir]; !seen {
			dirs = append(dirs, dir)
		}
		filesByDir[dir] = append(filesByDir[dir], backup.File{Target: target, ModuleDir: dir})
	}
	sort.Strings(dirs)
	var taken []BackupTaken
	for _, dir := range dirs {
		n, err := backup.Run(filesByDir[dir], gen)
		if err != nil {
			return nil, err
		}
		if n > 0 {
			targets := make([]string, len(filesByDir[dir]))
			for i, bf := range filesByDir[dir] {
				targets[i] = bf.Target
			}
			taken = append(taken, BackupTaken{
				Dir:   filepath.Join(dir, "backups", gen),
				Files: targets,
				Count: n,
			})
		}
	}
	return taken, nil
}

// moduleLayerDir reconstructs the module layer directory that declared a
// dotfile entry: the entry's Layer label plus the run's facts reproduce the
// same path resolve keyed the overlay to (a single host/user overlay per
// resolve, so the reconstruction is exact).
func moduleLayerDir(profileRoot string, e resolve.DotfileEntry, f *facts.Facts) string {
	switch e.Layer {
	case "host":
		return filepath.Join(profileRoot, "hosts", f.Hostname, "modules", e.Module)
	case "user":
		return filepath.Join(profileRoot, "users", f.Username, "modules", e.Module)
	default:
		return filepath.Join(profileRoot, "modules", e.Module)
	}
}

// buildSteps assembles the apply pipeline from the resolved plan, filtered
// by the executed sections. It splits dotfiles by scope, appends conditional
// steps (system files, mounts, smb, hooks) based on plan contents and the
// section selection, and returns them in pipeline order.
func buildSteps(sections SectionSet, plan *resolve.Plan, runner *mise.ExecMise,
	f *facts.Facts, profileRoot string, out io.Writer, misePluginsDir string,
	opts ApplyOpts, deps ApplyDeps, paths configPaths,
) []apply.Step {
	backend := deps.PackagesFor(f.Backend)

	// The dotfiles portion splits by scope: user entries apply as today via
	// the DotfilesStep (against a scope-filtered plan copy), system entries
	// get their own step applied with root privileges. The dotfiles-system
	// step is appended only when at least one system-scope entry exists.
	userPlan := *plan
	var userEntries, systemEntries []resolve.DotfileEntry
	if sections.Has("dotfiles") {
		for _, e := range plan.Dotfiles.Entries {
			if e.Scope == profile.ScopeSystem {
				systemEntries = append(systemEntries, e)
			} else {
				userEntries = append(userEntries, e)
			}
		}
	}
	userPlan.Dotfiles.Entries = userEntries

	// Mount destination dirs belong to the mounts section.
	var mountDests []string
	if sections.Has("mounts") {
		for _, e := range plan.Mounts.Entries {
			mountDests = append(mountDests, e.Spec.Destination)
		}
	}

	var steps []apply.Step
	if sections.Has("hooks") && len(plan.Hooks.Pre) > 0 {
		steps = append(steps, &mise.HooksStep{
			Exec: runner, Commands: plan.Hooks.Pre, ConfigPath: paths.shared,
			Task: "hooks-pre", StepName: "hooks-pre",
		})
	}
	if sections.Has("packages") {
		steps = append(steps, &packagesStep{runner: runner, backend: backend, plan: plan, backendStr: f.Backend, configPath: paths.packages, misePluginsDir: misePluginsDir})
	}
	if sections.Has("tools") {
		steps = append(steps, &mise.ToolsStep{Runner: runner, Plan: plan, ConfigPath: paths.tools, FragmentPath: mise.ToolsFragmentPath()})
	}
	if sections.Has("dotfiles") {
		steps = append(steps, &mise.DotfilesStep{Runner: runner, Plan: &userPlan, ConfigPath: paths.dotfiles, Yes: opts.Yes, Force: opts.Force})
	}
	// systemd user units → mise bootstrap --only linux-systemd-units.
	if sections.Has("systemd") && len(plan.Systemd.Units) > 0 {
		steps = append(steps, &systemdUnitsStep{
			runner: runner, units: plan.Systemd.Units, configPath: paths.systemd, yes: opts.Yes,
		})
	}
	// System dotfiles + mount directories → systemFilesStep.
	// Runs when there are system-scope dotfiles OR mount destinations.
	if len(systemEntries) > 0 || len(mountDests) > 0 {
		homeDir, _ := os.UserHomeDir()
		steps = append(steps, &systemFilesStep{
			exec: runner, entries: systemEntries, sourceRoot: profileRoot,
			homeDir: homeDir, dirs: mountDests, configPath: paths.system,
			editsPath: paths.systemEdits, yes: opts.Yes, force: opts.Force, secrets: plan.Secrets,
		})
	}
	// Mount unit services → mise bootstrap --only services.
	if sections.Has("mounts") && len(plan.Mounts.Entries) > 0 {
		steps = append(steps, &mountsServicesStep{
			runner: runner, entries: plan.Mounts.Entries, configPath: paths.mounts,
		})
	}
	// SMB accounts + services → mise bootstrap --only accounts,services,
	// then interactive smbpasswd/testparm post-actions.
	if sections.Has("smb") && len(plan.Smb.Modules) > 0 {
		sr := deps.NewSmbRunner()
		steps = append(steps, &smbBootstrapStep{
			runner: runner, modules: plan.Smb.Modules, configPath: paths.smb,
			smbRunner: sr, out: out,
		})
	}
	if sections.Has("hooks") && len(plan.Hooks.Post) > 0 {
		steps = append(steps, &mise.HooksStep{
			Exec: runner, Commands: plan.Hooks.Post, ConfigPath: paths.shared,
			Task: "hooks-post", StepName: "hooks-post",
		})
	}
	return steps
}
