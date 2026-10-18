package tui

// The one text input component (0135). Every text field in the TUI
// edits through a textInput — the dialog fields, the workspace
// inline editor, the multi-line editor's caret line, and the nav,
// palette, and picker queries — so typing, caret movement, and the
// window that follows the caret behave the same everywhere. Before
// this, each input kept its own buffer, caret, and window code and
// the behaviors drifted (the inline and dialog inputs had no
// home/end/delete; the queries could not move their caret at all).
//
// The elevation password is the one deliberate exception: its
// buffer is bytes that are zeroed on submit, and it has no caret.

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// fieldViewCells is the default visible window width, in cells:
// the dialog fields' fixed width (0131).
const fieldViewCells = 40

// textInput is a single-line text buffer with a caret and a
// scrolling window. Cells is the component's width prop: the owner
// sets it from its layout before rendering (a pane width, the
// dialog fields' fixed 40), and the render never exceeds it.
type textInput struct {
	buf   []rune
	cur   int
	off   int // the visible window's left edge, in runes
	Cells int
}

func newTextInput(s string) textInput {
	return textInput{buf: []rune(s), cur: len([]rune(s)), Cells: fieldViewCells}
}

func (t *textInput) String() string { return string(t.buf) }

// set replaces the buffer (a pick, a prefill); the caret lands at
// the end.
func (t *textInput) set(s string) {
	t.buf = []rune(s)
	t.cur = len(t.buf)
}

// insertRunes inserts runes at the caret — typing and paste share
// this path.
func (t *textInput) insertRunes(rs []rune) {
	if len(rs) == 0 {
		return
	}
	t.buf = append(t.buf[:t.cur], append(rs, t.buf[t.cur:]...)...)
	t.cur += len(rs)
}

func (t *textInput) backspace() {
	if t.cur == 0 {
		return
	}
	t.buf = append(t.buf[:t.cur-1], t.buf[t.cur:]...)
	t.cur--
}

func (t *textInput) deleteForward() {
	if t.cur < len(t.buf) {
		t.buf = append(t.buf[:t.cur], t.buf[t.cur+1:]...)
	}
}

func (t *textInput) left() {
	if t.cur > 0 {
		t.cur--
	}
}

func (t *textInput) right() {
	if t.cur < len(t.buf) {
		t.cur++
	}
}

func (t *textInput) home() { t.cur = 0 }

func (t *textInput) end() { t.cur = len(t.buf) }

// handleKey applies the shared editing keys — caret movement,
// backspace, delete, and text insertion — and reports whether the
// key was consumed. Owner keys (enter, esc, tab, arrows the owner
// moves with) are never consumed.
func (t *textInput) handleKey(k tea.KeyPressMsg) bool {
	switch k.String() {
	case "left":
		t.left()
	case "right":
		t.right()
	case "home":
		t.home()
	case "end":
		t.end()
	case "backspace":
		t.backspace()
	case "delete":
		t.deleteForward()
	default:
		if k.Text == "" {
			return false
		}
		t.insertRunes([]rune(k.Text))
	}
	return true
}

// frame computes the visible window and moves its edge to follow
// the caret.
func (t *textInput) frame() (window []rune, caret int, moreLeft, moreRight bool) {
	win, off, caret, moreLeft, moreRight := framedWindow(t.buf, t.cur, t.off, t.Cells)
	t.off = off
	return win, caret, moreLeft, moreRight
}

// render draws the window in the given text style, with a faded
// "..." at each edge where text continues (0133). caret shows the
// block caret at the cursor (an active input); without it the
// plain window renders (an unfocused field, an applied filter).
func (t *textInput) render(th theme, style lipgloss.Style, caret bool) string {
	win, at, moreLeft, moreRight := t.frame()
	left, right := "", ""
	if moreLeft {
		left = th.disabledMark.Render("...")
	}
	if moreRight {
		right = th.disabledMark.Render("...")
	}
	if !caret {
		return left + style.Render(string(win)) + right
	}
	cell, after := " ", ""
	if at < len(win) {
		cell = string(win[at])
		after = string(win[at+1:])
	} else {
		// The caret cell at the end of the text is one cell past
		// the window; drop runes on the left so the render still
		// fits Cells.
		marks := 0
		if moreLeft {
			marks += 3
		}
		if moreRight {
			marks += 3
		}
		if over := marks + len(win) + 1 - t.Cells; over > 0 {
			win = win[min(over, len(win)):]
			at = len(win)
		}
	}
	return left + style.Render(string(win[:at])) + th.caretCell(cell) + style.Render(after) + right
}

// runeWindow returns the window of runes that contains cur, at most
// cells runes wide, plus the window's new left edge and cur's offset
// inside the window (0131, 0132). The edge moves only when cur leaves
// the window, so the text does not slide on every keystroke. Every
// text input renders through this helper: clipped text is always
// reachable with the arrow keys because the window follows the caret.
func runeWindow(runes []rune, cur, off, cells int) (window []rune, newOff, caret int) {
	cells = max(cells, 1)
	cur = min(max(cur, 0), len(runes))
	if cur < off {
		off = cur
	}
	if cur >= off+cells {
		off = cur - cells + 1
	}
	if maxOff := max(len(runes)-cells, 0); off > maxOff {
		off = maxOff
	}
	return runes[off:min(off+cells, len(runes))], off, cur - off
}

// framedWindow is runeWindow with continuation marks accounted
// (0133): when text hides beyond an edge, three cells of the budget
// go to a faded "..." the caller renders at that edge, so the input
// never grows — the window just shows three fewer runes.
func framedWindow(runes []rune, cur, off, cells int) (window []rune, newOff, caret int, moreLeft, moreRight bool) {
	window, off, caret = runeWindow(runes, cur, off, cells)
	moreLeft = off > 0
	moreRight = off+len(window) < len(runes)
	if moreLeft || moreRight {
		budget := cells
		if moreLeft {
			budget -= 3
		}
		if moreRight {
			budget -= 3
		}
		window, off, caret = runeWindow(runes, cur, off, budget)
		moreLeft = off > 0
		moreRight = off+len(window) < len(runes)
	}
	return window, off, caret, moreLeft, moreRight
}
