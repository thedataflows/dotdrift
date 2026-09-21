package smb

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Verbose ExecRunner streams child stdout/stderr live AND still returns the
// combined capture — callers parse (id -Gn, pdbedit -L) and append (testparm
// gate) the returned output, so streaming must never starve them.
func TestExecRunner_verboseStreamsAndCaptures(t *testing.T) {
	var out, errW bytes.Buffer
	r := ExecRunner{Verbose: true, Out: &out, Err: &errW}

	got, err := r.Run(context.Background(), "sh", "-c", "echo out-line; echo err-line >&2")
	require.NoError(t, err)
	require.Contains(t, out.String(), "out-line", "stdout must stream live")
	require.Contains(t, errW.String(), "err-line", "stderr must stream live")
	require.Contains(t, got, "out-line", "returned capture must survive streaming (parsing contract)")
	require.Contains(t, got, "err-line", "returned capture must survive streaming (parsing contract)")
}

// Non-verbose ExecRunner keeps today's contract: combined output captured
// and returned, nothing on the writers.
func TestExecRunner_nonVerboseCapturesOnly(t *testing.T) {
	var out, errW bytes.Buffer
	r := ExecRunner{Verbose: false, Out: &out, Err: &errW}

	got, err := r.Run(context.Background(), "sh", "-c", "echo out-line; echo err-line >&2")
	require.NoError(t, err)
	require.Contains(t, got, "out-line")
	require.Contains(t, got, "err-line")
	require.Empty(t, out.String())
	require.Empty(t, errW.String())
}

// Verbose ExecRunner echoes the command line (bash set -x style: "+ argv")
// to Err before streaming; the echo never pollutes the returned capture
// (id -Gn / pdbedit -L parsing contract) nor Out.
func TestExecRunner_verboseEchoesCommandLine(t *testing.T) {
	var out, errW bytes.Buffer
	r := ExecRunner{Verbose: true, Out: &out, Err: &errW}

	got, err := r.Run(context.Background(), "sh", "-c", "echo out-line; echo err-line >&2")
	require.NoError(t, err)

	echo := "+ sh -c 'echo out-line; echo err-line >&2'\n"
	require.Contains(t, errW.String(), echo, "verbose must echo the command line to Err")
	require.Less(t, strings.Index(errW.String(), echo), strings.Index(errW.String(), "err-line"),
		"the echo must precede the command's own stderr output")
	require.NotContains(t, out.String(), "+ ", "the echo goes to Err, never Out")
	require.NotContains(t, got, "+ ", "the echo must not enter the captured output (parsing contract)")
}

func TestExecRunner_setVerbose(t *testing.T) {
	r := &ExecRunner{}
	require.False(t, r.Verbose)
	r.SetVerbose(true)
	require.True(t, r.Verbose)
}
