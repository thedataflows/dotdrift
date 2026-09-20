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

// wsToKey walks the cursor down to the row with the given key.
func wsToKey(t *testing.T, c *Compositor, key string) {
	t.Helper()
	for i := 0; i < 64 && c.ws.rows[c.ws.cursor].key != key; i++ {
		c = cpress(c, "j")
	}
	require.Equal(t, key, c.ws.rows[c.ws.cursor].key, "the cursor lands on the row")
}

func TestSystemd_directiveRowsRender(t *testing.T) {
	_, c := systemdShell(t)
	for !c.ws.atSection("systemd.units") {
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
