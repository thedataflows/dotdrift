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
