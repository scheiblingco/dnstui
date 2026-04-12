package main

import (
	"os"

	"github.com/scheiblingco/dnstui/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
