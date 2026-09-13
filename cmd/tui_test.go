package cmd

import (
	"strings"
	"testing"

	"charm.land/bubbletea/v2"
	"github.com/alecthomas/kong"
	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/tui"
)

// The tui command (T-tui-shell): the one interactive home. The adapter
// wires the reads area over the same seams the other commands use and
// launches the shell through the program-runner seam — so tests can
// observe the launch without a terminal.

func TestTUI_launchesShellOverServiceReads(t *testing.T) {
	origDetect := detectFacts
	detectFacts = func() (*facts.Facts, error) {
		return &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux"}, nil
	}
	t.Cleanup(func() { detectFacts = origDetect })

	var launched *tui.Shell
	origRun := runTUIProgram
	runTUIProgram = func(m *tui.Shell) error {
		launched = m
		return nil
	}
	t.Cleanup(func() { runTUIProgram = origRun })

	c := &TUICmd{Profile: "../testdata/profiles/resolve"}
	require.NoError(t, c.Run())
	require.NotNil(t, launched, "the command launches the shell")

	// The launched shell builds its tree from the real reads area: its
	// startup load is scheduled and the screen renders.
	init := launched.Init()
	require.NotNil(t, init, "the shell schedules its startup load")
	require.NotEmpty(t, launched.View().Content, "the shell renders")
}

// shellKey builds one key press for the shell (single-rune keys only).
func shellKey(s string) tea.KeyPressMsg { return tea.KeyPressMsg{Code: rune(s[0])} }

// shellStep feeds one message through the shell, running any follow-up
// command and feeding its message back (the async loads' results).
func shellStep(t *testing.T, m *tui.Shell, msg tea.Msg) {
	t.Helper()
	if msg == nil {
		return
	}
	_, cmd := m.Update(msg)
	if cmd != nil {
		if follow := cmd(); follow != nil {
			m.Update(follow)
		}
	}
}

// The apply gate is wired end to end: over the real shell, `a` on the
// plan view opens the gate — the shell's factory returns a live
// launcher over the apply area. With no wiring the pane would stay PLAN.
func TestTUI_applyGateWired(t *testing.T) {
	origDetect := detectFacts
	detectFacts = func() (*facts.Facts, error) {
		return &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux"}, nil
	}
	t.Cleanup(func() { detectFacts = origDetect })

	var launched *tui.Shell
	origRun := runTUIProgram
	runTUIProgram = func(m *tui.Shell) error {
		launched = m
		return nil
	}
	t.Cleanup(func() { runTUIProgram = origRun })

	c := &TUICmd{Profile: "../testdata/profiles/resolve"}
	require.NoError(t, c.Run())
	require.NotNil(t, launched)

	init := launched.Init()
	require.NotNil(t, init)
	shellStep(t, launched, init())
	shellStep(t, launched, tea.WindowSizeMsg{Width: 100, Height: 30})

	// Walk to the plan node: jump to the bottom, then up until the plan
	// view is the active one.
	shellStep(t, launched, shellKey("G"))
	atPlan := false
	for range 40 {
		if strings.Contains(launched.View().Content, "PLAN") {
			atPlan = true
			break
		}
		shellStep(t, launched, shellKey("k"))
	}
	require.True(t, atPlan, "the plan view is reachable")

	shellStep(t, launched, shellKey("a"))
	require.Contains(t, launched.View().Content, "run apply?",
		"a opens the apply gate over the wired apply area")
}

func TestTUI_unknownFlagsRejected(t *testing.T) {
	var cli CLI
	parser, err := kong.New(&cli, kong.Name(appName))
	require.NoError(t, err)
	_, err = parser.Parse([]string{"tui", "--wizard"})
	require.Error(t, err, "tui has no wizard flag — the strict flag mode applies")
	require.Contains(t, err.Error(), "--wizard")
}

func TestTUI_registeredInRoot(t *testing.T) {
	var cli CLI
	parser, err := kong.New(&cli, kong.Name(appName), kong.Exit(func(int) {}))
	require.NoError(t, err)
	_, err = parser.Parse([]string{"tui", "--help"})
	require.NoError(t, err, "tui is a registered command")
}
