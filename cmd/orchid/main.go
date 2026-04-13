package main

import (
	"os"

	cli "github.com/mrs-electronics-inc/orchid/internal/cli"
)

var (
	// Build scripts inject these at link time so release binaries can report
	// their version and commit without any extra runtime configuration.
	version = ""
	commit  = ""
)

func main() {
	cli.SetVersion(version, commit)
	os.Exit(cli.Run(os.Args[1:]))
}
