package tui

// T-tui-views slice 1b (issue 0107): the view row under the header —
// s cycles the views, 1-4 jump, a stale view's title carries the stale
// word, and the Modules view renders byte-identical when active.

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestViews_cycleAndJump(t *testing.T) {
	c := newTestCompositor()

	frame := ansiRe.ReplaceAllString(c.View().Content, "")
	require.Contains(t, frame, "Modules · Status · Plan · Profile", "the view row renders under the header")
	require.Contains(t, frame, "module: shell", "the Modules body renders by default")

	// s cycles forward through the four views and wraps.
	c = cpress(c, "s")
	require.Equal(t, viewStatus, c.view)
	frame = ansiRe.ReplaceAllString(c.View().Content, "")
	require.Contains(t, frame, "not loaded", "an empty view shows the placeholder")
	require.NotContains(t, frame, "module: shell", "the Modules body is gone off its view")

	c = cpress(c, "s")
	require.Equal(t, viewPlan, c.view)
	c = cpress(c, "s")
	require.Equal(t, viewProfile, c.view)
	c = cpress(c, "s")
	require.Equal(t, viewModules, c.view, "s wraps to Modules")

	// 1-4 jump from the nav pane.
	c = cpress(c, "3")
	require.Equal(t, viewPlan, c.view)
	c = cpress(c, "2")
	require.Equal(t, viewStatus, c.view)

	// 1-4 jump from the workspace pane too (both-panes bindings).
	c = cpress(c, "tab")
	require.Equal(t, focusWork, c.focus)
	c = cpress(c, "4")
	require.Equal(t, viewProfile, c.view)
	c = cpress(c, "1")
	require.Equal(t, viewModules, c.view)

	// Pane keys are dead on a view without a body (the hook is nil
	// until later slices): j must not move the nav cursor from Status.
	c = cpress(c, "2")
	before := c.nav.cursor
	c = cpress(c, "j")
	require.Equal(t, before, c.nav.cursor, "an empty view swallows pane keys")
}

func TestViews_footerHintsOffModules(t *testing.T) {
	c := newTestCompositor()
	c = cpress(c, "2") // the Status view
	frame := ansiRe.ReplaceAllString(c.View().Content, "")
	require.Contains(t, frame, "not loaded")
	require.NotContains(t, frame, "open in workspace", "a dead nav verb must not be hinted")
	require.NotContains(t, frame, "filter modules", "a dead nav verb must not be hinted")
	require.Contains(t, frame, "next view", "the footer hints the live view keys")
	require.Contains(t, frame, "refresh view", "r stays live and hinted")
	require.Contains(t, frame, "plan", "p stays live and hinted")
	require.Contains(t, frame, "apply", "P stays live and hinted")
	require.Contains(t, frame, "help")
	require.Contains(t, frame, "quit")
}

func TestViews_modulesUnchanged(t *testing.T) {
	c := newTestCompositor()
	before := c.View().Content
	c = cpress(c, "s")
	require.NotEqual(t, before, c.View().Content, "another view replaces the body")
	c = cpress(c, "1")
	require.Equal(t, before, c.View().Content, "the Modules view is byte-identical when active again")
}

func TestViews_staleMark(t *testing.T) {
	c := newTestCompositor()
	require.NotContains(t, ansiRe.ReplaceAllString(c.View().Content, ""), "stale", "nothing is stale at startup")

	c.markStale(viewStatus)
	frame := ansiRe.ReplaceAllString(c.View().Content, "")
	require.Contains(t, frame, "Status (stale)", "a marked view's title shows the stale word")
	require.NotContains(t, frame, "Plan (stale)", "unmarked views stay clean")

	// The mark is on the row title: it shows while the view is active too.
	c = cpress(c, "2")
	frame = ansiRe.ReplaceAllString(c.View().Content, "")
	require.Contains(t, frame, "Status (stale)")

	c.markStale(viewPlan)
	c.markStale(viewProfile)
	frame = ansiRe.ReplaceAllString(c.View().Content, "")
	require.Contains(t, frame, "Plan (stale)")
	require.Contains(t, frame, "Profile (stale)")
}
