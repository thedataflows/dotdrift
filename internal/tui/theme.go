package tui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// TUI style registry (ADR-0003), rebuilt on charm v2 for the shell
// (T-tui-shell). The hues are the wizard-era ThemeCharm palette — indigo
// for primary accents and active chrome, fuchsia for highlights, red for
// errors, amber for warnings — now resolved per background through
// LightDark(isDark) instead of AdaptiveColor (charm v2's replacement).
// Shell, tree, and view code pick styles from here; a new visible concept
// means a new registry entry, never an inline lipgloss.NewStyle chain.

type palette struct {
	indigo, fuchsia, red, amber, muted, dim, value, key color.Color
}

// paletteFor resolves the shared hues for one background. Light variants
// are huh's light theme values, dark variants its dark theme values —
// the palette the chrome has used since ADR-0003. The value hue (0080)
// is a bright neutral, not an accent: content is the brightest thing on
// screen, so labels and chrome recede around it.
func paletteFor(isDark bool) palette {
	ld := lipgloss.LightDark(isDark)
	return palette{
		indigo:  ld(lipgloss.Color("#5A56E0"), lipgloss.Color("#7571F9")),
		fuchsia: ld(lipgloss.Color("#D647A3"), lipgloss.Color("#F780E2")),
		red:     ld(lipgloss.Color("#FF4672"), lipgloss.Color("#ED567A")),
		amber:   ld(lipgloss.Color("#B45309"), lipgloss.Color("#F59E0B")),
		muted:   ld(lipgloss.Color("245"), lipgloss.Color("245")),
		dim:     ld(lipgloss.Color("248"), lipgloss.Color("240")),
		value:   ld(lipgloss.Color("234"), lipgloss.Color("255")),
		// The key hue (0103) sits one step between value and muted: an
		// entry row's key half recedes from its content without falling
		// all the way to the label/hint shades.
		key: ld(lipgloss.Color("238"), lipgloss.Color("250")),
	}
}

// theme is the resolved style registry for one background: one style per
// visible shell concept. Rebuilt when bubbletea reports the terminal
// background (BackgroundColorMsg).
type theme struct {
	// Focus is the border color (0063-approved treatment): indigo for the
	// focused pane, dim for the unfocused one.
	focusedBorder   lipgloss.Style
	unfocusedBorder lipgloss.Style

	// Chrome: the one-line header (profile root, host/user context, dirty
	// indicator) and the status bar.
	headerTitle   lipgloss.Style // "dotdrift tui"
	headerContext lipgloss.Style // host/user context
	dirtyMark     lipgloss.Style // the ● unsaved indicator
	statusBar     lipgloss.Style // help hints line
	applyBadge    lipgloss.Style // the ▶ apply header badge (M15)
	dimBase       lipgloss.Style // the base frame under a modal (M15)
	cursorRow     lipgloss.Style // the cursor row: bar + accent text (0075)
	rowText       lipgloss.Style // value/content text (0080); the truncation carrier
	rowKey        lipgloss.Style // an entry row's key half — one shade under the value (0103)
	caret         lipgloss.Style // the block caret: reverse video on the cell under it (0092)
	fieldLabel    lipgloss.Style // a dialog/form row's fixed label (0080)
	modalBorder   lipgloss.Style // the confirm modal's frame (M15)
	modalTitle    lipgloss.Style // the confirm modal's question (M15)

	// Tree: groups, module labels, overlay badges, origin markers.
	groupTitle   lipgloss.Style // MODULES / ACCOUNTS / PROFILE
	disabledMark lipgloss.Style // (disabled) suffix
	reasonMark   lipgloss.Style // requires-root and other skip reasons
	yankMark     lipgloss.Style // the ✓ yanked-module mark (0093)

	// Main pane: view titles and loading/placeholder bodies.
	viewTitle    lipgloss.Style // "MODULE shell", "STATUS", …
	loading      lipgloss.Style // resolving/probing placeholder text
	sectionLabel lipgloss.Style // the key half of "packages  …" lines
	meta         lipgloss.Style // parenthetical context under titles
	errorMark    lipgloss.Style // load errors surfaced in a view
}

func newTheme(isDark bool) theme {
	p := paletteFor(isDark)
	pane := lipgloss.NewStyle().Border(lipgloss.RoundedBorder())
	return theme{
		focusedBorder:   pane.BorderForeground(p.indigo),
		unfocusedBorder: pane.BorderForeground(p.dim),

		headerTitle:   lipgloss.NewStyle().Bold(true),
		headerContext: lipgloss.NewStyle().Foreground(p.muted),
		dirtyMark:     lipgloss.NewStyle().Bold(true).Foreground(p.fuchsia),
		statusBar:     lipgloss.NewStyle().Foreground(p.dim),
		applyBadge:    lipgloss.NewStyle().Bold(true).Foreground(p.amber),
		dimBase:       lipgloss.NewStyle().Foreground(p.dim),
		cursorRow: lipgloss.NewStyle().
			Bold(true).Foreground(p.fuchsia).
			Border(lipgloss.NormalBorder(), false, false, false, true).
			BorderForeground(p.fuchsia), // the bubbles delegate treatment
		rowText:     lipgloss.NewStyle().Foreground(p.value), // values read as content (0080)
		rowKey:      lipgloss.NewStyle().Foreground(p.key),   // the key half recedes one shade (0103)
		caret:       lipgloss.NewStyle().Reverse(true),       // the block caret: the cell itself, no glyph (0092)
		fieldLabel:  lipgloss.NewStyle().Foreground(p.muted), // fixed labels recede (0080)
		modalBorder: pane.BorderForeground(p.amber),
		modalTitle:  lipgloss.NewStyle().Bold(true).Foreground(p.amber),

		groupTitle:   lipgloss.NewStyle().Bold(true).Foreground(p.indigo),
		disabledMark: lipgloss.NewStyle().Foreground(p.dim),
		reasonMark:   lipgloss.NewStyle().Foreground(p.amber),
		yankMark:     lipgloss.NewStyle().Bold(true).Foreground(p.indigo),

		viewTitle:    lipgloss.NewStyle().Bold(true),
		loading:      lipgloss.NewStyle().Foreground(p.muted),
		sectionLabel: lipgloss.NewStyle().Bold(true).Foreground(p.indigo),
		meta:         lipgloss.NewStyle().Foreground(p.muted),
		errorMark:    lipgloss.NewStyle().Bold(true).Foreground(p.red),
	}
}

// The editor.Palette implementation: the editor chrome styles through the
// same registry-backed theme (one color source, no second palette).

// Label renders a section label or highlighted chrome text.
func (t theme) Label(s string) string { return t.sectionLabel.Render(s) }

// caretCell renders the 0092 block caret so it can nest inside a styled
// row: lipgloss closes a styled run with a full reset, which erases the
// enclosing row's color for the rest of the line (0102 — the text lost
// its accent when the caret moved left into it). The caret instead
// releases only its reverse attribute; the row's own style carries on.
func (t theme) caretCell(s string) string {
	cell := t.caret.Render(s)
	for _, reset := range []string{"\x1b[m", "\x1b[0m"} {
		if i := strings.LastIndex(cell, reset); i >= 0 {
			return cell[:i] + "\x1b[27m" + cell[i+len(reset):]
		}
	}
	return cell
}

// Meta renders dim, parenthetical chrome text.
func (t theme) Meta(s string) string { return t.meta.Render(s) }

// Error renders an error line.
func (t theme) Error(s string) string { return t.errorMark.Render(s) }

// Mark renders dirty/selection markers.
func (t theme) Mark(s string) string { return t.dirtyMark.Render(s) }
