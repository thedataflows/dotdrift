package profile_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/profile"
)

// The [mounts] and [smb] sections of module.toml parse into the new spec
// structs, including the startat and valid_users TOML key spellings.
func TestProfile_mountsSmbParse(t *testing.T) {
	dir := t.TempDir()
	moduleTOML := `
[mounts.data]
source = "UUID=abcd-1234"
destination = "/mnt/data"
type = "ext4"
options = ["noatime", "nofail"]
startat = "18:00"
state = "enabled"

[smb]
group = "WORKGROUP"
users = ["cri", "media"]
avahi = true

[smb.shares.media]
path = "/mnt/data/media"
comment = "Media library"
valid_users = "cri, media"
writable = true
public = false
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "module.toml"), []byte(moduleTOML), 0o644))

	cfg, err := profile.LoadModuleConfig(dir)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	require.Len(t, cfg.Mounts, 1)
	m := cfg.Mounts["data"]
	require.Equal(t, "UUID=abcd-1234", m.Source)
	require.Equal(t, "/mnt/data", m.Destination)
	require.Equal(t, "ext4", m.Type)
	require.Equal(t, []string{"noatime", "nofail"}, m.Options)
	require.Equal(t, "18:00", m.StartAt, "startat TOML key must map to StartAt")
	require.Equal(t, "enabled", m.State)

	require.Equal(t, "WORKGROUP", cfg.Smb.Group)
	require.Equal(t, []string{"cri", "media"}, cfg.Smb.Users)
	require.NotNil(t, cfg.Smb.Avahi, "explicit avahi = true must be distinguishable from unset")
	require.True(t, *cfg.Smb.Avahi)

	require.Len(t, cfg.Smb.Shares, 1)
	s := cfg.Smb.Shares["media"]
	require.Equal(t, "/mnt/data/media", s.Path)
	require.Equal(t, "Media library", s.Comment)
	require.Equal(t, "cri, media", s.ValidUsers, "valid_users TOML key must map to ValidUsers")
	require.True(t, s.Writable)
	require.False(t, s.Public)
}

// An omitted avahi key stays nil so downstream consumers can apply the
// documented default (true); nil is never confused with an explicit false.
func TestProfile_smbAvahiUnsetIsNil(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "module.toml"), []byte("[smb]\ngroup = \"WG\"\n"), 0o644))

	cfg, err := profile.LoadModuleConfig(dir)
	require.NoError(t, err)
	require.Equal(t, "WG", cfg.Smb.Group)
	require.Nil(t, cfg.Smb.Avahi, "unset avahi must decode to nil, not a defaulted bool")
}

// A module.toml without [mounts]/[smb] yields empty spec sections.
func TestProfile_noMountsNoSmb_emptySections(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "module.toml"), []byte("[packages]\npresent = [\"vim\"]\n"), 0o644))

	cfg, err := profile.LoadModuleConfig(dir)
	require.NoError(t, err)
	require.Empty(t, cfg.Mounts)
	require.Empty(t, cfg.Smb.Group)
	require.Empty(t, cfg.Smb.Users)
	require.Nil(t, cfg.Smb.Avahi)
	require.Empty(t, cfg.Smb.Shares)
}
