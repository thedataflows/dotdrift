package service

// The destructive-apply signal (M15, T-tui-modals): a step that knows
// which existing destinations it will overwrite surfaces them on its
// preview, so the TUI's confirm can name them before anything runs.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/apply"
)

type overwriteStep struct{ targets []string }

func (s overwriteStep) Name() string               { return "dotfiles" }
func (s overwriteStep) Run(context.Context) error  { return nil }
func (s overwriteStep) OverwriteTargets() []string { return s.targets }

func TestClassifySteps_overwriteTargetsSurface(t *testing.T) {
	previews := classifySteps(nil)
	require.Empty(t, previews)

	previews = classifySteps([]apply.Step{overwriteStep{targets: []string{"/home/cri/.bashrc"}}})
	require.Len(t, previews, 1)
	require.Equal(t, []string{"/home/cri/.bashrc"}, previews[0].Overwrites,
		"the preview carries the overwrite list for the destructive confirm")

	plain := classifySteps([]apply.Step{plainStep{}})
	require.Empty(t, plain[0].Overwrites, "steps without the seam report nothing")
}

type plainStep struct{}

func (plainStep) Name() string              { return "packages" }
func (plainStep) Run(context.Context) error { return nil }
