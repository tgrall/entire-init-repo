package wizard

import (
	"context"

	"github.com/entireio/entire-init/internal/detect"
	"github.com/entireio/entire-init/internal/hostcli"
	"github.com/entireio/entire-init/internal/ui"
)

const stepGit = "Git repository"

func (w *Wizard) stepPreflight(ctx context.Context) error {
	root, ok := detect.RepoRoot(ctx, w.Cwd)
	if !ok {
		w.Out.Warn("%s is not a git repository.", w.Cwd)
		yes, err := w.P.Confirm("Initialize a git repository here?",
			"Entire tracks sessions and checkpoints per git repository.", true)
		if err != nil {
			return err
		}
		if !yes {
			w.record(stepGit, StatusFailed, "not a git repository")
			w.Out.Fail("Entire needs a git repository. Run %s and try again.", ui.Code("git init"))
			return nil
		}
		w.Client.Dir = w.Cwd
		w.announce("git init")
		if _, err := w.Client.Git(ctx, hostcli.Stream, true, "init"); err != nil {
			w.record(stepGit, StatusFailed, errSummary(err))
			w.Out.Fail("git init failed: %v", err)
			return nil
		}
		root = w.Cwd
		ok = !w.Opts.DryRun
		w.isRepo = true
		if ok {
			w.Out.OK("Initialized git repository in %s", root)
		}
	}
	w.root = root
	w.isRepo = true
	w.Client.Dir = root

	w.hasCommits = ok && detect.HasCommits(ctx, root)
	if !w.hasCommits {
		w.Out.Warn("The repository has no commits yet (needed to push to Entire and to build a Brain).")
		yes, err := w.P.Confirm("Create an empty initial commit?", "Runs: git commit --allow-empty -m \"Initial commit\"", true)
		if err != nil {
			return err
		}
		if yes {
			w.announce(`git commit --allow-empty -m "Initial commit"`)
			if _, err := w.Client.Git(ctx, hostcli.Capture, true, "commit", "--allow-empty", "-m", "Initial commit"); err != nil {
				w.Out.Fail("Initial commit failed: %v", errSummary(err))
				w.Out.Info("Set your identity with %s and %s.", ui.Code("git config user.name"), ui.Code("git config user.email"))
			} else {
				w.hasCommits = true
				w.done("Created initial commit")
			}
		}
	}

	if origin := detect.RemoteURL(ctx, root, "origin"); origin != "" {
		if o, r, ok := detect.ParseGitHubRemote(origin); ok {
			w.githubOrigin = o + "/" + r
		}
		w.Out.OK("Repository %s %s", ui.Bold(root), ui.Muted("(origin: "+origin+")"))
	} else {
		w.Out.OK("Repository %s %s", ui.Bold(root), ui.Muted("(no origin remote)"))
	}
	detail := root
	if !w.hasCommits {
		detail += " (no commits)"
	}
	w.record(stepGit, StatusAlready, detail)
	return nil
}
