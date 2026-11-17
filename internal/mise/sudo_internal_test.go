package mise

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/executil"
)

// swapEUID pins the effective-uid seam for the duration of a test.
func swapEUID(t *testing.T, euid int) {
	t.Helper()
	orig := geteuid
	geteuid = func() int { return euid }
	t.Cleanup(func() { geteuid = orig })
}

// Non-root: system dotfiles apply goes through sudo -E (preserving the
// MISE_TRUSTED_CONFIG_PATHS env the trust plumbing sets on the child).
func TestDotfilesApplyArgv_nonRootUsesSudo(t *testing.T) {
	argv := dotfilesApplyArgv(1000, "/fake/mise", "/cfg/mise.toml", true, false)
	require.Equal(t, []string{"sudo", "-E", "/fake/mise", "dotfiles", "apply", "--cd", "/cfg", "--yes"}, argv)
}

// Root (e.g. containers): apply directly, no sudo invocation.
func TestDotfilesApplyArgv_rootSkipsSudo(t *testing.T) {
	argv := dotfilesApplyArgv(0, "/fake/mise", "/cfg/mise.toml", true, false)
	require.Equal(t, []string{"/fake/mise", "dotfiles", "apply", "--cd", "/cfg", "--yes"}, argv)
}

// --yes is only appended when requested.
func TestDotfilesApplyArgv_yesOmitted(t *testing.T) {
	argv := dotfilesApplyArgv(1000, "/fake/mise", "/cfg/mise.toml", false, false)
	require.Equal(t, []string{"sudo", "-E", "/fake/mise", "dotfiles", "apply", "--cd", "/cfg"}, argv)
}

// dotdrift apply --force (issue 0046) appends --force after --yes, on the
// elevated path too (system edit entries).
func TestDotfilesApplyArgv_forceAppended(t *testing.T) {
	argv := dotfilesApplyArgv(1000, "/fake/mise", "/cfg/mise.toml", true, true)
	require.Equal(t, []string{"sudo", "-E", "/fake/mise", "dotfiles", "apply", "--cd", "/cfg", "--yes", "--force"}, argv)
	argv = dotfilesApplyArgv(0, "/fake/mise", "/cfg/mise.toml", true, true)
	require.Equal(t, []string{"/fake/mise", "dotfiles", "apply", "--cd", "/cfg", "--yes", "--force"}, argv)
}

// DotfilesApplySudoCmd drives the argv decision off the live euid seam:
// sudo -E when non-root, direct when root (issue 0071 handover source).
func TestExecMise_dotfilesApplySudoSpec_argv(t *testing.T) {
	cases := []struct {
		name string
		euid int
		want []string
	}{
		{"nonRootSudo", 1000, []string{"sudo", "-E", "/fake/mise", "dotfiles", "apply", "--cd", "/cfg", "--yes"}},
		{"rootDirect", 0, []string{"/fake/mise", "dotfiles", "apply", "--cd", "/cfg", "--yes"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			swapEUID(t, tc.euid)
			em := NewExecMise(&Mise{
				LookPath: func(string) (string, error) { return "/fake/mise", nil },
				RunContext: func(_ context.Context, name string, args ...string) (string, error) {
					if len(args) > 0 && args[0] == "--version" {
						return MinMiseVersion + "\n", nil
					}
					return "", nil
				},
			})

			cmd, err := em.DotfilesApplySudoCmd(context.Background(), "/cfg/mise.toml", true, false)
			require.NoError(t, err)
			require.Equal(t, tc.want, cmd.Args)
			// exec.CommandContext resolves argv[0] on PATH: bare names stay in
			// Args, the resolved binary lands in Path.
			if tc.euid != 0 {
				require.True(t, strings.HasSuffix(cmd.Path, "/sudo"), "sudo resolves on PATH, got %q", cmd.Path)
			} else {
				require.Equal(t, tc.want[0], cmd.Path)
			}
		})
	}
}

// The trust plumbing must survive the elevation entry point: the spec's
// child env carries MISE_TRUSTED_CONFIG_PATHS for the generated config's
// directory (the consumer execs exactly this env, issue 0071).
func TestExecMise_dotfilesApplySudoSpec_trustsGeneratedConfigDir(t *testing.T) {
	swapEUID(t, 0)
	em := NewExecMise(&Mise{
		LookPath: func(string) (string, error) { return "/fake/mise", nil },
		RunContext: func(_ context.Context, name string, args ...string) (string, error) {
			if len(args) > 0 && args[0] == "--version" {
				return MinMiseVersion + "\n", nil
			}
			return "", nil
		},
	})
	cfgDir, cfg := generatedConfig(t)

	cmd, err := em.DotfilesApplySudoCmd(context.Background(), cfg, false, false)
	require.NoError(t, err)
	want := "MISE_TRUSTED_CONFIG_PATHS=" + cfgDir
	found := false
	for _, e := range cmd.Env {
		if e == want {
			found = true
		}
	}
	require.True(t, found, "child env must trust the generated config's directory")
}

// The elevated child must not anchor mise's state in the invoking user's
// home (issue 0157): sudo -E preserves HOME/XDG, so the sudo branch
// overrides the home-anchored dirs with root's. The already-root branch
// keeps the inherited environment.
func TestExecMise_dotfilesApplySudoSpec_elevatedEnvAnchorsRoot(t *testing.T) {
	orig := rootHomeDir
	rootHomeDir = func() string { return "/root" }
	t.Cleanup(func() { rootHomeDir = orig })
	t.Setenv("HOME", "/home/cri")
	t.Setenv("XDG_STATE_HOME", "/home/cri/.local/state")

	newExec := func() *ExecMise {
		return NewExecMise(&Mise{
			LookPath: func(string) (string, error) { return "/fake/mise", nil },
			RunContext: func(_ context.Context, name string, args ...string) (string, error) {
				if len(args) > 0 && args[0] == "--version" {
					return MinMiseVersion + "\n", nil
				}
				return "", nil
			},
		})
	}
	lastWins := func(cmd *exec.Cmd) map[string]string {
		env := map[string]string{}
		for _, e := range cmd.Env {
			k, v, _ := strings.Cut(e, "=")
			env[k] = v
		}
		return env
	}

	swapEUID(t, 1000)
	cmd, err := newExec().DotfilesApplySudoCmd(context.Background(), "/cfg/mise.toml", true, false)
	require.NoError(t, err)
	env := lastWins(cmd)
	require.Equal(t, "/root", env["HOME"])
	require.Equal(t, "/root/.local/state", env["XDG_STATE_HOME"])
	require.Equal(t, "/root/.local/state/mise", env["MISE_STATE_DIR"])
	require.Equal(t, "/root/.local/share/mise", env["MISE_DATA_DIR"])
	require.Equal(t, "/root/.cache/mise", env["MISE_CACHE_DIR"])
	require.Equal(t, "/root/.config/mise", env["MISE_CONFIG_DIR"])
	require.Contains(t, env["MISE_TRUSTED_CONFIG_PATHS"], "/cfg", "trust plumbing survives")

	swapEUID(t, 0)
	cmd, err = newExec().DotfilesApplySudoCmd(context.Background(), "/cfg/mise.toml", true, false)
	require.NoError(t, err)
	env = lastWins(cmd)
	require.Equal(t, "/home/cri", env["HOME"], "already-root child keeps the inherited environment")
	require.NotContains(t, env, "MISE_STATE_DIR")
}

// runOp streams the child's stdout/stderr straight to the destination — never
// through a MultiWriter/pipe — so the child's isatty checks pass and it keeps
// its color on a TTY (mise/paru emit zero ANSI when piped and honor no
// force-color override). A failure returns the bare exec error (the output
// already streamed live); permission retry is decided up front by the caller.
func TestRunOp_streamsStderrStraightToDestination(t *testing.T) {
	// Force the streaming path by making IsTerminal return true.
	orig := executil.IsTerminal
	executil.IsTerminal = func(io.Writer) bool { return true }
	t.Cleanup(func() { executil.IsTerminal = orig })

	script := filepath.Join(t.TempDir(), "mise")
	content := "#!/bin/sh\n" +
		"if [ \"$1\" = \"--version\" ]; then echo " + MinMiseVersion + "; exit 0; fi\n" +
		"echo color-me >&2\n" +
		"exit 1\n"
	require.NoError(t, os.WriteFile(script, []byte(content), 0o755))

	var out, errW bytes.Buffer
	m := &Mise{LookPath: func(string) (string, error) { return script, nil }, Out: &out, Err: &errW}
	em := NewExecMise(m)

	err := em.Bootstrap(context.Background(), "/cfg/mise.toml", true, "packages")
	require.Error(t, err)
	require.Contains(t, errW.String(), "color-me", "stderr must stream straight to the destination")
	// No captured-stderr wrapping: the error is the bare exec error, so the
	// child's stderr fd stays direct (no pipe/tee) and keeps color on a TTY.
	require.NotContains(t, err.Error(), "color-me", "streamed output must not be duplicated into the error")
}
