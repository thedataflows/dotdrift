package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/mise"
)

// --host/--user are plain string flags: empty = not selecting that
// overlay (base layer), a value selects that host/user layer.
func TestKong_onboardOverlayValues(t *testing.T) {
	parse := func(t *testing.T, args ...string) OnboardCmd {
		t.Helper()
		var cli CLI
		parser, err := kong.New(&cli, kong.Name(appName))
		require.NoError(t, err)
		_, err = parser.Parse(append([]string{"onboard"}, args...))
		require.NoError(t, err)
		return cli.Onboard
	}

	t.Run("host value", func(t *testing.T) {
		c := parse(t, "--host=cri-pc", "~/.bashrc")
		require.Equal(t, "cri-pc", c.Host)
		require.Equal(t, "", c.User)
	})
	t.Run("user value", func(t *testing.T) {
		c := parse(t, "--user=alice", "~/.bashrc")
		require.Equal(t, "alice", c.User)
		require.Equal(t, "", c.Host)
	})
	t.Run("both", func(t *testing.T) {
		c := parse(t, "--host=h2", "--user=u2", "~/.bashrc")
		require.Equal(t, "h2", c.Host)
		require.Equal(t, "u2", c.User)
	})
	t.Run("absent means empty", func(t *testing.T) {
		c := parse(t, "~/.bashrc")
		require.Equal(t, "", c.Host)
		require.Equal(t, "", c.User)
	})
}

// End to end through the command: an explicit --host/--user value lands
// the module in that layer; no flag lands in base.
func TestOnboard_overlayFlagsChooseLayers(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	orig := detectFacts
	detectFacts = func() (*facts.Facts, error) {
		return &facts.Facts{Hostname: "testhost", Username: "testuser"}, nil
	}
	t.Cleanup(func() { detectFacts = orig })

	run := func(t *testing.T, host, user string) string {
		t.Helper()
		prof := t.TempDir()
		live := filepath.Join(t.TempDir(), "live.conf")
		require.NoError(t, os.WriteFile(live, []byte("x\n"), 0o644))
		cmd := &OnboardCmd{
			Paths: []string{live}, Profile: prof, App: "app",
			Host: host, User: user, Mise: &mise.FakeRunner{},
		}
		require.NoError(t, cmd.Run())
		return prof
	}

	t.Run("no flag lands in base", func(t *testing.T) {
		prof := run(t, "", "")
		require.FileExists(t, filepath.Join(prof, "modules", "app", "module.toml"))
	})
	t.Run("explicit host", func(t *testing.T) {
		prof := run(t, "lab-pc", "")
		require.FileExists(t, filepath.Join(prof, "hosts", "lab-pc", "modules", "app", "module.toml"))
		require.NoDirExists(t, filepath.Join(prof, "modules", "app"))
	})
	t.Run("explicit user", func(t *testing.T) {
		prof := run(t, "", "alice")
		require.FileExists(t, filepath.Join(prof, "users", "alice", "modules", "app", "module.toml"))
	})
	t.Run("both layers", func(t *testing.T) {
		prof := run(t, "lab-pc", "alice")
		require.FileExists(t, filepath.Join(prof, "hosts", "lab-pc", "modules", "app", "module.toml"))
		require.FileExists(t, filepath.Join(prof, "users", "alice", "modules", "app", "module.toml"))
		require.NoDirExists(t, filepath.Join(prof, "modules", "app"))
	})
}
