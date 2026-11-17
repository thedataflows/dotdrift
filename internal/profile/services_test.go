package profile_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/profile"
)

// Issue 0159: [services.<name>] parses as a typed spec. Enabled is a
// pointer so an absent key differs from an explicit false.
func TestLoadModuleTOML_services(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/svc", "[services.my-agent]\ncommand = \"~/.local/bin/my-agent --serve\"\nstate = \"running\"\nenabled = true\nrestart = \"on-failure\"\nenvironment = { LOG_LEVEL = \"info\" }\nworking_directory = \"~\"\n\n[services.syncthing]\n")
	p, err := profile.Load(root, &facts.Facts{})
	require.NoError(t, err)
	m := findModule(t, p, "svc")

	agent := m.Config.Services["my-agent"]
	require.Equal(t, "~/.local/bin/my-agent --serve", agent.Command)
	require.Equal(t, "running", agent.State)
	require.NotNil(t, agent.Enabled)
	require.True(t, *agent.Enabled)
	require.Equal(t, "on-failure", agent.Restart)
	require.Equal(t, map[string]string{"LOG_LEVEL": "info"}, agent.Environment)
	require.Equal(t, "~", agent.WorkingDirectory)

	existing := m.Config.Services["syncthing"]
	require.Equal(t, "", existing.Command)
	require.Nil(t, existing.Enabled)
}
