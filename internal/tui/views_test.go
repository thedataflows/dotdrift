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
	for _, name := range []string{"Modules", "Status", "Plan", "Profile"} {
		require.Contains(t, frame, name, "the tab bar renders every view")
	}
	require.Contains(t, frame, "module: shell", "the Modules body renders by default")

	// The right arrow cycles forward through the four views and
	// wraps; the left arrow goes back (0115).
	c = cpress(c, "right")
	require.Equal(t, viewStatus, c.view)
	frame = ansiRe.ReplaceAllString(c.View().Content, "")
	require.Contains(t, frame, "not loaded", "an empty view shows the placeholder")
	require.NotContains(t, frame, "module: shell", "the Modules body is gone off its view")

	c = cpress(c, "right")
	require.Equal(t, viewPlan, c.view)
	c = cpress(c, "right")
	require.Equal(t, viewProfile, c.view)
	c = cpress(c, "right")
	require.Equal(t, viewModules, c.view, "right wraps to Modules")
	c = cpress(c, "left")
	require.Equal(t, viewProfile, c.view, "left goes to the previous view")
	c = cpress(c, "1")
	require.Equal(t, viewModules, c.view)

	// Neither s nor the literal > switches views (0113, 0115): view
	// keys are the arrows, no Shift needed.
	c = cpress(c, "s")
	require.Equal(t, viewModules, c.view, "s is not a view key anymore")
	c = cpress(c, ">")
	require.Equal(t, viewModules, c.view, "the literal > is not a view key")

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
	c = cpress(c, "right")
	require.NotEqual(t, before, c.View().Content, "another view replaces the body")
	c = cpress(c, "1")
	require.Equal(t, before, c.View().Content, "the Modules view is byte-identical when active again")
}

// 0111 slice 5: a loaded read numbers its rows on the view title —
// Status counts its findings, Plan counts the plan's entries; an
// unloaded view shows no count.
func TestViews_countsOnTabs(t *testing.T) {
	stub, c := statusShell(statusFixtureRead())
	frame := ansiRe.ReplaceAllString(c.View().Content, "")
	require.NotContains(t, frame, "Status (", "an unloaded view shows no count")

	c = loadStatusView(t, c)
	frame = ansiRe.ReplaceAllString(c.View().Content, "")
	require.Contains(t, frame, "Status (4)", "the Status title counts the findings")
	require.NotContains(t, frame, "Plan (", "an unloaded Plan view shows no count")

	stub.plan = planFixtureRead()
	c, cmd := cstep(c, keyPress("3"))
	require.NotNil(t, cmd, "entering the Plan view schedules the read")
	c, _ = cstep(c, mustMsg(cmd))
	frame = ansiRe.ReplaceAllString(c.View().Content, "")
	// The fixture: 2 install + 1 remove + 2 tools + 1 dotfile + 1 pre +
	// 1 post + 1 systemd + 1 mount + 1 smb.
	require.Contains(t, frame, "Plan (11)", "the Plan title counts the plan's entries")
}

func TestViews_staleMark(t *testing.T) {
	c := newTestCompositor()
	require.NotContains(t, ansiRe.ReplaceAllString(c.View().Content, ""), "stale", "nothing is stale at startup")

	c.markStale(viewStatus)
	frame := ansiRe.ReplaceAllString(c.View().Content, "")
	require.Contains(t, frame, "Status ●", "a marked view's tab shows the stale dot badge")
	require.NotContains(t, frame, "Plan ●", "unmarked views stay clean")
	require.NotContains(t, frame, "(stale)", "the stale word is gone from tabs")

	// The mark is on the tab: it shows while the view is active too.
	c = cpress(c, "2")
	frame = ansiRe.ReplaceAllString(c.View().Content, "")
	require.Contains(t, frame, "Status ●")

	c.markStale(viewPlan)
	c.markStale(viewProfile)
	frame = ansiRe.ReplaceAllString(c.View().Content, "")
	require.Contains(t, frame, "Plan ●")
	require.Contains(t, frame, "Profile ●")
}
