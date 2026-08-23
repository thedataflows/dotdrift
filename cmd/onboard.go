package cmd

import (
	"fmt"
	"os"

	"github.com/thedataflows/dotdrift/internal/mise"
	"github.com/thedataflows/dotdrift/internal/onboard"
)

// OnboardCmd copies live paths into a module and applies them.
type OnboardCmd struct {
	Paths []string `arg:"" optional:"" help:"Paths to onboard into the module"`

	Profile  string   `help:"Path to profile directory" type:"existingdir" default:"."`
	App      string   `help:"Module directory name (inferred from the first path when omitted)"`
	Mode     string   `help:"Dotfile mode" enum:"symlink,symlink-each,copy,template" default:"symlink"`
	Packages []string `help:"Distro packages to declare; each entry is a bare name or name=\"description\" (the description becomes a TOML comment)"`
	Tools    []string `help:"Mise tools to declare"`
	Host     string   `help:"Onboard into hosts/<hostname> (default: base layer)"`
	User     string   `help:"Onboard into users/<username> (default: base layer)"`
	DryRun   bool     `help:"Preview only"`
	Yes      bool     `help:"Answer yes to mise prompts" default:"false"`
	Verbose  bool     `help:"Stream package manager and mise output live, echoing each command line ('+ argv') to stderr before it runs" short:"v" default:"false"`
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
		m := defaultMise()
		m.Verbose = c.Verbose
		runner = mise.NewExecMise(m)
	}
	o := &onboard.Onboard{Mise: runner, Out: os.Stdout}
	pkgs, err := onboard.ParsePackages(c.Packages)
	if err != nil {
		return fmt.Errorf("parse packages: %w", err)
	}
	// Empty flag = not selecting that overlay; the hostname/username fall
	// back to the detected facts (layer-claim context for adoption).
	hostname, username := f.Hostname, f.Username
	if c.Host != "" {
		hostname = c.Host
	}
	if c.User != "" {
		username = c.User
	}
	return o.Run(onboard.Options{
		ProfileRoot: c.Profile,
		Paths:       c.Paths,
		App:         c.App,
		Mode:        c.Mode,
		Packages:    pkgs,
		Tools:       c.Tools,
		Host:        c.Host != "",
		Hostname:    hostname,
		User:        c.User != "",
		Username:    username,
		DryRun:      c.DryRun,
		Yes:         c.Yes,
	})
}
