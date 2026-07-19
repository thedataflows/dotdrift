package dotdrift_test

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	cmd "github.com/thedataflows/dotdrift/cmd"
	"github.com/thedataflows/dotdrift/internal/facts"
)

type planJSON struct {
	Fingerprint string   `json:"fingerprint"`
	Modules     []string `json:"modules"`
	Packages    struct {
		Install []string `json:"install"`
		Remove  []string `json:"remove"`
	} `json:"packages"`
	Tools    map[string]string `json:"tools"`
	Dotfiles []struct {
		Target string `json:"target"`
		Source string `json:"source"`
		Mode   string `json:"mode"`
		Module string `json:"module"`
		Layer  string `json:"layer"`
		Scope  string `json:"scope"`
	} `json:"dotfiles"`
	Hooks struct {
		Pre  []string `json:"pre"`
		Post []string `json:"post"`
	} `json:"hooks"`
}

func TestCLI_plan_output(t *testing.T) {
	var buf bytes.Buffer
	c := &cmd.PlanCmd{
		Profile: filepath.Join("..", "testdata", "profiles", "resolve"),
		Facts:   &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux"},
		Out:     &buf,
	}
	err := c.Run()
	require.NoError(t, err)

	out := buf.String()
	require.Contains(t, out, "fingerprint:")
	require.Contains(t, out, "packages:")
	require.Contains(t, out, "neovim")
	require.Contains(t, out, "tools:")
	require.Contains(t, out, "node:")
	require.Contains(t, out, "dotfiles:")
	require.Contains(t, out, "~/.bashrc")
	require.True(t, strings.Contains(out, "users/cri/modules/shell"), "plan should resolve user overlay file")
}

// Hooks are visible in the text plan: the actual pre/post commands, appended
// base → host → user.
func TestCLI_plan_hooksSection(t *testing.T) {
	var buf bytes.Buffer
	c := &cmd.PlanCmd{
		Profile: filepath.Join("..", "testdata", "profiles", "resolve"),
		Facts:   &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux"},
		Out:     &buf,
	}
	require.NoError(t, c.Run())

	out := buf.String()
	require.Contains(t, out, "hooks:")
	require.Contains(t, out, "  pre:")
	require.Contains(t, out, "  post:")
	for _, cmd := range []string{"echo base-pre", "echo host-pre", "echo user-pre",
		"echo base-post", "echo host-post", "echo user-post"} {
		require.Contains(t, out, "- "+cmd)
	}
	// Append order base → host → user must be visible in the rendering.
	require.Less(t, strings.Index(out, "echo base-pre"), strings.Index(out, "echo host-pre"))
	require.Less(t, strings.Index(out, "echo host-pre"), strings.Index(out, "echo user-pre"))
}

func TestCLI_plan_noSideEffects(t *testing.T) {
	var buf bytes.Buffer
	c := &cmd.PlanCmd{
		Profile: filepath.Join("..", "testdata", "profiles", "resolve"),
		Facts:   &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux"},
		Out:     &buf,
	}
	err := c.Run()
	require.NoError(t, err)
	require.Contains(t, buf.String(), "fingerprint:")
}

func TestCLI_plan_noModulesSelectedWarning(t *testing.T) {
	var buf bytes.Buffer
	c := &cmd.PlanCmd{
		Profile: filepath.Join("..", "testdata", "profiles", "minimal"),
		Facts:   &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux"},
		Out:     &buf,
	}
	err := c.Run()
	require.NoError(t, err)
	require.Contains(t, buf.String(), "warning: no modules selected")
}

func TestCLI_plan_jsonOutput(t *testing.T) {
	var buf bytes.Buffer
	c := &cmd.PlanCmd{
		Profile: filepath.Join("..", "testdata", "profiles", "resolve"),
		Facts:   &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux"},
		Out:     &buf,
		JSON:    true,
	}
	err := c.Run()
	require.NoError(t, err)

	var got planJSON
	require.NoError(t, json.Unmarshal(buf.Bytes(), &got), "plan --json must emit a single parseable JSON object")

	require.Equal(t, []string{"shell"}, got.Modules)
	require.Contains(t, got.Fingerprint, "selected=shell")
	require.Contains(t, got.Fingerprint, "hostname=myhost")
	require.Contains(t, got.Fingerprint, "username=cri")

	require.Equal(t, []string{"eza", "fd", "neovim", "ripgrep"}, got.Packages.Install)
	require.Equal(t, []string{"emacs", "nano"}, got.Packages.Remove)

	require.Equal(t, map[string]string{"node": "22", "python": "3.12"}, got.Tools)

	require.Len(t, got.Dotfiles, 2)
	byTarget := make(map[string]struct {
		Source string
		Mode   string
		Module string
		Layer  string
	})
	for _, d := range got.Dotfiles {
		byTarget[d.Target] = struct {
			Source string
			Mode   string
			Module string
			Layer  string
		}{d.Source, d.Mode, d.Module, d.Layer}
	}
	bashrc, ok := byTarget["~/.bashrc"]
	require.True(t, ok, "dotfiles must include ~/.bashrc")
	require.Equal(t, "copy", bashrc.Mode)
	require.Equal(t, "shell", bashrc.Module)
	require.Equal(t, "user", bashrc.Layer)
	require.True(t, strings.HasSuffix(bashrc.Source, filepath.Join("users", "cri", "modules", "shell", ".bashrc")),
		"bashrc source should resolve to the user overlay, got %q", bashrc.Source)

	fish, ok := byTarget["~/.config/fish"]
	require.True(t, ok, "dotfiles must include ~/.config/fish")
	require.Equal(t, "symlink-each", fish.Mode)
	require.Equal(t, "host", fish.Layer)

	require.Equal(t, []string{"echo base-pre", "echo host-pre", "echo user-pre"}, got.Hooks.Pre)
	require.Equal(t, []string{"echo base-post", "echo host-post", "echo user-post"}, got.Hooks.Post)
}

// System-scope entries are marked in the text plan (`module: <id> [system]`);
// user-scope entries stay unmarked.
func TestCLI_plan_scopeMarker(t *testing.T) {
	var buf bytes.Buffer
	c := &cmd.PlanCmd{
		Profile: filepath.Join("..", "testdata", "profiles", "scope"),
		Facts:   &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux"},
		Out:     &buf,
	}
	require.NoError(t, c.Run())

	out := buf.String()
	require.Contains(t, out, "module: demo [system]")
	require.Contains(t, out, "module: shell\n", "user-scope modules render without a scope marker")
}

// plan --json carries the scope on every dotfile entry.
func TestCLI_plan_jsonScope(t *testing.T) {
	var buf bytes.Buffer
	c := &cmd.PlanCmd{
		Profile: filepath.Join("..", "testdata", "profiles", "scope"),
		Facts:   &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux"},
		Out:     &buf,
		JSON:    true,
	}
	require.NoError(t, c.Run())

	var got planJSON
	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))

	byTarget := make(map[string]string, len(got.Dotfiles))
	for _, d := range got.Dotfiles {
		byTarget[d.Target] = d.Scope
	}
	require.Equal(t, "system", byTarget["/etc/demo.conf"])
	require.Equal(t, "user", byTarget["~/.bashrc"])
}

func TestCLI_plan_jsonNotInDefaultOutput(t *testing.T) {
	var buf bytes.Buffer
	c := &cmd.PlanCmd{
		Profile: filepath.Join("..", "testdata", "profiles", "resolve"),
		Facts:   &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux"},
		Out:     &buf,
	}
	err := c.Run()
	require.NoError(t, err)
	require.Contains(t, buf.String(), "fingerprint:")
	require.False(t, json.Valid(buf.Bytes()), "default plan output must remain the text rendering, not JSON")
}

// Decision: with --json the "warning: no modules selected" line is suppressed
// so stdout stays a single parseable JSON object (empty modules/packages
// arrays convey the same information to machine consumers).
func TestCLI_plan_jsonNoModulesStaysParseable(t *testing.T) {
	var buf bytes.Buffer
	c := &cmd.PlanCmd{
		Profile: filepath.Join("..", "testdata", "profiles", "minimal"),
		Facts:   &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux"},
		Out:     &buf,
		JSON:    true,
	}
	err := c.Run()
	require.NoError(t, err)
	require.NotContains(t, buf.String(), "warning: no modules selected")

	var got planJSON
	require.NoError(t, json.Unmarshal(buf.Bytes(), &got), "zero-module --json output must stay parseable")
	require.Empty(t, got.Modules)
	require.Empty(t, got.Packages.Install)
	require.Empty(t, got.Dotfiles)
}
