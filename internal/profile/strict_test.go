package profile_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/profile"
)

// A module.toml with unknown keys is a load-time error naming the file, the
// line, and the key. Typos like `preset = [...]` under [packages] must never
// decode silently.

func TestStrictModuleTOML_unknownTopLevelKey(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/m", `id = "m"
bogus = 1
`)
	_, err := profile.Load(root, &facts.Facts{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "module.toml:2")
	require.Contains(t, err.Error(), `unknown key "bogus"`)
}

func TestStrictModuleTOML_unknownKeyInKnownTable(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/m", `id = "m"

[packages]
preset = ["neovim"]
`)
	_, err := profile.Load(root, &facts.Facts{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "module.toml:4")
	require.Contains(t, err.Error(), `unknown key "preset"`)
	require.Contains(t, err.Error(), "[packages]")
}

func TestStrictModuleTOML_unknownKeyInQuotedDotfileTable(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/m", `id = "m"
[dotfiles."~/.zshrc"]
source = "zshrc"
mod = "symlink"
`)
	_, err := profile.Load(root, &facts.Facts{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "module.toml:4")
	require.Contains(t, err.Error(), `unknown key "mod"`)
	require.Contains(t, err.Error(), `dotfiles."~/.zshrc"`)
}

func TestStrictModuleTOML_unknownTable(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/m", `id = "m"
[package]
present = ["neovim"]
`)
	_, err := profile.Load(root, &facts.Facts{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "module.toml:2")
	require.Contains(t, err.Error(), `unknown key "package"`)
}

func TestStrictModuleTOML_unknownKeyInStructuredHook(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/m", `id = "m"
[[hooks.pre]]
command = "echo hi"
optionnal = true
`)
	_, err := profile.Load(root, &facts.Facts{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "module.toml:4")
	require.Contains(t, err.Error(), `unknown key "optionnal"`)
}

func TestStrictModuleTOML_multipleUnknownKeysAllReported(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/m", `id = "m"
bogus1 = 1
[when]
bogus2 = 2
`)
	_, err := profile.Load(root, &facts.Facts{})
	require.Error(t, err)
	require.Contains(t, err.Error(), `unknown key "bogus1"`)
	require.Contains(t, err.Error(), `unknown key "bogus2"`)
	require.Contains(t, err.Error(), "module.toml:2")
	require.Contains(t, err.Error(), "module.toml:4")
}

func TestStrictModuleTOML_unknownKeyInOverlay(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/m", `id = "m"
`)
	writeModule(t, root, "hosts/h/modules/m", `bogus = 1
`)
	_, err := profile.Load(root, &facts.Facts{Hostname: "h"})
	require.Error(t, err)
	require.Contains(t, err.Error(), `unknown key "bogus"`)
}

func TestStrictModuleTOML_dottedUnknownKey(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/m", `id = "m"
when.hostess = ["h"]
`)
	_, err := profile.Load(root, &facts.Facts{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "module.toml:2")
	require.Contains(t, err.Error(), `unknown key "hostess"`)
}

// Every documented attribute decodes without a schema error; this fixture
// doubles as the machine-checked half of the module.toml schema (the human
// half is docs/product/profile-layout.md).
func TestStrictModuleTOML_fullSchemaDecodes(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/m", `id = "m"
app = "MyApp"
description = "desc"
disabled = false
scope = "system"

[when]
hosts = ["h"]
users = ["u"]
os = ["arch"]
gpu = "nvidia"
kernel = ">= 7.1"

[packages]
present = ["neovim"]
absent = ["nano"]

[tools]
node = "20"

[dotfiles]
"~/.bashrc" = { source = ".bashrc", mode = "symlink" }
"~/.zshrc/aliases" = { block = "alias ll='ls -l'", comment = "#" }
"~/.zshrc/dev" = { line = "export X=1" }
"~/.gitconfig/id" = { source = "git.tmpl", template = "tera" }
"~/.zshrc/snippet" = { source = "snip.sh", mode = "edit" }

[hooks]
pre = ["echo pre", { command = "echo flaky", optional = true }]

[[hooks.post]]
command = "echo post"
optional = true

[mounts.data]
source = "UUID=abcd-1234"
destination = "/mnt/data"
type = "ext4"
options = ["noatime", "nofail"]
startat = "18:00"
state = "enabled"

[smb]
group = "smb"
users = ["cri"]
avahi = true

[smb.shares.media]
path = "/mnt/data/media"
comment = "Media"
valid_users = "cri"
writable = true
public = false
`)
	p, err := profile.Load(root, &facts.Facts{})
	require.NoError(t, err)
	m := findModule(t, p, "m")
	require.Equal(t, "MyApp", m.App)
	require.Equal(t, "system", m.Config.Scope)
	require.True(t, m.Config.Hooks.Post[0].Optional)
}

func TestLoadModuleConfigStrict_unknownKey(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/m", `id = "m"
bogus = 1
`)
	_, err := profile.LoadModuleConfig(filepath.Join(root, "modules", "m"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "module.toml:2")
	require.Contains(t, err.Error(), `unknown key "bogus"`)
}
