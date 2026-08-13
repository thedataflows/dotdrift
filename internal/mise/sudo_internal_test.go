package mise

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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

// IsPermissionDenied detects the OS "Permission denied" message mise emits
// on stderr, both in raw error strings (non-streaming path) and in the
// captured stderr attached to streaming-path errors.
func TestIsPermissionDenied(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"unrelated", errPlain, false},
		{"permissionInString", errPerm, true},
		{"streamErrorWithPermission", &streamError{
			err:    errPlain,
			stderr: "mise ERROR Permission denied (os error 13)",
		}, true},
		{"streamErrorWithoutPermission", &streamError{
			err:    errPlain,
			stderr: "mise ERROR config parse error",
		}, false},
		{"wrappedStreamError", fmt.Errorf("apply: %w", &streamError{
			err:    errPlain,
			stderr: "mise ERROR Permission denied (os error 13)",
		}), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, IsPermissionDenied(tt.err))
		})
	}
}

var errPlain = errors.New("exit status 1")
var errPerm = errors.New("exit status 1\nmise ERROR Permission denied (os error 13)")

// The streaming path tees stderr to a buffer so IsPermissionDenied works even
// when output is streamed live to the terminal (the interactive apply case).
func TestRunOp_streamingCapturesStderr(t *testing.T) {
	// Force the streaming path by making IsTerminal return true.
	orig := executil.IsTerminal
	executil.IsTerminal = func(io.Writer) bool { return true }
	t.Cleanup(func() { executil.IsTerminal = orig })

	// Script: version probe answers, otherwise print to stderr and exit 1.
	script := filepath.Join(t.TempDir(), "mise")
	content := "#!/bin/sh\n" +
		"if [ \"$1\" = \"--version\" ]; then echo " + MinMiseVersion + "; exit 0; fi\n" +
		"echo 'Permission denied (os error 13)' >&2\n" +
		"exit 1\n"
	require.NoError(t, os.WriteFile(script, []byte(content), 0o755))

	var out bytes.Buffer
	m := &Mise{
		LookPath: func(string) (string, error) { return script, nil },
		Out:      &out,
		Err:      &out,
	}
	em := NewExecMise(m)

	err := em.Bootstrap(context.Background(), "/cfg/mise.toml", true, "files")
	require.Error(t, err)
	require.True(t, IsPermissionDenied(err),
		"streaming path must capture stderr so IsPermissionDenied works, got: %v", err)
}
