package cli

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/gomezv/portucca/internal/application"
)

func confirmDevWrap(change application.DevScriptChange, in io.Reader, out io.Writer) bool {
	fmt.Fprintf(out, "package.json scripts.dev:\n  - %s\n  + %s\nApply? [y/N] ", change.Before, change.After)
	line, _ := bufio.NewReader(in).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}
