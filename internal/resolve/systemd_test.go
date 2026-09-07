package resolve_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
)

// Units merge whole-entry by name, nearer layer wins (user > host > base) —
// same contract as mounts (issue 0048).
func TestResolve_systemdMergeLayers(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "svc", `
[systemd.units.my-sync]
exec_start = "/usr/bin/base-sync"
restart = "on-failure"

[systemd.units.timer]
on_calendar = "daily"
`)
	writeOverlayModule(t, root, "users", "cri", "svc", `
[systemd.units.my-sync]
exec_start = "/usr/bin/user-sync"
`)

	plan, err := loadAndResolve(t, root, &facts.Facts{Hostname: "h", Username: "cri"})
	require.NoError(t, err)
	require.Len(t, plan.Systemd.Units, 2)
	byName := map[string]map[string]any{}
	layers := map[string]string{}
	for _, u := range plan.Systemd.Units {
		byName[u.Name] = u.Directives
		layers[u.Name] = u.Layer
	}
	require.Equal(t, "/usr/bin/user-sync", byName["my-sync"]["exec_start"],
		"user overlay replaces the whole unit entry")
	require.NotContains(t, byName["my-sync"], "restart",
		"whole-entry replacement: base keys do not leak through")
	require.Equal(t, "user", layers["my-sync"])
	require.Equal(t, "base", layers["timer"])
}

// User units are inherently user-scope: a scope = "system" module declaring
// units is a resolve-time error naming the module (mise skips user units
// under sudo — a system-scope declaration would silently no-op).
func TestResolve_systemdSystemScopeErrors(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "svc", `
scope = "system"

[systemd.units.my-sync]
exec_start = "/usr/bin/my-sync"
`)
	_, err := loadAndResolve(t, root, &facts.Facts{Hostname: "h", Username: "cri"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "svc")
	require.Contains(t, err.Error(), "systemd")
}

// Structural validation mirrors mise (issue 0048): a service needs a
// non-empty exec_start; a timer needs at least one trigger and must not set
// service-only directives; names follow systemd's charset.
func TestResolve_systemdValidation(t *testing.T) {
	cases := []struct {
		name    string
		toml    string
		wantErr string
	}{
		{"service without exec_start", `[systemd.units.svc]
description = "no exec"`, "exec_start"},
		{"timer without trigger", `[systemd.units.t]
persistent = true`, "on_boot_sec"},
		{"timer with service key", `[systemd.units.t]
on_calendar = "daily"
exec_start = "/bin/true"`, "service-only"},
		{"bad name", `[systemd.units."has space"]
exec_start = "/bin/true"`, "letters"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeModule(t, root, "m", tc.toml+"\n")
			_, err := loadAndResolve(t, root, &facts.Facts{Hostname: "h", Username: "cri"})
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

// A valid service and timer resolve with their kinds derived.
func TestResolve_systemdKinds(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "m", `
[systemd.units.svc]
exec_start = "/bin/true"

[systemd.units.t]
on_boot_sec = "1min"
`)
	plan, err := loadAndResolve(t, root, &facts.Facts{Hostname: "h", Username: "cri"})
	require.NoError(t, err)
	kinds := map[string]string{}
	for _, u := range plan.Systemd.Units {
		kinds[u.Name] = u.Kind
	}
	require.Equal(t, "service", kinds["svc"])
	require.Equal(t, "timer", kinds["t"])
}
