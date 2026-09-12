package cmd

import (
	"testing"

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
