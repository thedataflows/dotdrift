package profile_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/profile"
)

// [systemd.units.<name>] parses as a passthrough directive table (issue
// 0048): strings, bools, numbers, arrays, and inline tables all survive
// decode with their TOML types intact.
func TestLoadModuleTOML_systemdUnits(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/svc", `
[systemd.units.my-sync]
description = "sync files"
exec_start = "~/.local/bin/my-sync --watch"
after = ["network-online.target"]
environment = { PATH = "/usr/local/bin:/usr/bin" }
nice = 10
private_tmp = true
start = false

[systemd.units.healthcheck-timer]
on_boot_sec = "2min"
on_calendar = "daily"
persistent = true
unit = "healthcheck"
`)
	p, err := profile.Load(root, &facts.Facts{})
	require.NoError(t, err)
	m := findModule(t, p, "svc")

	svc := m.Config.Systemd.Units["my-sync"]
	require.Equal(t, "sync files", svc["description"])
	require.Equal(t, "~/.local/bin/my-sync --watch", svc["exec_start"])
	require.Equal(t, []any{"network-online.target"}, svc["after"])
	require.Equal(t, map[string]any{"PATH": "/usr/local/bin:/usr/bin"}, svc["environment"])
	require.Equal(t, int64(10), svc["nice"])
	require.Equal(t, true, svc["private_tmp"])
	require.Equal(t, false, svc["start"])

	timer := m.Config.Systemd.Units["healthcheck-timer"]
	require.Equal(t, "2min", timer["on_boot_sec"])
	require.Equal(t, true, timer["persistent"])
}

// Strict schema (contract #19): unknown keys in [systemd] itself — a typo'd
// table or a stray key — are load-time errors, even though unit directive
// keys are passthrough.
func TestStrictModuleTOML_systemdUnknownKey(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/svc", `
[systemd]
unit = {}
`)
	_, err := profile.Load(root, &facts.Facts{})
	require.Error(t, err)
	require.Contains(t, err.Error(), `unknown key "unit"`)
}

func TestStrictModuleTOML_systemdUnknownTable(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/svc", `
[systemd.unitz.foo]
exec_start = "/bin/true"
`)
	_, err := profile.Load(root, &facts.Facts{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unitz")
}
