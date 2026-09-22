package tui

import (
	"os"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/require"
) // ADR-0003 discipline for the shell: every style lives in the registry
// (theme.go); shell, tree, and view files pick from it and never spell
// their own lipgloss.NewStyle chains.
func TestPaletteRegistry_noInlineStyles(t *testing.T) {
	entries, err := os.ReadDir(".")
	require.NoError(t, err)
	var offenders []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || name == "theme.go" {
			continue
		}
		src, err := os.ReadFile(name)
		require.NoError(t, err)
		if strings.Contains(string(src), "lipgloss.NewStyle(") {
			offenders = append(offenders, name)
		}
	}
	require.Empty(t, offenders, "styles must be registered in theme.go, not inline: %v", offenders)
}

// The registry stays the wizard-era adaptive palette (huh's ThemeCharm
// hues, ADR-0003) — resolved per background through charm v2's
// LightDark(isDark), never static ANSI (the prototype's recorded
// simplification must not leak into the real shell, issue 0063).
func TestTheme_lightDarkPalette(t *testing.T) {
	dark, light := paletteFor(true), paletteFor(false)

	require.Equal(t, lipgloss.Color("#7571F9"), dark.indigo, "dark indigo = huh's dark variant")
	require.Equal(t, lipgloss.Color("#5A56E0"), light.indigo, "light indigo = huh's light variant")
	require.Equal(t, lipgloss.Color("#F780E2"), dark.fuchsia)
	require.Equal(t, lipgloss.Color("#D647A3"), light.fuchsia)
	require.Equal(t, lipgloss.Color("#ED567A"), dark.red)
	require.Equal(t, lipgloss.Color("#FF4672"), light.red)
	require.Equal(t, lipgloss.Color("#F59E0B"), dark.amber)
	require.Equal(t, lipgloss.Color("#B45309"), light.amber)
	require.NotEqual(t, dark.dim, light.dim, "dim differs per background")
	require.Equal(t, dark.muted, light.muted, "muted stays ANSI 245 on both backgrounds (the v1 registry's value)")
	require.Equal(t, lipgloss.Color("255"), dark.value, "dark value = bright neutral, the brightest chrome")
	require.Equal(t, lipgloss.Color("234"), light.value, "light value = near-black neutral")
}

// 0080 T-tui-value-color: values read as content and fixed labels recede.
// The distinction is visible in both styles and the value hue is not one
// of the accent hues (indigo/fuchsia/red/amber keep their semantics).
func TestTheme_valueVsLabel(t *testing.T) {
	for _, isDark := range []bool{true, false} {
		th := newTheme(isDark)
		p := paletteFor(isDark)
		require.NotEqual(t, th.fieldLabel.Render("x"), th.rowText.Render("x"),
			"labels and values are visibly different colors")
		require.NotEqual(t, "x", th.rowText.Render("x"), "the value style is visible")
		require.NotEqual(t, p.value, p.muted, "value differs from the label hue")
		require.NotEqual(t, p.value, p.dim, "value differs from the hint hue")
	}
}

// One style per visible shell concept; focus IS the border color
// (approved prototype, issue 0063), so focused and unfocused pane borders
// must differ visibly.
func TestTheme_styles(t *testing.T) {
	th := newTheme(true)
	focused := th.focusedBorder.Render("x")
	unfocused := th.unfocusedBorder.Render("x")
	require.NotEqual(t, unfocused, focused, "focus reads as a border color change")
}

// 0075 T-tui-selection: the cursor row uses the bubbles list-delegate
// treatment — a left bar in the accent hue plus bold text — and plain
// rows keep a two-space lead so text columns align across the two.
func TestTheme_cursorRow(t *testing.T) {
	th := newTheme(true)
	strip := func(s string) string { return ansiRe.ReplaceAllString(s, "") }
	cursor := strip(th.cursorRow.Render(" demo"))
	plain := strip(th.rowText.Render("  demo"))
	require.Equal(t, "│ demo", cursor, "the cursor row renders the bar")
	require.Equal(t, "  demo", plain, "plain text aligns with the cursor row")
	require.NotEqual(t, th.cursorRow.Render("x"), th.rowText.Render(" x"), "the cursor style is visible")
}

// 0103: an entry row's key half recedes one step toward the label hues
// while the value keeps the bright content hue — close enough to read as
// the same text, far enough to tell the field's name from its content.
// The key hue is its own registry entry, distinct from the label and
// hint hues on both backgrounds.
func TestTheme_rowKey(t *testing.T) {
	require.Equal(t, lipgloss.Color("250"), paletteFor(true).key, "dark key sits one step under the 255 value")
	require.Equal(t, lipgloss.Color("238"), paletteFor(false).key, "light key sits one step over the 234 value")
	for _, isDark := range []bool{true, false} {
		th := newTheme(isDark)
		p := paletteFor(isDark)
		require.NotEqual(t, "x", th.rowKey.Render("x"), "the key style is visible")
		require.NotEqual(t, th.rowKey.Render("x"), th.rowText.Render("x"),
			"key and value are visibly different shades")
		require.NotEqual(t, th.rowKey.Render("x"), th.fieldLabel.Render("x"),
			"the key shade is not the label hue")
		require.NotEqual(t, p.key, p.value)
		require.NotEqual(t, p.key, p.muted, "the key shade is not the label hue")
		require.NotEqual(t, p.key, p.dim, "the key shade is not the hint hue")
	}
}

// 0102: the caret renders nested inside styled rows, so it must release
// only its reverse attribute — a full reset would erase the enclosing
// row's color for the rest of the line.
func TestTheme_caretCell(t *testing.T) {
	th := newTheme(true)
	cell := th.caretCell("x")
	require.Contains(t, cell, "\x1b[7m", "the caret cell turns reverse video on")
	require.Contains(t, cell, "\x1b[27m", "the caret cell turns only reverse video off")
	require.NotContains(t, cell, "\x1b[0m", "no full reset")
	require.NotContains(t, cell, "\x1b[m", "no full reset")
}
