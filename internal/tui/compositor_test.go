package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/require"
)

// T-tui-compositor: the composited final frame (base + modal layers) is
// pinned by goldens at fixed sizes; focus, esc, capture, and chrome
// behavior is pinned by message-driven tests.

func cstep(c *Compositor, msg tea.Msg) (*Compositor, tea.Cmd) {
	next, cmd := c.Update(msg)
	return next.(*Compositor), cmd
}

func cpress(c *Compositor, s string) *Compositor {
	next, _ := cstep(c, keyPress(s))
	return next
}

func newTestCompositor() *Compositor {
	c := NewCompositor("/home/cri/profiles/main")
	c.host, c.user = "myhost", "cri"
	c.navText = "MODULES\n  shell\n  firefox"
	c.workText = "module: shell\npackages: 3"
	c, _ = cstep(c, tea.WindowSizeMsg{Width: 100, Height: 30})
	return c
}

type stubModal struct {
	box  string
	keys []string
}

func (s *stubModal) view(_, _ int) string { return s.box }

func (s *stubModal) update(msg tea.Msg) tea.Cmd {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		s.keys = append(s.keys, k.String())
	}
	return nil
}

func TestShell_layoutBase(t *testing.T) {
	c := newTestCompositor()
	subs := map[string]string{"/home/cri/profiles/main": "$PROFILE"}
	requireGolden(t, "compositor-base.golden", c.View().Content, subs)
}

func TestShell_reflowNarrow(t *testing.T) {
	c := newTestCompositor()
	c, _ = cstep(c, tea.WindowSizeMsg{Width: 64, Height: 24})
	subs := map[string]string{"/home/cri/profiles/main": "$PROFILE"}
	requireGolden(t, "compositor-narrow.golden", c.View().Content, subs)
}

func TestShell_reflowVeryNarrow(t *testing.T) {
	c := newTestCompositor()
	c, _ = cstep(c, tea.WindowSizeMsg{Width: 40, Height: 10})
	require.Contains(t, c.View().Content, "too small")
}

func TestCompositor_modalOverDimmedBase(t *testing.T) {
	c := newTestCompositor()
	c.modals = append(c.modals, &stubModal{box: "╭──────────╮\n│ CONFIRM? │\n╰──────────╯"})

	frame := c.View().Content
	subs := map[string]string{"/home/cri/profiles/main": "$PROFILE"}
	requireGolden(t, "compositor-modal.golden", frame, subs)

	probe := c.th.focusedBorder.Render("│")
	require.NotContains(t, frame, probe, "the base under a modal is dimmed: no focus color survives")
}

func TestCompositor_modalStackRendersTopOwnsInput(t *testing.T) {
	c := newTestCompositor()
	a := &stubModal{box: "AAAA"}
	b := &stubModal{box: "BBBB"}
	c.modals = append(c.modals, a, b)

	frame := c.View().Content
	require.Contains(t, frame, "BBBB", "the top modal renders")
	require.NotContains(t, frame, "AAAA", "a covered modal does not render")

	c = cpress(c, "x")
	require.Equal(t, []string{"x"}, b.keys, "the top modal receives input")
	require.Empty(t, a.keys, "a covered modal receives nothing")

	c = cpress(c, "esc")
	require.Contains(t, c.View().Content, "AAAA", "popping the top reveals the next modal")
}

func TestFocus_tabCyclesPanes(t *testing.T) {
	c := newTestCompositor()
	require.Equal(t, focusNav, c.focus)
	c = cpress(c, "tab")
	require.Equal(t, focusWork, c.focus)
	c = cpress(c, "tab")
	require.Equal(t, focusNav, c.focus, "tab wraps around")
}

func TestFocus_shiftTabReverses(t *testing.T) {
	c := newTestCompositor()
	c = cpress(c, "tab")
	require.Equal(t, focusWork, c.focus)
	c = cpress(c, "shift+tab")
	require.Equal(t, focusNav, c.focus)
}

func TestFocus_exactlyOnePaneFocused(t *testing.T) {
	c := newTestCompositor()
	// A styled border-left cell as it appears inside a rendered frame:
	// render a one-cell box, take the cell left of the content.
	borderCell := func(st lipgloss.Style) string {
		return strings.Split(strings.Split(st.Render("x"), "\n")[1], "x")[0]
	}
	for _, seq := range []string{"", "tab", "tab"} {
		if seq != "" {
			c = cpress(c, seq)
		}
		frame := c.View().Content
		require.Contains(t, frame, borderCell(c.th.focusedBorder), "one pane carries the focus color")
		require.Contains(t, frame, borderCell(c.th.unfocusedBorder), "the other pane does not")
		require.Contains(t, frame, "MODULES")
		require.Contains(t, frame, "module: shell")
	}
}

func TestEsc_popsTopLayer(t *testing.T) {
	c := newTestCompositor()
	c.editing = true
	c.focus = focusWork
	c.modals = append(c.modals, &stubModal{box: "AAAA"})

	c = cpress(c, "esc")
	require.Empty(t, c.modals, "esc pops the modal first")
	require.True(t, c.editing, "edit mode survives while a modal is open")

	c = cpress(c, "esc")
	require.False(t, c.editing, "esc exits edit mode next")
	require.Equal(t, focusWork, c.focus, "focus stays while editing exits")

	c = cpress(c, "esc")
	require.Equal(t, focusNav, c.focus, "esc returns focus to the nav last")
}

func TestModal_openModalCapturesAllInput(t *testing.T) {
	c := newTestCompositor()
	mod := &stubModal{box: "AAAA"}
	c.modals = append(c.modals, mod)

	c = cpress(c, "tab")
	require.Equal(t, focusNav, c.focus, "the base receives nothing while a modal is open")
	require.Equal(t, []string{"tab"}, mod.keys, "the modal sees every key")
}

func TestFooter_spinnerShowsOperationName(t *testing.T) {
	c := newTestCompositor()
	c, cmd := cstep(c, opStartedMsg{name: "apply"})
	require.NotNil(t, cmd, "a running operation ticks the spinner")
	frame := c.View().Content
	require.Contains(t, frame, "apply")
	require.Contains(t, frame, spinFrames[0], "the spinner frame shows")

	c, cmd = cstep(c, spinTickMsg{})
	require.NotNil(t, cmd, "the spinner keeps ticking while the operation runs")
	require.Contains(t, c.View().Content, spinFrames[1], "the spinner advances")
}

func TestFooter_messageSlotSuccessFades(t *testing.T) {
	c := newTestCompositor()
	c, cmd := cstep(c, opFinishedMsg{note: "applied 12 links"})
	require.NotNil(t, cmd, "a success message schedules its fade")
	require.Contains(t, c.View().Content, "applied 12 links")

	c, _ = cstep(c, msgFadeMsg{token: c.msgToken})
	require.NotContains(t, c.View().Content, "applied 12 links", "the fade clears the slot")
}

func TestFooter_failurePersistsUntilNextAction(t *testing.T) {
	c := newTestCompositor()
	c, cmd := cstep(c, opFinishedMsg{note: "apply failed: link /etc/x", err: errors.New("boom")})
	require.Nil(t, cmd, "a failure schedules no fade")
	require.Contains(t, c.View().Content, "apply failed")

	c, _ = cstep(c, msgFadeMsg{token: c.msgToken})
	require.Contains(t, c.View().Content, "apply failed", "a stale fade tick cannot clear a failure")

	c = cpress(c, "j")
	require.NotContains(t, c.View().Content, "apply failed", "the next user action clears the failure")
}

func TestHeader_dirtyCountAndApplyBadge(t *testing.T) {
	c := newTestCompositor()
	frame := c.View().Content
	require.NotContains(t, frame, "●", "a clean shell shows no dirty count")
	require.NotContains(t, frame, "apply", "no badge without a running apply")

	c.dirty = 2
	require.Contains(t, c.View().Content, "● 2", "the header counts dirty drafts")

	c.applying = true
	require.Contains(t, c.View().Content, "apply", "the badge shows while an apply runs")
}

func TestSpinInterval_isReasonable(t *testing.T) {
	require.GreaterOrEqual(t, spinInterval, 50*time.Millisecond, "faster flickers")
	require.LessOrEqual(t, spinInterval, 250*time.Millisecond, "slower looks stuck")
}
