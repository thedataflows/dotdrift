package tui

import (
	"fmt"
	"io"

	"github.com/charmbracelet/lipgloss"
)

// warnf, errorf, and infof write one-line notices to w (the wizard's
// stderr). Each prefixes the message with a colored ASCII severity label
// ("warning:", "error:", "info:") so the body stays greppable while the
// severity reads at a glance. They replace five plain fmt.Fprintln(stderr,
// ...) sites in the wizard that previously carried no visual hierarchy.
//
// The format+args signature mirrors fmt.Fprintf so callers can keep their
// existing format strings verbatim.

func warnf(w io.Writer, format string, args ...any) {
	writeNotice(w, noticeWarnStyle, "warning", format, args...)
}

func errorf(w io.Writer, format string, args ...any) {
	writeNotice(w, noticeErrorStyle, "error", format, args...)
}

func infof(w io.Writer, format string, args ...any) {
	writeNotice(w, noticeInfoStyle, "info", format, args...)
}

func writeNotice(w io.Writer, style lipgloss.Style, label, format string, args ...any) {
	prefix := style.Render(label + ":")
	fmt.Fprintf(w, "%s %s\n", prefix, fmt.Sprintf(format, args...))
}
