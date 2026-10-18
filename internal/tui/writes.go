package tui

import (
	"io"

	"github.com/thedataflows/dotdrift/internal/generate"
	"github.com/thedataflows/dotdrift/internal/service"
)

// Writes is the consumer-side narrow interface over the service writes
// areas (ADR-0008's doorway, T-tui-writes): exactly what the Profile
// dialogs need, satisfied implicitly by *service.WritesArea. Every write
// the TUI performs goes through here — the shell never calls a domain
// package to change anything.
type Writes interface {
	Onboard(opts service.OnboardOpts) error
	RestorePlan(opts service.RestoreOpts) ([]service.RestorePlanItem, error)
	Restore(opts service.RestoreOpts) error
	GenerateSelection(sel generate.Selection) (generate.Selection, error)
	WriteGenerate(root string, sel generate.Selection, input generate.Input, out io.Writer) error
	// Volumes lists the detected volumes for the generate mounts
	// dialog's source picker (0111), MANAGED-marked against the target
	// module's existing sources.
	Volumes(root string, sel generate.Selection) ([]generate.Volume, error)
}
