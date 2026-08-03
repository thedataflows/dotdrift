// Package executil holds tiny helpers shared by the exec runners.
package executil

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"unicode"
)

// needsQuoting reports whether an argv element must be single-quoted for a
// set -x-style echo: anything containing whitespace or a single quote.
func needsQuoting(arg string) bool {
	if strings.Contains(arg, "'") {
		return true
	}
	return strings.IndexFunc(arg, unicode.IsSpace) >= 0
}

// ShellJoin renders argv as a shell-style command line: elements are joined
// with single spaces; any element containing whitespace or a single quote is
// single-quoted (each ' escaped as '\”), bash set -x style. Simple elements
// pass through unquoted. Empty argv joins to the empty string.
func ShellJoin(argv []string) string {
	quoted := make([]string, len(argv))
	for i, arg := range argv {
		if needsQuoting(arg) {
			arg = "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
		}
		quoted[i] = arg
	}
	return strings.Join(quoted, " ")
}

// EchoCommand writes the set -x-style echo line "+ <argv>\n" to w.
func EchoCommand(w io.Writer, argv []string) {
	fmt.Fprintf(w, "+ %s\n", ShellJoin(argv))
}

// LockedWriter serializes concurrent writes to W. os/exec spawns one copy
// goroutine per stream when Stdout and Stderr are different writers, so two
// MultiWriters capturing into one bytes.Buffer race (and silently lose
// writes) without it.
type LockedWriter struct {
	mu sync.Mutex
	W  io.Writer
}

func (l *LockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.W.Write(p)
}

// IsStdinTerminal reports whether stdin is connected to a terminal. dotdrift
// uses it to decide whether a generated mise hook task may opt into
// interactive mode (interactive = true): when stdin is a TTY, an interactive
// command inside a hook (e.g. sudo) gets a controlling terminal so it can
// disable echo, instead of reading the password with echo on. Stdlib-only
// char-device check (golang.org/x/term is not vendored). A variable so tests
// and callers can substitute it.
var IsStdinTerminal = func() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
