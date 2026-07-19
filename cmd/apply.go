package dotdrift

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/rs/zerolog/log"
	"github.com/thedataflows/dotdrift/internal/apply"
	"github.com/thedataflows/dotdrift/internal/detect"
	"github.com/thedataflows/dotdrift/internal/mise"
	"github.com/thedataflows/dotdrift/internal/packages"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/resolve"
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
)

// pipelineStepNames is the single source of truth for the ordered pipeline
// step names: apply builds its steps in this order and status reports
// progress against it. Update this list when adding or removing a step.
// hooks-pre/hooks-post and dotfiles-system are conditional: they only run
// when the plan has hook commands / system-scope dotfile entries, so a
// completed apply may legitimately show fewer completed steps than the
// denominator.
var pipelineStepNames = []string{"hooks-pre", "packages", "tools", "dotfiles", "dotfiles-system", "hooks-post"}

// ApplyCmd runs the full pipeline and always resumes.
type ApplyCmd struct {
	Profile string    `help:"Path to profile directory" type:"existingdir" default:"."`
	State   string    `help:"Path to state file" type:"path" default:""`
	Yes     bool      `help:"Answer yes to mise prompts" default:"false"`
	NoHooks bool      `help:"Skip pre/post hook commands (also DOTDRIFT_NO_HOOKS=1)" default:"false"`
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
	// until the pipeline's last save, so two applies can never interleave
	// load→pipeline→save on the same state file.
	if err := store.Lock(); err != nil {
		return fmt.Errorf("lock state: %w", err)
	}
	defer func() { _ = store.Unlock() }()
	s, err := store.Load()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	fingerprint := resolve.Fingerprint(p, f)
	if s.Selection != fingerprint {
		if s.Selection != "" {
			log.Warn().Msg("facts or module selection changed since last apply; resume state was reset")
		}
		s.ResetForSelection()
		s.Selection = fingerprint
	}

	out := c.Out
	if out == nil {
		out = os.Stdout
	}
	if err := printPlan(out, plan, p, f); err != nil {
		return err
	}

	m := defaultMise()
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
	profileRoot, err := filepath.Abs(p.Root)
	if err != nil {
		return fmt.Errorf("resolve profile root: %w", err)
	}
	configDir := filepath.Join(filepath.Dir(statePath), "mise")
	configPath := filepath.Join(configDir, "mise.toml")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return fmt.Errorf("create mise config dir: %w", err)
	}
	if err := os.WriteFile(configPath, []byte(mise.GenerateApplyConfig(plan, profileRoot, f)), 0o644); err != nil {
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
	dotfilesSystemConfigPath := filepath.Join(configDir, "dotfiles-system", "mise.toml")

	// Hooks steps are skipped at construction when their command list is
	// empty or when the user opted out via --no-hooks / DOTDRIFT_NO_HOOKS=1
	// (HooksStep.Run also no-ops on an empty list as a second line of
	// defense). hooks-pre runs before packages so a pre-hook failure aborts
	// before any side effect; hooks-post runs last.
	hooksDisabled := c.NoHooks || os.Getenv("DOTDRIFT_NO_HOOKS") == "1"
	backend := packagesFor(f.Backend)

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
			Task: "hooks:pre", StepName: "hooks-pre",
		})
	}
	steps = append(steps,
		packages.NewStep(backend, plan),
		&mise.ToolsStep{Runner: runner, Plan: plan, ConfigPath: toolsConfigPath},
		&mise.DotfilesStep{Runner: runner, Plan: &userPlan, ConfigPath: dotfilesConfigPath, Yes: c.Yes},
	)
	if len(systemEntries) > 0 {
		steps = append(steps, &mise.DotfilesSystemStep{
			Exec: runner, Entries: systemEntries, ConfigPath: dotfilesSystemConfigPath, Yes: c.Yes,
		})
	}
	if !hooksDisabled && len(plan.Hooks.Post) > 0 {
		steps = append(steps, &mise.HooksStep{
			Exec: runner, Commands: plan.Hooks.Post, ConfigPath: configPath,
			Task: "hooks:post", StepName: "hooks-post",
		})
	}

	pipeline := apply.NewPipeline(steps, store.Save)
	pipeline.SetState(s)
	if err := pipeline.Run(context.Background()); err != nil {
		return fmt.Errorf("apply: %w", err)
	}
	return nil
}
