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
	"path/filepath"
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
// of the surface, so targets above the cursor resolve too). An optional
// section disambiguates keys that repeat across sections (when leaves
// vs smb scalars).
func wsToKey(t *testing.T, c *Compositor, key string, section ...string) {
	t.Helper()
	c.ws.cursor = 0
	for i := 0; !wsAtKey(c, key, section); i++ {
		require.Less(t, i, 128, "row %q never came into view", key)
		c = cpress(c, "j")
	}
	require.True(t, wsAtKey(c, key, section), "the cursor lands on the row")
}

// wsToHeader parks the cursor on a section's header row: empty sections
// and the when/smb scope headers are selectable rest points (0075).
func wsToHeader(t *testing.T, c *Compositor, section string) {
	t.Helper()
	c.ws.cursor = 0
	for i := 0; !wsAtHeader(c, section); i++ {
		require.Less(t, i, 128, "the %s header never came into view", section)
		c = cpress(c, "j")
	}
	require.True(t, c.ws.rows[c.ws.cursor].header, "the cursor rests on the header")
}

func wsAtHeader(c *Compositor, section string) bool {
	r := c.ws.rows[c.ws.cursor]
	return r.header && r.section == section
}

func wsAtKey(c *Compositor, key string, section []string) bool {
	r := c.ws.rows[c.ws.cursor]
	if len(section) > 0 && r.section != section[0] {
		return false
	}
	return r.key == key
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
	require.Equal(t, 4, secretFields, "only the set fields render (env/description/allow_empty, env)")
	require.Equal(t, 5, mountFields, "startat is unset and renders nothing")
	require.Equal(t, 3, shareFields, "path/comment/writable; valid_users/public unset")
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

	// allow_empty flips through the choice picker (0076) — junk values
	// are unreachable by construction.
	wsToKey(t, c, pathKey("API_KEY", "allow_empty"))
	c = cpress(c, "enter")
	ch := c.modals[0].(*choiceModel)
	require.Equal(t, []string{"false", "true"}, ch.choices)
	c = cpress(c, "up") // false
	c = cpress(c, "enter")
	require.False(t, c.ws.draft.cfg.Secrets["API_KEY"].AllowEmpty, "the bool lands")
	require.NotContains(t, c.ws.placeholderOrBody(), "allow_empty", "the zeroed bool row disappears")
}

func TestSecrets_addAndRemove(t *testing.T) {
	_, c := entriesShell(t)
	// The section header is the section-level scope: a adds a new entry
	// through the name/env rows.
	wsToHeader(t, c, "secrets")
	c = cpress(c, "a")
	c = typeText(c, "CACHE")
	c = cpress(c, "down")
	c = typeText(c, "MISE_CACHE")
	c = wsPress(t, c, "enter")
	require.Equal(t, "MISE_CACHE", c.ws.draft.cfg.Secrets["CACHE"].Env, "the short form lands")
	require.Contains(t, c.ws.placeholderOrBody(), "CACHE", "the container row renders")

	// a on the container adds a field INTO the entry. The field is a
	// closed choice now: an unknown field is unreachable, an empty value
	// still refuses loudly.
	wsToKey(t, c, "CACHE")
	c = cpress(c, "a")
	f := addFormTop(t, c)
	require.Contains(t, f.view(100, 30), "add field · CACHE")
	c = cpress(c, "enter") // env chosen, no value typed
	f = addFormTop(t, c)
	require.False(t, f.finished(), "a refused add keeps the form open")
	require.Contains(t, f.view(100, 30), "field = value")
	cpress(c, "esc")

	// a on a field row adds the section's next entry; a missing env
	// refuses.
	wsToKey(t, c, pathKey("CACHE", "env"))
	c = cpress(c, "a")
	c = typeText(c, "justname")
	c = cpress(c, "enter")
	f = addFormTop(t, c)
	require.Contains(t, f.view(100, 30), "name = ENV")
	c = cpress(c, "esc")
	require.Empty(t, c.modals)

	// d on the container removes the secret after the confirm.
	wsToKey(t, c, "CACHE")
	c = cpress(c, "d")
	require.Len(t, c.modals, 1)
	c = cpress(c, "y")
	require.NotContains(t, c.ws.placeholderOrBody(), "CACHE", "y removes the secret")
	require.NotContains(t, c.ws.draft.cfg.Secrets, "CACHE")
}

func TestSecrets_containerFieldAdd(t *testing.T) {
	_, c := entriesShell(t)
	// TOKEN carries only env: description is unset and hidden until the
	// container's a adds it.
	wsToKey(t, c, "TOKEN")
	require.NotContains(t, c.ws.placeholderOrBody(), "description token", "the unset field is hidden")
	c = cpress(c, "a")
	c = cpress(c, "right") // env → description
	c = cpress(c, "down")
	c = typeText(c, "token desc")
	c = wsPress(t, c, "enter")
	require.Equal(t, "token desc", c.ws.draft.cfg.Secrets["TOKEN"].Description, "the field lands in the entry")
	require.Contains(t, c.ws.placeholderOrBody(), "description token desc", "the field row now renders")
}

func TestMounts_fieldEdits(t *testing.T) {
	_, c := entriesShell(t)
	wsToKey(t, c, pathKey("media", "options"))
	c = cpress(c, "enter")
	require.Equal(t, "rw, relatime", c.ws.editing.inputString(), "lists seed comma-joined")
	c.ws.editing.input = []rune("rw, noatime")
	c = wsPress(t, c, "enter")
	require.Equal(t, []string{"rw", "noatime"}, c.ws.draft.cfg.Mounts["media"].Options)

	// state flips through the choice picker (0076) — unknown states are
	// unreachable by construction.
	wsToKey(t, c, pathKey("media", "state"))
	c = cpress(c, "enter")
	ch := c.modals[0].(*choiceModel)
	require.Equal(t, []string{"enabled", "disabled"}, ch.choices)
	c = cpress(c, "right") // disabled
	c = cpress(c, "enter")
	require.Equal(t, "disabled", c.ws.draft.cfg.Mounts["media"].State)

	// A required field refuses to empty (the mutate error keeps the field open).
	// (source/destination browse since 0094 — the picker cannot produce an
	// empty value; the text-kind type field pins the refusal.)
	wsToKey(t, c, pathKey("media", "type"))
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
	c = typeText(c, "backup")
	c = wsPress(t, c, "enter")
	require.NotNil(t, c.ws.draft.cfg.Mounts["backup"], "the bare entry lands")
	require.Contains(t, c.ws.placeholderOrBody(), `a adds "field = value"`,
		"the empty container names the gesture")

	// The save gate refuses an entry resolve could never accept.
	c, cmd := cstep(c, keyCtrl('s'))
	require.Nil(t, cmd, "a blocked save schedules nothing")
	require.Contains(t, c.message, "source is required", "tier-2 names the first gap")

	// Fill the required fields through the container's a — the unset
	// fields are hidden, the container is the way in. The field is a
	// choice now: cycle to it (source, destination, type), type the
	// value.
	for _, f := range []struct {
		rights int
		value  string
	}{
		{0, "/dev/disk/by-label/bak"}, // source
		{1, "/mnt/bak"},               // destination
		{2, "ext4"},                   // type
	} {
		wsToKey(t, c, "backup")
		c = cpress(c, "a")
		for i := 0; i < f.rights; i++ {
			c = cpress(c, "right")
		}
		c = cpress(c, "down")
		c = typeText(c, f.value)
		c = wsPress(t, c, "enter")
	}
	require.NotNil(t, cpressCmd(c, "ctrl+s"), "the save runs once the required fields stand")
}

func TestSmb_scalarAndShareEdits(t *testing.T) {
	_, c := entriesShell(t)
	wsToKey(t, c, "users", "smb")
	c = cpress(c, "enter")
	c.ws.editing.input = []rune("cri")
	c = wsPress(t, c, "enter")
	require.Equal(t, []string{"cri"}, c.ws.draft.cfg.Smb.Users)

	// avahi is unset and hidden: a on the smb header takes the what
	// choice (share/group/users/avahi) and a value.
	wsToHeader(t, c, "smb")
	c = cpress(c, "a")
	for i := 0; i < 3; i++ {
		c = cpress(c, "right") // share → group → users → avahi
	}
	c = cpress(c, "down")
	c = typeText(c, "false")
	c = wsPress(t, c, "enter")
	require.NotNil(t, c.ws.draft.cfg.Smb.Avahi)
	require.False(t, *c.ws.draft.cfg.Smb.Avahi, "the tri-state lands")
	require.Contains(t, c.ws.placeholderOrBody(), "avahi false", "the scalar row now renders")

	// 0094: the share path browses — enter opens the picker, and the
	// location bar types the new path (a share dir that does not exist
	// yet picks while its parent does).
	newPath := filepath.Join(t.TempDir(), "media2")
	wsToKey(t, c, pathKey("media", "path"))
	c = cpress(c, "enter")
	c = cpress(c, "ctrl+l")
	c = cpress(c, "ctrl+u")
	c = typeText(c, newPath)
	c = wsPress(t, c, "enter")
	require.Equal(t, newPath, c.ws.draft.cfg.Smb.Shares["media"].Path)
	require.Contains(t, c.ws.placeholderOrBody(), "path "+newPath)
}

func TestSmb_containerFieldAdd(t *testing.T) {
	_, c := entriesShell(t)
	// a on the share container adds a field INTO it: the field is a
	// choice (path, comment, valid_users, ...), the value is text.
	wsToKey(t, c, "media", "smb")
	c = cpress(c, "a")
	c = cpress(c, "right")
	c = cpress(c, "right") // path → comment → valid_users
	c = cpress(c, "down")
	c = typeText(c, "cri, root")
	c = wsPress(t, c, "enter")
	require.Equal(t, "cri, root", c.ws.draft.cfg.Smb.Shares["media"].ValidUsers, "the field lands in the share")
	require.Contains(t, c.ws.placeholderOrBody(), "valid_users cri, root", "the field row renders")
}

func TestSmb_shareAddAndRemove(t *testing.T) {
	_, c := entriesShell(t)
	for i := 0; !c.ws.atSection("smb"); i++ {
		require.Less(t, i, 64, "the smb section never came into view")
		c = cpress(c, "j")
	}
	c = cpress(c, "a")
	f := addFormTop(t, c)
	require.Contains(t, f.view(100, 30), "add smb · demo", "the what choice opens on the header")
	c = cpress(c, "down") // the share name lands in the value row
	c = typeText(c, "public")
	c = wsPress(t, c, "enter")
	require.Contains(t, c.ws.draft.cfg.Smb.Shares, "public", "the bare share lands")

	// The empty path blocks the save.
	c, cmd := cstep(c, keyCtrl('s'))
	require.Nil(t, cmd)
	require.Contains(t, c.message, "path is required")

	// Fill it through the container (the unset fields are hidden), then
	// remove the share again through the confirm. Path is the field
	// choice's first option.
	wsToKey(t, c, "public")
	c = cpress(c, "a")
	c = cpress(c, "down")
	c = typeText(c, "/srv/public")
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

// --- T-tui-structural-when: the when tree ---

const whenFixture = `id = "demo"

[when]
hosts = ["myhost"]
and = [
  { os = ["linux"], gpu = "nvidia" },
  { or = [{ hosts = ["work"] }, { kernel = ">= 6" }] },
]
not = { packages = ["emacs"] }
`

// whenRootFixture has no when section at all: the tree still renders its
// root leaves, so an unset condition becomes settable.
const whenRootFixture = `id = "demo"
`

func whenShell(t *testing.T, fixture string) (map[string]string, *Compositor) {
	t.Helper()
	subs, c := wsShell(t, map[string]string{"modules/demo/module.toml": fixture})
	return subs, cpress(c, "tab")
}

func TestWhen_treeRenders(t *testing.T) {
	subs, c := whenShell(t, whenFixture)
	for i := 0; !c.ws.atSection("when"); i++ {
		require.Less(t, i, 64, "the when section never came into view")
		c = cpress(c, "j")
	}
	body := c.ws.placeholderOrBody()
	require.Contains(t, body, "hosts myhost", "the set root leaf renders")
	require.Contains(t, body, "and[0]", "group containers render")
	require.Contains(t, body, "or[0]", "nested groups render")
	require.Contains(t, body, "not", "the not group renders")
	require.Contains(t, body, "kernel >= 6", "nested leaves render")
	require.NotContains(t, body, "\nusers", "the unset root leaf renders nothing")
	require.NotContains(t, body, "\nos\n", "unset leaves render nothing at any depth")

	var containers int
	for _, r := range c.ws.rows {
		if r.section == "when" && r.container {
			containers++
		}
	}
	require.Equal(t, 5, containers, "and[0], and[1], or[0], or[1], not")

	// Group leaf keys keep their path identity.
	wsToKey(t, c, pathKey("and", "0", "gpu"))
	require.False(t, c.ws.rows[c.ws.cursor].container)
	require.Equal(t, profile.FamilyWhen, c.ws.rows[c.ws.cursor].family)
	requireGolden(t, "structural-when.golden", c.View().Content, subs)
}

func TestWhen_headerAddsRootLeaf(t *testing.T) {
	// 0075 T-tui-disclosure: an unset condition renders nothing; the
	// when header is the root scope, and `a field = value` sets a leaf.
	_, c := whenShell(t, whenRootFixture)
	require.NotContains(t, c.ws.placeholderOrBody(), "gpu", "an unset when renders nothing")

	wsToHeader(t, c, "when")
	c = cpress(c, "a")
	// rows: kind (leaf/and/or/not), field, value
	c = cpress(c, "down")    // field
	for i := 0; i < 3; i++ { // hosts → users → os → gpu
		c = cpress(c, "right")
	}
	c = cpress(c, "down") // value
	c = typeText(c, "nvidia")
	c = wsPress(t, c, "enter")
	require.Equal(t, "nvidia", c.ws.draft.cfg.When.GPU, "field = value sets the root leaf")
	require.Equal(t, "gpu", c.ws.rows[c.ws.cursor].key, "the cursor lands on the new leaf")
	require.Contains(t, c.ws.placeholderOrBody(), "gpu nvidia", "the leaf row now renders")
}

func TestWhen_groupLeafEdit(t *testing.T) {
	_, c := whenShell(t, whenFixture)
	wsToKey(t, c, pathKey("and", "0", "gpu"))
	c = cpress(c, "enter")
	require.Equal(t, "nvidia", c.ws.editing.inputString(), "the leaf seeds from its group")
	c = typeText(c, "-open")
	c = wsPress(t, c, "enter")
	require.Equal(t, "nvidia-open", c.ws.draft.cfg.When.And[0].GPU)

	// Clearing the leaf empties it (the encoder drops it) and the row
	// disappears with it.
	wsToKey(t, c, pathKey("and", "0", "gpu"))
	c = cpress(c, "enter")
	c.ws.editing.input = nil
	c = wsPress(t, c, "enter")
	require.Empty(t, c.ws.draft.cfg.When.And[0].GPU)
	require.NotContains(t, c.ws.draft.raw, "nvidia-open", "the cleared leaf left the raw")
	require.NotContains(t, c.ws.placeholderOrBody(), "nvidia-open", "the cleared leaf renders nothing")
}

func TestWhen_groupAddNested(t *testing.T) {
	_, c := whenShell(t, whenRootFixture)
	wsToHeader(t, c, "when")

	// a on the header grows the root's and-list: kind right once.
	c = cpress(c, "a")
	c = cpress(c, "right")
	c = wsPress(t, c, "enter")
	require.Len(t, c.ws.draft.cfg.When.And, 1)
	require.Equal(t, pathKey("and", "0"), c.ws.rows[c.ws.cursor].key, "the cursor lands on the new group")

	// a on the group container nests an or beneath it: kind right twice.
	c = cpress(c, "a")
	c = cpress(c, "right")
	c = cpress(c, "right")
	c = wsPress(t, c, "enter")
	require.Len(t, c.ws.draft.cfg.When.And[0].Or, 1)
	require.Equal(t, pathKey("and", "0", "or", "0"), c.ws.rows[c.ws.cursor].key)

	// a on the group adds a leaf — the nested or's unset leaves are
	// hidden, the container is the way in. Leaf is the default kind;
	// hosts is the default field.
	c = cpress(c, "a")
	c = cpress(c, "down")
	c = cpress(c, "down")
	c = typeText(c, "work")
	c = wsPress(t, c, "enter")
	require.Equal(t, []string{"work"}, c.ws.draft.cfg.When.And[0].Or[0].Hosts)
	require.Contains(t, c.ws.draft.raw, `or = [{ hosts = ["work"] }]`, "the nested tree encodes")

	// An empty value refuses.
	c = cpress(c, "a")
	c = wsPress(t, c, "enter")
	f := addFormTop(t, c)
	require.False(t, f.finished(), "a refused add keeps the form open")
	require.Contains(t, f.view(100, 30), "field = value")
	cpress(c, "esc")
}

func TestWhen_notAddAndDuplicateRefuse(t *testing.T) {
	_, c := whenShell(t, whenRootFixture)
	wsToHeader(t, c, "when")
	c = cpress(c, "a")
	for i := 0; i < 3; i++ { // leaf → and → or → not
		c = cpress(c, "right")
	}
	c = wsPress(t, c, "enter")
	require.NotNil(t, c.ws.draft.cfg.When.Not, "the not group lands")
	require.Equal(t, "not", c.ws.rows[c.ws.cursor].key)

	// A second not on the SAME group refuses (the not container itself
	// nests — its `a not` is a deeper node, by grammar).
	wsToHeader(t, c, "when")
	c = cpress(c, "a")
	for i := 0; i < 3; i++ { // leaf → and → or → not
		c = cpress(c, "right")
	}
	c = cpress(c, "enter")
	f := addFormTop(t, c)
	require.False(t, f.finished(), "a refused add keeps the form open")
	require.Contains(t, f.view(100, 30), "already")
	cpress(c, "esc")
}

func TestWhen_groupRemove(t *testing.T) {
	_, c := whenShell(t, whenFixture)
	wsToKey(t, c, "not")
	c = cpress(c, "d")
	require.Len(t, c.modals, 1, "d asks before removing a group")
	c = cpress(c, "y")
	require.Nil(t, c.ws.draft.cfg.When.Not, "y removes the not group")
	require.NotContains(t, c.ws.placeholderOrBody(), "packages emacs")

	wsToKey(t, c, pathKey("and", "1"))
	c = cpress(c, "d")
	c = cpress(c, "y")
	require.Len(t, c.ws.draft.cfg.When.And, 1, "y removes the and group")

	// d on a leaf row prompts and clears the leaf too (0087) — no
	// silent no-op beside the group removal.
	wsToKey(t, c, pathKey("and", "0", "gpu"))
	c = cpress(c, "d")
	require.Len(t, c.modals, 1, "a leaf row removes with the same confirm")
	c = cpress(c, "y")
	require.Empty(t, c.ws.draft.cfg.When.And[0].GPU, "y clears the leaf")
}

func TestWhen_emptyGroupBlocksSave(t *testing.T) {
	_, c := whenShell(t, whenRootFixture)
	wsToHeader(t, c, "when")
	c = cpress(c, "a")
	c = cpress(c, "right") // kind: and
	c = wsPress(t, c, "enter")

	// The empty group is a load-time error class: the save refuses it.
	c, cmd := cstep(c, keyCtrl('s'))
	require.Nil(t, cmd, "a blocked save schedules nothing")
	require.Contains(t, c.message, "empty", "tier-2 names the empty group")

	// Fill a leaf through the group container; the save stands down.
	wsToKey(t, c, pathKey("and", "0"))
	c = cpress(c, "a")
	c = cpress(c, "down")  // field
	c = cpress(c, "right") // hosts → users → os
	c = cpress(c, "right")
	c = cpress(c, "down") // value
	c = typeText(c, "linux")
	c = wsPress(t, c, "enter")
	require.NotNil(t, cpressCmd(c, "ctrl+s"), "the save runs once the group has content")
}

// --- T-tui-structural-cleanup: the shell's chrome catches up ---

func TestPalette_structuralFieldSections(t *testing.T) {
	_, c := entriesShell(t)
	ids := map[string]bool{}
	for _, e := range c.paletteEntries() {
		ids[e.id] = true
	}
	require.True(t, ids["field:secrets"], "secrets is a field destination")
	require.True(t, ids["field:mounts"], "mounts is a field destination")
	require.True(t, ids["field:smb"], "smb is a field destination")
	require.False(t, ids["field:other"], "the dissolved group is gone")
}

func TestPalette_structuralFieldDeepLink(t *testing.T) {
	_, c := entriesShell(t)
	c = cpress(c, "/")
	require.Len(t, c.modals, 1, "/ opens the palette")
	c = typeText(c, "secrets")
	p := pal(t, c)
	for i, l := range p.visibleLabels() {
		if l == "secrets" && p.rows[i].section == sectionFields {
			p.sel = i
		}
	}
	c = cpress(c, "enter")
	require.True(t, c.ws.atSection("secrets"), "the cursor deep-links into the structural section")
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
	f := addFormTop(t, c)
	require.Contains(t, f.view(100, 30), "add directive · demo.service",
		"a on a directive row adds a directive into its unit")
	c = typeText(c, "WantedBy")
	c = cpress(c, "down")
	c = typeText(c, "default.target")
	c = wsPress(t, c, "enter")
	require.Empty(t, c.modals)
	require.Contains(t, c.ws.placeholderOrBody(), `WantedBy = "default.target"`, "the unquoted value lands as a string")
	require.Contains(t, c.ws.draft.raw, `WantedBy = "default.target"`, "the splice lands in the working raw")
}

func TestSystemd_unitAddAndRemove(t *testing.T) {
	_, c := systemdShell(t)
	dir := c.ws.activeDir()
	// The section header is the entry-level scope: a adds a unit.
	wsToHeader(t, c, "systemd.units")

	c = cpress(c, "a")
	f := addFormTop(t, c)
	require.Contains(t, f.view(100, 30), "add unit · demo",
		"a on the header adds a unit")
	c = typeText(c, "extra.service")
	c = wsPress(t, c, "enter")
	require.Empty(t, c.modals)
	require.True(t, c.ws.rows[c.ws.cursor].container, "the new unit's container row exists")
	require.Contains(t, c.ws.draft.raw, `[systemd.units."extra.service"]`, "the unit block lands in the working raw")
	require.True(t, c.ws.draft.edited[rowKey(profile.FamilySystemd, "extra.service")])
	require.Contains(t, c.ws.placeholderOrBody(), `a adds "field = value"`,
		"the empty unit names the gesture")

	// The cursor walk skips the hint row.
	before := c.ws.cursor
	c = cpress(c, "j")
	require.Greater(t, c.ws.cursor, before, "j leaves the container")
	require.False(t, c.ws.rows[c.ws.cursor].hint, "the cursor never rests on a hint row")

	// d on the container row removes the unit after the confirm.
	wsToKey(t, c, "extra.service")
	c = cpress(c, "d")
	require.Len(t, c.modals, 1, "d asks before removing")
	c = cpress(c, "y")
	require.NotContains(t, c.ws.placeholderOrBody(), "extra.service", "y removes the unit")
	require.NotNil(t, c.wsDraftFor(dir), "the draft stands")
	require.NotContains(t, c.ws.draft.raw, "extra.service", "the removal spliced")
}

func TestSystemd_containerFieldAdd(t *testing.T) {
	_, c := systemdShell(t)
	// a on a unit container adds a directive INTO it — the fresh-unit way
	// in once unset directives stop rendering.
	wsToKey(t, c, "backup.timer")
	c = cpress(c, "a")
	c = typeText(c, "OnUnitActiveSec")
	c = cpress(c, "down")
	c = typeText(c, "5")
	c = wsPress(t, c, "enter")
	require.Contains(t, c.ws.placeholderOrBody(), `OnUnitActiveSec = 5`, "the directive lands in the unit")
	require.Contains(t, c.ws.draft.raw, "OnUnitActiveSec = 5", "the splice lands in the working raw")
}

func TestSystemd_unitNameTier1(t *testing.T) {
	_, c := systemdShell(t)
	wsToHeader(t, c, "systemd.units")
	c = cpress(c, "a")
	c = typeText(c, "bad name!")
	c = cpress(c, "enter")
	f := addFormTop(t, c)
	require.False(t, f.finished(), "an invalid unit name refuses to commit")
	require.Contains(t, f.view(100, 30), "name", "tier-1 names the problem")
	require.Empty(t, c.ws.draftEdits(), "the refused add staged nothing")
}

// --- 0087: d removes the row's thing, everywhere ---
//
// The 0074 container-only rule left d a silent no-op on smb scalars and
// every structural field row; the writes block rows carried no family at
// all. The uniform rule: d removes the row's thing — an entry, a group,
// a scalar, or one field of an entry.

func TestRemove_smbScalarRows(t *testing.T) {
	_, c := entriesShell(t)
	wsToKey(t, c, "group", "smb")
	c = cpress(c, "d")
	require.Len(t, c.modals, 1, "d asks before clearing the scalar")
	c = cpress(c, "y")
	require.Empty(t, c.ws.draft.cfg.Smb.Group, "y clears the group scalar")
	require.NotContains(t, c.ws.placeholderOrBody(), "group media", "the row disappears")

	wsToKey(t, c, "users", "smb")
	c = cpress(c, "d")
	require.Len(t, c.modals, 1)
	c = cpress(c, "y")
	require.Empty(t, c.ws.draft.cfg.Smb.Users, "y clears the users scalar")

	// One undo step restores the removal.
	c.undoEdit()
	require.Equal(t, []string{"cri", "root"}, c.ws.draft.cfg.Smb.Users, "undo restores the scalar")
}

func TestRemove_smbShareField(t *testing.T) {
	_, c := entriesShell(t)
	wsToKey(t, c, pathKey("media", "comment"), "smb")
	c = cpress(c, "d")
	require.Len(t, c.modals, 1, "d asks before unsetting the field")
	c = cpress(c, "y")
	require.Empty(t, c.ws.draft.cfg.Smb.Shares["media"].Comment, "y unsets the field")

	// Removing the required path leaves an invalid share; the save
	// cross-check names it (undo restores).
	wsToKey(t, c, pathKey("media", "path"), "smb")
	c = cpress(c, "d")
	c = cpress(c, "y")
	require.Empty(t, c.ws.draft.cfg.Smb.Shares["media"].Path)
	c, cmd := cstep(c, keyCtrl('s'))
	require.Nil(t, cmd, "the invalid share blocks the save")
	require.Contains(t, c.message, "path is required")
}

func TestRemove_secretAndMountFields(t *testing.T) {
	_, c := entriesShell(t)
	wsToKey(t, c, pathKey("API_KEY", "description"), "secrets")
	c = cpress(c, "d")
	require.Len(t, c.modals, 1, "d asks before unsetting the secret field")
	c = cpress(c, "y")
	require.Empty(t, c.ws.draft.cfg.Secrets["API_KEY"].Description)

	wsToKey(t, c, pathKey("media", "options"), "mounts")
	c = cpress(c, "d")
	require.Len(t, c.modals, 1, "d asks before unsetting the mount field")
	c = cpress(c, "y")
	require.Empty(t, c.ws.draft.cfg.Mounts["media"].Options)
}

func TestRemove_whenLeaf(t *testing.T) {
	_, c := whenShell(t, whenFixture)
	wsToKey(t, c, "hosts", "when")
	c = cpress(c, "d")
	require.Len(t, c.modals, 1, "d asks before clearing the leaf")
	c = cpress(c, "y")
	require.Empty(t, c.ws.draft.cfg.When.Hosts, "y clears the leaf")
}

func TestRemove_writesBlockRow(t *testing.T) {
	_, c := wsShell(t, map[string]string{"modules/demo/module.toml": wsAllSections})
	c = cpress(c, "tab")

	// enter never opens a text edit on a block write (committing there
	// would write into Source) — the add-form fallback stands.
	wsToKey(t, c, "~/.profile", "writes")
	c = cpress(c, "enter")
	require.Nil(t, c.ws.editing, "a block write never opens a text edit")
	cpress(c, "esc") // close the fallback

	// d removes the entry with the same confirm as every other section.
	wsToKey(t, c, "~/.profile", "writes")
	c = cpress(c, "d")
	require.Len(t, c.modals, 1, "d asks before removing the block entry")
	c = cpress(c, "y")
	require.NotContains(t, c.ws.draft.cfg.Dotfiles, "~/.profile", "y removes the entry")
}

func TestRemove_metaHeadersAndHintsStayDead(t *testing.T) {
	_, c := entriesShell(t)
	wsToKey(t, c, "description")
	cpress(c, "d")
	require.Empty(t, c.modals, "meta rows have no remove confirm (identity, not options)")

	wsToHeader(t, c, "secrets")
	cpress(c, "d")
	require.Empty(t, c.modals, "section headers have no remove confirm")
}
