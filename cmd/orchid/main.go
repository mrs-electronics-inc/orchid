package main

import (
	"os"

	cli "github.com/mrs-electronics-inc/orchid/internal/cli"
)

var (
	version = ""
	commit  = ""
)

func main() {
	cli.SetVersion(version, commit)
	os.Exit(cli.Run(os.Args[1:]))
}
