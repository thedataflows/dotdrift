package mise_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/mise"
)

// Issue 0159: an entry with a command emits the table form with scope,
// command, state, enabled, restart, environment, and working_directory.
func TestGenerateBootstrapServices_userCommand(t *testing.T) {
	got := mise.GenerateBootstrapServices([]mise.BootstrapService{
		{Name: "my-agent", Scope: "user", Command: "~/.local/bin/my-agent --serve",
			State: "running", Enabled: true, Restart: "on-failure",
			Environment: map[string]string{"LOG_LEVEL": "info"}, WorkingDirectory: "~"},
	})
	require.Contains(t, got, "[bootstrap.services.my-agent]")
	require.Contains(t, got, "scope = \"user\"")
	require.Contains(t, got, "command = \"~/.local/bin/my-agent --serve\"")
	require.Contains(t, got, "state = \"running\"")
	require.Contains(t, got, "enabled = true")
	require.Contains(t, got, "restart = \"on-failure\"")
	require.Contains(t, got, "environment = { LOG_LEVEL = \"info\" }")
	require.Contains(t, got, "working_directory = \"~\"")
}

// An existing-unit entry (no command) still declares its scope, so mise
// runs systemctl --user and writes no unit definition.
func TestGenerateBootstrapServices_existingUserUnit(t *testing.T) {
	got := mise.GenerateBootstrapServices([]mise.BootstrapService{
		{Name: "syncthing", Scope: "user", State: "running", Enabled: true},
	})
	require.Contains(t, got, "[bootstrap.services.syncthing]")
	require.Contains(t, got, "scope = \"user\"")
	require.NotContains(t, got, "command")
}
