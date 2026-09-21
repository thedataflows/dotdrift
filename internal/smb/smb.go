// Package smb runs Samba's post-bootstrap actions: the testparm validation
// gate and interactive samba account passwords. It never writes config files
// — placement is mise's job (smb.conf and shares.conf are system dotfiles),
// and group/user/service convergence is mise bootstrap's ([bootstrap.groups/
// users/services]); this package only does the interactive/validator parts
// after bootstrap runs.
package smb

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/thedataflows/dotdrift/internal/executil"
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

// ExecRunner is the real command runner. Verbose streams child stdout/stderr
// live to Out/Err while still capturing and echoes each command line
// set -x-style ("+ argv") to Err before it runs: Run callers parse (id -Gn,
// pdbedit -L) and append (testparm gate) the returned output, so streaming
// must never starve them.
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

func (r ExecRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if r.Verbose {
		out, errW := r.writers()
		executil.EchoCommand(errW, append([]string{name}, args...))
		cap := &executil.LockedWriter{W: &buf}
		cmd.Stdout = io.MultiWriter(out, cap)
		cmd.Stderr = io.MultiWriter(errW, cap)
	}
	err := cmd.Run()
	return buf.String(), err
}

func (ExecRunner) RunInteractive(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

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

// PostBootstrap runs the post-mise-bootstrap actions that can't be expressed
// declaratively: the testparm validation gate and interactive smbpasswd for
// each declared user missing from pdbedit -L. Group/user/service convergence
// is handled by mise bootstrap ([bootstrap.groups/users/services]); this
// function only does the interactive/validator parts.
func PostBootstrap(ctx context.Context, runner Runner, modules []resolve.SmbModuleSpec, out io.Writer) error {
	if runner == nil {
		return fmt.Errorf("no runner configured")
	}
	// testparm validation gate — stop before anything else if the config is broken.
	argv := privArgv(os.Geteuid(), "testparm", "-s")
	testparmOut, err := runner.Run(ctx, argv[0], argv[1:]...)
	if err != nil {
		return fmt.Errorf("testparm validation failed: %w\n%s", err, strings.TrimSpace(testparmOut))
	}
	// smbpasswd for each declared user missing from pdbedit -L.
	for _, m := range modules {
		for _, user := range m.Spec.Users {
			if err := postBootstrapPassword(ctx, runner, user, out); err != nil {
				return err
			}
		}
	}
	return nil
}

func postBootstrapPassword(ctx context.Context, runner Runner, user string, out io.Writer) error {
	argv := privArgv(os.Geteuid(), "pdbedit", "-L")
	pdbeditOut, err := runner.Run(ctx, argv[0], argv[1:]...)
	if err == nil && hasSambaAccount(pdbeditOut, user) {
		return nil
	}
	if !executil.IsStdinTerminal() {
		w := out
		if w == nil {
			w = os.Stdout
		}
		fmt.Fprintf(w, "samba password missing for %s; run: sudo smbpasswd -a %s\n", user, user)
		return nil
	}
	argv = privArgv(os.Geteuid(), "smbpasswd", "-a", user)
	if err := runner.RunInteractive(ctx, argv[0], argv[1:]...); err != nil {
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
