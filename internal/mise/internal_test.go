package mise

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDefaultRunContext_errorIncludesOutput(t *testing.T) {
	_, err := defaultRunContext(context.Background(), "sh", "-c", "echo boom-output; exit 1")
	require.Error(t, err)
	require.Contains(t, err.Error(), "exit status 1")
	require.Contains(t, err.Error(), "boom-output")
}

func TestDefaultRunContext_ctxCancelKillsProcess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	_, err := defaultRunContext(ctx, "sh", "-c", "sleep 30")
	require.Error(t, err)
	require.Less(t, time.Since(start), 5*time.Second, "cancelled ctx must kill the process promptly")
}

func TestInstallFailure_hintsNetworkAndPreseed(t *testing.T) {
	err := installFailure("/home/u/.local/bin/mise", errors.New("exit status 7"), []byte("curl: (7) failed to connect"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "curl: (7) failed to connect")
	require.Contains(t, err.Error(), "network connectivity")
	require.Contains(t, err.Error(), "pre-seed")
	require.Contains(t, err.Error(), ".local/bin/mise")
}

// fakeMiseScript writes an executable fake `mise` that answers `--version`
// and otherwise records the trust-relevant environment it was invoked with.
func fakeMiseScript(t *testing.T, captureFile string) string {
	t.Helper()
	script := filepath.Join(t.TempDir(), "mise")
	content := "#!/bin/sh\n" +
		"if [ \"$1\" = \"--version\" ]; then echo 2026.7.10; exit 0; fi\n" +
		fmt.Sprintf("echo \"TRUSTED=$MISE_TRUSTED_CONFIG_PATHS\" >> %q\n", captureFile) +
		fmt.Sprintf("echo \"SENTINEL=$DOTDRIFT_TEST_SENTINEL\" >> %q\n", captureFile)
	require.NoError(t, os.WriteFile(script, []byte(content), 0o755))
	return script
}

// realExecMise returns an ExecMise whose default exec path runs the fake
// script (no Run/RunContext fakes, so the real subprocess env applies).
func realExecMise(t *testing.T, script string) *ExecMise {
	t.Helper()
	return NewExecMise(&Mise{
		LookPath: func(string) (string, error) { return script, nil },
	})
}

func generatedConfig(t *testing.T) (dir, path string) {
	t.Helper()
	dir = t.TempDir()
	path = filepath.Join(dir, "mise.toml")
	require.NoError(t, os.WriteFile(path, []byte("[tools]\n"), 0o644))
	return dir, path
}

func captureLines(t *testing.T, captureFile string) []string {
	t.Helper()
	data, err := os.ReadFile(captureFile)
	require.NoError(t, err)
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

// Every ExecMise entry point that runs mise against a dotdrift-generated
// config must pass MISE_TRUSTED_CONFIG_PATHS covering the config's directory,
// or real mise rejects the config as untrusted (dogfood-found bug).
func TestExecMise_trustsGeneratedConfigDir(t *testing.T) {
	cases := []struct {
		name   string
		invoke func(ctx context.Context, em *ExecMise, cfg string) error
	}{
		{"EnsureAndInstall", func(ctx context.Context, em *ExecMise, cfg string) error {
			return em.EnsureAndInstall(ctx, cfg)
		}},
		{"DotfilesApply", func(ctx context.Context, em *ExecMise, cfg string) error {
			return em.DotfilesApply(ctx, cfg, true)
		}},
		{"RunTask", func(ctx context.Context, em *ExecMise, cfg string) error {
			return em.RunTask(ctx, cfg, "hooks:pre")
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			capture := filepath.Join(t.TempDir(), "capture")
			em := realExecMise(t, fakeMiseScript(t, capture))
			cfgDir, cfg := generatedConfig(t)

			require.NoError(t, tc.invoke(context.Background(), em, cfg))

			lines := captureLines(t, capture)
			require.Len(t, lines, 2, "fake mise must be invoked exactly once (version probe writes nothing)")
			require.Equal(t, "TRUSTED="+cfgDir, lines[0],
				"mise subprocess env must trust the generated config's directory")
		})
	}
}

// A pre-existing MISE_TRUSTED_CONFIG_PATHS value must be preserved and
// extended (colon-separated), never overwritten.
func TestExecMise_trustMergesExistingTrustedPaths(t *testing.T) {
	t.Setenv("MISE_TRUSTED_CONFIG_PATHS", "/user/trusted")

	capture := filepath.Join(t.TempDir(), "capture")
	em := realExecMise(t, fakeMiseScript(t, capture))
	cfgDir, cfg := generatedConfig(t)

	require.NoError(t, em.EnsureAndInstall(context.Background(), cfg))

	lines := captureLines(t, capture)
	require.Equal(t, "TRUSTED=/user/trusted:"+cfgDir, lines[0],
		"user's trusted paths must come first, generated dir appended")
}

// Mise.Env carries extra environment to the default exec path.
func TestMise_envAppendedOnDefaultExecPath(t *testing.T) {
	capture := filepath.Join(t.TempDir(), "capture")
	script := fakeMiseScript(t, capture)
	em := NewExecMise(&Mise{
		LookPath: func(string) (string, error) { return script, nil },
		Env:      []string{"DOTDRIFT_TEST_SENTINEL=present"},
	})
	_, cfg := generatedConfig(t)

	require.NoError(t, em.EnsureAndInstall(context.Background(), cfg))

	lines := captureLines(t, capture)
	require.Equal(t, "SENTINEL=present", lines[1])
}
