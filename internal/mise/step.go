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

// DotfilesSystemStep applies system-scope dotfile entries with root
// privileges. It takes the concrete ExecMise (like HooksStep) because the
// sudo-aware entry point is deliberately not part of the Runner interface.
// cmd/apply.go only constructs it when at least one system-scope entry
// exists; Run also no-ops on an empty list as a second line of defense.
type DotfilesSystemStep struct {
	Exec       *ExecMise
	Entries    []resolve.DotfileEntry
	ConfigPath string
	Yes        bool
}

var _ apply.Step = (*DotfilesSystemStep)(nil)

func (s *DotfilesSystemStep) Name() string { return "dotfiles-system" }

func (s *DotfilesSystemStep) Run(ctx context.Context) error {
	if len(s.Entries) == 0 {
		return nil
	}
	if s.Exec == nil {
		return fmt.Errorf("no mise exec configured")
	}
	cfg := GenerateDotfiles(s.Entries)
	if err := writeConfig(s.ConfigPath, cfg); err != nil {
		return fmt.Errorf("write system dotfiles mise config: %w", err)
	}
	if err := s.Exec.DotfilesApplySudo(ctx, s.ConfigPath, s.Yes); err != nil {
		return fmt.Errorf("mise dotfiles apply (system): %w", err)
	}
	return nil
}

// HooksStep runs one pre/post hook command list as a mise task from the// generated apply config. cmd/apply.go only constructs HooksSteps for
// non-empty command lists; Run also no-ops on an empty list as a second line
// of defense.
type HooksStep struct {
	Exec       *ExecMise
	Commands   []string
	ConfigPath string
	Task       string // mise task name, e.g. "hooks:pre"
	StepName   string // pipeline step name, e.g. "hooks-pre"
}

var _ apply.Step = (*HooksStep)(nil)

func (s *HooksStep) Name() string { return s.StepName }

func (s *HooksStep) Run(ctx context.Context) error {
	if len(s.Commands) == 0 {
		return nil
	}
	if s.Exec == nil {
		return fmt.Errorf("no mise exec configured")
	}
	if err := s.Exec.RunTask(ctx, s.ConfigPath, s.Task); err != nil {
		return fmt.Errorf("mise task %s: %w", s.Task, err)
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
