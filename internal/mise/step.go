package mise

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/thedataflows/dotdrift/internal/apply"
	"github.com/thedataflows/dotdrift/internal/resolve"
)

// ToolsStep runs mise install for the tools in the resolved plan.
type ToolsStep struct {
	Runner     Runner
	Plan       *resolve.Plan
	ConfigPath string
}

var _ apply.Step = (*ToolsStep)(nil)

func (s *ToolsStep) Name() string { return "tools" }

func (s *ToolsStep) Run(ctx context.Context) error {
	if s.Runner == nil {
		return fmt.Errorf("no mise runner configured")
	}
	if s.Plan == nil {
		return fmt.Errorf("plan is nil")
	}
	if len(s.Plan.Tools.Versions) == 0 {
		return nil
	}
	cfg := GenerateTools(s.Plan.Tools.Versions)
	if err := writeConfig(s.ConfigPath, cfg); err != nil {
		return fmt.Errorf("write tools mise config: %w", err)
	}
	if err := s.Runner.EnsureAndInstall(ctx, s.ConfigPath); err != nil {
		return fmt.Errorf("mise install: %w", err)
	}
	return nil
}

// DotfilesStep runs mise dotfiles apply for the dotfiles in the resolved plan.
type DotfilesStep struct {
	Runner     Runner
	Plan       *resolve.Plan
	ConfigPath string
	Yes        bool
}

var _ apply.Step = (*DotfilesStep)(nil)

func (s *DotfilesStep) Name() string { return "dotfiles" }

func (s *DotfilesStep) Run(ctx context.Context) error {
	if s.Runner == nil {
		return fmt.Errorf("no mise runner configured")
	}
	if s.Plan == nil {
		return fmt.Errorf("plan is nil")
	}
	if len(s.Plan.Dotfiles.Entries) == 0 {
		return nil
	}
	cfg := GenerateDotfiles(s.Plan.Dotfiles.Entries)
	if err := writeConfig(s.ConfigPath, cfg); err != nil {
		return fmt.Errorf("write dotfiles mise config: %w", err)
	}
	if err := s.Runner.DotfilesApply(ctx, s.ConfigPath, s.Yes); err != nil {
		return fmt.Errorf("mise dotfiles apply: %w", err)
	}
	return nil
}

func writeConfig(path, content string) error {
	if path == "" {
		return fmt.Errorf("config path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}
