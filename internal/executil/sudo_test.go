package executil_test

// SudoValidate (M15, T-tui-modals): the elevation modal's checker —
// `sudo -k -S -v` with the password on stdin. The exec seam is swapped
// here; no real sudo runs in tests.

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/executil"
)

func TestSudoValidate_passwordOnStdin(t *testing.T) {
	var stdin []byte
	restore := executil.SwapSudoRunner(func(stdinBytes []byte) error {
		stdin = stdinBytes
		return nil
	})
	defer restore()

	require.NoError(t, executil.SudoValidate([]byte("hunter2")))
	require.Equal(t, "hunter2\n", string(stdin), "sudo -S reads the password line from stdin")
}

func TestSudoValidate_failurePropagates(t *testing.T) {
	restore := executil.SwapSudoRunner(func([]byte) error { return errors.New("sudo: 1 incorrect password attempt") })
	defer restore()

	err := executil.SudoValidate([]byte("wrong"))
	require.Error(t, err)
}
