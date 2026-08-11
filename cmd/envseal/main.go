package main

import (
	"os"

	"github.com/PeacexF/envseal/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:]))
}
