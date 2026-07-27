package tui

import (
	"fmt"
	"io"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// Minimal tab chrome: the two wizard flows are the tabs; the active one
// is rendered with lipgloss before the huh form runs.
var (
	activeTabStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("62")).
			Padding(0, 2)
	inactiveTabStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("245")).
				Padding(0, 2)
	quitTabStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Padding(0, 2)
)

// renderTabBar writes the "mounts | smb" tab bar to w with the active
// tab highlighted.
func renderTabBar(w io.Writer, active string) {
	tabs := []string{"mounts", "smb"}
	parts := make([]string, 0, len(tabs)+1)
	for _, t := range tabs {
		if t == active {
			parts = append(parts, activeTabStyle.Render(t))
		} else {
			parts = append(parts, inactiveTabStyle.Render(t))
		}
	}
	parts = append(parts, quitTabStyle.Render("quit"))
	fmt.Fprintln(w, lipgloss.JoinHorizontal(lipgloss.Top, parts...))
	fmt.Fprintln(w)
}

// selectTab runs the top-level tab switcher: mounts | smb | quit, with
// the invoking flow preselected.
func selectTab(active string) (string, error) {
	choice := active
	form := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("Generate a module").
			Options(
				huh.NewOption("mounts — systemd mount units", "mounts"),
				huh.NewOption("smb — samba shares", "smb"),
				huh.NewOption("quit", "quit"),
			).
			Value(&choice),
	))
	if err := form.Run(); err != nil {
		return "", err
	}
	return choice, nil
}
