package tui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// TUI style registry (ADR-0003), rebuilt on charm v2 for the shell
// (T-tui-shell). The hues are the wizard-era ThemeCharm palette — indigo
// for primary accents and active chrome, fuchsia for highlights, red for
// errors, amber for warnings — now resolved per background through
// LightDark(isDark) instead of AdaptiveColor (charm v2's replacement).
// Shell, tree, and view code pick styles from here; a new visible concept
// means a new registry entry, never an inline lipgloss.NewStyle chain.

type palette struct {
	indigo, fuchsia, red, amber, green, muted, dim, value, selectedBg color.Color
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
		green:   ld(lipgloss.Color("#16A34A"), lipgloss.Color("#3DF294")),
		muted:   ld(lipgloss.Color("245"), lipgloss.Color("245")),
		dim:     ld(lipgloss.Color("248"), lipgloss.Color("240")),
		value:   ld(lipgloss.Color("234"), lipgloss.Color("255")),
		// selectedBg (0113) is the filled-block background borrowed from
		// gh-dash: an indigo tint behind the active tab, the cursor row,
		// and detail header strips.
		selectedBg: ld(lipgloss.Color("#DDDBFF"), lipgloss.Color("#2E2B5C")),
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
	applyBadge    lipgloss.Style // the ▶ apply header badge (M15)
	dimBase       lipgloss.Style // the base frame under a modal (M15)
	cursorRow     lipgloss.Style // the cursor row: bar + accent text (0075)
	rowText       lipgloss.Style // value/content text (0080); the truncation carrier
	rowKey        lipgloss.Style // an entry row's key half — the indigo label hue (0113)
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

	// Visual system (0113, borrowed from gh-dash): filled tab blocks, a
	// footer bar, detail header strips, status pills, cards, and modal
	// severity frames. Labels use the indigo hue; values the bright
	// neutral — the two differ by hue, not by a shade of gray.
	tabActive        lipgloss.Style // the active view tab: filled indigo block
	tabInactive      lipgloss.Style // an inactive view tab: flat faint text
	staleBadge       lipgloss.Style // the amber ● on a stale view's tab
	successMark      lipgloss.Style // green ✓ / ok text (the first success role)
	footerBar        lipgloss.Style // the bottom bar background
	detailHeader     lipgloss.Style // the filled strip atop a detail pane
	sectionBar       lipgloss.Style // a workspace section heading bar
	pillOK           lipgloss.Style // filled status pill: ok
	pillDrift        lipgloss.Style // filled status pill: drift / unauthorized
	pillUnknown      lipgloss.Style // filled status pill: unknown
	pillUnauthorized lipgloss.Style // filled status pill: unauthorized
	cardBorder       lipgloss.Style // a titled card frame (Profile groups)
	modalDanger      lipgloss.Style // destructive confirm frame: red
	modalInfo        lipgloss.Style // information frame (help, palette): indigo
	helpKey          lipgloss.Style // a key chip in the footer bar
	helpDesc         lipgloss.Style // a help description in the footer bar
	tabBar           lipgloss.Style // the tab bar's bottom rule (0114)
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
		applyBadge:    lipgloss.NewStyle().Bold(true).Foreground(p.amber),
		dimBase:       lipgloss.NewStyle().Foreground(p.dim),
		cursorRow: lipgloss.NewStyle().
			Bold(true).Foreground(p.value).Background(p.selectedBg).
			Border(lipgloss.NormalBorder(), false, false, false, true).
			BorderForeground(p.fuchsia), // filled block, neutral text, fuchsia bar only (0114)
		rowText:     lipgloss.NewStyle().Foreground(p.value),  // values read as content (0080)
		rowKey:      lipgloss.NewStyle().Foreground(p.indigo), // labels differ from values by hue (0113)
		caret:       lipgloss.NewStyle().Reverse(true),        // the block caret: the cell itself, no glyph (0092)
		fieldLabel:  lipgloss.NewStyle().Foreground(p.indigo), // form labels share the label hue (0113)
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

		tabActive: lipgloss.NewStyle().Bold(true).Padding(0, 1).
			Foreground(lipgloss.Color("#FFFFFF")).Background(p.indigo),
		tabInactive: lipgloss.NewStyle().Padding(0, 1).Foreground(p.muted),
		staleBadge:  lipgloss.NewStyle().Bold(true).Foreground(p.amber),
		successMark: lipgloss.NewStyle().Bold(true).Foreground(p.green),
		footerBar:   lipgloss.NewStyle().Background(p.selectedBg),
		detailHeader: lipgloss.NewStyle().Bold(true).Padding(0, 1).
			Foreground(p.value).Background(p.selectedBg),
		sectionBar: lipgloss.NewStyle().Bold(true).Padding(0, 1).
			Foreground(p.indigo).Background(p.selectedBg),
		pillOK: lipgloss.NewStyle().Padding(0, 1).
			Foreground(lipgloss.Color("#FFFFFF")).Background(p.green),
		pillDrift: lipgloss.NewStyle().Padding(0, 1).
			Foreground(lipgloss.Color("#FFFFFF")).Background(p.red),
		pillUnknown: lipgloss.NewStyle().Padding(0, 1).
			Foreground(lipgloss.Color("#000000")).Background(p.amber),
		pillUnauthorized: lipgloss.NewStyle().Padding(0, 1).
			Foreground(lipgloss.Color("#FFFFFF")).Background(p.fuchsia),
		cardBorder:  pane.BorderForeground(p.dim).Padding(0, 1),
		modalDanger: pane.BorderForeground(p.red),
		modalInfo:   pane.BorderForeground(p.indigo),
		helpKey:     lipgloss.NewStyle().Bold(true).Foreground(p.value).Background(p.selectedBg),
		helpDesc:    lipgloss.NewStyle().Foreground(p.muted).Background(p.selectedBg),
		tabBar: lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), false, false, true, false).
			BorderForeground(p.dim),
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

// fillLine renders s in style and fills the rest of the width with
// plain background cells (0113, 0114): the styled run first, then the
// bar background to the edge — a style Width would wrap long rows
// instead of filling. All filled styles share the selectedBg fill.
func (t theme) fillLine(style lipgloss.Style, s string, w int) string {
	line := style.MaxWidth(w).Render(s)
	if pad := w - ansi.StringWidth(line); pad > 0 {
		line += t.footerBar.Render(strings.Repeat(" ", pad))
	}
	return line
}

// cursorLine renders the cursor row as a filled block across the pane.
func (t theme) cursorLine(s string, w int) string { return t.fillLine(t.cursorRow, s, w) }

// barLine renders a workspace section heading bar across the pane.
func (t theme) barLine(s string, w int) string { return t.fillLine(t.sectionBar, s, w) }

// detailLine renders a detail pane's header strip across the pane.
func (t theme) detailLine(s string, w int) string { return t.fillLine(t.detailHeader, s, w) }

// Meta renders dim, parenthetical chrome text.
func (t theme) Meta(s string) string { return t.meta.Render(s) }

// Error renders an error line.
func (t theme) Error(s string) string { return t.errorMark.Render(s) }

// Mark renders dirty/selection markers.
func (t theme) Mark(s string) string { return t.dirtyMark.Render(s) }
