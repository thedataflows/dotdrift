package tui

// 0076 T-tui-choice: a field whose value set is closed edits by
// selection — enter opens a centered picker instead of the free-text
// input. Picking commits through the same draft path as a text edit;
// picking the effective value (or esc) stages nothing.

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"
)

// cursorTo jumps home, then walks the workspace cursor down until the
// cursor row's text contains substr — a committed edit can move the
// cursor past the target, so the search always starts at the top.
func cursorTo(t *testing.T, c *Compositor, substr string) *Compositor {
	t.Helper()
	c = cpress(c, "home")
	for i := 0; i < len(c.ws.rows)+1; i++ {
		if c.ws.cursor < len(c.ws.rows) && strings.Contains(c.ws.rows[c.ws.cursor].text, substr) {
			return c
		}
		c = cpress(c, "j")
	}
	t.Fatalf("no selectable row matching %q", substr)
	return c
}

// choiceShell loads the edit fixture and parks the cursor on the scope
// row. It returns the active layer dir (the ledger key) with the shell.
func choiceShell(t *testing.T) (string, *Compositor) {
	t.Helper()
	_, c := wsShell(t, map[string]string{"modules/demo/module.toml": editFixture})
	c = cursorTo(t, cpress(c, "tab"), "scope")
	return c.ws.activeDir(), c
}

// choiceShellFile loads a custom fixture and focuses the workspace.
func choiceShellFile(t *testing.T, toml string) *Compositor {
	t.Helper()
	_, c := wsShell(t, map[string]string{"modules/demo/module.toml": toml})
	return cpress(c, "tab")
}

const choiceFixture = `id = "demo"

[secrets.token]
env = "TOKEN"
allow_empty = true

[mounts.backup]
source = "/srv/bak"
destination = "/mnt/bak"
type = "ext4"
state = "enabled"

[smb]
avahi = true

[smb.shares.media]
path = "/srv/media"
writable = true
`

const unitFixture = `id = "demo"

[systemd.units.demo]
Type = "oneshot"
Restart = "on-failure"
`

func TestChoice_scopeOpensPicker(t *testing.T) {
	_, c := choiceShell(t)

	c = cpress(c, "enter")
	require.Nil(t, c.ws.editing, "a closed-set field does not open the text input")
	require.Len(t, c.modals, 1, "enter opens the choice picker")
	ch := c.modals[0].(*choiceModel)
	require.Equal(t, []string{"user", "system"}, ch.choices)
	require.Equal(t, "user", ch.current, "the unset scope's effective value")
	require.Equal(t, 0, ch.sel, "the picker preselects the effective value")
}

func TestChoice_pickCommits(t *testing.T) {
	dir, c := choiceShell(t)

	c = cpress(c, "enter")
	c = cpress(c, "down")
	c = cpress(c, "enter")
	require.Empty(t, c.modals, "picking pops the picker")
	require.Nil(t, c.ws.editing)
	require.NotNil(t, c.ws.draft, "the pick stages in the draft")
	require.Contains(t, c.ws.placeholderOrBody(), "scope system", "the row re-renders")
	frame := ansiRe.ReplaceAllString(c.View().Content, "")
	require.Contains(t, frame, "scope system ●", "the row marks dirty")
	require.NotNil(t, c.store[dir], "the ledger holds the draft")
}

func TestChoice_pickCurrentStagesNothing(t *testing.T) {
	dir, c := choiceShell(t)

	c = cpress(c, "enter") // user is current — the raw scope is unset
	c = cpress(c, "enter")
	require.Empty(t, c.modals)
	require.Nil(t, c.ws.draft, "picking the effective value stages nothing")
	require.Nil(t, c.store[dir])
	require.Contains(t, c.ws.placeholderOrBody(), "scope user")
}

func TestChoice_escCancels(t *testing.T) {
	dir, c := choiceShell(t)

	c = cpress(c, "enter")
	c = cpress(c, "down")
	c = cpress(c, "esc")
	require.Empty(t, c.modals)
	require.Nil(t, c.ws.draft)
	require.Nil(t, c.store[dir])
}

func TestChoice_boolPickerUnsets(t *testing.T) {
	c := choiceShellFile(t, choiceFixture)

	c = cursorTo(t, c, "allow_empty")
	c = cpress(c, "enter")
	ch := c.modals[0].(*choiceModel)
	require.Equal(t, []string{"false", "true"}, ch.choices)
	require.Equal(t, "true", ch.current)
	c = cpress(c, "up") // false
	c = cpress(c, "enter")
	require.Empty(t, c.modals)
	require.NotContains(t, c.ws.placeholderOrBody(), "allow_empty", "false drops the field row")
	require.Contains(t, c.ws.placeholderOrBody(), "env TOKEN", "the sibling rows survive")
}

func TestChoice_mountStatePicker(t *testing.T) {
	c := choiceShellFile(t, choiceFixture)

	c = cursorTo(t, c, "state")
	c = cpress(c, "enter")
	ch := c.modals[0].(*choiceModel)
	require.Equal(t, []string{"enabled", "disabled"}, ch.choices)
	c = cpress(c, "right") // left/right walk too
	require.Equal(t, "disabled", ch.choices[ch.sel])
	c = cpress(c, "enter")
	require.Contains(t, c.ws.placeholderOrBody(), "state disabled")
}

func TestChoice_avahiThreeState(t *testing.T) {
	c := choiceShellFile(t, choiceFixture)

	c = cursorTo(t, c, "avahi")
	c = cpress(c, "enter")
	ch := c.modals[0].(*choiceModel)
	require.Equal(t, []string{"true", "false", "unset"}, ch.choices)
	c = cpress(c, "down")
	c = cpress(c, "down") // unset
	c = cpress(c, "enter")
	require.NotContains(t, c.ws.placeholderOrBody(), "avahi", "unset removes the scalar")
}

func TestChoice_systemdDirectivePickers(t *testing.T) {
	c := choiceShellFile(t, unitFixture)

	c = cursorTo(t, c, "Type")
	c = cpress(c, "enter")
	ch := c.modals[0].(*choiceModel)
	require.Contains(t, ch.choices, "oneshot")
	require.Contains(t, ch.choices, "notify")
	require.Equal(t, "oneshot", ch.current)
	require.Equal(t, "Type · demo", ch.title)
	c = cpress(c, "up")
	c = cpress(c, "up") // simple
	c = cpress(c, "enter")
	require.Contains(t, c.ws.placeholderOrBody(), `Type = "simple"`)

	c = cursorTo(t, c, "Restart")
	c = cpress(c, "enter")
	ch = c.modals[0].(*choiceModel)
	require.Contains(t, ch.choices, "on-failure")
	require.Equal(t, "on-failure", ch.current)
	c = cpress(c, "esc")
	require.Empty(t, c.modals)
}

func TestChoice_freeTextFieldsKeepTheInput(t *testing.T) {
	_, _, c := editShell(t) // parked on the app row

	c = cpress(c, "enter")
	require.NotNil(t, c.ws.editing, "a free-text field still opens the input")
	require.Empty(t, c.modals)
	c = cpress(c, "esc")

	c = cursorTo(t, c, "neovim")
	c = cpress(c, "enter")
	require.NotNil(t, c.ws.editing)
	require.Empty(t, c.modals)
}

func TestChoice_clickSelectsThenPicks(t *testing.T) {
	_, c := choiceShell(t)

	c = cpress(c, "enter")
	top := c.modals[0]
	top.view(100, 30) // pin the box geometry for hit-testing
	ch := top.(*choiceModel)
	first := ch.boxY + 3

	c = cstepMouse(t, c, tea.MouseClickMsg{Button: tea.MouseLeft, X: 50, Y: first + 1})
	ch = c.modals[0].(*choiceModel)
	require.Equal(t, 1, ch.sel, "the first click selects")

	c = cstepMouse(t, c, tea.MouseClickMsg{Button: tea.MouseLeft, X: 50, Y: first + 1})
	require.Empty(t, c.modals, "clicking the selected row picks")
	require.Contains(t, c.ws.placeholderOrBody(), "scope system")
}
