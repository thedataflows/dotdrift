package drift

import (
	"strings"

	"github.com/pmezard/go-difflib/difflib"
)

// ansiCyan for diff hunk headers; ansiGreen/ansiRed/ansiReset already exist
// in this package (drift.go).
const ansiCyan = "\033[36m"

// UnifiedDiff computes a unified diff between current (live) and desired
// (profile) file content. Returns empty string when content is identical.
// targetName/sourceName label the --- and +++ header lines.
func UnifiedDiff(targetName, targetContent, sourceName, sourceContent string) string {
	if targetContent == sourceContent {
		return ""
	}
	diff := difflib.UnifiedDiff{
		A:        difflib.SplitLines(targetContent),
		FromFile: targetName,
		B:        difflib.SplitLines(sourceContent),
		ToFile:   sourceName,
		Context:  3,
	}
	out, err := difflib.GetUnifiedDiffString(diff)
	if err != nil {
		return ""
	}
	return out
}

// ColorDiff adds ANSI colors to unified diff output. When color is false the
// input is returned unchanged. + lines (not +++) get green, - lines (not ---)
// get red, @@ hunk headers get cyan.
func ColorDiff(diff string, color bool) string {
	if !color || diff == "" {
		return diff
	}
	lines := strings.Split(diff, "\n")
	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			lines[i] = ansiGreen + line + ansiReset
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			lines[i] = ansiRed + line + ansiReset
		case strings.HasPrefix(line, "@@"):
			lines[i] = ansiCyan + line + ansiReset
		}
	}
	return strings.Join(lines, "\n")
}
