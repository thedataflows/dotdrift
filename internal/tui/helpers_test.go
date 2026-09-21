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
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
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
	case "ctrl+z":
		return tea.KeyPressMsg{Code: 'z', Mod: tea.ModCtrl}
	case "ctrl+shift+z":
		return tea.KeyPressMsg{Code: 'z', Mod: tea.ModCtrl | tea.ModShift}
	case "ctrl+r":
		return tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl}
	case "ctrl+enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModCtrl}
	case "ctrl+l":
		return tea.KeyPressMsg{Code: 'l', Mod: tea.ModCtrl}
	case "ctrl+o":
		return tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl}
	case "ctrl+u":
		return tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl}
	case "ctrl+g":
		return tea.KeyPressMsg{Code: 'g', Mod: tea.ModCtrl}
	default:
		return tea.KeyPressMsg{Code: rune(s[0])}
	}
}

// wsDraftFor returns the compositor's draft for a layer dir, if any.
func (m *Compositor) wsDraftFor(dir string) *wsDraft { return m.store[dir] }

// visibleLabels lists the palette's current row labels.
func (p *paletteModel) visibleLabels() []string {
	var out []string
	for _, r := range p.rows {
		out = append(out, r.label)
	}
	return out
}

// actionLabels lists the currently valid action labels.
func (p *paletteModel) actionLabels() []string {
	var out []string
	for _, e := range p.all {
		if e.section == sectionActions {
			out = append(out, e.label)
		}
	}
	return out
}

// placeholderOrBody renders the placeholder text or the joined row texts.
func (w *workspaceModel) placeholderOrBody() string {
	if len(w.rows) == 0 {
		return w.placeholder
	}
	var b strings.Builder
	for _, r := range w.rows {
		b.WriteString(r.text + "\n")
	}
	return b.String()
}

// atSection reports whether the cursor sits in the named section.
func (w *workspaceModel) atSection(name string) bool {
	return w.cursor < len(w.rows) && w.rows[w.cursor].section == name
}

// draftEdits returns the committed-edit marker set, nil without a draft.
func (w *workspaceModel) draftEdits() map[string]bool {
	if w.draft == nil {
		return nil
	}
	return w.draft.edited
}

// draftErrs returns the staged tier-1 errors, nil without a draft.
func (w *workspaceModel) draftErrs() map[string]string {
	if w.draft == nil {
		return nil
	}
	return w.draft.errs
}
