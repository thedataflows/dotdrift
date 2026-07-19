package mise

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
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
