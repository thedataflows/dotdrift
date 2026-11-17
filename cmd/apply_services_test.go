package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
)

// servicesFixture builds a profile with one module declaring a command
// service and an existing-unit service (issue 0159).
func servicesFixture(t *testing.T) string {
	t.Helper()
	profileDir := t.TempDir()
	modDir := filepath.Join(profileDir, "modules", "svc")
	require.NoError(t, os.MkdirAll(modDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(modDir, "module.toml"), []byte("[services.my-agent]\ncommand = \"~/.local/bin/my-agent --serve\"\n\n[services.syncthing]\n"), 0o644))
	return profileDir
}

// A profile with declared services gains a services step that converges
// via mise bootstrap --only services from its own config dir.
func TestApply_servicesStep(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Backend: "paru"}
	fk := stubApplyDeps(t, f)

	cmd := &ApplyCmd{deps: &fk.deps, Profile: servicesFixture(t), State: statePath, Yes: true}
	require.NoError(t, cmd.Run())

	found := false
	for _, e := range *fk.events {
		if strings.Contains(e, "bootstrap") && strings.Contains(e, filepath.Join("mise", "services")) && strings.Contains(e, "--only services") {
			found = true
		}
	}
	require.True(t, found, "services bootstrap missing in %v", *fk.events)

	cfg, err := os.ReadFile(filepath.Join(dir, "mise", "services", "mise.toml"))
	require.NoError(t, err)
	require.Contains(t, string(cfg), "[bootstrap.services.my-agent]")
	require.Contains(t, string(cfg), "scope = \"user\"")
	require.Contains(t, string(cfg), "command = \"~/.local/bin/my-agent --serve\"")
	require.Contains(t, string(cfg), "[bootstrap.services.syncthing]")
	require.Contains(t, string(cfg), "min_version = \"2026.11.9\"")
}

// --no-services drops the step and writes no services config.
func TestApply_sectionFlagsServices(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Backend: "paru"}
	fk := stubApplyDeps(t, f)

	cmd := &ApplyCmd{deps: &fk.deps, Profile: servicesFixture(t), State: statePath, Yes: true,
		onlySections: []string{"packages", "tools", "dotfiles", "systemd", "mounts", "smb", "hooks"}}
	require.NoError(t, cmd.Run())
	for _, e := range *fk.events {
		require.NotContains(t, e, filepath.Join("mise", "services"), "services step must not run when deselected")
	}
	_, err := os.Stat(filepath.Join(dir, "mise", "services"))
	require.True(t, os.IsNotExist(err), "no services config dir when deselected")
}
