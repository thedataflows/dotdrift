package tui

// Issue 0148: f or / opens a quickfilter on the focused panel of the
// Status, Plan, and Profile views — the nav filter's rules (0081) on
// whatever the panel shows.

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"
)

func TestQuickFilter_statusList(t *testing.T) {
	_, c := statusShell(statusFixtureRead())
	c = loadStatusView(t, c)
	c, _ = cstep(c, tea.WindowSizeMsg{Width: 140, Height: 30})

	c = cpress(c, "f")
	require.True(t, c.status.listFilter.filtering, "f opens the list filter")
	c = typeText(c, "fire")
	frame := plainFrame(c)
	require.Contains(t, frame, "apps: firefox - not installed")
	require.NotContains(t, frame, "ripgrep", "typing narrows the list live")
	require.NotContains(t, frame, "/etc/hosts")
	require.Contains(t, frame, "/ fire", "the query shows at the panel bottom")

	c = cpress(c, "enter")
	require.False(t, c.status.listFilter.filtering, "enter ends the typing mode")
	require.Contains(t, plainFrame(c), "/ fire · esc clears", "the applied filter stays named")
	require.NotContains(t, plainFrame(c), "ripgrep", "the filter still applies")

	c = cpress(c, "esc")
	require.Empty(t, c.status.listFilter.query.String(), "esc removes the filter")
	require.Contains(t, plainFrame(c), "ripgrep", "esc restores the full list")
}

func TestQuickFilter_statusListBySectionAndSlash(t *testing.T) {
	// / opens the filter too, and a section name narrows to that
	// section's findings (the old f section cycle's job, 0108).
	_, c := statusShell(statusFixtureRead())
	c = loadStatusView(t, c)
	c, _ = cstep(c, tea.WindowSizeMsg{Width: 140, Height: 30})

	c = cpress(c, "/")
	require.True(t, c.status.listFilter.filtering, "/ opens the list filter")
	c = typeText(c, "dotfiles")
	frame := plainFrame(c)
	require.Contains(t, frame, "/etc/hosts")
	require.NotContains(t, frame, "ripgrep")
	require.NotContains(t, frame, "firefox")

	// f is inert on the Modules view: the applied Status filter is
	// untouched, and no typing mode opens.
	c = cpress(c, "enter")
	c = cpress(c, "1")
	c = cpress(c, "f")
	require.False(t, c.status.listFilter.filtering, "f on Modules opens no filter")
	require.Equal(t, "dotfiles", c.status.listFilter.query.String(), "the applied filter is untouched")
}

func TestQuickFilter_statusDetail(t *testing.T) {
	_, c := statusShell(statusFixtureRead())
	c = loadStatusView(t, c)

	c = cpress(c, "right") // focus the detail pane
	c = cpress(c, "f")
	require.True(t, c.status.detailFilter.filtering, "f opens the detail filter")
	c = typeText(c, "module")
	lines := ansiRe.ReplaceAllString(c.statusDetail(60, 20), "")
	require.Contains(t, lines, "module:")
	require.NotContains(t, lines, "item:", "typing narrows the detail lines")
	require.Contains(t, lines, "/ module", "the query shows at the panel bottom")

	c = cpress(c, "esc")
	require.Empty(t, c.status.detailFilter.query.String())
	lines = ansiRe.ReplaceAllString(c.statusDetail(60, 20), "")
	require.Contains(t, lines, "item:", "esc restores the detail lines")
}

func TestQuickFilter_planListAndDetail(t *testing.T) {
	_, c := planShell(planFixtureRead())
	c = loadPlanView(t, c)
	c, _ = cstep(c, tea.WindowSizeMsg{Width: 140, Height: 30})

	c = cpress(c, "/")
	require.True(t, c.planv.listFilter.filtering, "/ opens the section list filter")
	c = typeText(c, "tool")
	frame := plainFrame(c)
	require.Contains(t, frame, "tools: 2")
	require.NotContains(t, frame, "packages: 2 install", "typing narrows the section list")
	c = cpress(c, "esc")
	require.Contains(t, plainFrame(c), "packages: 2 install", "esc restores the section list")

	// The detail pane filters its own lines.
	_, c = planShell(planFixtureRead())
	c = loadPlanView(t, c)
	c = cpress(c, "right")
	c = cpress(c, "f")
	require.True(t, c.planv.detailFilter.filtering, "f opens the detail filter")
	c = typeText(c, "fire")
	lines := ansiRe.ReplaceAllString(c.planDetail(60, 20), "")
	require.Contains(t, lines, "firefox")
	require.NotContains(t, lines, "ripgrep", "typing narrows the detail lines")
	require.NotContains(t, lines, "nano")
}

func TestQuickFilter_profile(t *testing.T) {
	_, c := profileShell()
	c = loadProfileView(t, c)

	c = cpress(c, "/")
	require.True(t, c.profile.filter.filtering, "/ opens the Profile filter")
	c = typeText(c, "alice")
	frame := plainFrame(c)
	require.Contains(t, frame, "users/alice")
	require.NotContains(t, frame, "users/root", "cards keep only matching rows")
	require.NotContains(t, frame, "myhost", "a card with no matching row drops out")
	require.Contains(t, frame, "/ alice", "the query shows under the cards")

	// A card title match keeps the whole card.
	c = cpress(c, "esc")
	c = cpress(c, "f")
	c = typeText(c, "facts")
	frame = plainFrame(c)
	require.Contains(t, frame, "myhost", "the FACTS title match keeps its rows")
	require.Contains(t, frame, "backend: paru")
	require.NotContains(t, frame, "users/alice")

	c = cpress(c, "esc")
	require.Contains(t, plainFrame(c), "myhost", "esc restores the cards")
	require.Contains(t, plainFrame(c), "users/alice")
}

func TestQuickFilter_movementAndCtrlCWhileTyping(t *testing.T) {
	_, c := statusShell(statusFixtureRead())
	c = loadStatusView(t, c)

	c = cpress(c, "/")
	c = typeText(c, "s") // every finding row contains s
	c = cpress(c, "j")
	require.NotNil(t, c.statusFindingAt(), "j still moves within the matches")
	require.Equal(t, "ripgrep", c.statusFindingAt().Item)

	_, cmd := cstep(c, keyPress("ctrl+c"))
	require.NotNil(t, cmd)
	require.IsType(t, tea.QuitMsg{}, mustMsg(cmd), "ctrl+c quits while typing, like at the base level")
}
