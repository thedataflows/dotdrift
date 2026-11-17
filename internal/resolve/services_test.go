package resolve_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
)

// Issue 0159: services merge whole-entry by name across layers, carry
// the module scope, and normalize state and enabled defaults.
func TestResolve_servicesMergeLayers(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "svc", "[services.agent]\ncommand = \"/usr/bin/base-agent\"\n\n[services.syncthing]\n")
	writeOverlayModule(t, root, "users", "cri", "svc", "[services.agent]\ncommand = \"/usr/bin/user-agent\"\nstate = \"stopped\"\nenabled = false\n")
	plan, err := loadAndResolve(t, root, &facts.Facts{Hostname: "h", Username: "cri"})
	require.NoError(t, err)
	require.Len(t, plan.Services.Entries, 2)
	byName := map[string]int{}
	for i, e := range plan.Services.Entries {
		byName[e.Name] = i
	}
	agent := plan.Services.Entries[byName["agent"]]
	require.Equal(t, "/usr/bin/user-agent", agent.Spec.Command)
	require.Equal(t, "stopped", agent.Spec.State)
	require.NotNil(t, agent.Spec.Enabled)
	require.False(t, *agent.Spec.Enabled)
	require.Equal(t, "user", agent.Layer)
	require.Equal(t, "user", agent.Scope)

	sync := plan.Services.Entries[byName["syncthing"]]
	require.Equal(t, "running", sync.Spec.State, "state defaults to running")
	require.NotNil(t, sync.Spec.Enabled)
	require.True(t, *sync.Spec.Enabled, "enabled defaults to true")
	require.Equal(t, "base", sync.Layer)
}

func TestResolve_servicesValidation(t *testing.T) {
	cases := []struct {
		name, toml, want string
	}{
		{"bad name", "[services.\"has space\"]\n", "has space"},
		{"bad state", "[services.a]\nstate = \"flying\"\n", "state"},
		{"bad restart", "[services.a]\ncommand = \"/bin/a\"\nrestart = \"sometimes\"\n", "restart"},
		{"restart without command", "[services.a]\nrestart = \"always\"\n", "command"},
		{"environment without command", "[services.a]\nenvironment = { A = \"1\" }\n", "command"},
		{"working_directory without command", "[services.a]\nworking_directory = \"/tmp\"\n", "command"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeModule(t, root, "svc", tc.toml)
			_, err := loadAndResolve(t, root, &facts.Facts{Hostname: "h", Username: "cri"})
			require.Error(t, err)
			require.Contains(t, err.Error(), "svc")
			require.Contains(t, err.Error(), tc.want)
		})
	}
}

// command is user scope only: a system module cannot define a program.
func TestResolve_servicesCommandSystemScopeErrors(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "svc", "scope = \"system\"\n\n[services.agent]\ncommand = \"/usr/bin/agent\"\n")
	_, err := loadAndResolve(t, root, &facts.Facts{Hostname: "h", Username: "cri"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "command")
}

// A name in both [services] and [systemd.units] is an error: both would
// write the same unit definition.
func TestResolve_servicesSystemdNameConflict(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "svc", "[services.my-sync]\ncommand = \"/usr/bin/my-sync\"\n\n[systemd.units.my-sync]\nexec_start = \"/usr/bin/my-sync\"\n")
	_, err := loadAndResolve(t, root, &facts.Facts{Hostname: "h", Username: "cri"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "my-sync")
	require.Contains(t, err.Error(), "systemd")
}
