package cmd

import (
	"os"

	"github.com/thedataflows/dotdrift/internal/mise"
	"github.com/thedataflows/dotdrift/internal/service"
)

// OnboardCmd copies live paths into a module and applies them.
type OnboardCmd struct {
	Paths []string `arg:"" optional:"" help:"Paths to onboard into the module"`

	Profile  string      `help:"Path to profile directory" type:"existingdir" default:"."`
	App      string      `help:"Module directory name (required)" required:""`
	Mode     string      `help:"Dotfile mode" enum:"symlink,symlink-each,copy,template" default:"symlink"`
	Packages []string    `help:"Distro packages to declare; each entry is a bare name or name=\"description\" (the description becomes a TOML comment)"`
	Tools    []string    `help:"Mise tools to declare"`
	Host     overlayFlag `help:"Onboard into hosts/<hostname>; no value = current host, --host=<name> = explicit, flag omitted = base layer"`
	User     overlayFlag `help:"Onboard into users/<username>; no value = current user, --user=<name> = explicit, flag omitted = base layer"`
	DryRun   bool        `help:"Preview only"`
	Yes      bool        `help:"Answer yes to mise prompts" default:"false"`
	Verbose  bool        `help:"Stream package manager and mise output live, echoing each command line ('+ argv') to stderr before it runs" short:"v" default:"false"`
	// Mise injects a runner for tests; nil uses the real mise bootstrap.
	Mise mise.Runner `kong:"-"`
}

// Run translates flags onto the service writes area and reports to the
// CLI's stdout (T-tui-writes: the orchestration lives in the service).
func (c *OnboardCmd) Run() error {
	return service.NewWritesArea(service.WritesDeps{
		Detect: detectFacts,
		NewMise: func(verbose bool) mise.Runner {
			if c.Mise != nil {
				return c.Mise
			}
			m := defaultMise()
			m.Verbose = verbose
			return mise.NewExecMise(m)
		},
	}).Onboard(service.OnboardOpts{
		ProfileRoot: c.Profile,
		Paths:       c.Paths,
		App:         c.App,
		Mode:        c.Mode,
		Packages:    c.Packages,
		Tools:       c.Tools,
		HostSet:     c.Host.Set,
		Hostname:    c.Host.Value,
		UserSet:     c.User.Set,
		Username:    c.User.Value,
		DryRun:      c.DryRun,
		Yes:         c.Yes,
		Verbose:     c.Verbose,
		Out:         os.Stdout,
	})
}
