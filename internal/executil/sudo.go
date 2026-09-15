package executil

// SudoValidate is the elevation modal's checker (M15): it runs
// `sudo -k -S -v` — drop any cached timestamp, read the password line
// from stdin, validate. A successful check leaves a fresh sudo
// timestamp, so the apply run's privileged steps elevate without a
// terminal prompt; expiry re-prompts in the same shape. The password
// travels as a []byte and is never logged.

import (
	"bytes"
	"os/exec"
)

// sudoRunner is the exec seam; tests swap it via SwapSudoRunner. No test
// runs real sudo.
var sudoRunner = func(line []byte) error {
	cmd := exec.Command("sudo", "-k", "-S", "-v")
	cmd.Stdin = bytes.NewReader(line)
	return cmd.Run()
}

// SudoValidate validates pw against sudo, refreshing the timestamp. The
// password line (pw + newline, cloned) is what sudo -S reads.
func SudoValidate(pw []byte) error { return sudoRunner(append(bytes.Clone(pw), '\n')) }

// SwapSudoRunner replaces the exec seam and returns the restore func.
func SwapSudoRunner(run func(pw []byte) error) func() {
	prev := sudoRunner
	sudoRunner = run
	return func() { sudoRunner = prev }
}
