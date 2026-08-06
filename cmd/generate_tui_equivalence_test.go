package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/generate"
	"github.com/thedataflows/dotdrift/internal/tui"
)

// S5: the wizard's spec assembly and the CLI assembly are the same code,
// so the same logical inputs must yield a byte-identical module tree.

func readTreeFile(dir, name string) (string, error) {
	data, err := os.ReadFile(filepath.Join(dir, name))
	return string(data), err
}

func TestGenerate_cliTuiEquivalence_mounts(t *testing.T) {
	uid, gid, _, err := tui.InvokingUser()
	require.NoError(t, err)

	// CLI path: the happy-path invocation from TestGenerateMountsCLI_happyPath.
	cliProfile := newGenerateProfile(t)
	var out bytes.Buffer
	require.NoError(t, runGenerateArgs(t, &out,
		"generate", "mounts",
		"--profile", cliProfile,
		"--layer", "host",
		"--hostname", "h1",
		"--module", "nas",
		"--name", "syn01",
		"--source", "synology.local:/volume1/syn01",
		"--destination", "/mnt/synology/syn01",
		"--type", "nfs",
		"--startat", "*-*-* 18:05:00",
	))

	// Wizard path: the state machine fed the same logical choices.
	wizProfile := newGenerateProfile(t)
	reg, err := generate.Load()
	require.NoError(t, err)
	w := tui.NewMountsWizard(generate.Selection{
		Layer: generate.LayerHost, Hostname: "h1", ModuleID: "nas",
	}, reg)
	require.NoError(t, w.AddMount(tui.MountChoice{
		Name:        "syn01",
		Source:      "synology.local:/volume1/syn01",
		Destination: "/mnt/synology/syn01",
		Type:        "nfs",
		StartAt:     "*-*-* 18:05:00",
	}))
	require.NoError(t, w.Write(wizProfile, uid, gid))

	cliTree := generateTreeManifest(t, filepath.Join(cliProfile, "hosts", "h1", "modules", "nas"))
	wizTree := generateTreeManifest(t, filepath.Join(wizProfile, "hosts", "h1", "modules", "nas"))
	require.Equal(t, cliTree, wizTree,
		"CLI and wizard assemblies must produce a byte-identical module tree")
}

func TestGenerate_cliTuiEquivalence_smb(t *testing.T) {
	uid, gid, username, err := tui.InvokingUser()
	require.NoError(t, err)

	// CLI path: the happy-path invocation from TestGenerateSmbCLI_happyPath.
	cliProfile := newGenerateProfile(t)
	var out bytes.Buffer
	require.NoError(t, runGenerateArgs(t, &out,
		"generate", "smb",
		"--profile", cliProfile,
		"--share", "media=/srv/media",
		"--share", "data=/mnt/data",
		"--user", "cri",
		"--no-avahi",
	))

	// Wizard path: same choices through the state machine; avahi
	// answered "no" records an explicit false, matching --no-avahi.
	wizProfile := newGenerateProfile(t)
	avahi := false
	sw := tui.NewSmbWizard(generate.Selection{Layer: generate.LayerBase, ModuleID: "smb"})
	sw.SetServer(tui.SmbServerChoice{
		Group: "smb",
		Users: []string{"cri"},
		Avahi: &avahi,
	})
	require.NoError(t, sw.AddShare(tui.ShareChoice{Name: "media", Path: "/srv/media", Writable: true}))
	require.NoError(t, sw.AddShare(tui.ShareChoice{Name: "data", Path: "/mnt/data", Writable: true}))
	require.NoError(t, sw.Write(wizProfile, username, uid, gid))

	cliTree := generateTreeManifest(t, filepath.Join(cliProfile, "modules", "smb"))
	wizTree := generateTreeManifest(t, filepath.Join(wizProfile, "modules", "smb"))
	require.Equal(t, cliTree, wizTree,
		"CLI and wizard assemblies must produce a byte-identical module tree")
}

// The wizard default-avahi answer ("yes") must match the CLI default
// (avahi key unset), not an explicit true.
func TestGenerate_cliTuiEquivalence_smbDefaultAvahi(t *testing.T) {
	uid, gid, username, err := tui.InvokingUser()
	require.NoError(t, err)

	cliProfile := newGenerateProfile(t)
	var out bytes.Buffer
	require.NoError(t, runGenerateArgs(t, &out,
		"generate", "smb",
		"--profile", cliProfile,
		"--share", "media=/srv/media",
	))

	wizProfile := newGenerateProfile(t)
	sw := tui.NewSmbWizard(generate.Selection{Layer: generate.LayerBase, ModuleID: "smb"})
	sw.SetServer(tui.SmbServerChoice{Group: "smb", Users: []string{username}})
	require.NoError(t, sw.AddShare(tui.ShareChoice{Name: "media", Path: "/srv/media", Writable: true}))
	require.NoError(t, sw.Write(wizProfile, username, uid, gid))

	cliTree := generateTreeManifest(t, filepath.Join(cliProfile, "modules", "smb"))
	wizTree := generateTreeManifest(t, filepath.Join(wizProfile, "modules", "smb"))
	require.Equal(t, cliTree, wizTree)
}

// Volume-kind mounts are the only presets carrying bare uid/gid tokens;
// both assemblies must expand them identically into the rendered units.
func TestGenerate_cliTuiEquivalence_volumeKindUidGid(t *testing.T) {
	uid, gid, _, err := tui.InvokingUser()
	require.NoError(t, err)

	cliProfile := newGenerateProfile(t)
	var out bytes.Buffer
	require.NoError(t, runGenerateArgs(t, &out,
		"generate", "mounts",
		"--profile", cliProfile,
		"--module", "vol",
		"--name", "data",
		"--source", "UUID=7FFD-B6CF",
		"--destination", "/mnt/ventoy",
		"--type", "exfat",
	))

	wizProfile := newGenerateProfile(t)
	reg, err := generate.Load()
	require.NoError(t, err)
	w := tui.NewMountsWizard(generate.Selection{Layer: generate.LayerBase, ModuleID: "vol"}, reg)
	require.NoError(t, w.AddMount(tui.MountChoice{
		Name:        "data",
		Source:      "UUID=7FFD-B6CF",
		Destination: "/mnt/ventoy",
		Type:        "exfat",
	}))
	require.NoError(t, w.Write(wizProfile, uid, gid))

	cliTree := generateTreeManifest(t, filepath.Join(cliProfile, "modules", "vol"))
	wizTree := generateTreeManifest(t, filepath.Join(wizProfile, "modules", "vol"))
	require.Equal(t, cliTree, wizTree)

	unit, err := readTreeFile(filepath.Join(cliProfile, "modules", "vol"), "mnt-ventoy.mount")
	require.NoError(t, err)
	require.Contains(t, unit, "uid=", "volume preset must expand the uid token")
	require.Contains(t, unit, "gid=", "volume preset must expand the gid token")
}

// Custom options (--option) must record identically from both assemblies.
func TestGenerate_cliTuiEquivalence_customOptions(t *testing.T) {
	uid, gid, _, err := tui.InvokingUser()
	require.NoError(t, err)

	cliProfile := newGenerateProfile(t)
	var out bytes.Buffer
	require.NoError(t, runGenerateArgs(t, &out,
		"generate", "mounts",
		"--profile", cliProfile,
		"--module", "vol",
		"--name", "data",
		"--source", "UUID=7FFD-B6CF",
		"--destination", "/mnt/ventoy",
		"--type", "exfat",
		"--option", "rw", "--option", "uid", "--option", "gid", "--option", "noatime",
	))

	wizProfile := newGenerateProfile(t)
	reg, err := generate.Load()
	require.NoError(t, err)
	w := tui.NewMountsWizard(generate.Selection{Layer: generate.LayerBase, ModuleID: "vol"}, reg)
	require.NoError(t, w.AddMount(tui.MountChoice{
		Name:        "data",
		Source:      "UUID=7FFD-B6CF",
		Destination: "/mnt/ventoy",
		Type:        "exfat",
		Options:     []string{"rw", "uid", "gid", "noatime"},
	}))
	require.NoError(t, w.Write(wizProfile, uid, gid))

	cliTree := generateTreeManifest(t, filepath.Join(cliProfile, "modules", "vol"))
	wizTree := generateTreeManifest(t, filepath.Join(wizProfile, "modules", "vol"))
	require.Equal(t, cliTree, wizTree)
}

// --state disabled must record identically from both assemblies.
func TestGenerate_cliTuiEquivalence_stateDisabled(t *testing.T) {
	uid, gid, _, err := tui.InvokingUser()
	require.NoError(t, err)

	cliProfile := newGenerateProfile(t)
	var out bytes.Buffer
	require.NoError(t, runGenerateArgs(t, &out,
		"generate", "mounts",
		"--profile", cliProfile,
		"--module", "vol",
		"--name", "data",
		"--source", "UUID=7FFD-B6CF",
		"--destination", "/mnt/ventoy",
		"--type", "exfat",
		"--state", "disabled",
	))

	wizProfile := newGenerateProfile(t)
	reg, err := generate.Load()
	require.NoError(t, err)
	w := tui.NewMountsWizard(generate.Selection{Layer: generate.LayerBase, ModuleID: "vol"}, reg)
	require.NoError(t, w.AddMount(tui.MountChoice{
		Name:        "data",
		Source:      "UUID=7FFD-B6CF",
		Destination: "/mnt/ventoy",
		Type:        "exfat",
		State:       "disabled",
	}))
	require.NoError(t, w.Write(wizProfile, uid, gid))

	cliTree := generateTreeManifest(t, filepath.Join(cliProfile, "modules", "vol"))
	wizTree := generateTreeManifest(t, filepath.Join(wizProfile, "modules", "vol"))
	require.Equal(t, cliTree, wizTree)
}

// --readonly must record writable = no identically from both assemblies.
func TestGenerate_cliTuiEquivalence_smbReadonly(t *testing.T) {
	uid, gid, username, err := tui.InvokingUser()
	require.NoError(t, err)

	cliProfile := newGenerateProfile(t)
	var out bytes.Buffer
	require.NoError(t, runGenerateArgs(t, &out,
		"generate", "smb",
		"--profile", cliProfile,
		"--share", "media=/srv/media",
		"--user", "cri",
		"--readonly",
	))

	wizProfile := newGenerateProfile(t)
	sw := tui.NewSmbWizard(generate.Selection{Layer: generate.LayerBase, ModuleID: "smb"})
	sw.SetServer(tui.SmbServerChoice{Group: "smb", Users: []string{"cri"}})
	require.NoError(t, sw.AddShare(tui.ShareChoice{Name: "media", Path: "/srv/media", Writable: false}))
	require.NoError(t, sw.Write(wizProfile, username, uid, gid))

	cliTree := generateTreeManifest(t, filepath.Join(cliProfile, "modules", "smb"))
	wizTree := generateTreeManifest(t, filepath.Join(wizProfile, "modules", "smb"))
	require.Equal(t, cliTree, wizTree)

	conf, err := readTreeFile(filepath.Join(cliProfile, "modules", "smb"), "shares.conf")
	require.NoError(t, err)
	require.Contains(t, conf, "writable = no")
}
