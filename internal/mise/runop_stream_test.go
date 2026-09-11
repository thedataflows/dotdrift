package mise

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Cancel on the streaming runOp path kills the whole process group: the
// call returns promptly even though a shell wrapper forked a background
// sleeper that would outlive the direct child (0064-D5, research 0059 §5).
func TestRunOp_cancelKillsProcessGroup(t *testing.T) {
	m := &Mise{ForceStream: true, Out: io.Discard, Err: io.Discard}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = m.runOp(ctx, nil, "sh", "-c", "sleep 30 & sleep 30")
	}()
	time.Sleep(150 * time.Millisecond) // let the child start
	start := time.Now()
	cancel()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("runOp did not return after ctx cancel — process group survived")
	}
	require.Less(t, time.Since(start), 10*time.Second)
}

// ForceStream streams operation output to non-terminal writers without
// the verbose echo; without it a piped run captures silently (today's
// semantics, unchanged).
func TestRunOp_forceStreamWritesToPipedWriters(t *testing.T) {
	out := &bytes.Buffer{}
	errW := &bytes.Buffer{}
	m := &Mise{ForceStream: true, Out: out, Err: errW}
	res, err := m.runOp(context.Background(), nil, "echo", "streamed-line")
	require.NoError(t, err)
	require.Empty(t, res, "streamed output is not also captured")
	require.Contains(t, out.String(), "streamed-line\n")
	require.Empty(t, errW.String(), "no + argv echo without Verbose")

	out.Reset()
	m2 := &Mise{Out: out, Err: errW}
	res, err = m2.runOp(context.Background(), nil, "echo", "captured-line")
	require.NoError(t, err)
	require.Contains(t, res, "captured-line")
	require.Empty(t, out.String(), "non-forced piped runs capture, not stream")
}
