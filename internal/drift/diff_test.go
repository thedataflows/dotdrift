package drift

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUnifiedDiff_contentDiffers(t *testing.T) {
	old := "theme = \"dark\"\nfont = \"mono\"\n"
	new := "theme = \"light\"\nfont = \"mono\"\n"
	out := UnifiedDiff("/etc/conf", old, "profile/conf", new)
	require.NotEmpty(t, out)
	require.Contains(t, out, "--- /etc/conf")
	require.Contains(t, out, "+++ profile/conf")
	require.Contains(t, out, "@@")
	require.Contains(t, out, "-theme = \"dark\"")
	require.Contains(t, out, "+theme = \"light\"")
	// context line (unchanged) present with a leading space, not + or -
	require.Contains(t, out, " font = \"mono\"")
}

func TestUnifiedDiff_identical(t *testing.T) {
	content := "same\ncontent\n"
	require.Empty(t, UnifiedDiff("/a", content, "/b", content))
}

func TestColorDiff_appliesColors(t *testing.T) {
	diff := "--- /old\n+++ /new\n@@ -1,2 +1,2 @@\n-old line\n context\n+new line\n"

	colored := ColorDiff(diff, true)
	require.Contains(t, colored, ansiGreen+"+new line"+ansiReset)
	require.Contains(t, colored, ansiRed+"-old line"+ansiReset)
	require.Contains(t, colored, ansiCyan+"@@ -1,2 +1,2 @@"+ansiReset)
	// Headers (---/+++) and context lines are NOT colored
	require.NotContains(t, colored, ansiRed+"---")
	require.NotContains(t, colored, ansiGreen+"+++")

	// No color → input unchanged
	require.Equal(t, diff, ColorDiff(diff, false))
}

func TestColorDiff_empty(t *testing.T) {
	require.Empty(t, ColorDiff("", true))
	require.Empty(t, ColorDiff("", false))
}
