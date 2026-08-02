// Package mounts implements the mounts apply step: activation of systemd
// mount units whose unit files are already placed by mise. This step never
// writes unit files — placement is mise's job; it only runs mkdir for the
// destinations and systemctl for activation.
package mounts

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/thedataflows/dotdrift/internal/apply"
	"github.com/thedataflows/dotdrift/internal/executil"
	"github.com/thedataflows/dotdrift/internal/generate"
	"github.com/thedataflows/dotdrift/internal/resolve"
)

// Runner executes one privileged command argv.
type Runner interface {
	Run(ctx context.Context, argv []string) error
}

// ExecRunner runs commands via exec.CommandContext. Verbose streams child
// stdout/stderr live to Out/Err while still capturing and echoes each
// command line set -x-style ("+ argv") to Err before it runs, so failure
// errors keep their output suffix.
type ExecRunner struct {
	Verbose bool
	// Out/Err are the Verbose streaming destinations; nil defaults to
	// os.Stdout/os.Stderr.
	Out io.Writer
	Err io.Writer
}

// SetVerbose toggles live output streaming.
func (r *ExecRunner) SetVerbose(v bool) { r.Verbose = v }

func (r ExecRunner) writers() (io.Writer, io.Writer) {
	out, errW := r.Out, r.Err
	if out == nil {
		out = os.Stdout
	}
	if errW == nil {
		errW = os.Stderr
	}
	return out, errW
}

// Run executes argv[0] with argv[1:], returning the combined output on
// failure so callers can log systemctl's own diagnostics (e.g. "not
// loaded"). In Verbose mode the output also streams live (MultiWriter) and
// the command line is echoed set -x-style ("+ argv") to Err immediately
// before execution, so the error contract is unchanged.
func (r ExecRunner) Run(ctx context.Context, argv []string) error {
	if len(argv) == 0 {
		return fmt.Errorf("empty argv")
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if r.Verbose {
		out, errW := r.writers()
		executil.EchoCommand(errW, argv)
		cap := &executil.LockedWriter{W: &buf}
		cmd.Stdout = io.MultiWriter(out, cap)
		cmd.Stderr = io.MultiWriter(errW, cap)
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w: %s", strings.Join(argv, " "), err, strings.TrimSpace(buf.String()))
	}
	return nil
}

// geteuid is a test seam for the already-root check in the argv helpers.
var geteuid = os.Geteuid

// systemctlArgv builds the argv for a systemctl invocation: directly when
// already root (EUID 0, e.g. containers), otherwise elevated as
// sudo -E systemctl ... (mirroring the mise sudo pattern).
func systemctlArgv(euid int, args ...string) []string {
	return privilegedArgv(euid, append([]string{"systemctl"}, args...)...)
}

// mkdirArgv builds the argv for creating a mount destination: directly when
// root, otherwise sudo mkdir -p (plain sudo; no systemctl wrapper).
func mkdirArgv(euid int, destination string) []string {
	argv := []string{"mkdir", "-p", destination}
	if euid == 0 {
		return argv
	}
	return append([]string{"sudo"}, argv...)
}

// privilegedArgv prepends sudo -E unless the process is already root.
func privilegedArgv(euid int, argv ...string) []string {
	if euid == 0 {
		return argv
	}
	return append([]string{"sudo", "-E"}, argv...)
}

// Step activates the mount units of a resolved plan: it creates every
// destination directory, reloads systemd once so it picks up the unit
// files mise placed, then enables (or disables) each mount unit — plus its
// timer unit when the entry declares StartAt.
type Step struct {
	Runner Runner
	Plan   resolve.MountsStep
}

var _ apply.Step = (*Step)(nil)

// Name returns the pipeline step name.
func (s *Step) Name() string { return "mounts" }

// Run executes the mounts step. An empty plan is a no-op success.
func (s *Step) Run(ctx context.Context) error {
	if len(s.Plan.Entries) == 0 {
		return nil
	}
	if s.Runner == nil {
		return fmt.Errorf("no mounts runner configured")
	}
	euid := geteuid()

	// Create every destination before touching systemctl.
	for _, e := range s.Plan.Entries {
		if err := s.Runner.Run(ctx, mkdirArgv(euid, e.Spec.Destination)); err != nil {
			return fmt.Errorf("mkdir -p %s: %w", e.Spec.Destination, err)
		}
	}

	// Pick up the unit files mise placed — exactly once, before any enable.
	if err := s.Runner.Run(ctx, systemctlArgv(euid, "daemon-reload")); err != nil {
		return fmt.Errorf("systemctl daemon-reload: %w", err)
	}

	for _, e := range s.Plan.Entries {
		escaped := generate.EscapePath(e.Spec.Destination)
		unit := escaped + ".mount"
		timer := escaped + ".timer"
		if e.Spec.State == "disabled" {
			s.disable(ctx, euid, unit)
			if e.Spec.StartAt != "" {
				s.disable(ctx, euid, timer)
			}
			continue
		}
		if err := s.Runner.Run(ctx, systemctlArgv(euid, "enable", "--now", unit)); err != nil {
			return fmt.Errorf("systemctl enable --now %s: %w", unit, err)
		}
		if e.Spec.StartAt != "" {
			if err := s.Runner.Run(ctx, systemctlArgv(euid, "enable", "--now", timer)); err != nil {
				return fmt.Errorf("systemctl enable --now %s: %w", timer, err)
			}
		}
	}
	return nil
}

// disable tolerates failure: an absent or already-disabled unit must never
// fail the step — resume re-runs disable against partially applied state.
func (s *Step) disable(ctx context.Context, euid int, unit string) {
	if err := s.Runner.Run(ctx, systemctlArgv(euid, "disable", "--now", unit)); err != nil {
		log.Warn().Err(err).Msgf("systemctl disable --now %s failed; continuing (unit may be absent or already disabled)", unit)
	}
}
