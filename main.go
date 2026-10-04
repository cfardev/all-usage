// Command all-usage shows Codex, Kiro and Cursor subscription usage at a glance.
package main

import (
	"os"

	"github.com/cfardev/all-usage/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
