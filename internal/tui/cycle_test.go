package tui

// T-tui-cycle: h/l on a closed-set row cycles its value in place
// through the same commit seam as the picker — no modal for a one-key
// flip. The cycle wraps; non-choice rows ignore the keys; every step is
// one undo step. The arrows move pane focus instead (0118).

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func wsScopeValue(c *Compositor) string {
	return c.ws.rows[wsRowBySectionText(c, "meta", "scope")].value
}

func TestCycle_choiceRowsCycleInPlace(t *testing.T) {
	_, c := wsShell(t, map[string]string{"modules/demo/module.toml": wsAllSections})
	c.focus = focusWork
	c.ws.cursor = wsRowBySectionText(c, "meta", "scope")
	require.Equal(t, "user", wsScopeValue(c))

	c = wsPress(t, c, "l")
	require.Equal(t, "system", wsScopeValue(c), "l flips to the next value")
	require.Equal(t, 1, c.ws.draft.changes, "the cycle stages through the draft")
	require.Nil(t, c.modals, "no picker modal opens")

	c = wsPress(t, c, "l")
	require.Equal(t, "user", wsScopeValue(c), "the cycle wraps")

	c = wsPress(t, c, "h")
	require.Equal(t, "system", wsScopeValue(c), "h steps back")

	// Each flip is one undo step.
	c.undoEdit()
	require.Equal(t, "user", wsScopeValue(c))
}

func TestCycle_nonChoiceRowsIgnoreHL(t *testing.T) {
	_, c := wsShell(t, map[string]string{"modules/demo/module.toml": wsAllSections})
	c.focus = focusWork
	c.ws.cursor = wsRowBySectionText(c, "meta", "description")

	c = wsPress(t, c, "l")
	require.Nil(t, c.ws.draft, "a free-text row stages nothing")
	require.Empty(t, c.modals, "no modal opens")
}

// 0118: in the workspace the arrows move pane focus instead of
// cycling, and tab leaves the view.
func TestCycle_arrowsMoveFocusFromWorkspace(t *testing.T) {
	_, c := wsShell(t, map[string]string{"modules/demo/module.toml": wsAllSections})
	c.focus = focusWork
	c.ws.cursor = wsRowBySectionText(c, "meta", "scope")

	c = wsPress(t, c, "left")
	require.Equal(t, focusNav, c.focus, "left focuses the nav")
	require.Equal(t, viewModules, c.view, "the view did not change")
	require.Equal(t, "user", wsScopeValue(c), "the value did not cycle")

	c = wsPress(t, c, "right")
	require.Equal(t, focusWork, c.focus, "right focuses the workspace")
	c = wsPress(t, c, "tab")
	require.Equal(t, viewStatus, c.view, "tab switches to the next view")
}
