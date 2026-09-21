package tui

// T-tui-typed-inputs (0094): the multi-line editor's unit tests. The
// editor is the modal for values that ARE many lines — a writes block,
// a hook command. enter splits, backspace joins, arrows walk across
// line boundaries, paste keeps its newlines (unlike the single-line
// inputs' sanitizer), ctrl+enter commits through the caller's callback
// (a refusal renders inside), esc cancels through the compositor's pop,
// and `c` toggles source coloring when the target's extension names a
// known lexer. The caret is the 0092 reverse-video block, never an
// inserted glyph.

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"
)

// mtype sends each rune as a key with Text set — what a real terminal
// delivers while the editor is active.
func mtype(e *multiline, s string) {
	for _, r := range s {
		e.update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

func mkey(e *multiline, s string) { e.update(keyPress(s)) }

func commitRecorder() (func(string) string, *string) {
	got := new(string)
	return func(s string) string { *got = s; return "" }, got
}

func TestMultiline_seedRoundTripAndCommit(t *testing.T) {
	commit, got := commitRecorder()
	e := newMultiline(newTheme(true), "edit block", "one\ntwo", "", commit)
	require.Equal(t, "one\ntwo", e.value(), "the seed splits into lines")
	require.Equal(t, 1, e.cy, "the caret seeds at the end of the content")
	require.Equal(t, 3, e.cx)

	mkey(e, "ctrl+enter")
	require.True(t, e.finished(), "ctrl+enter commits")
	require.Equal(t, "one\ntwo", *got, "the commit carries the full text")
}

func TestMultiline_typingAndEnterSplit(t *testing.T) {
	commit, got := commitRecorder()
	e := newMultiline(newTheme(true), "edit", "ab", "", commit)
	mtype(e, "c")
	mkey(e, "enter") // enter is a newline here, never a commit
	require.False(t, e.finished())
	mtype(e, "d")
	require.Equal(t, "abc\nd", e.value())
	require.Equal(t, []int{1, 1}, []int{e.cy, e.cx}, "the caret follows the split")
	mkey(e, "ctrl+enter")
	require.Equal(t, "abc\nd", *got)
}

func TestMultiline_backspaceDeletesAndJoins(t *testing.T) {
	commit, _ := commitRecorder()
	e := newMultiline(newTheme(true), "edit", "ab\ncd", "", commit)
	// caret seeded at (1, 2); two backspaces clear line 1, the third joins.
	mkey(e, "backspace")
	mkey(e, "backspace")
	require.Equal(t, "ab\n", e.value())
	mkey(e, "backspace")
	require.Equal(t, "ab", e.value(), "backspace at column 0 joins with the previous line")
	require.Equal(t, []int{0, 2}, []int{e.cy, e.cx}, "the caret lands at the join point")
}

func TestMultiline_caretMovement(t *testing.T) {
	commit, _ := commitRecorder()
	e := newMultiline(newTheme(true), "edit", "abc\nde\nfghi", "", commit)
	// seeded at (2, 4)
	mkey(e, "left")
	require.Equal(t, []int{2, 3}, []int{e.cy, e.cx})
	mkey(e, "home")
	require.Equal(t, []int{2, 0}, []int{e.cy, e.cx})
	mkey(e, "left")
	require.Equal(t, []int{1, 2}, []int{e.cy, e.cx}, "left at column 0 wraps to the previous line's end")
	mkey(e, "right")
	require.Equal(t, []int{2, 0}, []int{e.cy, e.cx}, "right at a line's end wraps to the next line's start")
	mkey(e, "up")
	require.Equal(t, []int{1, 0}, []int{e.cy, e.cx})
	mkey(e, "up")
	mkey(e, "up") // clamps at the first line
	require.Equal(t, 0, e.cy)
	e.cx = 3 // abc: cx beyond the next line's length
	mkey(e, "down")
	require.Equal(t, []int{1, 2}, []int{e.cy, e.cx}, "down clamps the column to the shorter line")
	mkey(e, "end")
	require.Equal(t, []int{1, 2}, []int{e.cy, e.cx})
	mkey(e, "pgdown")
	require.Equal(t, 2, e.cy, "pgdn walks a page of lines and clamps")
	mkey(e, "pgup")
	require.Equal(t, 0, e.cy)
}

func TestMultiline_pasteKeepsNewlines(t *testing.T) {
	commit, _ := commitRecorder()
	e := newMultiline(newTheme(true), "edit", "ab", "", commit)
	e.update(tea.PasteMsg{Content: "X\nY\r\nZ"})
	require.Equal(t, "abX\nY\nZ", e.value(), "paste is one message; its newlines (CR-normalized) become line breaks")
	require.Equal(t, []int{2, 1}, []int{e.cy, e.cx}, "the caret lands past the pasted block")
}

func TestMultiline_tabInsertsSpaces(t *testing.T) {
	commit, _ := commitRecorder()
	e := newMultiline(newTheme(true), "edit", "", "", commit)
	mkey(e, "tab")
	require.Equal(t, "  ", e.value(), "tab inserts spaces (a literal tab would break caret math)")
}

func TestMultiline_commitRefusalStaysOpen(t *testing.T) {
	e := newMultiline(newTheme(true), "edit", "x", "", func(string) string { return "block must not be empty" })
	mkey(e, "ctrl+enter")
	require.False(t, e.finished(), "a refused commit keeps the editor open")
	require.Equal(t, "block must not be empty", e.err)
	mtype(e, "y")
	mkey(e, "ctrl+enter")
	require.Equal(t, "block must not be empty", e.err, "the refusal clears only on a successful commit")
}

func TestMultiline_ownsText(t *testing.T) {
	commit, _ := commitRecorder()
	e := newMultiline(newTheme(true), "edit", "", "", commit)
	require.True(t, e.ownsText(), "the editor owns every printable key (? is content, not help)")
}

func TestMultiline_colorToggleNeedsALexer(t *testing.T) {
	commit, _ := commitRecorder()
	e := newMultiline(newTheme(true), "edit", "echo hi", "~/.bashrc", commit)
	require.False(t, e.color)
	mkey(e, "ctrl+g")
	require.True(t, e.color, "c toggles coloring when the target names a known lexer")

	plain := newMultiline(newTheme(true), "edit", "echo hi", "", commit)
	mkey(plain, "ctrl+g")
	require.False(t, plain.color, "without a lexer hint c changes nothing")
}

func TestMultiline_coloredViewKeepsCaretLinePlain(t *testing.T) {
	commit, _ := commitRecorder()
	e := newMultiline(newTheme(true), "edit", "echo one\necho two", "~/.bashrc", commit)
	mkey(e, "ctrl+g")
	e.update(keyPress("home")) // caret on line 0… wait: home moves within line; walk up instead
	e.cy = 0
	e.cx = 0
	view := e.view(100, 30)
	lines := strings.Split(view, "\n")
	var caretLine string
	for _, l := range lines {
		if strings.Contains(l, "cho one") { // the caret cell splits "echo one"
			caretLine = l
		}
	}
	require.NotEmpty(t, caretLine, "the caret's line renders")
	require.NotContains(t, caretLine, "\x1b[38;5;", "the caret's own line carries no token colors (the terminal256 formatter's 38;5 signature; the border's truecolor is not the line's)")
}

func TestMultiline_viewShowsHintAndTitle(t *testing.T) {
	commit, _ := commitRecorder()
	e := newMultiline(newTheme(true), "edit block · ~/.profile", "x", "", commit)
	plain := ansiRe.ReplaceAllString(e.view(100, 30), "")
	require.Contains(t, plain, "edit block · ~/.profile")
	require.Contains(t, plain, "ctrl+enter")
	require.Contains(t, plain, "esc")
}
