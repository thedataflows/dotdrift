package mise

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
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
	argv := dotfilesApplyArgv(1000, "/fake/mise", "/cfg/mise.toml", true)
	require.Equal(t, []string{"sudo", "-E", "/fake/mise", "dotfiles", "apply", "--cd", "/cfg", "--yes"}, argv)
}

// Root (e.g. containers): apply directly, no sudo invocation.
func TestDotfilesApplyArgv_rootSkipsSudo(t *testing.T) {
	argv := dotfilesApplyArgv(0, "/fake/mise", "/cfg/mise.toml", true)
	require.Equal(t, []string{"/fake/mise", "dotfiles", "apply", "--cd", "/cfg", "--yes"}, argv)
}

// --yes is only appended when requested.
func TestDotfilesApplyArgv_yesOmitted(t *testing.T) {
	argv := dotfilesApplyArgv(1000, "/fake/mise", "/cfg/mise.toml", false)
	require.Equal(t, []string{"sudo", "-E", "/fake/mise", "dotfiles", "apply", "--cd", "/cfg"}, argv)
}

// DotfilesApplySudo drives the argv decision off the live euid seam: sudo
// when non-root, direct when root.
func TestExecMise_dotfilesApplySudo_invocationArgv(t *testing.T) {
	cases := []struct {
		name     string
		euid     int
		wantName string
		wantArgs []string
	}{
		{"nonRootSudo", 1000, "sudo", []string{"-E", "/fake/mise", "dotfiles", "apply", "--cd", "/cfg", "--yes"}},
		{"rootDirect", 0, "/fake/mise", []string{"dotfiles", "apply", "--cd", "/cfg", "--yes"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			swapEUID(t, tc.euid)
			var gotName string
			var gotArgs []string
			em := NewExecMise(&Mise{
				LookPath: func(string) (string, error) { return "/fake/mise", nil },
				RunContext: func(_ context.Context, name string, args ...string) (string, error) {
					if len(args) > 0 && args[0] == "--version" {
						return MinMiseVersion + "\n", nil
					}
					gotName = name
					gotArgs = append([]string{}, args...)
					return "", nil
				},
			})

			require.NoError(t, em.DotfilesApplySudo(context.Background(), "/cfg/mise.toml", true))
			require.Equal(t, tc.wantName, gotName)
			require.Equal(t, tc.wantArgs, gotArgs)
		})
	}
}

// The trust plumbing must survive the sudo entry point: running as root (no
// sudo needed) the generated config dir still lands in
// MISE_TRUSTED_CONFIG_PATHS on the real exec path.
func TestExecMise_dotfilesApplySudo_trustsGeneratedConfigDir(t *testing.T) {
	swapEUID(t, 0)
	capture := filepath.Join(t.TempDir(), "capture")
	em := realExecMise(t, fakeMiseScript(t, capture))
	cfgDir, cfg := generatedConfig(t)

	require.NoError(t, em.DotfilesApplySudo(context.Background(), cfg, false))

	lines := captureLines(t, capture)
	require.Equal(t, "TRUSTED="+cfgDir, lines[0],
		"mise subprocess env must trust the generated config's directory")
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
