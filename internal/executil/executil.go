// Package executil holds tiny helpers shared by the exec runners.
package executil

import (
	"fmt"
	"io"
	"strings"
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
// single-quoted (each ' escaped as '\''), bash set -x style. Simple elements
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
