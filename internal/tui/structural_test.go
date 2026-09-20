package tui

// Issue 0074 T-tui-systemd: systemd.units rows become editable. Unit rows
// turn into container rows (d removes the unit, a adds one) and each
// unit's directives render as child rows with typed TOML values (the 0048
// passthrough: dotdrift validates structure only). Directive edits parse
// the input as a TOML value — quoted strings, numbers, bools, lists,
// inline tables — and fall back to a plain string, so the common
// `ExecStart = /usr/bin/demo` needs no quoting. Every commit re-encodes
// the systemd family through profile.EncodeSystemdSection and splices it
// into the draft's working raw (the tomlsplice round-trip runs on every
// commit).

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/profile"
)

const systemdFixture = `id = "demo"

[systemd.units."demo.service"]
Description = "demo unit"
Service = { ExecStart = "/usr/bin/demo", Restart = "on-failure" }

[systemd.units."backup.timer"]
OnBootSec = 30
`

// systemdShell loads the systemd fixture and focuses the workspace.
func systemdShell(t *testing.T) (map[string]string, *Compositor) {
	t.Helper()
	subs, c := wsShell(t, map[string]string{"modules/demo/module.toml": systemdFixture})
	return subs, cpress(c, "tab")
}

// wsToKey parks the cursor on the row with the given key (from the top
// of the surface, so targets above the cursor resolve too).
func wsToKey(t *testing.T, c *Compositor, key string) {
	t.Helper()
	c.ws.cursor = 0
	for i := 0; c.ws.rows[c.ws.cursor].key != key; i++ {
		require.Less(t, i, 128, "row %q never came into view", key)
		c = cpress(c, "j")
	}
	require.Equal(t, key, c.ws.rows[c.ws.cursor].key, "the cursor lands on the row")
}

func TestSystemd_directiveRowsRender(t *testing.T) {
	_, c := systemdShell(t)
	for i := 0; !c.ws.atSection("systemd.units"); i++ {
		require.Less(t, i, 64, "the systemd.units section never came into view")
		c = cpress(c, "j")
	}
	body := c.ws.placeholderOrBody()
	require.Contains(t, body, "demo.service", "the unit container row renders")
	require.Contains(t, body, `Description = "demo unit"`, "directive rows render their value")
	require.Contains(t, body, `OnBootSec = 30`, "numbered directives render typed")

	var containers, directives int
	for _, r := range c.ws.rows {
		if r.section != "systemd.units" {
			continue
		}
		if r.container {
			containers++
			continue
		}
		if r.family == profile.FamilySystemd {
			directives++
		}
	}
	require.Equal(t, 2, containers, "one container row per unit")
	require.Equal(t, 3, directives, "one directive row per passthrough key")
}

func TestSystemd_golden(t *testing.T) {
	subs, c := systemdShell(t)
	wsToKey(t, c, pathKey("demo.service", "Description"))
	requireGolden(t, "structural-systemd.golden", c.View().Content, subs)
}

// --- T-tui-structural-entries: secrets, mounts, smb ---

const entriesFixture = `id = "demo"

[secrets]
API_KEY = { env = "DEMO_API_KEY", description = "demo key", allow_empty = true }
TOKEN = "MISE_TOKEN"

[mounts.media]
source = "/dev/disk/by-label/media"
destination = "/media"
type = "ext4"
options = ["rw", "relatime"]
state = "enabled"

[smb]
group = "media"
users = ["cri", "root"]

[smb.shares.media]
path = "/srv/media"
comment = "media share"
writable = true
`

func entriesShell(t *testing.T) (map[string]string, *Compositor) {
	t.Helper()
	subs, c := wsShell(t, map[string]string{"modules/demo/module.toml": entriesFixture})
	return subs, cpress(c, "tab")
}

func TestEntries_sectionsReplaceOther(t *testing.T) {
	_, c := entriesShell(t)
	body := c.ws.placeholderOrBody()
	require.Contains(t, body, "secrets")
	require.Contains(t, body, "mounts")
	require.Contains(t, body, "smb")
	require.NotContains(t, body, "\nother\n", "the read-only other group is gone")

	var secretContainers, secretFields, mountFields, shareFields int
	for _, r := range c.ws.rows {
		switch r.section {
		case "secrets":
			if r.container {
				secretContainers++
			} else if r.family == profile.FamilySecrets {
				secretFields++
			}
		case "mounts":
			if !r.container && r.family == profile.FamilyMounts && strings.HasPrefix(r.key, "media\x1f") {
				mountFields++
			}
		case "smb":
			if !r.container && r.family == profile.FamilySmb && strings.HasPrefix(r.key, "media\x1f") {
				shareFields++
			}
		}
	}
	require.Equal(t, 2, secretContainers, "one container per secret")
	require.Equal(t, 6, secretFields, "env/description/allow_empty per secret")
	require.Equal(t, 6, mountFields, "source/destination/type/options/startat/state")
	require.Equal(t, 5, shareFields, "path/comment/valid_users/writable/public")
}

func TestSecrets_envEditAndBool(t *testing.T) {
	_, c := entriesShell(t)
	wsToKey(t, c, pathKey("API_KEY", "env"))
	c = cpress(c, "enter")
	require.Equal(t, "DEMO_API_KEY", c.ws.editing.inputString())
	c = typeText(c, "_2")
	c = wsPress(t, c, "enter")
	require.Equal(t, "DEMO_API_KEY_2", c.ws.draft.cfg.Secrets["API_KEY"].Env)
	require.Contains(t, c.ws.placeholderOrBody(), "env DEMO_API_KEY_2", "the row re-renders")

	// allow_empty flips as a bool; junk refuses.
	wsToKey(t, c, pathKey("API_KEY", "allow_empty"))
	c = cpress(c, "enter")
	c.ws.editing.input = []rune("bogus")
	c = cpress(c, "enter")
	require.NotNil(t, c.ws.editing, "junk bool refuses")
	require.Contains(t, c.ws.editing.err, "true", "tier-1 names the allowed values")
	c.ws.editing.input = []rune("false")
	c = wsPress(t, c, "enter")
	require.False(t, c.ws.draft.cfg.Secrets["API_KEY"].AllowEmpty, "the bool lands")
}

func TestSecrets_addAndRemove(t *testing.T) {
	_, c := entriesShell(t)
	for i := 0; !c.ws.atSection("secrets"); i++ {
		require.Less(t, i, 64, "the secrets section never came into view")
		c = cpress(c, "j")
	}
	c = cpress(c, "a")
	c = typeText(c, "CACHE = MISE_CACHE")
	c = wsPress(t, c, "enter")
	require.Equal(t, "MISE_CACHE", c.ws.draft.cfg.Secrets["CACHE"].Env, "the short form lands")
	require.Contains(t, c.ws.placeholderOrBody(), "CACHE", "the container row renders")

	// Malformed add refuses.
	c = cpress(c, "a")
	c.ws.editing.input = []rune("justname")
	c = cpress(c, "enter")
	require.NotNil(t, c.ws.editing)
	require.Contains(t, c.ws.editing.err, "name = ENV")
	c = cpress(c, "esc")

	// d on the container removes the secret after the confirm.
	wsToKey(t, c, "CACHE")
	c = cpress(c, "d")
	require.Len(t, c.modals, 1)
	c = cpress(c, "y")
	require.NotContains(t, c.ws.placeholderOrBody(), "CACHE", "y removes the secret")
	require.NotContains(t, c.ws.draft.cfg.Secrets, "CACHE")
}

func TestMounts_fieldEdits(t *testing.T) {
	_, c := entriesShell(t)
	wsToKey(t, c, pathKey("media", "options"))
	c = cpress(c, "enter")
	require.Equal(t, "rw, relatime", c.ws.editing.inputString(), "lists seed comma-joined")
	c.ws.editing.input = []rune("rw, noatime")
	c = wsPress(t, c, "enter")
	require.Equal(t, []string{"rw", "noatime"}, c.ws.draft.cfg.Mounts["media"].Options)

	wsToKey(t, c, pathKey("media", "state"))
	c = cpress(c, "enter")
	c.ws.editing.input = []rune("paused")
	c = cpress(c, "enter")
	require.NotNil(t, c.ws.editing, "an unknown state refuses")
	require.Contains(t, c.ws.editing.err, "enabled")
	c.ws.editing.input = []rune("disabled")
	c = wsPress(t, c, "enter")
	require.Equal(t, "disabled", c.ws.draft.cfg.Mounts["media"].State)

	// A required field refuses to empty (the mutate error keeps the field open).
	wsToKey(t, c, pathKey("media", "source"))
	c = cpress(c, "enter")
	c.ws.editing.input = nil
	c = cpress(c, "enter")
	require.NotNil(t, c.ws.editing)
	require.Contains(t, c.ws.editing.err, "required")
	cpress(c, "esc")
}

func TestMounts_addBlocksSaveUntilFilled(t *testing.T) {
	_, c := entriesShell(t)
	for i := 0; !c.ws.atSection("mounts"); i++ {
		require.Less(t, i, 64, "the mounts section never came into view")
		c = cpress(c, "j")
	}
	c = cpress(c, "a")
	c.ws.editing.input = []rune("backup")
	c = wsPress(t, c, "enter")
	require.NotNil(t, c.ws.draft.cfg.Mounts["backup"], "the bare entry lands")

	// The save gate refuses an entry resolve could never accept.
	c, cmd := cstep(c, keyCtrl('s'))
	require.Nil(t, cmd, "a blocked save schedules nothing")
	require.Contains(t, c.message, "source is required", "tier-2 names the first gap")

	// Fill the required fields; the save gate stands down.
	for _, f := range []struct{ field, value string }{
		{"source", "/dev/disk/by-label/bak"}, {"destination", "/mnt/bak"}, {"type", "ext4"},
	} {
		wsToKey(t, c, pathKey("backup", f.field))
		c = cpress(c, "enter")
		c.ws.editing.input = []rune(f.value)
		c = wsPress(t, c, "enter")
	}
	require.NotNil(t, cpressCmd(c, "ctrl+s"), "the save runs once the required fields stand")
}

func TestSmb_scalarAndShareEdits(t *testing.T) {
	_, c := entriesShell(t)
	wsToKey(t, c, "users")
	c = cpress(c, "enter")
	c.ws.editing.input = []rune("cri")
	c = wsPress(t, c, "enter")
	require.Equal(t, []string{"cri"}, c.ws.draft.cfg.Smb.Users)

	wsToKey(t, c, "avahi")
	c = cpress(c, "enter")
	c.ws.editing.input = []rune("false")
	c = wsPress(t, c, "enter")
	require.NotNil(t, c.ws.draft.cfg.Smb.Avahi)
	require.False(t, *c.ws.draft.cfg.Smb.Avahi, "the tri-state lands")

	wsToKey(t, c, pathKey("media", "path"))
	c = cpress(c, "enter")
	c.ws.editing.input = []rune("/srv/media2")
	c = wsPress(t, c, "enter")
	require.Equal(t, "/srv/media2", c.ws.draft.cfg.Smb.Shares["media"].Path)
	require.Contains(t, c.ws.placeholderOrBody(), "path /srv/media2")
}

func TestSmb_shareAddAndRemove(t *testing.T) {
	_, c := entriesShell(t)
	for i := 0; !c.ws.atSection("smb"); i++ {
		require.Less(t, i, 64, "the smb section never came into view")
		c = cpress(c, "j")
	}
	c = cpress(c, "a")
	c.ws.editing.input = []rune("public")
	c = wsPress(t, c, "enter")
	require.Contains(t, c.ws.draft.cfg.Smb.Shares, "public", "the bare share lands")

	// The empty path blocks the save.
	c, cmd := cstep(c, keyCtrl('s'))
	require.Nil(t, cmd)
	require.Contains(t, c.message, "path is required")

	// Fill it, then remove the share again through the confirm.
	wsToKey(t, c, pathKey("public", "path"))
	c = cpress(c, "enter")
	c.ws.editing.input = []rune("/srv/public")
	c = wsPress(t, c, "enter")
	wsToKey(t, c, "public")
	c = cpress(c, "d")
	require.Len(t, c.modals, 1)
	c = cpress(c, "y")
	require.NotContains(t, c.ws.draft.cfg.Smb.Shares, "public")
}

func TestEntries_golden(t *testing.T) {
	subs, c := entriesShell(t)
	wsToKey(t, c, pathKey("API_KEY", "env"))
	requireGolden(t, "structural-entries.golden", c.View().Content, subs)
}

func TestSystemd_containerRowRefusesFieldEdit(t *testing.T) {
	_, c := systemdShell(t)
	wsToKey(t, c, "backup.timer")
	c = cpress(c, "enter")
	require.Nil(t, c.ws.editing, "a container row is not a field")
}

func TestSystemd_directiveEdit(t *testing.T) {
	_, c := systemdShell(t)
	dir := c.ws.activeDir()
	wsToKey(t, c, pathKey("demo.service", "Description"))

	// The input seeds from the bare string (not the quoted TOML form).
	c = cpress(c, "enter")
	require.Equal(t, "demo unit", c.ws.editing.inputString(), "string values seed unquoted")

	c = typeText(c, " v2")
	c = wsPress(t, c, "enter")
	require.Nil(t, c.ws.editing, "enter commits")
	require.Contains(t, c.ws.placeholderOrBody(), `Description = "demo unit v2"`, "the row re-renders")
	require.NotNil(t, c.wsDraftFor(dir), "the commit forks the draft")
	require.Contains(t, c.ws.draft.raw, `Description = "demo unit v2"`, "the splice lands in the working raw")
	require.True(t, c.ws.draft.edited[rowKey(profile.FamilySystemd, pathKey("demo.service", "Description"))], "the row is dirty")
}

func TestSystemd_directiveTableEdit(t *testing.T) {
	_, c := systemdShell(t)
	wsToKey(t, c, pathKey("demo.service", "Service"))
	c = cpress(c, "enter")
	// Table values seed as inline TOML; drop Restart by editing the table.
	c.ws.editing.input = []rune(`{ ExecStart = "/usr/bin/demo" }`)
	c = wsPress(t, c, "enter")
	require.Nil(t, c.ws.editing)
	require.Contains(t, c.ws.placeholderOrBody(), `ExecStart = "/usr/bin/demo"`, "the kept directive stands")
	svc := c.ws.draft.cfg.Systemd.Units["demo.service"]["Service"].(map[string]any)
	require.Len(t, svc, 1, "the dropped table key is gone from the draft config")
}

func TestSystemd_directiveValueTypes(t *testing.T) {
	_, c := systemdShell(t)
	wsToKey(t, c, pathKey("backup.timer", "OnBootSec"))
	c = cpress(c, "enter")
	c.ws.editing.input = []rune(`90`)
	c = wsPress(t, c, "enter")
	require.Equal(t, int64(90), c.ws.draft.cfg.Systemd.Units["backup.timer"]["OnBootSec"], "numbers parse typed")

	wsToKey(t, c, pathKey("backup.timer", "OnBootSec"))
	c = cpress(c, "enter")
	c.ws.editing.input = []rune(`["a", "b"]`)
	c = wsPress(t, c, "enter")
	require.Equal(t, []any{"a", "b"}, c.ws.draft.cfg.Systemd.Units["backup.timer"]["OnBootSec"], "lists parse typed")
}

func TestSystemd_directiveEmptyValueRefuses(t *testing.T) {
	_, c := systemdShell(t)
	wsToKey(t, c, pathKey("demo.service", "Description"))
	c = cpress(c, "enter")
	c.ws.editing.input = nil
	c = cpress(c, "enter")
	require.NotNil(t, c.ws.editing, "an empty directive value refuses to commit")
	require.Contains(t, c.ws.editing.err, "empty", "tier-1 names the problem")
	c = cpress(c, "esc")
	require.Empty(t, c.ws.draftEdits(), "the refused edit staged nothing")
}

func TestSystemd_directiveAdd(t *testing.T) {
	_, c := systemdShell(t)
	wsToKey(t, c, pathKey("demo.service", "Description"))
	c = cpress(c, "a")
	require.True(t, c.ws.editing.add, "a on a directive row adds a directive")
	c = typeText(c, "WantedBy = default.target")
	c = wsPress(t, c, "enter")
	require.Nil(t, c.ws.editing)
	require.Contains(t, c.ws.placeholderOrBody(), `WantedBy = "default.target"`, "the unquoted value lands as a string")
	require.Contains(t, c.ws.draft.raw, `WantedBy = "default.target"`, "the splice lands in the working raw")
}

func TestSystemd_unitAddAndRemove(t *testing.T) {
	_, c := systemdShell(t)
	dir := c.ws.activeDir()
	wsToKey(t, c, "backup.timer")

	// a on a container row adds a unit.
	c = cpress(c, "a")
	require.True(t, c.ws.editing.add, "a on a container row adds a unit")
	c.ws.editing.input = []rune("extra.service")
	c = wsPress(t, c, "enter")
	require.Nil(t, c.ws.editing)
	require.True(t, c.ws.rows[c.ws.cursor].container, "the new unit's container row exists")
	require.Contains(t, c.ws.draft.raw, `[systemd.units."extra.service"]`, "the unit block lands in the working raw")
	require.True(t, c.ws.draft.edited[rowKey(profile.FamilySystemd, "extra.service")])

	// d on the container row removes the unit after the confirm.
	c = cpress(c, "d")
	require.Len(t, c.modals, 1, "d asks before removing")
	c = cpress(c, "y")
	require.NotContains(t, c.ws.placeholderOrBody(), "extra.service", "y removes the unit")
	require.NotNil(t, c.wsDraftFor(dir), "the draft stands")
	require.NotContains(t, c.ws.draft.raw, "extra.service", "the removal spliced")
}

func TestSystemd_unitNameTier1(t *testing.T) {
	_, c := systemdShell(t)
	wsToKey(t, c, "backup.timer")
	c = cpress(c, "a")
	c.ws.editing.input = []rune("bad name!")
	c = cpress(c, "enter")
	require.NotNil(t, c.ws.editing, "an invalid unit name refuses to commit")
	require.Contains(t, c.ws.editing.err, "name", "tier-1 names the problem")
	require.Empty(t, c.ws.draftEdits(), "the refused add staged nothing")
}
