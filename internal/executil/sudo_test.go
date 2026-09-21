package executil

// SudoValidate (M15, T-tui-modals): the elevation modal's checker —
// `sudo -k -S -v` with the password on stdin. The exec seam is stubbed
// directly (in-package test); no real sudo runs in tests.

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSudoValidate_passwordOnStdin(t *testing.T) {
	var stdin []byte
	prev := sudoRunner
	sudoRunner = func(stdinBytes []byte) error {
		stdin = stdinBytes
		return nil
	}
	t.Cleanup(func() { sudoRunner = prev })

	require.NoError(t, SudoValidate([]byte("hunter2")))
	require.Equal(t, "hunter2\n", string(stdin), "sudo -S reads the password line from stdin")
}

func TestSudoValidate_failurePropagates(t *testing.T) {
	prev := sudoRunner
	sudoRunner = func([]byte) error { return errors.New("sudo: 1 incorrect password attempt") }
	t.Cleanup(func() { sudoRunner = prev })

	err := SudoValidate([]byte("wrong"))
	require.Error(t, err)
}
