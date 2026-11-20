// Package wizard implements the `entire init` onboarding flow.
//
// Each step detects the current state first, skips itself when the work is
// already done, prompts only for what is missing, and applies the change by
// running an existing `entire` command. Steps record a Result for the final
// summary; a failed step does not abort the run unless a later step depends
// on what it would have produced.
package wizard

import (
	"context"
	"errors"
	"fmt"

	"github.com/entireio/entire-init/internal/hostcli"
	"github.com/entireio/entire-init/internal/ui"
)

// Options carries command-line answers. Zero values mean "ask".
type Options struct {
	Yes    bool // accept recommended defaults without prompting
	DryRun bool // print mutating commands instead of running them

	Agents        []string // agent names to enable; nil = ask
	ImportHistory bool     // import the last 30 days of agent history on enable

	RepoMode   string // "", "mirror", "create" or "skip"
	Project    string // project for `repo create`
	RepoName   string // name for `repo create`
	Visibility string // "private" or "public"
	Cluster    string // cluster host for `repo mirror add`

	Brain         *bool // nil = ask
	BrainBackfill bool  // allow token-spending backfill + watcher daemon
}

// Status values for a step result.
const (
	StatusDone    = "done"
	StatusAlready = "already"
	StatusSkipped = "skipped"
	StatusFailed  = "failed"
	StatusPending = "pending"
	StatusPlanned = "planned" // dry-run: would be done
)

// Result is one row of the summary.
type Result struct {
	Step   string
	Status string
	Detail string
}

// Wizard holds the flow's dependencies and the state steps share.
type Wizard struct {
	Opts   Options
	Client *hostcli.Client
	P      ui.Prompter
	Out    ui.Printer
	Cwd    string

	// state discovered along the way
	root          string
	isRepo        bool
	hasCommits    bool
	auth          hostcli.AuthStatus
	enabled       bool
	agents        []string // agent names with hooks installed
	repo          *hostcli.RepoView
	repoRef       string // /gh/o/r or /et/p/r once known
	viaOrigin     bool   // `origin` resolves to the Entire repo
	brainSetup    bool
	brainPending  bool // brain set up but connect still to do
	githubOrigin  string
	results       []Result
	totalSteps    int
	currentStepNo int
}

// ErrFailed is returned by Run when at least one step failed.
var ErrFailed = errors.New("one or more steps failed")

// Run executes every step in order and prints the summary.
func (w *Wizard) Run(ctx context.Context) error {
	steps := []struct {
		title string
		fn    func(context.Context) error
	}{
		{"Git repository", w.stepPreflight},
		{"Entire account", w.stepAuth},
		{"Agents & session capture", w.stepAgents},
		{"Entire repository", w.stepRepo},
		{"Entire Brain", w.stepBrain},
	}
	w.totalSteps = len(steps)

	sub := "Sets up this repository with agents, sessions, an Entire repo and Brain."
	if w.Opts.DryRun {
		sub += " (dry run: changes are printed, not made)"
	}
	w.Out.Banner("entire init", sub)

	for i, s := range steps {
		w.currentStepNo = i + 1
		w.Out.Step(i+1, len(steps), s.title)
		if err := s.fn(ctx); err != nil {
			if errors.Is(err, ui.ErrAborted) || errors.Is(err, context.Canceled) {
				w.Out.Warn("Cancelled.")
				w.printSummary()
				return ui.ErrAborted
			}
			// Unexpected errors are recorded and the flow continues.
			w.record(s.title, StatusFailed, err.Error())
			w.Out.Fail("%v", err)
		}
	}

	w.printSummary()
	for _, r := range w.results {
		if r.Status == StatusFailed {
			return ErrFailed
		}
	}
	return nil
}

// Results returns the recorded step results (for tests).
func (w *Wizard) Results() []Result { return w.results }

func (w *Wizard) record(step, status, detail string) {
	if w.Opts.DryRun && status == StatusDone {
		status = StatusPlanned
	}
	w.results = append(w.results, Result{Step: step, Status: status, Detail: detail})
}

// announce echoes a command before it runs (dry-run prints its own line).
func (w *Wizard) announce(c string) {
	if !w.Opts.DryRun {
		w.Out.Run(c)
	}
}

// done reports a completed change. In dry-run the "[dry-run] <cmd>" line
// already said what would happen, so claiming success would mislead.
func (w *Wizard) done(format string, a ...any) {
	if !w.Opts.DryRun {
		w.Out.OK(format, a...)
	}
}

func (w *Wizard) loggedIn() bool { return w.auth.LoggedIn }

func handle(user string) string {
	// "github:tgrall" -> "tgrall"
	for i := 0; i < len(user); i++ {
		if user[i] == ':' {
			return user[i+1:]
		}
	}
	return user
}

func errSummary(err error) string {
	var exitErr *hostcli.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.Error()
	}
	return fmt.Sprint(err)
}
