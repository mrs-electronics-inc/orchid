package main

import (
	"os"

	"github.com/mrs-electronics-inc/orchid/internal/orchidcli"
)

func main() {
	os.Exit(orchidcli.Run(os.Args[1:]))
}
