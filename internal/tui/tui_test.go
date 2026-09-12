package tui

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/generate"
)

func TestWizard_prefillFromFlags(t *testing.T) {
	p := MountsParams{
		Profile:     "/tmp/prof",
		Layer:       "host",
		ModuleID:    "nas",
		Hostname:    "h1",
		Name:        "syn01",
		Source:      "synology.local:/volume1/syn01",
		Destination: "/mnt/synology/syn01",
		Type:        "nfs",
		Options:     []string{"rw", "soft"},
		StartAt:     "*-*-* 18:05:00",
		State:       "disabled",
	}
	pre := p.Prefill()
	require.Equal(t, generate.MountChoice{
		Name:        "syn01",
		Source:      "synology.local:/volume1/syn01",
		Destination: "/mnt/synology/syn01",
		Type:        "nfs",
		Options:     []string{"rw", "soft"},
		StartAt:     "*-*-* 18:05:00",
		State:       "disabled",
	}, pre, "parsed input flags pre-fill the wizard's first mount defaults")
}
