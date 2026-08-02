package executil_test

import (
	"bytes"
	"fmt"
	"io"
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
