package dotdrift

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/thedataflows/dotdrift/internal/apply"
	"github.com/thedataflows/dotdrift/internal/detect"
	"github.com/thedataflows/dotdrift/internal/mise"
	"github.com/thedataflows/dotdrift/internal/packages"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/resolve"
	"github.com/thedataflows/dotdrift/internal/state"
)

// ApplyCmd runs the full pipeline and always resumes.
type ApplyCmd struct {
	Profile string `help:"Path to profile directory" type:"existingdir" default:"."`
	State   string `help:"Path to state file" type:"path" default:""`
	Yes     bool   `help:"Answer yes to mise prompts" default:"false"`
}

// Run executes the apply pipeline with resume semantics.
func (c *ApplyCmd) Run() error {
	f, err := detect.Detect()
	if err != nil {
		return fmt.Errorf("detect: %w", err)
	}

	p, err := profile.Load(c.Profile, f)
	if err != nil {
		return fmt.Errorf("load profile: %w", err)
	}

	plan, err := resolve.Resolve(p, f)
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
		s.ResetForSelection()
		s.Selection = fingerprint
	}

	m := mise.DefaultMise()
	path, err := m.Ensure()
	if err != nil {
		return fmt.Errorf("ensure mise: %w", err)
	}
	_ = path
	runner := mise.NewExecMise(m)

	configDir := filepath.Join(filepath.Dir(statePath), "mise")
	configPath := filepath.Join(configDir, "mise.toml")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return fmt.Errorf("create mise config dir: %w", err)
	}
	if err := os.WriteFile(configPath, []byte(mise.GenerateConfig(plan)), 0o644); err != nil {
		return fmt.Errorf("write mise config: %w", err)
	}

	backend := packages.For(f.Backend)
	steps := []apply.Step{
		packages.NewStep(backend, plan),
		&mise.ToolsStep{Runner: runner, Plan: plan, ConfigPath: configPath},
		&mise.DotfilesStep{Runner: runner, Plan: plan, ConfigPath: configPath, Yes: c.Yes},
		&mise.HooksStep{},
	}

	pipeline := apply.NewPipeline(steps, store.Save)
	pipeline.SetState(s)
	if err := pipeline.Run(context.Background()); err != nil {
		return fmt.Errorf("apply: %w", err)
	}
	return nil
}
