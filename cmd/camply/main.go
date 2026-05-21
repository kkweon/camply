package main

import (
	"fmt"
	"os"

	"github.com/kkweon/camply/cmd/camply/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
