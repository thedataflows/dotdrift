package cmd

import (
	"testing"

	"github.com/alecthomas/kong"
	"github.com/stretchr/testify/require"
)

// generate is CLI-only (issue 0066, D1/D4): the wizard flags are gone
// and kong's own unknown-flag error is the whole answer. There is no
// deprecated stub — README and docs/log.md carry the announcement.

// parseGenerateArgs parses args through a real kong parser and returns
// the parse result (no run), so flag-surface errors are observable.
func parseGenerateArgs(args ...string) error {
	var cli CLI
	parser, err := kong.New(&cli, kong.Name(appName))
	if err != nil {
		return err
	}
	_, err = parser.Parse(args)
	return err
}

func TestGenerate_tuiFlagsRemoved_unknownFlag(t *testing.T) {
	for _, sub := range []string{"mounts", "smb"} {
		for _, flag := range []string{"--tui", "--no-tui"} {
			t.Run(sub+" "+flag, func(t *testing.T) {
				err := parseGenerateArgs("generate", sub, flag)
				require.Error(t, err, "%s %s must be an unknown flag", sub, flag)
				require.Contains(t, err.Error(), flag)
			})
		}
	}
}
