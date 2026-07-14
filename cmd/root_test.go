package dotdrift_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	cmd "github.com/thedataflows/dotdrift/cmd"
)

func TestHelp_exitsZero(t *testing.T) {
	err := cmd.Run("dev", []string{"--help"})
	require.NoError(t, err)
}

func TestSubcommands_registered(t *testing.T) {
	subcommands := []string{"init", "detect", "modules", "plan", "apply", "status", "onboard"}
	for _, name := range subcommands {
		t.Run(name, func(t *testing.T) {
			err := cmd.Run("dev", []string{name, "--help"})
			require.NoError(t, err, "command %q is not registered", name)
		})
	}
}
