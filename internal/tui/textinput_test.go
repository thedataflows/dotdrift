package tui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// 0135: the shared text input — one buffer, one caret, one window.
// Every text field in the TUI edits through it, so typing, caret
// movement, and the window that follows the caret behave the same
// in the dialog fields, the workspace editor, and the queries.
func TestTextInput_editingKeys(t *testing.T) {
	ti := newTextInput("hello")
	require.Equal(t, "hello", ti.String())
	require.Equal(t, 5, ti.cur, "the caret starts at the end")

	ti.handleKey(keyPress("left"))
	ti.handleKey(keyPress("left"))
	ti.insertRunes([]rune("X"))
	require.Equal(t, "helXlo", ti.String(), "typing inserts at the caret")

	ti.handleKey(keyPress("home"))
	ti.insertRunes([]rune("»"))
	require.Equal(t, "»helXlo", ti.String(), "home reaches the start")

	ti.handleKey(keyPress("end"))
	ti.handleKey(keyPress("backspace"))
	require.Equal(t, "»helXl", ti.String(), "backspace deletes left of the caret")

	ti.handleKey(keyPress("home"))
	ti.handleKey(keyPress("delete"))
	require.Equal(t, "helXl", ti.String(), "delete deletes right of the caret")

	require.False(t, ti.handleKey(keyPress("enter")), "enter stays the owner's key")
	require.False(t, ti.handleKey(keyPress("esc")), "esc stays the owner's key")
	require.False(t, ti.handleKey(keyPress("up")), "arrows the component does not own are not consumed")
}

func TestTextInput_windowFollowsCaret(t *testing.T) {
	th := newTheme(true)
	ti := newTextInput(strings.Repeat("a", 30) + strings.Repeat("b", 30))
	ti.Cells = 20

	plain := ansiRe.ReplaceAllString(ti.render(th, th.rowText, true), "")
	require.Contains(t, plain, strings.Repeat("b", 10), "the window shows the text at the caret")
	require.Contains(t, plain, "...", "the left edge is marked")
	require.LessOrEqual(t, len([]rune(plain)), 20, "the render fits the Cells prop")

	ti.home()
	plain = ansiRe.ReplaceAllString(ti.render(th, th.rowText, true), "")
	require.Contains(t, plain, strings.Repeat("a", 10), "the window follows the caret home")
	require.NotContains(t, plain, strings.Repeat("b", 10))

	plain = ansiRe.ReplaceAllString(ti.render(th, th.rowText, false), "")
	require.NotContains(t, plain, "\x1b[7m", "without the caret flag there is no caret cell")
}
