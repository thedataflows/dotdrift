package packages_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/packages"
)

// streamFake implements both Runner and the streaming surface ExecRunner
// exposes, recording which path each call took.
type streamFake struct {
	runCalls    []recordedCall
	streamCalls []recordedCall
	err         error
}

func (s *streamFake) Run(_ context.Context, name string, args ...string) (string, error) {
	s.runCalls = append(s.runCalls, recordedCall{Name: name, Args: args})
	return "", s.err
}

func (s *streamFake) RunStream(_ context.Context, name string, args ...string) (string, error) {
	s.streamCalls = append(s.streamCalls, recordedCall{Name: name, Args: args})
	return "", s.err
}

// Verbose RunStream wires the child's stdout/stderr live to the configured
// writers and returns no captured output.
func TestExecRunner_RunStream_verboseStreamsToWriters(t *testing.T) {
	var out, errW bytes.Buffer
	r := packages.ExecRunner{Verbose: true, Out: &out, Err: &errW}

	got, err := r.RunStream(context.Background(), "sh", "-c", "echo out-line; echo err-line >&2")
	require.NoError(t, err)
	require.Empty(t, got, "streaming returns no captured output")
	require.Contains(t, out.String(), "out-line")
	require.Contains(t, errW.String(), "err-line")
}

// Non-verbose RunStream is byte-identical to Run: captured output, nothing
// on the writers.
func TestExecRunner_RunStream_nonVerboseBehavesLikeRun(t *testing.T) {
	var out, errW bytes.Buffer
	r := packages.ExecRunner{Verbose: false, Out: &out, Err: &errW}

	got, err := r.RunStream(context.Background(), "echo", "hello")
	require.NoError(t, err)
	require.Equal(t, "hello\n", got)
	require.Empty(t, out.String())
	require.Empty(t, errW.String())
}

// Present/Absent route through the streaming surface when the runner offers
// one, so a verbose ExecRunner can stream package-manager output live.
func TestBackend_presentAbsentUseStreamingPath(t *testing.T) {
	cases := []struct {
		name       string
		newBackend func(r *streamFake) packages.Backend
		wantStream []recordedCall
	}{
		{"Paru", func(r *streamFake) packages.Backend { return &packages.Paru{Runner: r} }, []recordedCall{
			{Name: "paru", Args: []string{"-S", "--needed", "--noconfirm", "ripgrep"}},
			{Name: "paru", Args: []string{"-R", "--noconfirm", "nano"}},
		}},
		{"Apt", func(r *streamFake) packages.Backend { return &packages.Apt{Runner: r} }, []recordedCall{
			{Name: "apt-get", Args: []string{"update"}},
			{Name: "apt-get", Args: []string{"install", "-y", "ripgrep"}},
			{Name: "apt-get", Args: []string{"remove", "-y", "nano"}},
		}},
		{"Dnf", func(r *streamFake) packages.Backend { return &packages.Dnf{Runner: r} }, []recordedCall{
			{Name: "dnf", Args: []string{"install", "-y", "ripgrep"}},
			{Name: "dnf", Args: []string{"remove", "-y", "nano"}},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &streamFake{}
			b := tc.newBackend(r)

			require.NoError(t, b.Present(context.Background(), []string{"ripgrep"}))
			require.NoError(t, b.Absent(context.Background(), []string{"nano"}))

			require.Equal(t, tc.wantStream, r.streamCalls, "install/remove must use the streaming path")
			require.Empty(t, r.runCalls, "install/remove must not use the captured probe path")
		})
	}
}

// IsInstalled is a quiet existence probe: it stays on the captured Run path
// even when the runner could stream.
func TestBackend_isInstalledStaysOnCapturedPath(t *testing.T) {
	cases := []struct {
		name       string
		newBackend func(r *streamFake) packages.Backend
		wantCall   recordedCall
	}{
		{"Paru", func(r *streamFake) packages.Backend { return &packages.Paru{Runner: r} },
			recordedCall{Name: "pacman", Args: []string{"-Q", "ripgrep"}}},
		{"Apt", func(r *streamFake) packages.Backend { return &packages.Apt{Runner: r} },
			recordedCall{Name: "dpkg", Args: []string{"-l", "ripgrep"}}},
		{"Dnf", func(r *streamFake) packages.Backend { return &packages.Dnf{Runner: r} },
			recordedCall{Name: "rpm", Args: []string{"-q", "ripgrep"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &streamFake{}
			b := tc.newBackend(r)

			_, err := b.IsInstalled(context.Background(), "ripgrep")
			require.NoError(t, err)
			require.Equal(t, []recordedCall{tc.wantCall}, r.runCalls, "probes must stay captured")
			require.Empty(t, r.streamCalls, "probes must never stream")
		})
	}
}

// SetVerbose flips the real ExecRunner inside a default backend; an injected
// fake runner is left untouched.
func TestBackend_setVerbose(t *testing.T) {
	t.Run("real runner gets verbose", func(t *testing.T) {
		paru := packages.NewParu()
		paru.SetVerbose(true)
		require.True(t, paru.Runner.(packages.ExecRunner).Verbose)

		apt := packages.NewApt()
		apt.SetVerbose(true)
		require.True(t, apt.Runner.(packages.ExecRunner).Verbose)

		dnf := packages.NewDnf()
		dnf.SetVerbose(true)
		require.True(t, dnf.Runner.(packages.ExecRunner).Verbose)
	})
	t.Run("injected fake left untouched", func(t *testing.T) {
		fake := &recordingRunner{}
		paru := &packages.Paru{Runner: fake}
		paru.SetVerbose(true)
		require.True(t, paru.Runner == packages.Runner(fake),
			"SetVerbose must not replace an injected fake runner")
	})
}
