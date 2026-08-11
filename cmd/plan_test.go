package cmd_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	cmd "github.com/thedataflows/dotdrift/cmd"
	"github.com/thedataflows/dotdrift/internal/facts"
)

type planJSONHook struct {
	Command  string `json:"command"`
	Optional bool   `json:"optional"`
}

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
		Pre  []planJSONHook `json:"pre"`
		Post []planJSONHook `json:"post"`
	} `json:"hooks"`
	Mounts []struct {
		Module      string   `json:"module"`
		Name        string   `json:"name"`
		Source      string   `json:"source"`
		Destination string   `json:"destination"`
		Type        string   `json:"type"`
		Options     []string `json:"options"`
		StartAt     string   `json:"startat"`
		State       string   `json:"state"`
		Layer       string   `json:"layer"`
		Scope       string   `json:"scope"`
	} `json:"mounts"`
	Smb []struct {
		Module string   `json:"module"`
		Group  string   `json:"group"`
		Users  []string `json:"users"`
		Avahi  *bool    `json:"avahi"`
		Shares map[string]struct {
			Path       string `json:"path"`
			Comment    string `json:"comment"`
			ValidUsers string `json:"valid_users"`
			Writable   bool   `json:"writable"`
			Public     bool   `json:"public"`
		} `json:"shares"`
	} `json:"smb"`
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

	require.Equal(t, []planJSONHook{{Command: "echo base-pre"}, {Command: "echo host-pre"}, {Command: "echo user-pre"}}, got.Hooks.Pre)
	require.Equal(t, []planJSONHook{{Command: "echo base-post"}, {Command: "echo host-post"}, {Command: "echo user-post"}}, got.Hooks.Post)
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

// The mounts section lists each resolved entry as
// `<module>: <name> <type> <source> -> <destination> [layer][scope]` with
// startat/state markers only when set, after the dotfiles/hooks sections.
func TestCLI_plan_mountsSection(t *testing.T) {
	var buf bytes.Buffer
	c := &cmd.PlanCmd{
		Profile: filepath.Join("..", "testdata", "profiles", "mounts"),
		Facts:   &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux"},
		Out:     &buf,
	}
	require.NoError(t, c.Run())

	out := buf.String()
	t.Log(out)
	require.Contains(t, out, "mounts:\n")
	require.Contains(t, out, "  nas: backup ext4 UUID=backup-1 -> /mnt/backup [base][system] state=disabled\n")
	require.Contains(t, out, "  nas: data cifs //server/data -> /mnt/data [base][system] startat=*-*-* 04:00\n")
	require.NotContains(t, out, "backup ext4 UUID=backup-1 -> /mnt/backup [base][system] startat",
		"startat marker only renders when set")
	require.NotContains(t, out, "data cifs //server/data -> /mnt/data [base][system] state",
		"state marker only renders when set")
	require.Greater(t, strings.Index(out, "mounts:"), strings.Index(out, "hooks:"),
		"mounts renders after the hooks section")
	// Entries are sorted by (module, name): backup before data.
	require.Less(t, strings.Index(out, "nas: backup"), strings.Index(out, "nas: data"))
}

// The smb section lists per module the effective group/users/avahi and each
// share as `name -> path` (sorted by share name).
func TestCLI_plan_smbSection(t *testing.T) {
	var buf bytes.Buffer
	c := &cmd.PlanCmd{
		Profile: filepath.Join("..", "testdata", "profiles", "mounts"),
		Facts:   &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux"},
		Out:     &buf,
	}
	require.NoError(t, c.Run())

	out := buf.String()
	t.Log(out)
	require.Contains(t, out, "smb:\n")
	require.Contains(t, out, "  nas:\n")
	require.Contains(t, out, "    group: nas\n")
	require.Contains(t, out, "    users: cri\n")
	require.Contains(t, out, "    avahi: false\n")
	require.Contains(t, out, "    shares:\n")
	require.Contains(t, out, "      data -> /mnt/data\n")
	require.Contains(t, out, "      media -> /srv/media\n")
	require.Less(t, strings.Index(out, "data -> /mnt/data"), strings.Index(out, "media -> /srv/media"),
		"shares render sorted by name")
	require.Greater(t, strings.Index(out, "smb:"), strings.Index(out, "mounts:"),
		"smb renders after the mounts section")
}

// S3 guard: a profile without mounts/smb aggregates renders neither section —
// output stays byte-identical to before the sections existed.
func TestCLI_plan_noMountsSmbSectionsOmitted(t *testing.T) {
	var buf bytes.Buffer
	c := &cmd.PlanCmd{
		Profile: filepath.Join("..", "testdata", "profiles", "resolve"),
		Facts:   &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux"},
		Out:     &buf,
	}
	require.NoError(t, c.Run())

	out := buf.String()
	require.NotContains(t, out, "mounts:")
	require.NotContains(t, out, "smb:")
	require.True(t, strings.HasSuffix(out, "    - echo user-post\n"),
		"hooks post list must remain the tail of the output")
}

// plan --json carries the mounts and smb aggregates with the documented keys;
// smb mirrors the merged spec (avahi stays a *bool, null when unset).
func TestCLI_plan_jsonMountsSmb(t *testing.T) {
	var buf bytes.Buffer
	c := &cmd.PlanCmd{
		Profile: filepath.Join("..", "testdata", "profiles", "mounts"),
		Facts:   &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux"},
		Out:     &buf,
		JSON:    true,
	}
	require.NoError(t, c.Run())

	var got planJSON
	require.NoError(t, json.Unmarshal(buf.Bytes(), &got), "plan --json must stay a single parseable object")

	require.Len(t, got.Mounts, 2)
	data := got.Mounts[1]
	require.Equal(t, "data", data.Name, "mounts sorted by (module, name): backup first")
	require.Equal(t, "nas", data.Module)
	require.Equal(t, "//server/data", data.Source)
	require.Equal(t, "/mnt/data", data.Destination)
	require.Equal(t, "cifs", data.Type)
	require.Equal(t, []string{"noatime", "nofail"}, data.Options)
	require.Equal(t, "*-*-* 04:00", data.StartAt)
	require.Empty(t, data.State)
	require.Equal(t, "base", data.Layer)
	require.Equal(t, "system", data.Scope)
	backup := got.Mounts[0]
	require.Equal(t, "backup", backup.Name)
	require.Equal(t, "disabled", backup.State)
	require.Empty(t, backup.StartAt)
	require.Empty(t, backup.Options)

	require.Len(t, got.Smb, 1)
	smbMod := got.Smb[0]
	require.Equal(t, "nas", smbMod.Module)
	require.Equal(t, "nas", smbMod.Group)
	require.Equal(t, []string{"cri"}, smbMod.Users)
	require.NotNil(t, smbMod.Avahi, "explicit avahi = false must survive as *bool")
	require.False(t, *smbMod.Avahi)
	require.Len(t, smbMod.Shares, 2)
	require.Equal(t, "/srv/media", smbMod.Shares["media"].Path)
	require.Equal(t, "media share", smbMod.Shares["media"].Comment)
	require.True(t, smbMod.Shares["media"].Writable)
	require.Equal(t, "/mnt/data", smbMod.Shares["data"].Path)
	require.True(t, smbMod.Shares["data"].Public)

	// Existing keys are untouched.
	require.Equal(t, []string{"nas"}, got.Modules)
	require.Equal(t, []string{"cifs-utils"}, got.Packages.Install)
	require.Equal(t, []planJSONHook{{Command: "echo nas-pre"}}, got.Hooks.Pre)
}

// A positional module filter limits the plan: the --json modules array
// contains only the filtered ids and the excluded module's dotfile entries
// are absent. The scope fixture selects demo (system) and shell (user).
func TestCLI_plan_filterLimitsScope(t *testing.T) {
	var buf bytes.Buffer
	c := &cmd.PlanCmd{
		Profile: filepath.Join("..", "testdata", "profiles", "scope"),
		Facts:   &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux"},
		Out:     &buf,
		JSON:    true,
		Modules: []string{"shell"},
	}
	require.NoError(t, c.Run())

	var got planJSON
	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))

	require.Equal(t, []string{"shell"}, got.Modules)
	for _, d := range got.Dotfiles {
		require.NotEqual(t, "demo", d.Module, "excluded module must not contribute dotfiles")
		require.NotEqual(t, "/etc/demo.conf", d.Target)
	}
	require.Len(t, got.Dotfiles, 1)
	require.Equal(t, "~/.bashrc", got.Dotfiles[0].Target)
	require.Equal(t, "shell", got.Dotfiles[0].Module)
}

// Optional hooks are marked in both renderings: `(optional)` in text, and an
// `"optional": true` field in JSON. Required hooks carry no marker.
func TestCLI_plan_optionalHookMarker(t *testing.T) {
	root := t.TempDir()
	modDir := filepath.Join(root, "modules", "shell")
	require.NoError(t, os.MkdirAll(modDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(modDir, "module.toml"), []byte(`
[[hooks.pre]]
command = "echo required"
[[hooks.pre]]
command = "echo flaky"
optional = true
`), 0o644))
	facts := &facts.Facts{Hostname: "h", Username: "u", OS: "linux"}

	// Text rendering.
	var buf bytes.Buffer
	require.NoError(t, (&cmd.PlanCmd{Profile: root, Facts: facts, Out: &buf}).Run())
	out := buf.String()
	require.Contains(t, out, "- echo required\n", "required hook has no marker")
	require.Contains(t, out, "- echo flaky (optional)\n", "optional hook is marked")

	// JSON rendering.
	buf.Reset()
	require.NoError(t, (&cmd.PlanCmd{Profile: root, Facts: facts, Out: &buf, JSON: true}).Run())
	var got planJSON
	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
	require.Len(t, got.Hooks.Pre, 2)
	require.Equal(t, "echo required", got.Hooks.Pre[0].Command)
	require.False(t, got.Hooks.Pre[0].Optional)
	require.Equal(t, "echo flaky", got.Hooks.Pre[1].Command)
	require.True(t, got.Hooks.Pre[1].Optional, "optional flag must reach JSON output")
}

// Edit entries render their fields in the text plan (line/block[+comment]/
// source+template) with no empty mode: line. Whole-file entries are unchanged.
func TestCLI_plan_editEntryTextOutput(t *testing.T) {
	root := t.TempDir()
	modDir := filepath.Join(root, "modules", "edits")
	require.NoError(t, os.MkdirAll(modDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(modDir, "git.tmpl"), []byte("name = x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(modDir, "module.toml"), []byte(`
[dotfiles]
"~/.zshrc/dev" = { line = "127.0.0.1 dev.local" }
"~/.zshrc/aliases" = { block = "alias ll='ls -l'", comment = "#" }
"~/.gitconfig/id" = { source = "git.tmpl", template = "tera" }
`), 0o644))
	f := &facts.Facts{Hostname: "h", Username: "u", OS: "linux"}

	var buf bytes.Buffer
	require.NoError(t, (&cmd.PlanCmd{Profile: root, Facts: f, Out: &buf}).Run())
	out := buf.String()

	require.Contains(t, out, "~/.zshrc/dev:")
	require.Contains(t, out, `line: "127.0.0.1 dev.local"`)
	require.Contains(t, out, "~/.zshrc/aliases:")
	require.Contains(t, out, `block: "alias ll='ls -l'"`)
	require.Contains(t, out, `comment: "#"`)
	require.Contains(t, out, "~/.gitconfig/id:")
	require.Contains(t, out, "    source: ")
	require.Contains(t, out, "    template: tera")
	// Edit entries must not render an empty mode: line.
	for _, line := range strings.Split(out, "\n") {
		require.NotEqual(t, "    mode:", strings.TrimSpace(line))
	}
}

// plan --json carries the edit fields (line/block/comment/template) on each
// edit entry; whole-file entries stay unchanged (omitempty).
func TestCLI_plan_editEntryJSON(t *testing.T) {
	root := t.TempDir()
	modDir := filepath.Join(root, "modules", "edits")
	require.NoError(t, os.MkdirAll(modDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(modDir, "git.tmpl"), []byte("name = x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(modDir, "module.toml"), []byte(`
[dotfiles]
"~/.zshrc/dev" = { line = "127.0.0.1 dev.local" }
"~/.zshrc/aliases" = { block = "alias ll='ls -l'", comment = "#" }
"~/.gitconfig/id" = { source = "git.tmpl", template = "tera" }
`), 0o644))
	f := &facts.Facts{Hostname: "h", Username: "u", OS: "linux"}

	var buf bytes.Buffer
	require.NoError(t, (&cmd.PlanCmd{Profile: root, Facts: f, Out: &buf, JSON: true}).Run())

	var doc struct {
		Dotfiles []struct {
			Target   string `json:"target"`
			Line     string `json:"line"`
			Block    string `json:"block"`
			Comment  string `json:"comment"`
			Template string `json:"template"`
			Mode     string `json:"mode"`
		} `json:"dotfiles"`
	}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &doc))
	byTarget := make(map[string]struct {
		Line, Block, Comment, Template, Mode string
	}, len(doc.Dotfiles))
	for _, d := range doc.Dotfiles {
		byTarget[d.Target] = struct {
			Line, Block, Comment, Template, Mode string
		}{d.Line, d.Block, d.Comment, d.Template, d.Mode}
	}
	require.Equal(t, "127.0.0.1 dev.local", byTarget["~/.zshrc/dev"].Line)
	require.Equal(t, "alias ll='ls -l'", byTarget["~/.zshrc/aliases"].Block)
	require.Equal(t, "#", byTarget["~/.zshrc/aliases"].Comment)
	require.Equal(t, "tera", byTarget["~/.gitconfig/id"].Template)
	// Edit entries carry no mode (omitempty keeps it absent).
	for _, d := range byTarget {
		require.Empty(t, d.Mode)
	}
}
