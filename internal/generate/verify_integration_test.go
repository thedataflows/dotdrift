//go:build integration

package generate

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/profile"
)

// Integration validators: rendered artifacts are checked against the real
// system validators (systemd-analyze verify, testparm), skipping cleanly
// when the tools are absent. Goldens prove intent; these prove validity.

func whereOf(t *testing.T, unit string) string {
	t.Helper()
	for _, line := range strings.Split(unit, "\n") {
		if where, ok := strings.CutPrefix(line, "Where="); ok {
			return where
		}
	}
	t.Fatalf("no Where= line in unit:\n%s", unit)
	return ""
}

func verifyUnit(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	cmd := exec.Command("systemd-analyze", "verify", path)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "systemd-analyze verify %s: %s", name, out)
}

// Every golden unit must pass systemd-analyze verify when written under
// its proper systemd unit name (derived from Where= via EscapePath).
func TestIntegration_systemdAnalyzeVerify_goldenUnits(t *testing.T) {
	if _, err := exec.LookPath("systemd-analyze"); err != nil {
		t.Skip("systemd-analyze not available")
	}
	goldenDir := filepath.Join("testdata", "golden")
	entries, err := os.ReadDir(goldenDir)
	require.NoError(t, err)

	families := map[string]map[string]string{} // prefix -> suffix -> file
	for _, e := range entries {
		prefix, suffix, ok := strings.Cut(e.Name(), ".")
		if !ok || e.IsDir() {
			continue
		}
		if families[prefix] == nil {
			families[prefix] = map[string]string{}
		}
		families[prefix][suffix] = e.Name()
	}

	for prefix, units := range families {
		mountFile, ok := units["mount"]
		if !ok {
			continue
		}
		t.Run(prefix, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(goldenDir, mountFile))
			require.NoError(t, err)
			stem := EscapePath(whereOf(t, string(data)))
			dir := t.TempDir()
			for suffix, file := range units {
				content, err := os.ReadFile(filepath.Join(goldenDir, file))
				require.NoError(t, err)
				verifyUnit(t, dir, stem+"."+suffix, string(content))
			}
		})
	}
}

// The raw-content/escaped-name contract: unit CONTENT paths stay raw while
// the unit NAME is escaped — verified against live systemd (an escaped
// Where= is rejected as not matching the unescaped unit name).
func TestIntegration_systemdAnalyzeVerify_spacePathContract(t *testing.T) {
	if _, err := exec.LookPath("systemd-analyze"); err != nil {
		t.Skip("systemd-analyze not available")
	}
	noOverride(t)
	reg, err := Load()
	require.NoError(t, err)
	preset, ok := reg.Entry("btrfs")
	require.True(t, ok)

	spec := profile.MountSpec{Source: "UUID=1234", Destination: "/mnt/My Disk", Type: "btrfs"}
	content := RenderMountUnit("disk", spec, preset, 1000, 1000)
	dir := t.TempDir()
	verifyUnit(t, dir, EscapePath(spec.Destination)+".mount", content)

	// Negative control: escaping the content path must FAIL verification —
	// this locks the contract against a future "fix" that escapes content.
	escaped := strings.ReplaceAll(content, "/mnt/My Disk", `/mnt/My\x20Disk`)
	path := filepath.Join(dir, "negative.mount")
	require.NoError(t, os.WriteFile(path, []byte(escaped), 0o644))
	out, err := exec.Command("systemd-analyze", "verify", path).CombinedOutput()
	require.Error(t, err, "escaped content path must be rejected: %s", out)
	require.Contains(t, string(out), "doesn't match unit name")
}

// The golden shares.conf must parse under the real testparm.
func TestIntegration_testparm_sharesConf(t *testing.T) {
	if _, err := exec.LookPath("testparm"); err != nil {
		t.Skip("testparm not available")
	}
	path := filepath.Join("testdata", "golden", "shares.conf.golden")
	out, err := exec.Command("testparm", "-s", path).CombinedOutput()
	require.NoError(t, err, "testparm -s shares.conf.golden: %s", out)
	require.Contains(t, string(out), "Loaded services file OK")
}
