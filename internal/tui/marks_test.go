package tui

// T-tui-marks: rows announce their gesture, computed from the same
// registry that decides behavior — an editable field row shows ✎, a
// closed-set row shows ◂▸ (it cycles), an addable section header and a
// structural container show ＋. Read-only rows stay plain. A committed
// edit's ● replaces the gesture mark (the row has proven itself).

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// plainRowView renders one row without ANSI, at a fixed width.
func plainRowView(c *Compositor, i int) string {
	out := c.ws.rowView(c.ws.rows[i], i, c.th, 120)
	return ansiRe.ReplaceAllString(strings.Join(out, "\n"), "")
}

func wsRowBySectionText(c *Compositor, section, prefix string) int {
	for i, r := range c.ws.rows {
		if r.section == section && strings.HasPrefix(r.text, prefix) {
			return i
		}
	}
	return -1
}

func TestMarks_rowsAnnounceTheirGesture(t *testing.T) {
	_, c := wsShell(t, map[string]string{"modules/demo/module.toml": wsAllSections})

	// Editable field rows show ✎.
	desc := wsRowBySectionText(c, "meta", "description")
	require.Contains(t, plainRowView(c, desc), "✎", "a free-text field announces editing")

	// A closed-set row shows the cycle mark instead.
	scope := wsRowBySectionText(c, "meta", "scope")
	require.Contains(t, plainRowView(c, scope), "◂▸", "a choice row announces cycling")
	require.NotContains(t, plainRowView(c, scope), "✎")

	// Addable section headers show ＋.
	pkgHeader := wsRowBySectionText(c, "packages", "packages")
	require.Contains(t, plainRowView(c, pkgHeader), "＋", "an addable header announces growth")

	// A structural container shows ＋ (enter adds into it).
	unit := wsRowBySectionText(c, "systemd.units", "demo.service")
	require.Contains(t, plainRowView(c, unit), "＋")

	// Read-only rows stay plain.
	id := wsRowBySectionText(c, "meta", "id demo")
	require.NotContains(t, plainRowView(c, id), "✎")
	require.NotContains(t, plainRowView(c, id), "＋")

	// A block write shows ✎ like any editable row (0094: it edits in
	// the multi-line editor — the mark cannot lie).
	block := wsRowBySectionText(c, "writes", "~/.profile")
	require.GreaterOrEqual(t, block, 0)
	require.Contains(t, plainRowView(c, block), "✎")
}

func TestMarks_dirtyRowShowsTheDotNotTheMark(t *testing.T) {
	_, c := wsShell(t, map[string]string{"modules/demo/module.toml": wsAllSections})
	desc := wsRowBySectionText(c, "meta", "description")
	c.commitFieldAt(desc, "changed")

	line := plainRowView(c, wsRowBySectionText(c, "meta", "description"))
	require.Contains(t, line, "●", "the committed edit keeps its dot")
	require.NotContains(t, line, "✎", "the dot replaces the gesture mark")
}
