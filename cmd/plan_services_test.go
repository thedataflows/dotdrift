package cmd_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	cmd "github.com/thedataflows/dotdrift/cmd"
	"github.com/thedataflows/dotdrift/internal/facts"
)

// The plan services section lists each declared service (issue 0159).
func TestCLI_plan_servicesSection(t *testing.T) {
	profileDir := t.TempDir()
	modDir := filepath.Join(profileDir, "modules", "svc")
	require.NoError(t, os.MkdirAll(modDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(modDir, "module.toml"), []byte("[services.my-agent]\ncommand = \"~/.local/bin/my-agent --serve\"\n\n[services.syncthing]\n"), 0o644))

	var buf bytes.Buffer
	c := &cmd.PlanCmd{
		Profile: profileDir,
		Facts:   &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux"},
		Out:     &buf,
	}
	require.NoError(t, c.Run())
	out := buf.String()
	require.Contains(t, out, "services:\n")
	require.Contains(t, out, "svc: my-agent [base][user] command \"~/.local/bin/my-agent --serve\" state=running enabled=true")
	require.Contains(t, out, "svc: syncthing [base][user] state=running enabled=true")
}

func TestCLI_plan_jsonServices(t *testing.T) {
	profileDir := t.TempDir()
	modDir := filepath.Join(profileDir, "modules", "svc")
	require.NoError(t, os.MkdirAll(modDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(modDir, "module.toml"), []byte("[services.syncthing]\n"), 0o644))

	var buf bytes.Buffer
	c := &cmd.PlanCmd{
		Profile: profileDir,
		Facts:   &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux"},
		JSON:    true,
		Out:     &buf,
	}
	require.NoError(t, c.Run())

	var doc planJSON
	require.NoError(t, json.Unmarshal(buf.Bytes(), &doc))
	require.Len(t, doc.Services, 1)
	require.Equal(t, "syncthing", doc.Services[0].Name)
	require.Equal(t, "running", doc.Services[0].State)
	require.Equal(t, "user", doc.Services[0].Scope)
	require.Equal(t, "svc", doc.Services[0].Module)
}
