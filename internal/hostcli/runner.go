// Package hostcli runs the host `entire` CLI (and git) as child processes.
//
// The plugin never talks to Entire APIs directly: the CLI passes plugins no
// auth token, and every capability the onboarding needs already exists as an
// `entire` subcommand. Everything goes through a Runner so the wizard can be
// tested with a fake and previewed with --dry-run.
package hostcli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Mode selects how a child process is attached to the terminal.
type Mode int

const (
	// Capture collects stdout and stderr. Stdin is /dev/null and the child is
	// told it is not interactive, so it never blocks on a prompt.
	Capture Mode = iota
	// Stream passes stdout/stderr through to the user (long-running commands
	// with useful progress output) but still never prompts.
	Stream
	// Interactive inherits the full terminal, stdin included. Used only for
	// commands that are interactive by nature, such as `entire login`.
	Interactive
)

// Cmd describes one child process.
type Cmd struct {
	Name string // executable, e.g. "entire" or "git"
	Args []string
	Dir  string
	Mode Mode
	// Mutates marks commands that change state. --dry-run prints these
	// instead of running them; read-only commands still run so the preview
	// reflects the real current state.
	Mutates bool
}

// String renders the command the way a user would type it.
func (c Cmd) String() string {
	parts := make([]string, 0, len(c.Args)+1)
	parts = append(parts, c.Name)
	for _, a := range c.Args {
		parts = append(parts, shellQuote(a))
	}
	return strings.Join(parts, " ")
}

// Result is what a finished child produced. Stdout/Stderr are only populated
// in Capture mode.
type Result struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

// ExitError is returned when the child ran but exited non-zero.
type ExitError struct {
	Cmd      Cmd
	ExitCode int
	Stderr   string
}

func (e *ExitError) Error() string {
	msg := strings.TrimSpace(e.Stderr)
	if msg == "" {
		return fmt.Sprintf("%s: exit status %d", e.Cmd, e.ExitCode)
	}
	return fmt.Sprintf("%s: %s", e.Cmd, lastLines(msg, 3))
}

// Runner executes commands.
type Runner interface {
	Run(ctx context.Context, c Cmd) (Result, error)
}

// ExecRunner is the real Runner backed by os/exec.
type ExecRunner struct {
	Stdout io.Writer // defaults to os.Stdout for Stream/Interactive
	Stderr io.Writer // defaults to os.Stderr for Stream/Interactive
}

// Run implements Runner.
func (r ExecRunner) Run(ctx context.Context, c Cmd) (Result, error) {
	cmd := exec.CommandContext(ctx, c.Name, c.Args...)
	cmd.Dir = c.Dir
	cmd.Env = os.Environ()
	stdout, stderr := r.Stdout, r.Stderr
	if stdout == nil {
		stdout = os.Stdout
	}
	if stderr == nil {
		stderr = os.Stderr
	}

	var outBuf, errBuf bytes.Buffer
	switch c.Mode {
	case Capture:
		cmd.Stdout = &outBuf
		cmd.Stderr = &errBuf
		cmd.Env = append(cmd.Env, nonInteractiveEnv()...)
	case Stream:
		cmd.Stdout = stdout
		cmd.Stderr = io.MultiWriter(stderr, &errBuf)
		cmd.Env = append(cmd.Env, nonInteractiveEnv()...)
	case Interactive:
		cmd.Stdin = os.Stdin
		cmd.Stdout = stdout
		cmd.Stderr = stderr
	}

	err := cmd.Run()
	res := Result{Stdout: outBuf.Bytes(), Stderr: errBuf.Bytes()}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			res.ExitCode = exitErr.ExitCode()
			return res, &ExitError{Cmd: c, ExitCode: res.ExitCode, Stderr: errBuf.String()}
		}
		return res, fmt.Errorf("%s: %w", c, err)
	}
	return res, nil
}

// nonInteractiveEnv makes child `entire` processes skip prompts. The CLI
// probes /dev/tty rather than stdin, so redirecting stdin alone is not
// enough; GIT_TERMINAL_PROMPT=0 is one of the markers its
// CanPromptInteractively check honours.
func nonInteractiveEnv() []string {
	return []string{"GIT_TERMINAL_PROMPT=0"}
}

// DryRunner wraps a Runner: mutating commands are printed, not executed.
type DryRunner struct {
	Inner Runner
	Out   io.Writer
}

// Run implements Runner.
func (d DryRunner) Run(ctx context.Context, c Cmd) (Result, error) {
	if c.Mutates {
		fmt.Fprintf(d.Out, "  [dry-run] %s\n", c)
		return Result{}, nil
	}
	return d.Inner.Run(ctx, c)
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if strings.ContainsAny(s, " \t\n\"'$`\\|&;<>()*?[]#~{}") {
		return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
	}
	return s
}

func lastLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " ")
}
