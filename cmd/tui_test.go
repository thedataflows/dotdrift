package cmd

// The tui command (T-tui-cleanup): the compositor shell is the only
// `dotdrift tui` — no flag, no fallback layer. The adapter wires the
// reads/config/apply/writes areas over the same seams the other commands
// use and launches through the program-runner seam, so tests observe the
// launch without a terminal.

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/alecthomas/kong"
	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/tui"
)

func TestTUI_launchesCompositorOverServiceReads(t *testing.T) {
	origDetect := detectFacts
	detectFacts = func() (*facts.Facts, error) {
		return &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux"}, nil
	}
	t.Cleanup(func() { detectFacts = origDetect })

	var launched *tui.Compositor
	origRun := runTUIProgram
	runTUIProgram = func(m *tui.Compositor) error {
		launched = m
		return nil
	}
	t.Cleanup(func() { runTUIProgram = origRun })

	c := &TUICmd{Profile: "../testdata/profiles/resolve"}
	require.NoError(t, c.Run())
	require.NotNil(t, launched, "the command launches the compositor")

	init := launched.Init()
	require.NotNil(t, init, "the compositor schedules its startup load")
	next, cmd := launched.Update(init())
	require.NotNil(t, next)
	for i := 0; i < 5 && cmd != nil; i++ { // settle the layer read
		msg := cmd()
		if msg == nil {
			break
		}
		_, cmd = next.Update(msg)
	}
	next.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	frame := next.(*tui.Compositor).View().Content
	require.Contains(t, frame, "shell", "the nav lists the profile's module")
	require.Contains(t, frame, "MODULES")
}

// TestTUI_noCompositorFlag pins the cleanup: the migration flag is gone,
// the compositor is the only shell.
func TestTUI_noCompositorFlag(t *testing.T) {
	var cli struct {
		TUI TUICmd `cmd:""`
	}
	parser, err := kong.New(&cli)
	require.NoError(t, err)
	_, err = parser.Parse([]string{"tui", "--compositor"})
	require.Error(t, err, "--compositor is gone")
	_, err = parser.Parse([]string{"tui", "--profile", "../testdata/profiles/resolve"})
	require.NoError(t, err)
}
