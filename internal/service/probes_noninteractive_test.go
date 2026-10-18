package service

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTUIProbeRetryNeverPrompts(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "valid ticket", true: "expired ticket"}[fail], func(t *testing.T) {
			calls := 0
			stubSudo(t, func(name string, args ...string) ([]byte, error) {
				calls++
				require.Equal(t, "-n", name)
				require.Equal(t, []string{"cat", "/private"}, args)
				if fail {
					return nil, errors.New("sudo: a password is required")
				}
				return []byte("secret"), nil
			})
			original := &os.PathError{Op: "open", Path: "/private", Err: os.ErrPermission}
			probe := elevateReadFile(func(string) ([]byte, error) { return nil, original }, true)
			got, err := probe("/private")
			require.Equal(t, 1, calls)
			if fail {
				require.ErrorIs(t, err, original)
				require.True(t, os.IsPermission(err))
			} else {
				require.NoError(t, err)
				require.Equal(t, "secret", string(got))
			}
		})
	}
}
