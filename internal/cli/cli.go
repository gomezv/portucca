package cli

import (
	"fmt"
	"io"

	"github.com/gomezv/portucca/internal/buildinfo"
)

func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(stdout, "portucca — sticky ports per worktree")
		fmt.Fprintln(stdout, "usage: portucca <init|ensure|setup|reallocate|prune|doctor|env|list|version> [flags]")
		return 0
	}
	if args[0] == "version" || args[0] == "--version" {
		fmt.Fprintf(stdout, "portucca %s (%s)\n", buildinfo.Version, buildinfo.Commit)
		return 0
	}
	fmt.Fprintf(stderr, "error: unknown command %q\n", args[0])
	return 2
}
