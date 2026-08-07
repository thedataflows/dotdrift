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
// plain. lipgloss is the codebase's styling convention (see internal/tui).
var errorStyle = lipgloss.NewRenderer(os.Stderr).NewStyle().
	Bold(true).
	Foreground(lipgloss.Color("1"))

func main() {
	if err := cmd.Run(version, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, errorStyle.Render(err.Error()))
		os.Exit(1)
	}
}
