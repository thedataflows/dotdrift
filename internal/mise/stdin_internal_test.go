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

// The wiring is gated on stdin being a terminal: interactive runs inherit
// the real stdin; piped/CI runs keep the null device so nothing blocks
// waiting on input nobody will provide.
func TestOpStdin_terminalGate(t *testing.T) {
	orig := executil.IsStdinTerminal
	t.Cleanup(func() { executil.IsStdinTerminal = orig })

	executil.IsStdinTerminal = func() bool { return true }
	require.Equal(t, os.Stdin, opStdin(), "interactive stdin is inherited")

	executil.IsStdinTerminal = func() bool { return false }
	require.Nil(t, opStdin(), "non-interactive runs keep the null device")
}
