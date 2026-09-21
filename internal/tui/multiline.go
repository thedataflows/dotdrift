package tui

// T-tui-typed-inputs (0094): the multi-line editor modal — the input
// for values that ARE many lines (a writes block, a hook command).
// enter splits, backspace joins at column 0, arrows/home/end/pgup/pgdn
// walk across line boundaries, paste keeps its newlines (the
// single-line inputs' sanitizer drops them — here they are the point),
// and ctrl+enter commits through the caller's callback; a refusal
// renders inside and the editor stays open. esc cancels through the
// compositor's pop (nothing commits). ctrl+g toggles source coloring
// when the target's extension names a known chroma lexer — a bare
// letter can never be a binding here, because every printable key is
// content. The caret is the 0092 reverse-video block, and the caret's
// own line renders uncolored: token colors and the reversed caret cell
// cannot share a line.

import (
	"bytes"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/quick"
	"github.com/charmbracelet/x/ansi"
)

// multiline is the multi-line editor modal. commit returns "" on
// success; anything else keeps the editor open with the error inside
// (the add form's contract).
type multiline struct {
	th     theme
	title  string
	lines  [][]rune
	cy     int
	cx     int
	off    int
	pageH  int // the last render's body height: the paging step
	hint   string
	color  bool
	err    string
	commit func(string) string
	done   bool
}

// newMultiline opens the editor seeded with initial, the caret at the
// content's end. hint is the coloring lexer guess (a writes target
// filename); "" disables coloring.
func newMultiline(th theme, title, initial, hint string, commit func(string) string) *multiline {
	var lines [][]rune
	for _, l := range strings.Split(initial, "\n") {
		lines = append(lines, []rune(l))
	}
	if len(lines) == 0 {
		lines = [][]rune{{}}
	}
	return &multiline{
		th: th, title: title, lines: lines, hint: hint, commit: commit,
		cy: len(lines) - 1, cx: len(lines[len(lines)-1]), pageH: 10,
	}
}

func (e *multiline) finished() bool { return e.done }

// ownsText keeps every printable key for the editor (? is content, not
// the help gesture) — the compositor consults this before opening help
// over the modal.
func (e *multiline) ownsText() bool { return true }

// value joins the lines back into the field's text.
func (e *multiline) value() string {
	var b strings.Builder
	for i, l := range e.lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(string(l))
	}
	return b.String()
}

// lexer resolves the coloring lexer from the hint, nil when unknown.
func (e *multiline) lexer() string {
	if e.hint == "" {
		return ""
	}
	if l := lexers.Match(e.hint); l != nil && l.Config() != nil {
		return l.Config().Name
	}
	return ""
}

func (e *multiline) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.PasteMsg:
		e.paste(msg.Content)
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+enter":
			if err := e.commit(e.value()); err != "" {
				e.err = err
			} else {
				e.err = ""
				e.done = true
			}
		case "ctrl+g":
			if e.lexer() != "" {
				e.color = !e.color
			}
		case "enter":
			e.splitLine()
		case "backspace":
			e.backspace()
		case "delete":
			e.deleteForward()
		case "left":
			if e.cx > 0 {
				e.cx--
			} else if e.cy > 0 {
				e.cy--
				e.cx = len(e.lines[e.cy])
			}
		case "right":
			if e.cx < len(e.lines[e.cy]) {
				e.cx++
			} else if e.cy < len(e.lines)-1 {
				e.cy++
				e.cx = 0
			}
		case "up":
			if e.cy > 0 {
				e.cy--
				e.clampX()
			}
		case "down":
			if e.cy < len(e.lines)-1 {
				e.cy++
				e.clampX()
			}
		case "home":
			e.cx = 0
		case "end":
			e.cx = len(e.lines[e.cy])
		case "pgup":
			e.cy = max(0, e.cy-max(e.pageH, 1))
			e.clampX()
		case "pgdown":
			e.cy = min(len(e.lines)-1, e.cy+max(e.pageH, 1))
			e.clampX()
		case "tab":
			// A literal tab would break the caret's cell math; two
			// spaces are the TUI's tab.
			e.insert([]rune("  "))
		default:
			if msg.Text != "" {
				e.insert([]rune(msg.Text))
			}
		}
	}
	return nil
}

// clampX pulls the column back onto a shorter line after vertical
// movement.
func (e *multiline) clampX() { e.cx = min(e.cx, len(e.lines[e.cy])) }

// insert puts runes at the caret.
func (e *multiline) insert(rs []rune) {
	e.lines[e.cy] = slices.Insert(e.lines[e.cy], e.cx, rs...)
	e.cx += len(rs)
}

// splitLine breaks the current line at the caret (enter).
func (e *multiline) splitLine() {
	line := e.lines[e.cy]
	tail := slices.Clone(line[e.cx:])
	e.lines[e.cy] = line[:e.cx]
	e.lines = slices.Insert(e.lines, e.cy+1, tail)
	e.cy++
	e.cx = 0
}

// backspace deletes left, joining with the previous line at column 0.
func (e *multiline) backspace() {
	line := e.lines[e.cy]
	if e.cx > 0 {
		e.lines[e.cy] = slices.Delete(line, e.cx-1, e.cx)
		e.cx--
		return
	}
	if e.cy == 0 {
		return
	}
	prev := e.lines[e.cy-1]
	e.lines[e.cy-1] = append(prev, line...)
	e.lines = slices.Delete(e.lines, e.cy, e.cy+1)
	e.cy--
	e.cx = len(prev)
}

// deleteForward deletes right, joining with the next line at the end.
func (e *multiline) deleteForward() {
	line := e.lines[e.cy]
	if e.cx < len(line) {
		e.lines[e.cy] = slices.Delete(line, e.cx, e.cx+1)
		return
	}
	if e.cy == len(e.lines)-1 {
		return
	}
	e.lines[e.cy] = append(line, e.lines[e.cy+1]...)
	e.lines = slices.Delete(e.lines, e.cy+1, e.cy+2)
}

// paste inserts bracketed-paste content at the caret (0091): newlines
// are the point here — they become line breaks (CR-normalized); other
// control runes drop out through the single-line sanitizer.
func (e *multiline) paste(content string) {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	parts := strings.Split(content, "\n")
	line := e.lines[e.cy]
	head, tail := slices.Clone(line[:e.cx]), line[e.cx:]
	fresh := [][]rune{append(head, pasteRunes(parts[0])...)}
	for _, p := range parts[1:] {
		fresh = append(fresh, pasteRunes(p))
	}
	fresh[len(fresh)-1] = append(fresh[len(fresh)-1], tail...)
	e.cx = len(fresh[len(fresh)-1]) - len(tail)
	e.lines = slices.Replace(e.lines, e.cy, e.cy+1, fresh...)
	e.cy += len(fresh) - 1
}

// highlighted renders the content through chroma, one output line per
// source line, or false when coloring is off, unknown, or the
// highlighter's line accounting drifts (a safe plain fallback).
func (e *multiline) highlighted() ([]string, bool) {
	name := e.lexer()
	if !e.color || name == "" {
		return nil, false
	}
	var buf bytes.Buffer
	if err := quick.Highlight(&buf, e.value(), name, "terminal256", "monokai"); err != nil {
		return nil, false
	}
	out := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	if len(out) != len(e.lines) {
		return nil, false
	}
	return out, true
}

func (e *multiline) view(w, h int) string {
	lines := []string{e.th.modalTitle.Render(e.title)}
	hint := "ctrl+enter commit · esc cancel"
	if e.lexer() != "" {
		hint += " · ctrl+g color"
	}
	lines = append(lines, e.th.meta.Render(hint))
	if e.err != "" {
		lines = append(lines, e.th.errorMark.Render("✗ "+e.err))
	}
	lines = append(lines, "")
	bodyH := min(max(h-14, 3), 20)
	e.pageH = bodyH
	if e.cy < e.off {
		e.off = e.cy
	}
	if e.cy >= e.off+bodyH {
		e.off = e.cy - bodyH + 1
	}
	colored, ok := e.highlighted()
	maxw := max(w-16, 20)
	for i := e.off; i < len(e.lines) && i < e.off+bodyH; i++ {
		var body string
		switch {
		case i == e.cy:
			// The caret line: the 0092 reverse-video block on the cell
			// under the caret, never an inserted glyph — and never
			// token colors, which the reversed cell cannot survive.
			line := e.lines[i]
			at, after := " ", ""
			if e.cx < len(line) {
				at = string(line[e.cx])
				after = string(line[e.cx+1:])
			}
			body = string(line[:e.cx]) + e.th.caret.Render(at) + after
		case ok:
			body = colored[i]
		default:
			body = string(e.lines[i])
		}
		lines = append(lines, "  "+ansi.Truncate(body, maxw, "…"))
	}
	return e.th.modalBorder.Padding(1, 2).Render(strings.Join(lines, "\n"))
}
