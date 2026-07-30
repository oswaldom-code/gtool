package main

import (
	"os"

	"github.com/oswaldom-code/gtool/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
