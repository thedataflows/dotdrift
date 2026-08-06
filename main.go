package main

import (
	"os"

	cmd "github.com/thedataflows/dotdrift/cmd"

	"github.com/rs/zerolog/log"
)

var version = "dev"

func main() {
	if err := cmd.Run(version, os.Args[1:]); err != nil {
		log.Fatal().Err(err).Msg("Failed to run application")
	}
}
