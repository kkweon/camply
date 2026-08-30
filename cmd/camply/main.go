package main

import (
	"fmt"
	"os"

	"github.com/kkweon/camply/cmd/camply/cli"
)

// version is injected at build time with -ldflags "-X main.version=...".
// Releases take it from the git tag; a plain `go build` leaves it as "dev".
var version = "dev"

func main() {
	if err := cli.Execute(version); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
