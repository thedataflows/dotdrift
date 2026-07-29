package executil_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/executil"
)

// bash set -x style: simple argv elements pass through unquoted, joined
// with single spaces.
func TestShellJoin_plainArgsUnquoted(t *testing.T) {
	require.Equal(t, "paru -S --needed --noconfirm jq",
		executil.ShellJoin([]string{"paru", "-S", "--needed", "--noconfirm", "jq"}))
}

// An element containing whitespace is single-quoted.
func TestShellJoin_argWithSpacesSingleQuoted(t *testing.T) {
	require.Equal(t, "sh -c 'echo hi there'",
		executil.ShellJoin([]string{"sh", "-c", "echo hi there"}))
}

// Any whitespace (tab here) triggers quoting, not just spaces.
func TestShellJoin_tabTriggersQuoting(t *testing.T) {
	require.Equal(t, "sh -c 'echo hi\tthere'",
		executil.ShellJoin([]string{"sh", "-c", "echo hi\tthere"}))
}

// An embedded single quote is escaped '\'' style, even without whitespace.
func TestShellJoin_singleQuoteEscaped(t *testing.T) {
	require.Equal(t, `echo 'it'\''s'`,
		executil.ShellJoin([]string{"echo", "it's"}))
}

// Whitespace and a quote together: quoted once, quote escaped.
func TestShellJoin_spaceAndQuote(t *testing.T) {
	require.Equal(t, `sh -c 'echo it'\''s done'`,
		executil.ShellJoin([]string{"sh", "-c", "echo it's done"}))
}

// Empty argv joins to the empty string.
func TestShellJoin_emptyArgv(t *testing.T) {
	require.Equal(t, "", executil.ShellJoin(nil))
	require.Equal(t, "", executil.ShellJoin([]string{}))
}

// EchoCommand writes exactly "+ <argv>\n" — the bash set -x line.
func TestEchoCommand_setXStyleLine(t *testing.T) {
	var buf bytes.Buffer
	executil.EchoCommand(&buf, []string{"sudo", "-E", "/home/u/.local/bin/mise", "dotfiles", "apply", "--cd", "/path"})
	require.Equal(t, "+ sudo -E /home/u/.local/bin/mise dotfiles apply --cd /path\n", buf.String())
}
