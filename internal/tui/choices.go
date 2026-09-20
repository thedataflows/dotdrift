package tui

// Choice pickers (0076 T-tui-choice): a field whose value set is closed
// edits by selection, not free text. enter/e on such a row opens a
// centered picker modal; a pick commits through the same draft path as
// the text input's enter, so splicing, validation, and the ledger are
// the editing pipeline's, not the picker's.

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/thedataflows/dotdrift/internal/profile"
)

// choiceSet returns the closed value set for a row's field, or nil when
// the row edits as free text. Directive names match case-insensitively —
// systemd accepts both, the TOML keys keep the file's spelling.
func choiceSet(row wsRow) []string {
	switch row.family {
	case profile.FamilyKeys:
		if row.key == "scope" {
			return []string{profile.ScopeUser, profile.ScopeSystem}
		}
	case profile.FamilySecrets:
		if lastPathSegment(row.key) == "allow_empty" {
			return []string{"false", "true"}
		}
	case profile.FamilyMounts:
		if lastPathSegment(row.key) == "state" {
			return []string{"enabled", "disabled"}
		}
	case profile.FamilySmb:
		switch lastPathSegment(row.key) {
		case "writable", "public":
			return []string{"false", "true"}
		case "avahi":
			return []string{"true", "false", "unset"}
		}
	case profile.FamilySystemd:
		switch strings.ToLower(lastPathSegment(row.key)) {
		case "type":
			return []string{"simple", "forking", "oneshot", "dbus", "notify", "exec"}
		case "restart":
			return []string{"no", "on-success", "on-failure", "on-abnormal", "on-watchdog", "always"}
		}
	}
	return nil
}

// effectiveChoice is the value the picker marks as current: the row's
// value, except scope where an unset raw value still means user.
func effectiveChoice(row wsRow) string {
	if row.family == profile.FamilyKeys && row.key == "scope" {
		return profile.ModuleConfig{Scope: row.value}.ScopeOrDefault()
	}
	return row.value
}

// choiceTitle names the picker: the field, and for a systemd directive
// the unit it belongs to.
func choiceTitle(row wsRow) string {
	field := lastPathSegment(row.key)
	if row.family == profile.FamilySystemd {
		return field + " · " + splitPath(row.key)[0]
	}
	return field
}

// commitValue maps a picker label to the value the edit pipeline
// receives: "unset" is the field's zero value (avahi's three-state nil).
func commitValue(label string) string {
	if label == "unset" {
		return ""
	}
	return label
}

// choiceModel is the picker: one row per value, the effective value
// marked, the cursor preselected on it. A pick reports finished(); the
// compositor pops the layer and the onPick seam has already committed.
type choiceModel struct {
	th      theme
	title   string
	choices []string
	current string // the effective value; picking it stages nothing
	sel     int
	done    bool
	clicked int // the last clicked row: clicking it again picks
	boxY    int // the last render's box origin, for row hit-testing
	onPick  func(value string)
}

func newChoiceModel(th theme, title string, choices []string, current string, onPick func(value string)) *choiceModel {
	c := &choiceModel{
		th:      th,
		title:   title,
		choices: choices,
		current: current,
		clicked: -1,
		onPick:  onPick,
	}
	for i, choice := range choices {
		if choice == current {
			c.sel = i
		}
	}
	return c
}

func (c *choiceModel) finished() bool { return c.done }

func (c *choiceModel) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "up", "k", "left":
			if c.sel > 0 {
				c.sel--
			}
		case "down", "j", "right":
			if c.sel < len(c.choices)-1 {
				c.sel++
			}
		case "enter":
			c.done = true
			if c.onPick != nil {
				c.onPick(c.choices[c.sel])
			}
		}
	case tea.MouseWheelMsg:
		if msg.Button == tea.MouseWheelUp && c.sel > 0 {
			c.sel--
		}
		if msg.Button == tea.MouseWheelDown && c.sel < len(c.choices)-1 {
			c.sel++
		}
	case tea.MouseClickMsg:
		if msg.Button == tea.MouseLeft {
			idx := c.hitRow(msg.Y)
			switch {
			case idx < 0:
			case idx == c.clicked:
				c.done = true
				if c.onPick != nil {
					c.onPick(c.choices[idx])
				}
			default:
				c.sel = idx
				c.clicked = idx
			}
		}
	}
	return nil
}

// hitRow maps a click Y to a choice index, or -1: one padding line, the
// title, one blank line, then the choices.
func (c *choiceModel) hitRow(y int) int {
	idx := y - (c.boxY + 3)
	if idx < 0 || idx >= len(c.choices) {
		return -1
	}
	return idx
}

func (c *choiceModel) view(w, h int) string {
	lines := []string{c.th.modalTitle.Render(c.title), ""}
	for i, choice := range c.choices {
		label := choice
		if choice == c.current {
			label += "  " + c.th.meta.Render("(current)")
		}
		if i == c.sel {
			lines = append(lines, c.th.cursorRow.Render(" "+label))
		} else {
			lines = append(lines, c.th.rowText.Render("  "+label))
		}
	}
	lines = append(lines, "", c.th.meta.Render("up/down pick · enter choose · esc cancel"))
	box := c.th.modalBorder.Padding(1, 2).Render(strings.Join(lines, "\n"))
	c.boxY = max((h-lipgloss.Height(box))/2, 0)
	return box
}
