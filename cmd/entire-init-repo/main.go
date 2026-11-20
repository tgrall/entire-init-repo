// Command entire-init-repo is an Entire CLI plugin. Installed on PATH (or via
// `entire plugin install`), it runs as `entire init-repo` and walks the user
// through onboarding the current repository onto Entire.
package main

import (
	"os"

	"github.com/entireio/entire-init-repo/internal/cli"
)

// version is stamped at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	os.Exit(cli.Execute(version, os.Args[1:]))
}
