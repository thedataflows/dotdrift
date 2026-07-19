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
var pipelineStepNames = []string{"packages", "tools", "dotfiles", "hooks"}

// ApplyCmd runs the full pipeline and always resumes.
type ApplyCmd struct {
	Profile string    `help:"Path to profile directory" type:"existingdir" default:"."`
	State   string    `help:"Path to state file" type:"path" default:""`
	Yes     bool      `help:"Answer yes to mise prompts" default:"false"`
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
	// [dotfiles]) before the pipeline starts. The tools/dotfiles steps later
	// rewrite this file section-by-section, so if apply crashes or fails
	// before them, the on-disk config still mirrors the whole resolved plan
	// for crash recovery and manual mise runs.
	configDir := filepath.Join(filepath.Dir(statePath), "mise")
	configPath := filepath.Join(configDir, "mise.toml")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return fmt.Errorf("create mise config dir: %w", err)
	}
	if err := os.WriteFile(configPath, []byte(mise.GenerateConfig(plan)), 0o644); err != nil {
		return fmt.Errorf("write mise config: %w", err)
	}

	backend := packagesFor(f.Backend)
	steps := []apply.Step{
		packages.NewStep(backend, plan),
		&mise.ToolsStep{Runner: runner, Plan: plan, ConfigPath: configPath},
		&mise.DotfilesStep{Runner: runner, Plan: plan, ConfigPath: configPath, Yes: c.Yes},
	}

	pipeline := apply.NewPipeline(steps, store.Save)
	pipeline.SetState(s)
	if err := pipeline.Run(context.Background()); err != nil {
		return fmt.Errorf("apply: %w", err)
	}
	return nil
}
