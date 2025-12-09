package main

import (
	"github.com/marcus/td/cmd"
)

// Version is set at build time
var Version = "dev"

func main() {
	cmd.SetVersion(Version)
	cmd.Execute()
}
