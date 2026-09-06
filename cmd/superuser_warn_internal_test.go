package cmd

import (
	"bytes"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/profile"
)

func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	orig := log.Logger
	log.Logger = zerolog.New(&buf)
	t.Cleanup(func() { log.Logger = orig })
	return &buf
}

func TestWarnSuperuserOverlays_emitsOnceWithCount(t *testing.T) {
	buf := captureLogs(t)
	p := &profile.Profile{Skipped: []profile.Skip{
		{Module: profile.Module{ID: "a"}, Reason: profile.ReasonSuperuserOverlay},
		{Module: profile.Module{ID: "b"}, Reason: profile.ReasonSuperuserOverlay},
		{Module: profile.Module{ID: "c"}, Reason: "disabled"},
	}}

	warnSuperuserOverlays(p)

	out := buf.String()
	require.Contains(t, out, "sudo")
	require.Contains(t, out, "2", "warning names the number of skipped superuser-overlay modules")
}

func TestWarnSuperuserOverlays_silentWithoutSuperuserSkips(t *testing.T) {
	buf := captureLogs(t)
	warnSuperuserOverlays(&profile.Profile{Skipped: []profile.Skip{
		{Module: profile.Module{ID: "c"}, Reason: "disabled"},
	}})
	require.Empty(t, buf.String())
}

func TestWarnMisplacedModules_emitsOnceWithCount(t *testing.T) {
	buf := captureLogs(t)
	p := &profile.Profile{Skipped: []profile.Skip{
		{Module: profile.Module{ID: "loose"}, Reason: profile.ReasonMisplacedModule + ": module belongs at users/root/modules/loose"},
		{Module: profile.Module{ID: "sup"}, Reason: profile.ReasonSuperuserOverlay},
	}}

	warnMisplacedModules(p)

	out := buf.String()
	require.Contains(t, out, "misplaced")
	require.Contains(t, out, "1")
}

func TestWarnMisplacedModules_silentWithoutMisplacedSkips(t *testing.T) {
	buf := captureLogs(t)
	warnMisplacedModules(&profile.Profile{Skipped: []profile.Skip{
		{Module: profile.Module{ID: "sup"}, Reason: profile.ReasonSuperuserOverlay},
	}})
	require.Empty(t, buf.String())
}
