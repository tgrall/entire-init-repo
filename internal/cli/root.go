// Package cli wires the cobra command for the entire-init-repo plugin.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/entireio/entire-init-repo/internal/detect"
	"github.com/entireio/entire-init-repo/internal/hostcli"
	"github.com/entireio/entire-init-repo/internal/ui"
	"github.com/entireio/entire-init-repo/internal/wizard"
)

// Exit codes.
const (
	exitOK      = 0
	exitFailed  = 1
	exitUsage   = 2
	exitAborted = 130
)

// Execute runs the plugin and returns the process exit code.
func Execute(version string, args []string) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()
	return run(ctx, version, args, os.Stdout, os.Stderr)
}

func run(ctx context.Context, version string, args []string, stdout, stderr io.Writer) int {
	code := exitOK
	root := newRootCmd(version, stdout, &code)
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)
	if err := root.ExecuteContext(ctx); err != nil {
		if code == exitOK {
			code = exitUsage
		}
		fmt.Fprintln(stderr, "Error:", err)
	}
	return code
}

type flags struct {
	yes           bool
	dryRun        bool
	agents        string
	importHistory bool
	repo          string
	project       string
	repoName      string
	visibility    string
	cluster       string
	brain         bool
	noBrain       bool
	brainBackfill bool
}

func newRootCmd(version string, stdout io.Writer, code *int) *cobra.Command {
	var f flags
	cmd := &cobra.Command{
		Use:   "entire-init-repo",
		Short: "Set up this repository on Entire: agents, sessions, repo and Brain",
		Long: `entire init-repo walks you through onboarding the current repository onto Entire:

  1. checks the git repository (offers git init / an initial commit)
  2. logs you in to Entire if needed
  3. enables session capture for the coding agents you use
  4. links the repository to Entire: mirrors its GitHub origin, or creates a
     new Entire repository and pushes to it
  5. optionally builds Entire Brain and connects it to the hosted repo

Every step detects what is already in place and skips it, so it is safe to
re-run. Everything is done by running existing entire commands, which are
printed as they run.`,
		Example: `  entire init-repo
  entire init-repo --dry-run
  entire init-repo --yes --agents claude-code,codex --repo mirror --no-brain
  entire init-repo --repo create --project my-project --repo-name api --visibility private`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts, err := f.toOptions(cmd)
			if err != nil {
				return err
			}
			return runWizard(cmd.Context(), opts, stdout, code)
		},
	}
	fl := cmd.Flags()
	fl.BoolVarP(&f.yes, "yes", "y", false, "Accept recommended defaults without prompting")
	fl.BoolVar(&f.dryRun, "dry-run", false, "Show the commands that would change things, without running them")
	fl.StringVar(&f.agents, "agents", "", "Comma-separated agents to enable (e.g. claude-code,codex); skips the agent picker")
	fl.BoolVar(&f.importHistory, "import-history", false, "Import the last 30 days of agent history when enabling Entire")
	fl.StringVar(&f.repo, "repo", "", "How to link the repository: mirror, create or skip")
	fl.StringVar(&f.project, "project", "", "Project that owns a newly created repository")
	fl.StringVar(&f.repoName, "repo-name", "", "Name for a newly created repository (default: directory name)")
	fl.StringVar(&f.visibility, "visibility", "", "Visibility of a newly created repository: private or public")
	fl.StringVar(&f.cluster, "cluster", "", "Cluster host for a GitHub mirror (default: your account's region)")
	fl.BoolVar(&f.brain, "brain", false, "Set up Entire Brain without asking")
	fl.BoolVar(&f.noBrain, "no-brain", false, "Skip Entire Brain")
	fl.BoolVar(&f.brainBackfill, "brain-backfill", false, "Let Brain distill past sessions in the background and install its watcher (spends agent tokens)")
	cmd.MarkFlagsMutuallyExclusive("brain", "no-brain")

	cmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print the plugin version",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintln(cmd.OutOrStdout(), version)
		},
	})
	return cmd
}

func (f flags) toOptions(cmd *cobra.Command) (wizard.Options, error) {
	o := wizard.Options{
		Yes:           f.yes,
		DryRun:        f.dryRun,
		ImportHistory: f.importHistory,
		Project:       f.project,
		RepoName:      f.repoName,
		Cluster:       f.cluster,
		BrainBackfill: f.brainBackfill,
	}
	if cmd.Flags().Changed("agents") {
		o.Agents = []string{}
		for _, a := range strings.Split(f.agents, ",") {
			if a = strings.TrimSpace(a); a != "" {
				o.Agents = append(o.Agents, a)
			}
		}
	}
	switch f.repo {
	case "", "mirror", "create", "skip":
		o.RepoMode = f.repo
	default:
		return o, fmt.Errorf("--repo must be mirror, create or skip (got %q)", f.repo)
	}
	if o.RepoMode == "" && (f.project != "" || f.repoName != "") {
		o.RepoMode = "create"
	}
	switch f.visibility {
	case "", "private", "public":
		o.Visibility = f.visibility
	default:
		return o, fmt.Errorf("--visibility must be private or public (got %q)", f.visibility)
	}
	switch {
	case f.brain:
		t := true
		o.Brain = &t
	case f.noBrain:
		fa := false
		o.Brain = &fa
	case f.brainBackfill:
		t := true
		o.Brain = &t
	}
	return o, nil
}

func runWizard(ctx context.Context, opts wizard.Options, stdout io.Writer, code *int) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	canPrompt := detect.CanPrompt()
	var prompter ui.Prompter = ui.HuhPrompter{Accessible: detect.Accessible()}
	if opts.Yes {
		prompter = ui.DefaultsPrompter{}
	} else if !canPrompt {
		*code = exitUsage
		return errors.New("no terminal to prompt in: re-run with --yes (and flags such as --agents, --repo, --no-brain) or in an interactive terminal")
	}
	wizard.CanLogin = canPrompt

	var runner hostcli.Runner = hostcli.ExecRunner{}
	if opts.DryRun {
		runner = hostcli.DryRunner{Inner: runner, Out: stdout}
	}
	w := &wizard.Wizard{
		Opts:   opts,
		Client: &hostcli.Client{R: runner, Dir: cwd},
		P:      prompter,
		Out:    ui.Printer{W: stdout},
		Cwd:    cwd,
	}
	switch err := w.Run(ctx); {
	case err == nil:
		*code = exitOK
	case errors.Is(err, ui.ErrAborted):
		*code = exitAborted
	default:
		*code = exitFailed
	}
	return nil
}
