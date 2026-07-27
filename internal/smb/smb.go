// Package smb implements the apply step that activates the Samba server:
// group and user membership, avahi, config validation, service enablement,
// and samba account passwords. It never writes config files — placement is
// mise's job (smb.conf and shares.conf are system dotfiles); this step only
// activates what is already placed.
package smb

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/thedataflows/dotdrift/internal/apply"
	"github.com/thedataflows/dotdrift/internal/resolve"
)

// DefaultGroup is used when a module's smb spec declares no group.
const DefaultGroup = "smb"

// Runner runs commands for the step. Run captures combined output (stdout
// and stderr merged, so validation errors like testparm's carry their
// diagnostics); cancelling ctx kills the child process. RunInteractive
// attaches the terminal so a command can prompt (smbpasswd password entry).
type Runner interface {
	Run(ctx context.Context, name string, args ...string) (string, error)
	RunInteractive(ctx context.Context, name string, args ...string) error
}

// ExecRunner is the real command runner.
type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (ExecRunner) RunInteractive(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// geteuid is a test seam for the already-root check in privArgv.
var geteuid = os.Geteuid

// privArgv builds the argv for a privileged command: directly when already
// root (EUID 0, e.g. containers), otherwise elevated as sudo -E <cmd> ....
// Mirrors the dotfilesApplyArgv precedent in internal/mise — a deliberate
// small duplication over a shared helper.
func privArgv(euid int, name string, args ...string) []string {
	argv := append([]string{name}, args...)
	if euid == 0 {
		return argv
	}
	return append([]string{"sudo", "-E"}, argv...)
}

// isTTY reports whether stdin is a terminal. Stdlib-only char-device check
// (golang.org/x/term is not vendored). A variable so tests can substitute it.
var isTTY = func() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// Step is the apply pipeline step for Samba server activation, consuming the
// resolved smb aggregate. Out receives intentional user-facing messages (the
// no-TTY smbpasswd skip warning); it defaults to os.Stdout.
type Step struct {
	Runner Runner
	Plan   resolve.SmbStep
	Out    io.Writer
}

var _ apply.Step = (*Step)(nil)

// Name returns the step name.
func (s *Step) Name() string { return "smb" }

// Run activates Samba in script order (mise-tasks/system/smb.sh): per module
// ensure the group, user membership, and avahi; then gate on testparm; then
// enable/restart the smb service; finally ensure each declared user has a
// samba password. An empty aggregate is a no-op.
func (s *Step) Run(ctx context.Context) error {
	if s.Runner == nil {
		return fmt.Errorf("no runner configured")
	}
	if len(s.Plan.Modules) == 0 {
		return nil
	}
	for _, m := range s.Plan.Modules {
		if err := s.activateModule(ctx, m); err != nil {
			return err
		}
	}
	if err := s.validateConfig(ctx); err != nil {
		return err
	}
	if err := s.ensureService(ctx); err != nil {
		return err
	}
	for _, m := range s.Plan.Modules {
		for _, user := range m.Spec.Users {
			if err := s.ensurePassword(ctx, user); err != nil {
				return err
			}
		}
	}
	return nil
}

// activateModule ensures one module's group, user membership, and avahi.
func (s *Step) activateModule(ctx context.Context, m resolve.SmbModuleSpec) error {
	group := m.Spec.Group
	if group == "" {
		group = DefaultGroup
	}
	if err := s.ensureGroup(ctx, group); err != nil {
		return err
	}
	for _, user := range m.Spec.Users {
		if err := s.ensureMember(ctx, group, user); err != nil {
			return err
		}
	}
	if m.Spec.Avahi == nil || *m.Spec.Avahi {
		if _, err := s.runPriv(ctx, "systemctl", "enable", "--now", "avahi-daemon"); err != nil {
			return fmt.Errorf("enable avahi-daemon: %w", err)
		}
	}
	return nil
}

// ensureGroup creates the group only when getent does not find it.
func (s *Step) ensureGroup(ctx context.Context, group string) error {
	if _, err := s.runPriv(ctx, "getent", "group", group); err == nil {
		return nil
	}
	if _, err := s.runPriv(ctx, "groupadd", group); err != nil {
		return fmt.Errorf("create group %q: %w", group, err)
	}
	return nil
}

// ensureMember adds the user to the group only when id -Gn does not list it.
func (s *Step) ensureMember(ctx context.Context, group, user string) error {
	out, err := s.runPriv(ctx, "id", "-Gn", user)
	if err != nil {
		return fmt.Errorf("list groups for %q: %w", user, err)
	}
	if slices.Contains(strings.Fields(out), group) {
		return nil
	}
	if _, err := s.runPriv(ctx, "usermod", "-aG", group, user); err != nil {
		return fmt.Errorf("add %q to group %q: %w", user, group, err)
	}
	return nil
}

// validateConfig gates on testparm; a failure stops the step before any
// service restart, with the captured testparm output in the error.
func (s *Step) validateConfig(ctx context.Context) error {
	out, err := s.runPriv(ctx, "testparm", "-s")
	if err != nil {
		return fmt.Errorf("testparm validation failed: %w\n%s", err, strings.TrimSpace(out))
	}
	return nil
}

// ensureService mirrors the script's checks: enable when not enabled,
// restart when not active.
func (s *Step) ensureService(ctx context.Context) error {
	if _, err := s.runPriv(ctx, "systemctl", "is-enabled", "smb"); err != nil {
		if _, err := s.runPriv(ctx, "systemctl", "enable", "smb"); err != nil {
			return fmt.Errorf("enable smb: %w", err)
		}
	}
	if _, err := s.runPriv(ctx, "systemctl", "is-active", "smb"); err != nil {
		if _, err := s.runPriv(ctx, "systemctl", "restart", "smb"); err != nil {
			return fmt.Errorf("restart smb: %w", err)
		}
	}
	return nil
}

// ensurePassword adds the user's samba account when pdbedit -L does not list
// it. With a terminal it runs smbpasswd interactively; without one it warns
// on Out and skips — a missing password never fails the step.
func (s *Step) ensurePassword(ctx context.Context, user string) error {
	out, err := s.runPriv(ctx, "pdbedit", "-L")
	if err == nil && hasSambaAccount(out, user) {
		return nil
	}
	if !isTTY() {
		fmt.Fprintf(s.out(), "samba password missing for %s; run: sudo smbpasswd -a %s\n", user, user)
		return nil
	}
	argv := privArgv(geteuid(), "smbpasswd", "-a", user)
	if err := s.Runner.RunInteractive(ctx, argv[0], argv[1:]...); err != nil {
		return fmt.Errorf("smbpasswd -a %s: %w", user, err)
	}
	return nil
}

// hasSambaAccount reports whether pdbedit -L output lists the user (lines
// look like "alice:1000:Alice ...").
func hasSambaAccount(pdbeditOut, user string) bool {
	prefix := user + ":"
	for line := range strings.Lines(pdbeditOut) {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

// runPriv runs a command through the privilege escalation argv.
func (s *Step) runPriv(ctx context.Context, name string, args ...string) (string, error) {
	argv := privArgv(geteuid(), name, args...)
	return s.Runner.Run(ctx, argv[0], argv[1:]...)
}

func (s *Step) out() io.Writer {
	if s.Out != nil {
		return s.Out
	}
	return os.Stdout
}
