package mise

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/executil"
)

// A real interactive apply shows mise's confirmation prompts (mise asks
// before changing files) but without wiring stdin the prompt reads the
// null device, resolves as "No", and silently skips the change — the
// pipeline still records the step complete. A child spawned through runOp
// must receive stdin so the user can actually answer (issue 0028).
func TestRunOp_childReceivesStdin(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(map[bool]string{false: "captured", true: "streamed"}[streaming], func(t *testing.T) {
			r, w, err := os.Pipe()
			require.NoError(t, err)
			orig := opStdin
			opStdin = func() *os.File { return r }
			t.Cleanup(func() { opStdin = orig })

			var out, errBuf bytes.Buffer
			m := &Mise{Verbose: streaming, Out: &out, Err: &errBuf}
			_, err = w.WriteString("prompt answer\n")
			require.NoError(t, err)
			require.NoError(t, w.Close())

			got, runErr := m.runOp(context.Background(), nil, "sh", "-c", "cat")
			require.NoError(t, runErr)
			require.Contains(t, out.String()+got, "prompt answer",
				"the child must read from the wired stdin, not the null device")
		})
	}
}

// Every mise subprocess is always marked interactive (issue 0136): the
// child receives the real stdin whether or not dotdrift's own stdin is
// a terminal — a gate here is what made mise prompts unanswerable again.
func TestOpStdin_alwaysInteractive(t *testing.T) {
	orig := executil.IsStdinTerminal
	t.Cleanup(func() { executil.IsStdinTerminal = orig })

	for _, terminal := range []bool{true, false} {
		executil.IsStdinTerminal = func() bool { return terminal }
		require.Equal(t, os.Stdin, opStdin(),
			"mise stdin is always the real stdin (terminal=%v)", terminal)
	}
}

// Probes carry the same rule: a probe child also receives the real stdin.
func TestRunProbe_childReceivesStdin(t *testing.T) {
	r, w, err := os.Pipe()
	require.NoError(t, err)
	orig := opStdin
	opStdin = func() *os.File { return r }
	t.Cleanup(func() { opStdin = orig })

	_, err = w.WriteString("probe answer\n")
	require.NoError(t, err)
	require.NoError(t, w.Close())

	m := &Mise{}
	got, runErr := m.runProbe(context.Background(), "sh", "-c", "cat")
	require.NoError(t, runErr)
	require.Equal(t, "probe answer", got, "the probe child must read the wired stdin")
}
