package tui

// Add forms (0077 T-tui-addform): `a` opens a centered form instead of
// the grammar input — the inline add input rendered nowhere (the row
// renderer gated it behind !editing.add), so users typed blind. The
// form's rows reuse the dialog primitives; enter synthesizes the
// grammar string the pipeline already parses and commits through the
// same applyEdit path, so validation, splicing, and the ledger stay the
// editing pipeline's. A refused commit keeps the form open with the
// error inside it.

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/thedataflows/dotdrift/internal/profile"
)

// addForm is the `a` modal: labeled rows, one focused at a time.
// up/down moves, left/right moves a field cursor or cycles a choice,
// typing edits the focused field, enter commits, esc pops (the modal
// stack's own esc). A click focuses the row. Letters are never
// navigation — j/k type like any other rune while a text row holds the
// focus.
type addForm struct {
	th     theme
	title  string
	rows   []dlgRow
	build  func(rows []dlgRow) string // the grammar string the pipeline parses
	commit func(input string) string  // "" on success; the error keeps the form open
	cur    int
	err    string
	done   bool
	boxY   int // the last render's box origin, for row hit-testing
}

func newAddForm(th theme, title string, rows []dlgRow, build func([]dlgRow) string, commit func(string) string) *addForm {
	return &addForm{th: th, title: title, rows: rows, build: build, commit: commit}
}

func (f *addForm) finished() bool { return f.done }

func (f *addForm) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "up":
			if f.cur > 0 {
				f.cur--
			}
		case "down":
			if f.cur < len(f.rows)-1 {
				f.cur++
			}
		case "left":
			f.rows[f.cur].left()
		case "right":
			f.rows[f.cur].right()
		case "backspace":
			f.rows[f.cur].backspace()
		case "enter":
			if err := f.commit(f.build(f.rows)); err != "" {
				f.err = err
				return nil
			}
			f.done = true
		default:
			if msg.Text != "" {
				for _, r := range msg.Text {
					f.rows[f.cur].typeRune(r)
				}
			}
		}
	case tea.MouseWheelMsg:
		if msg.Button == tea.MouseWheelUp && f.cur > 0 {
			f.cur--
		}
		if msg.Button == tea.MouseWheelDown && f.cur < len(f.rows)-1 {
			f.cur++
		}
	case tea.MouseClickMsg:
		if msg.Button == tea.MouseLeft {
			if idx := f.hitRow(msg.Y); idx >= 0 {
				f.cur = idx
			}
		}
	}
	return nil
}

// hitRow maps a click Y to a row index, or -1: one padding line, the
// title, one blank line, then the rows (the picker's layout).
func (f *addForm) hitRow(y int) int {
	idx := y - (f.boxY + 3)
	if idx < 0 || idx >= len(f.rows) {
		return -1
	}
	return idx
}

func (f *addForm) view(w, h int) string {
	lines := []string{f.th.modalTitle.Render(f.title), ""}
	for i, r := range f.rows {
		lines = append(lines, r.renderRow(f.th, i == f.cur))
	}
	if f.err != "" {
		lines = append(lines, "", f.th.errorMark.Render("✗ "+f.err))
	}
	lines = append(lines, "", f.th.meta.Render("up/down row · type edits · < > changes · enter adds · esc cancels"))
	box := f.th.modalBorder.Padding(1, 2).Render(strings.Join(lines, "\n"))
	f.boxY = max((h-lipgloss.Height(box))/2, 0)
	return box
}

// addScope is the scope an `a` commit lands in: the section and, for
// structural containers and scoped rows, the entry path the add goes
// into (a header is the section's entry-level scope; a systemd
// directive and a when leaf add into their entry's scope).
func addScope(row wsRow) (section, addPath string) {
	section = row.section
	switch {
	case row.header:
		// the section's entry-level scope: addPath stays ""
	case row.container:
		addPath = row.key
	default:
		switch row.family {
		case profile.FamilySystemd:
			addPath = splitPath(row.key)[0]
		case profile.FamilyWhen:
			parts := splitPath(row.key)
			if len(parts) > 1 {
				addPath = pathKey(parts[:len(parts)-1]...)
			}
		}
	}
	return section, addPath
}

// addFormSpec is one section's form: title, rows, and the grammar
// synthesizer. The identity row leads (it takes the opening focus and
// the typing), choices follow. Sections without a structured spec fall
// back to a single free-text row labeled with the section's own grammar
// hint — validateAdd's refusal text for the empty input.
func addFormSpec(moduleID, section, addPath string) (string, []dlgRow, func([]dlgRow) string) {
	switch section {
	case "packages":
		name := newDlgField("name", "")
		return "add package · " + moduleID, []dlgRow{
				fieldRow(name),
				choiceRow(newDlgChoice("state", "present", "absent")),
			}, func(rows []dlgRow) string {
				pkg := strings.TrimSpace(name.String())
				if rows[1].choice.String() == "absent" {
					return "-" + pkg
				}
				return pkg
			}
	case "links":
		target := newDlgField("target", "")
		source := newDlgField("source", "")
		return "add link · " + moduleID, []dlgRow{fieldRow(target), fieldRow(source)},
			func([]dlgRow) string {
				return strings.TrimSpace(target.String()) + " " + strings.TrimSpace(source.String())
			}
	case "tools":
		name := newDlgField("name", "")
		constraint := &dlgField{label: "constraint", hint: "empty = any"}
		return "add tool · " + moduleID, []dlgRow{fieldRow(name), fieldRow(constraint)},
			func([]dlgRow) string {
				return strings.TrimSpace(name.String()) + "=" + strings.TrimSpace(constraint.String())
			}
	case "hooks":
		cmd := newDlgField("command", "")
		return "add hook · " + moduleID, []dlgRow{
				fieldRow(cmd),
				choiceRow(newDlgChoice("phase", "pre", "post")),
			}, func(rows []dlgRow) string {
				if rows[1].choice.String() == "post" {
					return "post: " + strings.TrimSpace(cmd.String())
				}
				return strings.TrimSpace(cmd.String())
			}
	}
	return "add · " + section + " · " + moduleID,
		[]dlgRow{fieldRow(&dlgField{label: section, hint: validateAdd(addFamily(section), addPath, "")})},
		func(rows []dlgRow) string { return rows[0].field.String() }
}
