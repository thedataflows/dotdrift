package drift_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/drift"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/resolve"
)

func servicePlan(entries ...resolve.ServiceEntry) *resolve.Plan {
	return &resolve.Plan{Services: resolve.ServicesStep{Entries: entries}}
}

func boolPtr(b bool) *bool { return &b }

// A declared user service probes systemctl --user for the unit itself;
// a command service probes the dev.mise unit (issue 0159).
func TestCheck_services(t *testing.T) {
	enabled := boolPtr(true)
	plan := servicePlan(
		resolve.ServiceEntry{Name: "syncthing", Module: "svc", Scope: "user", Spec: profile.ServiceSpec{State: "running", Enabled: enabled}},
		resolve.ServiceEntry{Name: "agent", Module: "svc", Scope: "user", Spec: profile.ServiceSpec{Command: "/bin/agent", State: "running", Enabled: enabled}},
	)
	pr := fakeProbes()
	var calls []string
	pr.Run = func(_ context.Context, name string, args ...string) (string, error) {
		calls = append(calls, name+" "+args[len(args)-1])
		verb := args[len(args)-2]
		switch verb {
		case "is-enabled":
			return "enabled", nil
		case "is-active":
			return "active", nil
		}
		return "", errors.New("unexpected")
	}
	fs := drift.Check(context.Background(), plan, t.TempDir(), pr, drift.CheckOptions{})
	require.Len(t, fs, 2)
	for _, f := range fs {
		require.Equal(t, "services", f.Section)
		require.Equal(t, drift.OK, f.Status, f)
	}
	require.Contains(t, calls, "systemctl syncthing.service")
	require.Contains(t, calls, "systemctl dev.mise.agent.service")
}

// A service whose unit is not enabled reports drift.
func TestCheck_servicesNotEnabled(t *testing.T) {
	plan := servicePlan(
		resolve.ServiceEntry{Name: "syncthing", Module: "svc", Scope: "user", Spec: profile.ServiceSpec{State: "running", Enabled: boolPtr(true)}},
	)
	pr := fakeProbes()
	pr.Run = func(_ context.Context, _ string, args ...string) (string, error) {
		if args[len(args)-2] == "is-enabled" {
			return "disabled", nil
		}
		return "inactive", nil
	}
	fs := drift.Check(context.Background(), plan, t.TempDir(), pr, drift.CheckOptions{})
	require.Len(t, fs, 1)
	require.Equal(t, drift.Drift, fs[0].Status)
	require.Equal(t, "not enabled", fs[0].Detail)
}
