package service

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/mise"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/resolve"
)

// Issue 0162: a failed services step names the declared services with
// their modules, like the packages step (issue 0152).
func TestServicesStep_failureNamesServices(t *testing.T) {
	enabled := true
	step := &servicesStep{
		runner: &mise.FakeRunner{Err: errors.New("mise bootstrap services: exit status 1")},
		entries: []resolve.ServiceEntry{
			{Name: "limine-snapper-sync", Module: "boot", Scope: profile.ScopeSystem, Spec: profile.ServiceSpec{State: "running", Enabled: &enabled}},
			{Name: "dsh", Module: "dsh", Scope: profile.ScopeUser, Spec: profile.ServiceSpec{Command: "/bin/dsh", State: "running", Enabled: &enabled}},
		},
		configPath: filepath.Join(t.TempDir(), "mise.toml"),
		yes:        true,
	}
	err := step.Run(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "limine-snapper-sync (module boot, system)")
	require.Contains(t, err.Error(), "dsh (module dsh, user)")
}
