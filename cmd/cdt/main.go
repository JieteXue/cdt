package main

import (
	"fmt"
	"os"

	"github.com/JieteXue/cdt/internal/cli"
)

var version = "dev"

func main() {
	if err := cli.Execute(os.Args[1:], os.Stdin, os.Stdout, version); err != nil {
		fmt.Fprintln(os.Stderr, "cdt:", err)
		os.Exit(1)
	}
}
