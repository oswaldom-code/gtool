package main

import (
	"os"

	"github.com/oswaldo-montano/gtool/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
