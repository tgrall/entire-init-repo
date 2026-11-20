package detect

import (
	"os"
	"strings"

	"golang.org/x/term"
)

// Accessible reports whether ACCESSIBLE mode is on (simpler prompts for
// screen readers), matching the Entire CLI.
func Accessible() bool { return os.Getenv("ACCESSIBLE") != "" }

// CanPrompt reports whether we can ask the user questions: stdin and stdout
// are terminals and we are not in CI.
func CanPrompt() bool {
	if v := os.Getenv("ENTIRE_TEST_TTY"); v != "" {
		return v == "1"
	}
	if ci := strings.ToLower(os.Getenv("CI")); ci != "" && ci != "0" && ci != "false" {
		return false
	}
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}
