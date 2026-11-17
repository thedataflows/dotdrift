package service

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/mise"
	"github.com/thedataflows/dotdrift/internal/resolve"
)

// When mise fails inside the packages step (for example the paru hook
// error), the step error must say which module declared each package in
// the run, so the user can find the config to fix (0152).
func TestPackagesStep_bootstrapFailure_namesDeclaringModules(t *testing.T) {
	plan := &resolve.Plan{Packages: resolve.PackagesStep{
		Install:        []string{"ripgrep", "bat"},
		PresentModules: map[string][]string{"ripgrep": {"shell"}, "bat": {"editor", "shell"}},
	}}
	step := &packagesStep{
		runner:     &mise.FakeRunner{Err: errors.New("exit status 1")},
		plan:       plan,
		backendStr: "paru",
		configPath: filepath.Join(t.TempDir(), "mise.toml"),
	}
	err := step.Run(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "ripgrep")
	require.Contains(t, err.Error(), "shell")
	require.Contains(t, err.Error(), "bat")
	require.Contains(t, err.Error(), "editor")
}
