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

func strPtr(s string) *string { return &s }

// --host/--user are string flags: a value selects that host/user layer,
// an EMPTY value (`--host=`) means the current one, and omitting the
// flag means the base layer. (nil distinguishes omitted from `--host=`.)
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

	t.Run("empty value means current", func(t *testing.T) {
		c := parse(t, "--host=", "~/.bashrc")
		require.NotNil(t, c.Host)
		require.Equal(t, "", *c.Host)
		require.Nil(t, c.User)
	})
	t.Run("explicit host", func(t *testing.T) {
		c := parse(t, "--host=cri-pc", "~/.bashrc")
		require.NotNil(t, c.Host)
		require.Equal(t, "cri-pc", *c.Host)
	})
	t.Run("empty user value means current", func(t *testing.T) {
		c := parse(t, "--user=", "~/.bashrc")
		require.NotNil(t, c.User)
		require.Equal(t, "", *c.User)
	})
	t.Run("both explicit", func(t *testing.T) {
		c := parse(t, "--host=h2", "--user=u2", "~/.bashrc")
		require.Equal(t, "h2", *c.Host)
		require.Equal(t, "u2", *c.User)
	})
	t.Run("omitted is nil", func(t *testing.T) {
		c := parse(t, "~/.bashrc")
		require.Nil(t, c.Host)
		require.Nil(t, c.User)
	})
}

// End to end through the command: nil flag = base layer, empty value =
// the DETECTED host/user layer, explicit value = that layer.
func TestOnboard_overlayFlagsChooseLayers(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	orig := detectFacts
	detectFacts = func() (*facts.Facts, error) {
		return &facts.Facts{Hostname: "testhost", Username: "testuser"}, nil
	}
	t.Cleanup(func() { detectFacts = orig })

	run := func(t *testing.T, host, user *string) string {
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
		prof := run(t, nil, nil)
		require.FileExists(t, filepath.Join(prof, "modules", "app", "module.toml"))
	})
	t.Run("empty host value uses detected host", func(t *testing.T) {
		prof := run(t, strPtr(""), nil)
		require.FileExists(t, filepath.Join(prof, "hosts", "testhost", "modules", "app", "module.toml"))
		require.NoDirExists(t, filepath.Join(prof, "modules", "app"))
	})
	t.Run("explicit host", func(t *testing.T) {
		prof := run(t, strPtr("lab-pc"), nil)
		require.FileExists(t, filepath.Join(prof, "hosts", "lab-pc", "modules", "app", "module.toml"))
		require.NoDirExists(t, filepath.Join(prof, "modules", "app"))
	})
	t.Run("empty user value uses detected user", func(t *testing.T) {
		prof := run(t, nil, strPtr(""))
		require.FileExists(t, filepath.Join(prof, "users", "testuser", "modules", "app", "module.toml"))
	})
	t.Run("explicit user", func(t *testing.T) {
		prof := run(t, nil, strPtr("alice"))
		require.FileExists(t, filepath.Join(prof, "users", "alice", "modules", "app", "module.toml"))
	})
	t.Run("both layers", func(t *testing.T) {
		prof := run(t, strPtr("lab-pc"), strPtr("alice"))
		require.FileExists(t, filepath.Join(prof, "hosts", "lab-pc", "modules", "app", "module.toml"))
		require.FileExists(t, filepath.Join(prof, "users", "alice", "modules", "app", "module.toml"))
		require.NoDirExists(t, filepath.Join(prof, "modules", "app"))
	})
}
