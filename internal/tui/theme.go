package tui

import (
	"image/color"

	"charm.land/bubbles/v2/tree"
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
	indigo, fuchsia, red, amber, muted, dim color.Color
}

// paletteFor resolves the shared hues for one background. Light variants
// are huh's light theme values, dark variants its dark theme values —
// the palette the chrome has used since ADR-0003.
func paletteFor(isDark bool) palette {
	ld := lipgloss.LightDark(isDark)
	return palette{
		indigo:  ld(lipgloss.Color("#5A56E0"), lipgloss.Color("#7571F9")),
		fuchsia: ld(lipgloss.Color("#D647A3"), lipgloss.Color("#F780E2")),
		red:     ld(lipgloss.Color("#FF4672"), lipgloss.Color("#ED567A")),
		amber:   ld(lipgloss.Color("#B45309"), lipgloss.Color("#F59E0B")),
		muted:   ld(lipgloss.Color("245"), lipgloss.Color("245")),
		dim:     ld(lipgloss.Color("248"), lipgloss.Color("240")),
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
	confirmLine   lipgloss.Style // quit-with-dirty confirmation
	applyBadge    lipgloss.Style // the ▶ apply header badge (M15)
	dimBase       lipgloss.Style // the base frame under a modal (M15)
	selection     lipgloss.Style // the cursor row (M15 nav/workspace)
	rowText       lipgloss.Style // plain row text (truncation carrier)
	modalBorder   lipgloss.Style // the confirm modal's frame (M15)
	modalTitle    lipgloss.Style // the confirm modal's question (M15)

	// Tree: groups, module labels, overlay badges, origin markers.
	groupTitle   lipgloss.Style // MODULES / ACCOUNTS / PROFILE
	originMark   lipgloss.Style // base/, hosts/<h>/, users/<u>/ children
	badge        lipgloss.Style // overlay-count badge
	disabledMark lipgloss.Style // (disabled) suffix
	reasonMark   lipgloss.Style // requires-root and other skip reasons

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
		confirmLine:   lipgloss.NewStyle().Bold(true).Foreground(p.amber),
		applyBadge:    lipgloss.NewStyle().Bold(true).Foreground(p.amber),
		dimBase:       lipgloss.NewStyle().Foreground(p.dim),
		selection:     lipgloss.NewStyle().Bold(true),
		rowText:       lipgloss.NewStyle(),
		modalBorder:   pane.BorderForeground(p.amber),
		modalTitle:    lipgloss.NewStyle().Bold(true).Foreground(p.amber),

		groupTitle:   lipgloss.NewStyle().Bold(true).Foreground(p.indigo),
		originMark:   lipgloss.NewStyle().Foreground(p.fuchsia),
		badge:        lipgloss.NewStyle().Foreground(p.muted),
		disabledMark: lipgloss.NewStyle().Foreground(p.dim),
		reasonMark:   lipgloss.NewStyle().Foreground(p.amber),

		viewTitle:    lipgloss.NewStyle().Bold(true),
		loading:      lipgloss.NewStyle().Foreground(p.muted),
		sectionLabel: lipgloss.NewStyle().Bold(true).Foreground(p.indigo),
		meta:         lipgloss.NewStyle().Foreground(p.muted),
		errorMark:    lipgloss.NewStyle().Bold(true).Foreground(p.red),
	}
}

// treeStyles maps the registry onto the bubbles tree (the registry→bubbles
// mechanism for this milestone): plain nodes, bold selection, dim
// enumerators.
func (t theme) treeStyles() tree.Styles {
	var s tree.Styles
	s.RootNodeStyle = t.groupTitle
	s.NodeStyle = lipgloss.NewStyle()
	s.SelectedNodeStyle = lipgloss.NewStyle().Bold(true)
	s.EnumeratorStyle = t.badge
	return s
}

// The editor.Palette implementation: the editor chrome styles through the
// same registry-backed theme (one color source, no second palette).

// Label renders a section label or highlighted chrome text.
func (t theme) Label(s string) string { return t.sectionLabel.Render(s) }

// Meta renders dim, parenthetical chrome text.
func (t theme) Meta(s string) string { return t.meta.Render(s) }

// Error renders an error line.
func (t theme) Error(s string) string { return t.errorMark.Render(s) }

// Mark renders dirty/selection markers.
func (t theme) Mark(s string) string { return t.dirtyMark.Render(s) }
