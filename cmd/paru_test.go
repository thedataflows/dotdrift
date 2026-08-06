package cmd

import (
	"testing"

	"github.com/alecthomas/kong"
	"github.com/stretchr/testify/require"
)

func TestKong_paruInstalledSubcommand(t *testing.T) {
	var cli CLI
	parser, err := kong.New(&cli)
	require.NoError(t, err)
	_, err = parser.Parse([]string{"paru", "installed", "neovim", "curl"})
	require.NoError(t, err)
	require.Equal(t, []string{"neovim", "curl"}, cli.Paru.Installed.Names)
}

func TestKong_paruInstallFlags(t *testing.T) {
	var cli CLI
	parser, err := kong.New(&cli)
	require.NoError(t, err)
	_, err = parser.Parse([]string{"paru", "install", "--dry-run", "--update", "neovim"})
	require.NoError(t, err)
	require.Equal(t, []string{"neovim"}, cli.Paru.Install.Names)
	require.True(t, cli.Paru.Install.DryRun)
	require.True(t, cli.Paru.Install.Update)
}
