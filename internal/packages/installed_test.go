package packages_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/packages"
)

// outRunner is a fakeRunner whose Run returns canned stdout (fakeRunner in
// packages_test.go returns "" always; Installed must parse real output).
type outRunner struct {
	calls []call
	out   string
	err   error
}

func (f *outRunner) Run(_ context.Context, name string, args ...string) (string, error) {
	f.calls = append(f.calls, call{name: name, args: args})
	return f.out, f.err
}

// Installed lists every installed package name — the regex support behind
// when.packages entries like "apollo.*" (one list query instead of per-name
// IsInstalled probes).
func TestBackends_Installed(t *testing.T) {
	tests := []struct {
		name    string
		backend packages.Backend
		runner  *outRunner
		wantArg []string
	}{
		{"paru", &packages.Paru{}, nil, []string{"-Qq"}},
		{"apt", &packages.Apt{}, nil, []string{"-W", "-f", "${Package}\n"}},
		{"dnf", &packages.Dnf{}, nil, []string{"-qa", "--qf", "%{NAME}\n"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &outRunner{out: "apollo\napollo-cuda-git\n\nripgrep\n"}
			switch b := tt.backend.(type) {
			case *packages.Paru:
				b.Runner = f
			case *packages.Apt:
				b.Runner = f
			case *packages.Dnf:
				b.Runner = f
			}
			got, err := tt.backend.Installed(context.Background())
			require.NoError(t, err)
			require.Equal(t, []string{"apollo", "apollo-cuda-git", "ripgrep"}, got)
			require.Equal(t, f.calls[0].name, map[string]string{
				"paru": "pacman", "apt": "dpkg-query", "dnf": "rpm",
			}[tt.name])
			require.Equal(t, tt.wantArg, f.calls[0].args)
		})
	}
}

// A failing list query is an error — callers fail the filter open.
func TestParu_Installed_errorPropagates(t *testing.T) {
	f := &outRunner{err: context.Canceled}
	b := &packages.Paru{Runner: f}
	_, err := b.Installed(context.Background())
	require.Error(t, err)
}
