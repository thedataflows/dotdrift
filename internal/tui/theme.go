package tui

import "github.com/charmbracelet/lipgloss"

// TUI palette and style registry. Sourced from / shared with huh's
// ThemeCharm (the wizard's default form theme, vendor/.../huh/theme.go) so
// the chrome — tab bar, review screens, stderr notices — matches the form
// rendered directly below it instead of clashing with a second palette.
//
// Color names mirror huh's: indigo for primary accents and active chrome,
// fuchsia for highlights, red for errors, amber for warnings. All colors
// are lipgloss.AdaptiveColor so a light-background terminal picks the
// readable variant.
var (
	indigo  = lipgloss.AdaptiveColor{Light: "#5A56E0", Dark: "#7571F9"}
	fuchsia = lipgloss.AdaptiveColor{Light: "#D647A3", Dark: "#F780E2"}
	red     = lipgloss.AdaptiveColor{Light: "#FF4672", Dark: "#ED567A"}
	amber   = lipgloss.AdaptiveColor{Light: "#B45309", Dark: "#F59E0B"}
	muted   = lipgloss.AdaptiveColor{Light: "245", Dark: "245"}
	dim     = lipgloss.AdaptiveColor{Light: "248", Dark: "240"}
)

// Style registry. Each concept used in the wizard has exactly one style;
// every call site picks from this list rather than spelling its own colors.
// Adding a new visible element means adding a style here, not a new lipgloss
// .NewStyle() chain in a wizard file.
var (
	// activeTab is the selected "mounts" or "smb" tab. Bold indigo text on
	// an indigo-tinted background mirrors huh's focused-field treatment.
	activeTabStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.AdaptiveColor{Light: "#FFFDF5", Dark: "#FFFDF5"}).
			Background(indigo).
			Padding(0, 2)

	// inactiveTab is a non-selected tab. Muted foreground, no background —
	// the active tab stands out by contrast rather than by weight.
	inactiveTabStyle = lipgloss.NewStyle().
				Foreground(muted).
				Padding(0, 2)

	// quitTab is the destructively-toned "quit" affordance. Dimmer than
	// inactive so the eye reads past it.
	quitTabStyle = lipgloss.NewStyle().
			Foreground(dim).
			Padding(0, 2)

	// label renders the key half of a "key: value" review line. Bold indigo
	// keeps it readable in a huh.NewNote body, which is otherwise plain.
	labelStyle = lipgloss.NewStyle().Bold(true).Foreground(indigo)

	// valueStyle renders the value half of a review line. Default
	// foreground; muted when the value is "(registry preset)" or similar.
	valueStyle     = lipgloss.NewStyle()
	valueMutedStyle = lipgloss.NewStyle().Foreground(muted)

	// noticeWarn / noticeError / noticeInfo style the ASCII prefix of
	// one-line wizard stderr notices. The prefix carries the color so the
	// message body stays readable on any background.
	noticeWarnStyle  = lipgloss.NewStyle().Foreground(amber).Bold(true)
	noticeErrorStyle = lipgloss.NewStyle().Foreground(red).Bold(true)
	noticeInfoStyle  = lipgloss.NewStyle().Foreground(fuchsia).Bold(true)
)
