package cmd

import (
	"context"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/thedataflows/dotdrift/internal/drift"
	"github.com/thedataflows/dotdrift/internal/executil"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/mise"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/service"
)

// StatusCmd reports drift between the resolved profile and the live system,
// plus the apply resume cursor. Read-only; exits 0 even when drift is found.
type StatusCmd struct {
	Profile string   `help:"Path to profile directory" type:"existingdir" default:"."`
	State   string   `help:"Path to state file" type:"path" default:""`
	Verbose bool     `help:"Show each probe as it starts ('checking <section>: <item>') on stderr" short:"v" default:"false"`
	Jobs    int      `help:"Concurrent probe workers (0 = number of CPUs)" short:"j" default:"0"`
	Diff    string   `help:"Show diff for files whose content differs; bare = internal diff, --diff=tool uses the named tool" default:""`
	Modules []string `arg:"" optional:"" name:"modules" help:"Limit scope to these modules (space or comma separated)"`
	out     io.Writer
	err     io.Writer
}

// Run loads state, resolves the plan, probes the live system, and prints
// the report through the service reads area (T-tui-reads): the adapter
// owns the probes (its sudo-elevation and backend/mise seams) and the
// rendering order; the service owns the read and the canonical renderers.
// Only real errors (detect/load/resolve/corrupt state) return non-nil;
// drift is reported, never an error.
func (c *StatusCmd) Run() error {
	area := service.NewReadsArea(service.ReadsDeps{
		Detect:        detectFacts,
		LoadProfile:   profileLoad,
		Resolve:       resolvePlan,
		OtherAccounts: otherAccounts,
		WarnLoad:      warnLoadNudges,
	})
	var verbose io.Writer
	if c.Verbose {
		verbose = c.err
		if verbose == nil {
			verbose = os.Stderr
		}
	}
	r, err := area.Status(context.Background(), service.StatusOpts{
		ProfilePath: c.Profile,
		StatePath:   c.State,
		Modules:     c.Modules,
		Jobs:        c.Jobs,
		Verbose:     verbose,
		ProbesFor: func(f *facts.Facts) drift.Probes {
			pr := drift.DefaultProbes()
			pr.IsInstalled = packagesFor(f.Backend).IsInstalled
			pr.ToolCurrent = mise.NewExecMise(defaultMise()).Current
			return elevateProbes(pr) // retry elevated on permission denied (system files)
		},
	})
	if err != nil {
		return err
	}

	out := c.out
	if out == nil {
		out = os.Stdout
	}
	if err := service.RenderStatusHeader(out, r); err != nil {
		return err
	}
	if c.Diff != "" {
		entries, err := area.Diff(r.Plan, r.ProfileRoot)
		if err != nil {
			return err
		}
		if err := service.RenderDiff(out, entries, c.Diff, executil.ColorEnabled(out)); err != nil {
			return err
		}
	}
	service.RenderStatusNote(out, r)
	return nil
}

// sudoRead executes `sudo <name> <args>` and returns stdout. A test seam so
// status tests can assert elevation behavior without running real sudo.
var sudoRead = func(name string, args ...string) ([]byte, error) {
	return exec.Command("sudo", append([]string{name}, args...)...).Output()
}

// otherAccounts lists the other OS accounts with configuration in the
// profile; a test seam (tests cannot reach profile's unexported lookup seam).
var otherAccounts = profile.OtherAccounts

// elevateProbes wraps the file-access probes (Readlink, ReadFile, StatDir) so
// each retries elevated via sudo when the OS denies access. Only permission
// denied triggers the retry — other errors (not-exist, invalid path) propagate
// as-is. Already-root processes skip the wrapping entirely. Run (external
// commands like systemctl/getent) is left untouched; those commands work for
// normal users in the read-only status path.
func elevateProbes(pr drift.Probes) drift.Probes {
	if os.Geteuid() == 0 {
		return pr
	}
	if pr.Readlink != nil {
		pr.Readlink = elevateReadlink(pr.Readlink)
	}
	if pr.ReadFile != nil {
		pr.ReadFile = elevateReadFile(pr.ReadFile)
	}
	if pr.StatDir != nil {
		pr.StatDir = elevateStatDir(pr.StatDir)
	}
	if pr.ListDir != nil {
		pr.ListDir = elevateListDir(pr.ListDir)
	}
	return pr
}

// elevate wraps a probe function so it retries elevated via sudo when the OS
// denies access. The inner function runs first; on permission denied, sudo
// runs and its result is returned. When sudo itself fails, the original
// permission error is returned.
func elevate[T any](inner func(string) (T, error), sudo func(string) (T, error)) func(string) (T, error) {
	return func(path string) (T, error) {
		result, err := inner(path)
		if err == nil || !os.IsPermission(err) {
			return result, err
		}
		sudoResult, sudoErr := sudo(path)
		if sudoErr != nil {
			return result, err
		}
		return sudoResult, nil
	}
}

func elevateReadlink(inner func(string) (string, error)) func(string) (string, error) {
	return elevate(inner, func(path string) (string, error) {
		out, err := sudoRead("readlink", path)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(out)), nil
	})
}

func elevateReadFile(inner func(string) ([]byte, error)) func(string) ([]byte, error) {
	return elevate(inner, func(path string) ([]byte, error) {
		return sudoRead("cat", path)
	})
}

func elevateStatDir(inner func(string) (bool, error)) func(string) (bool, error) {
	return elevate(inner, func(path string) (bool, error) {
		out, err := sudoRead("stat", "-c", "%F", path)
		if err != nil {
			return false, err
		}
		return strings.TrimSpace(string(out)) == "directory", nil
	})
}

func elevateListDir(inner func(string) ([]string, error)) func(string) ([]string, error) {
	return elevateSlice(inner, func(path string) ([]string, error) {
		out, err := sudoRead("ls", "-1", path)
		if err != nil {
			return nil, err
		}
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(lines) == 1 && lines[0] == "" {
			return nil, nil
		}
		return lines, nil
	})
}

// elevateSlice is elevate for slice-returning probes (ListDir): retry
// elevated via sudo when the OS denies access.
func elevateSlice[T any](inner func(string) ([]T, error), sudo func(string) ([]T, error)) func(string) ([]T, error) {
	return func(path string) ([]T, error) {
		result, err := inner(path)
		if err == nil || !os.IsPermission(err) {
			return result, err
		}
		sudoResult, sudoErr := sudo(path)
		if sudoErr != nil {
			return result, err
		}
		return sudoResult, nil
	}
}
