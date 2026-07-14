package mise_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/mise"
	"github.com/thedataflows/dotdrift/internal/resolve"
)

func TestGenerateTools(t *testing.T) {
	out := mise.GenerateTools(map[string]string{
		"node":    "20",
		"python":  "3.12",
		"rust":    "stable",
	})
	require.Contains(t, out, "[tools]")
	require.Contains(t, out, `node = "20"`)
	require.Contains(t, out, `python = "3.12"`)
	require.Contains(t, out, `rust = "stable"`)
}

func TestGenerateDotfiles(t *testing.T) {
	out := mise.GenerateDotfiles([]resolve.DotfileEntry{
		{Target: "~/.bashrc", Source: ".bashrc", Mode: "link"},
		{Target: "~/.config/nvim", Source: "nvim", Mode: "symlink-each"},
	})
	require.Contains(t, out, "[dotfiles]")
	require.Contains(t, out, `"~/.bashrc" = { source = ".bashrc", mode = "link" }`)
	require.Contains(t, out, `"~/.config/nvim" = { source = "nvim", mode = "symlink-each" }`)
}

func TestGenerateConfig(t *testing.T) {
	plan := &resolve.Plan{
		Tools: resolve.ToolsStep{Versions: map[string]string{"node": "20"}},
		Dotfiles: resolve.DotfilesStep{Entries: []resolve.DotfileEntry{
			{Target: "~/.bashrc", Source: ".bashrc", Mode: "link"},
		}},
	}
	out := mise.GenerateConfig(plan)
	require.Contains(t, out, "[tools]")
	require.Contains(t, out, "[dotfiles]")
	require.Contains(t, out, `node = "20"`)
	require.Contains(t, out, `"~/.bashrc" = { source = ".bashrc", mode = "link" }`)
}

func TestToolsStep_callsInstall(t *testing.T) {
	fr := &mise.FakeRunner{}
	plan := &resolve.Plan{Tools: resolve.ToolsStep{Versions: map[string]string{"node": "20"}}}
	step := &mise.ToolsStep{Runner: fr, Plan: plan, ConfigPath: "/tmp/mise-tools.toml"}
	require.Equal(t, "tools", step.Name())
	require.NoError(t, step.Run(context.Background()))
	require.True(t, fr.InstallCalled)
	require.False(t, fr.DotfilesCalled)
}

func TestToolsStep_failurePersistsError(t *testing.T) {
	boom := errors.New("boom")
	fr := &mise.FakeRunner{Err: boom}
	plan := &resolve.Plan{Tools: resolve.ToolsStep{Versions: map[string]string{"node": "20"}}}
	step := &mise.ToolsStep{Runner: fr, Plan: plan, ConfigPath: "/tmp/mise-tools.toml"}
	err := step.Run(context.Background())
	require.ErrorIs(t, err, boom)
}

func TestDotfilesStep_callsApply(t *testing.T) {
	fr := &mise.FakeRunner{}
	plan := &resolve.Plan{Dotfiles: resolve.DotfilesStep{Entries: []resolve.DotfileEntry{
		{Target: "~/.bashrc", Source: ".bashrc", Mode: "link"},
	}}}
	step := &mise.DotfilesStep{Runner: fr, Plan: plan, ConfigPath: "/tmp/mise-dotfiles.toml", Yes: true}
	require.Equal(t, "dotfiles", step.Name())
	require.NoError(t, step.Run(context.Background()))
	require.True(t, fr.DotfilesCalled)
	require.True(t, fr.Yes)
	require.False(t, fr.InstallCalled)
}

func TestDotfilesStep_conflictStops(t *testing.T) {
	boom := errors.New("conflict")
	fr := &mise.FakeRunner{Err: boom}
	plan := &resolve.Plan{Dotfiles: resolve.DotfilesStep{Entries: []resolve.DotfileEntry{
		{Target: "~/.bashrc", Source: ".bashrc", Mode: "link"},
	}}}
	step := &mise.DotfilesStep{Runner: fr, Plan: plan, ConfigPath: "/tmp/mise-dotfiles.toml", Yes: false}
	err := step.Run(context.Background())
	require.ErrorIs(t, err, boom)
}

func TestHooksStep_noop(t *testing.T) {
	step := &mise.HooksStep{}
	require.Equal(t, "hooks", step.Name())
	require.NoError(t, step.Run(context.Background()))
}
