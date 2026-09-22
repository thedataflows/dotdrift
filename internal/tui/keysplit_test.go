package tui

// 0103: a workspace key/value row shades its key half darker than the
// value (the `rowKey` registry entry), so the field's name recedes from
// its content. Every key/value section splits; single-shade rows
// (packages, containers, headers, hints) stay flat; the cursor row
// keeps the flat accent — selection already owns its color.

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// rowViewANSI renders one row with ANSI kept, at a generous width so
// MaxWidth never truncates the assertions' runs.
func rowViewANSI(c *Compositor, i int) string {
	return strings.Join(c.ws.rowView(c.ws.rows[i], i, c.th, 200), "\n")
}

func TestKeySplit_entryRowsShadeTheKey(t *testing.T) {
	_, c := wsShell(t, map[string]string{"modules/demo/module.toml": wsAllSections})
	c.ws.cursor = -1 // render every row plain; the cursor row never splits

	cases := []struct {
		section string
		prefix  string // the row's text prefix, for wsRowBySectionText
		keyHalf string // the exact key half, separator included
		value   string // the exact value half (leading separator space included)
	}{
		{"meta", "id demo", "id ", "demo"},
		{"meta", "description", "description ", "the demo module"},
		{"meta", "scope", "scope ", "user"},
		{"links", "~/.bashrc", "~/.bashrc", " ← .bashrc (symlink)"},
		{"writes", "~/.config/demo.toml", "~/.config/demo.toml", " (edit: line)"},
		{"writes", "~/.profile", "~/.profile", " (edit: block)"},
		{"when", "os linux", "os ", "linux"},
		{"when", "hosts myhost", "hosts ", "myhost"},
		{"hooks", "pre: echo pre", "pre: ", "echo pre"},
		{"systemd.units", "  Service", "  Service = ", `{ ExecStart = "/usr/bin/demo" }`},
		{"tools", "node = 20", "node = ", "20"},
		{"secrets", "  env DEMO_API_KEY", "  env ", "DEMO_API_KEY"},
		{"secrets", "  description demo key", "  description ", "demo key"},
	}
	for _, tc := range cases {
		i := wsRowBySectionText(c, tc.section, tc.prefix)
		require.GreaterOrEqual(t, i, 0, "row %q in %q exists", tc.prefix, tc.section)
		out := rowViewANSI(c, i)
		require.Contains(t, out, c.th.rowKey.Render(tc.keyHalf),
			"%s %q: the key half renders in the key shade", tc.section, tc.prefix)
		require.Contains(t, out, c.th.rowText.Render(tc.value),
			"%s %q: the value renders in the content hue", tc.section, tc.prefix)
	}
}

func TestKeySplit_singleShadeRowsStayFlat(t *testing.T) {
	_, c := wsShell(t, map[string]string{"modules/demo/module.toml": wsAllSections})
	c.ws.cursor = -1

	// The rowKey run's opening sequence, probed from the style itself so
	// the assertion tracks the registry instead of hardcoding ANSI.
	keyOpen := strings.SplitN(c.th.rowKey.Render("§"), "§", 2)[0]

	// A package row is all value — no key half to recede.
	pkg := wsRowBySectionText(c, "packages", "+ neovim")
	require.GreaterOrEqual(t, pkg, 0)
	require.NotContains(t, rowViewANSI(c, pkg), keyOpen, "a package row has no key half")

	// A container row is a name, not a key/value pair.
	unit := wsRowBySectionText(c, "systemd.units", "demo.service")
	require.GreaterOrEqual(t, unit, 0)
	require.NotContains(t, rowViewANSI(c, unit), keyOpen, "a container row has no key half")

	// A bare flag row (no value) stays flat too.
	_, c2 := wsShell(t, map[string]string{"modules/demo/module.toml": "id = \"demo\"\ndisabled = true\n"})
	c2.ws.cursor = -1
	disabled := wsRowBySectionText(c2, "meta", "disabled")
	require.GreaterOrEqual(t, disabled, 0)
	require.NotContains(t, rowViewANSI(c2, disabled), keyOpen, "a valueless flag row has no key half")
}

func TestKeySplit_cursorRowKeepsTheAccent(t *testing.T) {
	_, c := wsShell(t, map[string]string{"modules/demo/module.toml": wsAllSections})

	i := wsRowBySectionText(c, "meta", "description")
	require.GreaterOrEqual(t, i, 0)
	c.ws.cursor = i
	out := rowViewANSI(c, i)
	require.NotContains(t, out, c.th.rowKey.Render("description "),
		"selection owns the row's color — the cursor row does not split")
	require.Contains(t, out, "description the demo module",
		"the row's text renders contiguously under the one cursor treatment")
}
