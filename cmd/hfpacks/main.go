package main

import (
	"fmt"
	"os"

	"github.com/adamsiwiec1/hfpacks/internal/cli"
)

func main() {
	if err := cli.Run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "[x] %v\n", err)
		os.Exit(1)
	}
}
