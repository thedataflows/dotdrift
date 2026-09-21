package tui

// T-tui-cycle: left/right on a closed-set row cycles its value in place
// through the same commit seam as the picker — no modal for a one-key
// flip. The cycle wraps; non-choice rows ignore the keys; every step is
// one undo step.

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

	c = wsPress(t, c, "right")
	require.Equal(t, "system", wsScopeValue(c), "right flips to the next value")
	require.Equal(t, 1, c.ws.draft.changes, "the cycle stages through the draft")
	require.Nil(t, c.modals, "no picker modal opens")

	c = wsPress(t, c, "right")
	require.Equal(t, "user", wsScopeValue(c), "the cycle wraps")

	c = wsPress(t, c, "left")
	require.Equal(t, "system", wsScopeValue(c), "left steps back")

	// Each flip is one undo step.
	c.undoEdit()
	require.Equal(t, "user", wsScopeValue(c))
}

func TestCycle_nonChoiceRowsIgnoreLeftRight(t *testing.T) {
	_, c := wsShell(t, map[string]string{"modules/demo/module.toml": wsAllSections})
	c.focus = focusWork
	c.ws.cursor = wsRowBySectionText(c, "meta", "description")

	c = wsPress(t, c, "right")
	require.Nil(t, c.ws.draft, "a free-text row stages nothing")
	require.Empty(t, c.modals, "no modal opens")
}
