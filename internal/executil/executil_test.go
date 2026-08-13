package executil_test

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
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

// An embedded single quote is escaped '\” style, even without whitespace.
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

// Two MultiWriters sharing one LockedWriter capture (the verbose exec
// pattern: stdout and stderr pipes copied by concurrent goroutines) must
// deliver every byte exactly once — run under -race to catch a regression
// to an unsynchronized buffer.
func TestLockedWriter_concurrentStreamsDeliverAllBytes(t *testing.T) {
	var buf bytes.Buffer
	cap1 := &executil.LockedWriter{W: &buf}
	var liveOut, liveErr bytes.Buffer
	stdout := io.MultiWriter(&liveOut, cap1)
	stderr := io.MultiWriter(&liveErr, cap1)

	const lines = 200
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := range lines {
			fmt.Fprintf(stdout, "out-%d\n", i)
		}
	}()
	go func() {
		defer wg.Done()
		for i := range lines {
			fmt.Fprintf(stderr, "err-%d\n", i)
		}
	}()
	wg.Wait()

	for i := range lines {
		require.Contains(t, buf.String(), fmt.Sprintf("out-%d\n", i))
		require.Contains(t, buf.String(), fmt.Sprintf("err-%d\n", i))
	}
	require.Equal(t, 2*lines, strings.Count(buf.String(), "\n"), "no write may be lost or duplicated")
}

// A char device (here /dev/null) reads as a terminal.
func TestIsStdinTerminal_charDeviceIsTrue(t *testing.T) {
	orig := os.Stdin
	devnull, err := os.Open(os.DevNull)
	require.NoError(t, err)
	os.Stdin = devnull
	t.Cleanup(func() { _ = devnull.Close(); os.Stdin = orig })
	require.True(t, executil.IsStdinTerminal())
}

// A regular file is not a terminal — the check is the ModeCharDevice bit.
func TestIsStdinTerminal_regularFileIsFalse(t *testing.T) {
	orig := os.Stdin
	f, err := os.CreateTemp(t.TempDir(), "stdin")
	require.NoError(t, err)
	os.Stdin = f
	t.Cleanup(func() { _ = f.Close(); os.Stdin = orig })
	require.False(t, executil.IsStdinTerminal())
}

// IsTerminal is false for non-file writers (a capture buffer never counts as
// interactive), and for a regular file — only a char device (terminal) qualifies.
func TestIsTerminal_nonFileAndRegularFileAreFalse(t *testing.T) {
	require.False(t, executil.IsTerminal(&bytes.Buffer{}))
	f, err := os.CreateTemp("", "executil-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = f.Close(); _ = os.Remove(f.Name()) })
	require.False(t, executil.IsTerminal(f))
}

// StreamLive decides whether a child streams live or is captured: verbose
// streams unconditionally (the set -x trace); otherwise both destinations must
// be terminals (an interactive apply); a piped, non-verbose run captures so a
// failure carries self-contained diagnostics.
func TestStreamLive_table(t *testing.T) {
	orig := executil.IsTerminal
	t.Cleanup(func() { executil.IsTerminal = orig })
	cases := []struct {
		name    string
		verbose bool
		term    bool
		want    bool
	}{
		{"verbose streams off-terminal", true, false, true},
		{"interactive terminal streams", false, true, true},
		{"piped non-verbose captures", false, false, false},
		{"verbose terminal streams", true, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			executil.IsTerminal = func(io.Writer) bool { return tc.term }
			require.Equal(t, tc.want, executil.StreamLive(tc.verbose, &bytes.Buffer{}, &bytes.Buffer{}))
		})
	}
}

func TestOrStdout_nilDefaultsToOsStdout(t *testing.T) {
	require.Equal(t, os.Stdout, executil.OrStdout(nil))
}

func TestOrStdout_nonNilReturnsArg(t *testing.T) {
	var b bytes.Buffer
	require.Equal(t, &b, executil.OrStdout(&b))
}

func TestOrStderr_nilDefaultsToOsStderr(t *testing.T) {
	require.Equal(t, os.Stderr, executil.OrStderr(nil))
}

func TestOrStderr_nonNilReturnsArg(t *testing.T) {
	var b bytes.Buffer
	require.Equal(t, &b, executil.OrStderr(&b))
}
