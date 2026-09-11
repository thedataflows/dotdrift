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

// --host/--user are bool-like flags with an optional =value: no value
// selects the current host/user, --host=<name> an explicit one, an
// omitted flag means the base layer. Bool-like flags bind values ONLY
// via =, so a valueless --host never eats the next token and
// `--host --user` composes (plain string flags would error:
// "--host: expected string value").
func TestKong_onboardOverlayValues(t *testing.T) {
	parse := func(t *testing.T, args ...string) OnboardCmd {
		t.Helper()
		var cli CLI
		parser, err := kong.New(&cli, kong.Name(appName))
		require.NoError(t, err)
		// --app is required (issue 0055); these tests exercise --host/--user.
		_, err = parser.Parse(append([]string{"onboard", "--app=x"}, args...))
		require.NoError(t, err)
		return cli.Onboard
	}

	t.Run("bare host means current", func(t *testing.T) {
		c := parse(t, "~/.bashrc", "--host")
		require.True(t, c.Host.Set)
		require.Equal(t, "", c.Host.Value)
		require.Equal(t, []string{"~/.bashrc"}, c.Paths, "bare --host must not eat the path")
	})
	t.Run("explicit host", func(t *testing.T) {
		c := parse(t, "--host=cri-pc", "~/.bashrc")
		require.True(t, c.Host.Set)
		require.Equal(t, "cri-pc", c.Host.Value)
	})
	t.Run("bare user means current", func(t *testing.T) {
		c := parse(t, "--user", "~/.bashrc")
		require.True(t, c.User.Set)
		require.Equal(t, "", c.User.Value)
		require.Equal(t, []string{"~/.bashrc"}, c.Paths)
	})
	t.Run("explicit user", func(t *testing.T) {
		c := parse(t, "--user=alice", "~/.bashrc")
		require.Equal(t, "alice", c.User.Value)
	})
	t.Run("both bare together", func(t *testing.T) {
		c := parse(t, "--host", "--user", "~/.bashrc")
		require.True(t, c.Host.Set)
		require.True(t, c.User.Set)
		require.Equal(t, []string{"~/.bashrc"}, c.Paths)
	})
	t.Run("both explicit together", func(t *testing.T) {
		c := parse(t, "--host=h2", "--user=u2", "~/.bashrc")
		require.Equal(t, "h2", c.Host.Value)
		require.Equal(t, "u2", c.User.Value)
	})
	t.Run("bare and explicit mixed", func(t *testing.T) {
		c := parse(t, "--host", "--user=u2", "~/.bashrc")
		require.True(t, c.Host.Set)
		require.Equal(t, "", c.Host.Value)
		require.Equal(t, "u2", c.User.Value)
	})
	t.Run("omitted stays unset", func(t *testing.T) {
		c := parse(t, "~/.bashrc")
		require.False(t, c.Host.Set)
		require.False(t, c.User.Set)
	})
	t.Run("empty value also means current", func(t *testing.T) {
		c := parse(t, "--host=", "~/.bashrc")
		require.True(t, c.Host.Set)
		require.Equal(t, "", c.Host.Value)
	})
}

// overlayOwner: explicit value wins, bare flag falls back to the detected
// fact, an unset flag keeps detection (base-layer path).
func TestOnboard_overlayFlagResolution(t *testing.T) {
	require.Equal(t, "detected-host", overlayOwner(overlayFlag{Set: true}, "detected-host"))
	require.Equal(t, "other-host", overlayOwner(overlayFlag{Set: true, Value: "other-host"}, "detected-host"))
	require.Equal(t, "detected-user", overlayOwner(overlayFlag{}, "detected-user"))
}

// End to end through the command: bare flag lands in the DETECTED
// host/user overlay, explicit value in that layer, no flag in base.
func TestOnboard_overlayFlagsChooseLayers(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	orig := detectFacts
	detectFacts = func() (*facts.Facts, error) {
		return &facts.Facts{Hostname: "testhost", Username: "testuser"}, nil
	}
	t.Cleanup(func() { detectFacts = orig })

	run := func(t *testing.T, host, user overlayFlag) string {
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
		prof := run(t, overlayFlag{}, overlayFlag{})
		require.FileExists(t, filepath.Join(prof, "modules", "app", "module.toml"))
	})
	t.Run("bare host uses detected hostname", func(t *testing.T) {
		prof := run(t, overlayFlag{Set: true}, overlayFlag{})
		require.FileExists(t, filepath.Join(prof, "hosts", "testhost", "modules", "app", "module.toml"))
		require.NoDirExists(t, filepath.Join(prof, "modules", "app"))
	})
	t.Run("explicit host wins over detection", func(t *testing.T) {
		prof := run(t, overlayFlag{Set: true, Value: "lab-pc"}, overlayFlag{})
		require.FileExists(t, filepath.Join(prof, "hosts", "lab-pc", "modules", "app", "module.toml"))
		require.NoDirExists(t, filepath.Join(prof, "hosts", "testhost"))
	})
	t.Run("bare user uses detected username", func(t *testing.T) {
		prof := run(t, overlayFlag{}, overlayFlag{Set: true})
		require.FileExists(t, filepath.Join(prof, "users", "testuser", "modules", "app", "module.toml"))
	})
	t.Run("explicit user wins over detection", func(t *testing.T) {
		prof := run(t, overlayFlag{}, overlayFlag{Set: true, Value: "alice"})
		require.FileExists(t, filepath.Join(prof, "users", "alice", "modules", "app", "module.toml"))
	})
	t.Run("both layers", func(t *testing.T) {
		prof := run(t, overlayFlag{Set: true, Value: "lab-pc"}, overlayFlag{Set: true, Value: "alice"})
		require.FileExists(t, filepath.Join(prof, "hosts", "lab-pc", "modules", "app", "module.toml"))
		require.FileExists(t, filepath.Join(prof, "users", "alice", "modules", "app", "module.toml"))
		require.NoDirExists(t, filepath.Join(prof, "modules", "app"))
	})
}
