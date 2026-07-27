package dotdrift

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/alecthomas/kong"
	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/profile"
)

// --- helpers ---------------------------------------------------------------

// stubGenerateTTY pins the isTTY seam for the duration of the test.
func stubGenerateTTY(t *testing.T, tty bool) {
	t.Helper()
	orig := isTTY
	isTTY = func() bool { return tty }
	t.Cleanup(func() { isTTY = orig })
}

// stubRunWizard records wizard invocations and returns nil.
func stubRunWizard(t *testing.T) *[]wizardInvocation {
	t.Helper()
	var calls []wizardInvocation
	orig := runWizard
	runWizard = func(inv wizardInvocation) error {
		calls = append(calls, inv)
		return nil
	}
	t.Cleanup(func() { runWizard = orig })
	return &calls
}

// generateTreeManifest returns a relpath -> sha256 map of every file under dir.
func generateTreeManifest(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		require.NoError(t, err)
		if d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		rel, err := filepath.Rel(dir, path)
		require.NoError(t, err)
		out[rel] = fmt.Sprintf("%x", sha256.Sum256(data))
		return nil
	})
	require.NoError(t, err)
	return out
}

// decodeGenerateModule reads and decodes the module.toml under dir.
func decodeGenerateModule(t *testing.T, dir string) profile.ModuleConfig {
	t.Helper()
	var cfg profile.ModuleConfig
	_, err := toml.DecodeFile(filepath.Join(dir, "module.toml"), &cfg)
	require.NoError(t, err)
	return cfg
}

// newGenerateProfile returns a temp profile root with the modules/ dir present.
func newGenerateProfile(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "modules"), 0o755))
	return root
}

// runGenerateArgs parses args through a real kong parser, injects out as the
// leaf command's Out writer, and runs the selected command.
func runGenerateArgs(t *testing.T, out *bytes.Buffer, args ...string) error {
	t.Helper()
	var cli CLI
	parser, err := kong.New(&cli, kong.Name(appName))
	require.NoError(t, err)
	kctx, err := parser.Parse(args)
	require.NoError(t, err)
	cli.Generate.Mounts.Out = out
	cli.Generate.Smb.Out = out
	return kctx.Run()
}

// --- S1: mounts happy path ---------------------------------------------------

func TestGenerateMountsCLI_happyPath(t *testing.T) {
	profileDir := newGenerateProfile(t)
	var out bytes.Buffer

	err := runGenerateArgs(t, &out,
		"generate", "mounts",
		"--profile", profileDir,
		"--layer", "host",
		"--hostname", "h1",
		"--module", "nas",
		"--name", "syn01",
		"--source", "synology.local:/volume1/syn01",
		"--destination", "/mnt/synology/syn01",
		"--type", "nfs",
		"--startat", "*-*-* 18:05:00",
	)
	require.NoError(t, err)

	dir := filepath.Join(profileDir, "hosts", "h1", "modules", "nas")
	stem := "mnt-synology-syn01" // EscapePath("/mnt/synology/syn01")
	for _, f := range []string{stem + ".mount", stem + ".service", stem + ".timer", "module.toml"} {
		require.FileExists(t, filepath.Join(dir, f), "generated file %s", f)
	}

	cfg := decodeGenerateModule(t, dir)
	require.Equal(t, profile.ScopeSystem, cfg.Scope)
	require.Equal(t, profile.MountSpec{
		Source:      "synology.local:/volume1/syn01",
		Destination: "/mnt/synology/syn01",
		Type:        "nfs",
		StartAt:     "*-*-* 18:05:00",
		State:       "enabled",
	}, cfg.Mounts["syn01"], "[mounts.syn01] records the full spec")

	// Dotfile entries place the units under the system systemd dir.
	for _, f := range []string{stem + ".mount", stem + ".service", stem + ".timer"} {
		df, ok := cfg.Dotfiles["/usr/local/lib/systemd/system/"+f]
		require.True(t, ok, "dotfile entry for %s", f)
		require.Equal(t, profile.Dotfile{Source: f, Mode: "copy"}, df)
	}
	require.Contains(t, cfg.Packages.Present, "nfs-utils")
	require.Contains(t, cfg.Packages.Present, "nfsidmap")

	// The summary names the module dir and the written files.
	require.Contains(t, out.String(), filepath.Join("hosts", "h1", "modules", "nas"))
	require.Contains(t, out.String(), stem+".mount")
	require.Contains(t, out.String(), "module.toml")
}

func TestGenerateMountsCLI_missingDestinationLoudError(t *testing.T) {
	cmd := &GenerateMountsCmd{
		Profile: newGenerateProfile(t),
		Name:    "syn01",
		Source:  "synology.local:/volume1/syn01",
		Type:    "nfs",
	}
	err := cmd.Run()
	require.Error(t, err)
	require.Contains(t, err.Error(), "--destination", "error must name the missing flag")
}

// CLI-level S2: the same command run twice produces a byte-identical tree.
func TestGenerateMountsCLI_rerunByteIdentical(t *testing.T) {
	profileDir := newGenerateProfile(t)
	run := func() {
		cmd := &GenerateMountsCmd{
			Profile:     profileDir,
			Layer:       "base",   // kong default; set explicitly on direct construction
			Module:      "mounts", // kong default; set explicitly on direct construction
			Name:        "syn01",
			Source:      "synology.local:/volume1/syn01",
			Destination: "/mnt/synology/syn01",
			Type:        "nfs",
			StartAt:     "*-*-* 18:05:00",
			Out:         &bytes.Buffer{},
		}
		require.NoError(t, cmd.Run())
	}
	run()
	first := generateTreeManifest(t, filepath.Join(profileDir, "modules", "mounts"))
	run()
	second := generateTreeManifest(t, filepath.Join(profileDir, "modules", "mounts"))
	require.Equal(t, first, second, "same CLI invocation must produce a byte-identical module tree")
}

// --- S1: smb happy path ------------------------------------------------------

func TestGenerateSmbCLI_happyPath(t *testing.T) {
	profileDir := newGenerateProfile(t)
	var out bytes.Buffer

	err := runGenerateArgs(t, &out,
		"generate", "smb",
		"--profile", profileDir,
		"--share", "media=/srv/media",
		"--share", "data=/mnt/data",
		"--user", "cri",
		"--no-avahi",
	)
	require.NoError(t, err)

	dir := filepath.Join(profileDir, "modules", "smb")
	sharesConf, err := os.ReadFile(filepath.Join(dir, "shares.conf"))
	require.NoError(t, err)
	dataIdx := strings.Index(string(sharesConf), "[data]")
	mediaIdx := strings.Index(string(sharesConf), "[media]")
	require.NotEqual(t, -1, dataIdx, "shares.conf carries the data stanza")
	require.NotEqual(t, -1, mediaIdx, "shares.conf carries the media stanza")
	require.Less(t, dataIdx, mediaIdx, "stanzas are sorted by share name")

	smbConf, err := os.ReadFile(filepath.Join(dir, "smb.conf"))
	require.NoError(t, err)
	require.Contains(t, string(smbConf), "include = /etc/samba/smb.conf.d/shares.conf",
		"smb.conf seed wires the generated shares.conf")

	cfg := decodeGenerateModule(t, dir)
	require.Equal(t, profile.ScopeSystem, cfg.Scope)
	require.NotNil(t, cfg.Smb.Avahi, "--no-avahi records an explicit avahi=false")
	require.False(t, *cfg.Smb.Avahi)
	require.Equal(t, "smb", cfg.Smb.Group, "group defaults to smb")
	require.Equal(t, []string{"cri"}, cfg.Smb.Users)
	require.True(t, cfg.Smb.Shares["media"].Writable, "shares are writable by default")
	require.Equal(t, "/srv/media", cfg.Smb.Shares["media"].Path)
	require.Contains(t, cfg.Packages.Present, "samba")
	require.NotContains(t, cfg.Packages.Present, "avahi", "avahi package only when avahi enabled")

	require.Contains(t, out.String(), "shares.conf")
}

func TestGenerateSmbCLI_shareFlagParsing(t *testing.T) {
	for _, bad := range []string{"noequals", "=/path", "name="} {
		t.Run(bad, func(t *testing.T) {
			cmd := &GenerateSmbCmd{
				Profile: newGenerateProfile(t),
				Shares:  []string{bad},
			}
			err := cmd.Run()
			require.Error(t, err)
			require.Contains(t, err.Error(), "--share")
			require.Contains(t, err.Error(), bad, "error must name the offending value")
		})
	}

	t.Run("no shares at all", func(t *testing.T) {
		cmd := &GenerateSmbCmd{
			Profile: newGenerateProfile(t),
			Group:   "media", // an input flag: forces CLI mode
		}
		err := cmd.Run()
		require.Error(t, err)
		require.Contains(t, err.Error(), "--share", "CLI mode requires at least one share")
	})
}

// --- mode selection ----------------------------------------------------------

func TestGenerate_noFlagsNoTTY_helpfulError(t *testing.T) {
	stubGenerateTTY(t, false)

	t.Run("mounts", func(t *testing.T) {
		cmd := &GenerateMountsCmd{Profile: newGenerateProfile(t)}
		err := cmd.Run()
		require.Error(t, err)
		for _, flag := range []string{"--name", "--source", "--destination", "--type"} {
			require.Contains(t, err.Error(), flag, "error must name required flag %s", flag)
		}
	})

	t.Run("smb", func(t *testing.T) {
		cmd := &GenerateSmbCmd{Profile: newGenerateProfile(t)}
		err := cmd.Run()
		require.Error(t, err)
		require.Contains(t, err.Error(), "--share")
	})
}

func TestGenerate_tuiFlagNoTTY_errors(t *testing.T) {
	stubGenerateTTY(t, false)
	tui := true
	cmd := &GenerateMountsCmd{Profile: newGenerateProfile(t), TUI: &tui}
	err := cmd.Run()
	require.Error(t, err)
	require.Contains(t, err.Error(), "--tui")
	require.Contains(t, err.Error(), "terminal")
}

func TestGenerate_zeroFlagsTTY_invokesWizardSeam(t *testing.T) {
	stubGenerateTTY(t, true)
	calls := stubRunWizard(t)

	t.Run("mounts", func(t *testing.T) {
		cmd := &GenerateMountsCmd{Profile: newGenerateProfile(t)}
		require.NoError(t, cmd.Run())
		require.Len(t, *calls, 1)
		require.Same(t, cmd, (*calls)[0].Mounts, "the wizard receives the parsed command")
		require.Nil(t, (*calls)[0].Smb)
	})

	t.Run("smb", func(t *testing.T) {
		*calls = nil
		cmd := &GenerateSmbCmd{Profile: newGenerateProfile(t)}
		require.NoError(t, cmd.Run())
		require.Len(t, *calls, 1)
		require.Same(t, cmd, (*calls)[0].Smb)
		require.Nil(t, (*calls)[0].Mounts)
	})

	t.Run("tui flag forces wizard with input flags for pre-fill", func(t *testing.T) {
		*calls = nil
		tui := true
		cmd := &GenerateMountsCmd{
			Profile:     newGenerateProfile(t),
			TUI:         &tui,
			Name:        "syn01",
			Destination: "/mnt/synology/syn01",
		}
		require.NoError(t, cmd.Run())
		require.Len(t, *calls, 1, "--tui forces the wizard even when input flags are present")
		require.Equal(t, "syn01", (*calls)[0].Mounts.Name, "input flags pass through for pre-fill")
	})
}

// --- --list-volumes ----------------------------------------------------------

func TestGenerate_listVolumes_printsTable(t *testing.T) {
	if _, err := exec.LookPath("lsblk"); err != nil {
		t.Skip("lsblk not available on this machine")
	}
	var out bytes.Buffer
	cmd := &GenerateMountsCmd{
		Profile:     newGenerateProfile(t),
		Layer:       "base",   // kong default; set explicitly on direct construction
		Module:      "mounts", // kong default; set explicitly on direct construction
		ListVolumes: true,
		Out:         &out,
	}
	require.NoError(t, cmd.Run())
	header, _, _ := strings.Cut(out.String(), "\n")
	for _, col := range []string{"UUID", "FSTYPE", "LABEL", "SIZE", "MOUNTPOINTS", "MANAGED"} {
		require.Contains(t, header, col, "volume table header carries %s", col)
	}
}

// --- registration ------------------------------------------------------------

func TestGenerate_registered(t *testing.T) {
	for _, args := range [][]string{
		{"generate", "--help"},
		{"generate", "mounts", "--help"},
		{"generate", "smb", "--help"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			require.NoError(t, Run("dev", args), "%v must be registered", args)
		})
	}
}
