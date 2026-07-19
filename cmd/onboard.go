package dotdrift

import (
	"fmt"

	"github.com/thedataflows/dotdrift/internal/mise"
	"github.com/thedataflows/dotdrift/internal/onboard"
)

// OnboardCmd copies live paths into a module and applies them.
type OnboardCmd struct {
	Paths []string `arg:"" optional:"" help:"Paths to onboard into the module"`

	Profile  string   `help:"Path to profile directory" type:"existingdir" default:"."`
	App      string   `help:"Module app id"`
	Mode     string   `help:"Dotfile mode" enum:"link,copy,template" default:"link"`
	Packages []string `help:"Distro packages to declare"`
	Tools    []string `help:"Mise tools to declare"`
	Host     bool         `help:"Host overlay only"`
	DryRun   bool         `help:"Preview only"`
	Yes      bool         `help:"Answer yes to mise prompts" default:"false"`
	// Mise injects a runner for tests; nil uses the real mise bootstrap.
	Mise mise.Runner `kong:"-"`
}

// Run implements the onboard command.
func (c *OnboardCmd) Run() error {
	f, err := detectFacts()
	if err != nil {
		return fmt.Errorf("detect: %w", err)
	}

	runner := c.Mise
	if runner == nil {
		runner = mise.NewExecMise(mise.DefaultMise())
	}
	o := &onboard.Onboard{Mise: runner}
	return o.Run(onboard.Options{
		ProfileRoot: c.Profile,
		Paths:       c.Paths,
		App:         c.App,
		Mode:        c.Mode,
		Packages:    c.Packages,
		Tools:       c.Tools,
		Host:        c.Host,
		DryRun:      c.DryRun,
		Yes:         c.Yes,
		Hostname:    f.Hostname,
	})
}
