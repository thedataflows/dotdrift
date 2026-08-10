package main

import (
	"fmt"
	"os"

	"github.com/charmbracelet/lipgloss"

	cmd "github.com/thedataflows/dotdrift/cmd"
)

var version = "dev"

// errorStyle renders against stderr's terminal profile so the fatal error is
// colored only when stderr is a terminal — piped/redirected output stays
// plain. Created inside main() so the NO_COLOR env var (set by --no-color in
// AfterApply) is already active when lipgloss profiles the writer.
func main() {
	if err := cmd.Run(version, os.Args[1:]); err != nil {
		style := lipgloss.NewRenderer(os.Stderr).NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("1"))
		fmt.Fprintln(os.Stderr, style.Render(err.Error()))
		os.Exit(1)
	}
}
