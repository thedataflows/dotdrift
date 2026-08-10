package cmd

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/alecthomas/kong"
	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/thedataflows/dotdrift/internal/executil"
)

const (
	appName      = "dotdrift"
	appDesc      = "DotDrift is a CLI tool for managing Linux configuration"
	appEnvPrefix = "DD"
)

// CurrentLogFormat records the active log format set at runtime.
var CurrentLogFormat string

// RootFlags holds global CLI flags.
type RootFlags struct {
	LogLevel      string `help:"Log level (trace,debug,info,warn,error)" enum:"trace,debug,info,warn,error" default:"info" env:"LOG_LEVEL"`
	LogFormat     string `help:"Log format (console,json)" enum:"console,json" default:"console" env:"LOG_FORMAT"`
	PProf         bool   `help:"Enable pprof profiling server" default:"false"`
	PProfListenOn string `help:"Listen address for pprof profiling server" default:"127.0.0.1:6060"`
	NoColor       bool   `help:"Disable colored output" default:"false" env:"NO_COLOR"`
}

// root CLI structure.
type CLI struct {
	RootFlags `kong:"embed"`

	Init     InitCmd     `cmd:"" help:"Create or clone a profile"`
	Detect   DetectCmd   `cmd:"" help:"Detect system facts"`
	Modules  ModulesCmd  `cmd:"" help:"List selected and skipped modules"`
	Plan     PlanCmd     `cmd:"" help:"Print the effective plan"`
	Apply    ApplyCmd    `cmd:"" help:"Apply the profile"`
	Status   StatusCmd   `cmd:"" help:"Show status"`
	Onboard  OnboardCmd  `cmd:"" help:"Onboard paths into a module" aliases:"add"`
	Generate GenerateCmd `cmd:"" help:"Generate mounts/smb modules"`
	Paru     ParuCmd     `cmd:"" help:"paru mise plugin backend (internal)"`
	Version  VersionCmd  `cmd:"" help:"Show version information"`

	args []string // original args, used to detect --help before side effects
}

// AfterApply is called after Kong parses the CLI but before the command runs.
func (cli *CLI) AfterApply(kctx *kong.Context) error {
	if kctx.Command() == "version" || slices.Contains(kctx.Args, "--help") || slices.Contains(kctx.Args, "-h") {
		return nil
	}
	if err := setGlobalLoggerLogLevel(cli.LogLevel); err != nil {
		return fmt.Errorf("set log level: %w", err)
	}
	if err := setGlobalLoggerLogFormat(cli.LogFormat); err != nil {
		return fmt.Errorf("set log format: %w", err)
	}
	if cli.PProf {
		log.Info().Str("listen", cli.PProfListenOn).Msg("Starting pprof profiling server")
		runtime.SetBlockProfileRate(1)
		go func() {
			if err := http.ListenAndServe(cli.PProfListenOn, nil); err != nil {
				log.Error().Err(err).Str("listen", cli.PProfListenOn).Msg("pprof profiling server stopped")
			}
		}()
	}
	if cli.NoColor || os.Getenv("NO_COLOR") != "" {
		executil.NoColor = true
		os.Setenv("NO_COLOR", "1") // propagate to child processes (mise, paru)
	}
	return nil
}

func setGlobalLoggerLogLevel(levelStr string) error {
	level, err := zerolog.ParseLevel(levelStr)
	if err != nil {
		return fmt.Errorf("parse log level %q: %w", levelStr, err)
	}
	zerolog.SetGlobalLevel(level)
	log.Logger = log.Logger.Level(level)
	return nil
}

func setGlobalLoggerLogFormat(format string) error {
	CurrentLogFormat = format
	switch format {
	case "console":
		log.Logger = log.Logger.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})
	case "json":
		log.Logger = log.Logger.Output(os.Stderr)
	default:
		return fmt.Errorf("invalid log format %q", format)
	}
	return nil
}

func loadDotenvFiles() {
	_ = godotenv.Load(".env", ".local.env", ".dev.env")
}

// Run executes the CLI with the given version.
func Run(version string, args []string) error {
	loadDotenvFiles()

	var cli CLI
	parser, err := kong.New(
		&cli,
		kong.Name(appName),
		kong.Description(appDesc),
		kong.UsageOnError(),
		kong.DefaultEnvars(appEnvPrefix),
		kong.Exit(func(code int) {
			if testing.Testing() {
				return
			}
			os.Exit(code)
		}),
	)
	if err != nil {
		return fmt.Errorf("create CLI parser: %w", err)
	}

	cli.args = args
	// Normalize bare --diff to --diff=internal: kong string flags require a
	// value, but apply --diff (no value) means "use the internal diff".
	for i, a := range args {
		if a == "--" {
			break
		}
		if a == "--diff" {
			args[i] = "--diff=internal"
		}
	}
	kctx, err := parser.Parse(args)
	if slices.Contains(args, "--help") || slices.Contains(args, "-h") {
		return nil
	}
	if err != nil {
		parser.FatalIfErrorf(err)
	}

	// Check if this is the version command - handle it specially without logging/config
	if kctx.Command() == "version" {
		return kctx.Run(version)
	}

	log.Logger.Debug().Str("app", kctx.Model.Name).Str("version", version).Msg("Starting application")

	return kctx.Run(kctx, &cli)
}
