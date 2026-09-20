package tui

// Shared test helpers for the compositor suite (moved from the deleted
// M14 test files in T-tui-cleanup).

import (
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"
)

var updateGoldens = flag.Bool("update", false, "rewrite golden files with actual render output")

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;:?]*[a-zA-Z]`)

func requireGolden(t *testing.T, name, got string, subs map[string]string) {
	t.Helper()
	got = ansiRe.ReplaceAllString(got, "")
	for from, to := range subs {
		got = strings.ReplaceAll(got, from, to)
	}
	// Viewport padding trails every line; its width depends on substituted
	// path lengths, so trailing spaces are normalized away.
	var lines []string
	for _, line := range strings.Split(got, "\n") {
		lines = append(lines, strings.TrimRight(line, " "))
	}
	got = strings.Join(lines, "\n")
	path := filepath.Join("testdata", "golden", name)
	if *updateGoldens {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(got), 0o644))
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err, "golden %s", name)
	require.Equal(t, string(want), got, "golden %s", name)
}

// mustMsg runs a command that must return a message.
func mustMsg(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	return cmd()
}

func keyPress(s string) tea.KeyPressMsg {
	switch s {
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "shift+tab":
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "pgup":
		return tea.KeyPressMsg{Code: tea.KeyPgUp}
	case "pgdown":
		return tea.KeyPressMsg{Code: tea.KeyPgDown}
	case "home":
		return tea.KeyPressMsg{Code: tea.KeyHome}
	case "end":
		return tea.KeyPressMsg{Code: tea.KeyEnd}
	case "ctrl+c":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	case "ctrl+n":
		return tea.KeyPressMsg{Code: 'n', Mod: tea.ModCtrl}
	case "ctrl+s":
		return tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}
	default:
		return tea.KeyPressMsg{Code: rune(s[0])}
	}
}
