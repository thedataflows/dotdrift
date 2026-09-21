package tui

// T-tui-undo: ctrl+z / ctrl+shift+z step the active draft back and
// forward. Every staged mutation (field commits, adds, removes, raw-line
// repairs) pushes a snapshot before mutating; undo pops one, including
// back past the first change (the clean landed file); a new commit
// truncates the redo stack.

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// wsRowByKey returns a row's index by its stable key, or -1.
func wsRowByKey(c *Compositor, key string) int {
	for i, r := range c.ws.rows {
		if r.key == key {
			return i
		}
	}
	return -1
}

func TestUndo_fieldEditStepsBackAndForward(t *testing.T) {
	_, c := wsShell(t, map[string]string{"modules/demo/module.toml": "id = \"demo\"\ndescription = \"old\"\n"})
	row := wsRowByKey(c, "description")
	require.GreaterOrEqual(t, row, 0)

	c.commitFieldAt(row, "one")
	require.Equal(t, 1, c.ws.draft.changes)
	c.commitFieldAt(wsRowByKey(c, "description"), "two")
	require.Equal(t, 2, c.ws.draft.changes)
	require.Contains(t, c.ws.draft.raw, "two")

	c.undoEdit()
	require.NotNil(t, c.ws.draft)
	require.Equal(t, 1, c.ws.draft.changes)
	require.Contains(t, c.ws.draft.raw, `"one"`)
	require.NotContains(t, c.ws.draft.raw, `"two"`)
	require.Equal(t, "one", c.ws.rows[wsRowByKey(c, "description")].value)

	// Back past the first change: the clean landed file, draft kept
	// (redo lives) but changeless.
	c.undoEdit()
	require.NotNil(t, c.ws.draft)
	require.Equal(t, 0, c.ws.draft.changes)
	require.Equal(t, "old", c.ws.rows[wsRowByKey(c, "description")].value)

	c.undoEdit()
	require.Equal(t, "nothing to undo", c.message)

	c.redoEdit()
	require.Equal(t, 1, c.ws.draft.changes)
	require.Contains(t, c.ws.draft.raw, `"one"`)
	c.redoEdit()
	require.Equal(t, 2, c.ws.draft.changes)
	require.Equal(t, "two", c.ws.rows[wsRowByKey(c, "description")].value)
}

func TestUndo_removeRestoresRowAndCursor(t *testing.T) {
	_, c := wsShell(t, map[string]string{"modules/demo/module.toml": "id = \"demo\"\n\n[packages]\npresent = [\"vim\"]\n"})
	row := wsRowByKey(c, "present:vim")
	require.GreaterOrEqual(t, row, 0)
	c.ws.cursor = row

	c = wsPress(t, c, "tab") // to the work pane
	c = wsPress(t, c, "d")   // the remove confirm
	c = wsPress(t, c, "y")   // removed
	require.Equal(t, 1, c.ws.draft.changes)
	require.Equal(t, -1, wsRowByKey(c, "present:vim"))

	c.undoEdit()
	require.Equal(t, 0, c.ws.draft.changes)
	back := wsRowByKey(c, "present:vim")
	require.GreaterOrEqual(t, back, 0)
	require.Equal(t, back, c.ws.cursor)
}

func TestUndo_newCommitTruncatesRedo(t *testing.T) {
	_, c := wsShell(t, map[string]string{"modules/demo/module.toml": "id = \"demo\"\ndescription = \"old\"\n"})
	c.commitFieldAt(wsRowByKey(c, "description"), "one")
	c.undoEdit()
	c.commitFieldAt(wsRowByKey(c, "description"), "fork")

	c.redoEdit()
	require.Equal(t, "nothing to redo", c.message)
	require.Equal(t, "fork", c.ws.rows[wsRowByKey(c, "description")].value)
}

func TestUndo_rawModeRestoresBrokenState(t *testing.T) {
	_, c := wsShell(t, map[string]string{"modules/demo/module.toml": "id = \"demo\"\nbroken line without equals\n"})
	require.NotNil(t, c.ws.schemaErr)
	banner := c.ws.schemaErr.Error()

	row := wsRowByKey(c, "line:1")
	require.GreaterOrEqual(t, row, 0)
	c.commitFieldAt(row, `description = "fixed"`) // a schema-valid line parses again
	require.Nil(t, c.ws.schemaErr)

	c.undoEdit()
	require.NotNil(t, c.ws.schemaErr)
	// The snapshot keeps the banner's first line — the view renders the
	// first line of the error either way.
	require.Equal(t, firstLineOf(banner), c.ws.schemaErr.Error())
	back := wsRowByKey(c, "line:1")
	require.GreaterOrEqual(t, back, 0)
	require.Equal(t, "raw", c.ws.rows[back].family)
	require.Equal(t, "broken line without equals", c.ws.rows[back].value)
}

func TestUndo_footerHintsUndoOnlyWhenStaged(t *testing.T) {
	_, c := wsShell(t, map[string]string{"modules/demo/module.toml": "id = \"demo\"\ndescription = \"old\"\n"})
	c.focus = focusWork // the hint is the work pane's
	require.False(t, shortHelpHas(c, "ctrl+z"))

	c.commitFieldAt(wsRowByKey(c, "description"), "one")
	require.True(t, shortHelpHas(c, "ctrl+z"))

	c.undoEdit()
	c.undoEdit()
	require.False(t, shortHelpHas(c, "ctrl+z"))
}

// shortHelpHas reports whether the footer's short help names the key.
func shortHelpHas(c *Compositor, key string) bool {
	for _, b := range c.ShortHelp() {
		if b.Help().Key == key {
			return true
		}
	}
	return false
}
