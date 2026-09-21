package main

import (
	"errors"
	"os"

	"charm.land/lipgloss/v2"

	cmd "github.com/thedataflows/dotdrift/cmd"
	"github.com/thedataflows/dotdrift/internal/service"
)

var version = "dev"

// errorStyle renders against stderr's terminal profile so the fatal error is
// colored only when stderr is a terminal — piped/redirected output stays
// plain. Fprintln wraps stderr in a colorprofile writer, which reads the
// environment (NO_COLOR, set by --no-color in AfterApply) at print time.
func main() {
	if err := cmd.Run(version, os.Args[1:]); err != nil {
		// Structured failures surface through the service error taxonomy
		// (0061-D3): match with errors.As. SchemaError renders through its
		// Error() (the original string), so CLI output is byte-identical
		// while the TUI reads Path/Line instead of parsing text.
		var schemaErr *service.SchemaError
		if errors.As(err, &schemaErr) {
			err = schemaErr
		}
		style := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("1"))
		lipgloss.Fprintln(os.Stderr, style.Render(err.Error()))
		os.Exit(1)
	}
}
