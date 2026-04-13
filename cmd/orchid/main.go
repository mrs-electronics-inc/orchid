package main

import (
	"os"

	cli "github.com/mrs-electronics-inc/orchid/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
