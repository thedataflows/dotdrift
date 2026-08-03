package tui

import (
	"fmt"
	"io"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// Tab-bar chrome for the wizard: the two wizard flows are the tabs; the
// active one is rendered via the package-level styles in theme.go (which
// share huh's Charm palette) before the huh form runs.

// renderTabBar writes the "mounts | smb" tab bar to w with the active tab
// highlighted, then a blank line so the huh form below has breathing room.
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

// selectTab runs the top-level tab switcher: mounts | smb | quit, with the
// invoking flow preselected.
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
