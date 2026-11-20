package wizard

import (
	"context"

	"github.com/entireio/entire-init/internal/hostcli"
	"github.com/entireio/entire-init/internal/ui"
)

const stepBrain = "Entire Brain"

func (w *Wizard) stepBrain(ctx context.Context) error {
	want := true
	if w.Opts.Brain != nil {
		want = *w.Opts.Brain
	} else {
		var err error
		want, err = w.P.Confirm("Set up Entire Brain for this repository?",
			"Brain indexes code, sessions and history so agents get a briefing before they work. The basic setup spends no agent tokens.", true)
		if err != nil {
			return err
		}
	}
	if !want {
		w.record(stepBrain, StatusSkipped, "skipped by choice")
		w.Out.Info("Skipped. Set it up later with %s.", ui.Code("entire init"))
		return nil
	}
	if !w.isRepo || !w.hasCommits {
		w.record(stepBrain, StatusSkipped, "needs a repository with at least one commit")
		w.Out.Warn("Brain needs a repository with at least one commit; skipped.")
		return nil
	}

	installed, version, err := w.Client.PluginInstalled(ctx, "brain")
	if err != nil {
		w.record(stepBrain, StatusFailed, errSummary(err))
		w.Out.Fail("Could not check the brain plugin: %v", errSummary(err))
		return nil
	}
	if installed {
		w.Out.OK("Brain plugin installed %s", ui.Muted("("+version+")"))
	} else {
		w.announce("entire plugin install brain --yes")
		if err := w.Client.PluginInstall(ctx, "brain"); err != nil {
			w.record(stepBrain, StatusFailed, errSummary(err))
			w.Out.Fail("Installing the brain plugin failed: %v", errSummary(err))
			return nil
		}
		w.done("Installed the brain plugin")
	}

	// Skip what is already in place.
	var existing hostcli.BrainStatus
	if installed {
		if st, err := w.Client.BrainStatus(ctx); err == nil {
			existing = st
		}
	}
	if existing.Built() && (existing.Connected() || w.repoRef == "") {
		w.brainSetup = true
		if existing.Connected() {
			w.Out.OK("Brain is already built and connected")
			w.record(stepBrain, StatusAlready, "built + connected")
		} else {
			w.brainPending = true
			w.Out.OK("Brain is already built")
			w.record(stepBrain, StatusPending, "built locally; connect after linking an Entire repo")
		}
		return nil
	}
	if existing.Built() {
		w.brainSetup = true
		w.Out.OK("Brain is already built")
		return w.connectBrain(ctx, false)
	}

	backfill := w.Opts.BrainBackfill
	if !backfill && !w.Opts.Yes {
		backfill, err = w.P.Confirm("Also distill past sessions into facts in the background?",
			"Runs a cheap agent model and installs a background watcher. This SPENDS AGENT TOKENS. You can enable it later with: entire brain setup --backfill --daemon", false)
		if err != nil {
			return err
		}
	}
	opts := hostcli.BrainSetupOptions{Backfill: backfill}
	if backfill {
		opts.SupportsYes = w.Client.BrainSetupSupportsYes(ctx)
	}
	cmd := "entire brain setup --no-backfill --no-daemon"
	if backfill {
		cmd = "entire brain setup --backfill --daemon"
		if opts.SupportsYes {
			cmd += " --yes"
		}
	}
	w.announce(cmd)
	if err := w.Client.BrainSetup(ctx, opts); err != nil {
		w.record(stepBrain, StatusFailed, errSummary(err))
		w.Out.Fail("Brain setup failed: %v", errSummary(err))
		w.Out.Info("Diagnose with %s.", ui.Code("entire brain doctor"))
		return nil
	}
	w.brainSetup = true
	w.done("Brain is built and queryable")
	return w.connectBrain(ctx, backfill)
}

// connectBrain binds the brain to the hosted repo once it exists on Entire.
func (w *Wizard) connectBrain(ctx context.Context, backfill bool) error {
	// Connect to the hosted brain once the repo exists on Entire.
	if w.repoRef == "" {
		w.brainPending = true
		w.record(stepBrain, StatusPending, "built locally; connect after linking an Entire repo")
		w.Out.Info("Connect it to Entire later with %s once the repository is on Entire.", ui.Code("entire brain connect"))
		return nil
	}
	repoID, apiURL := "", ""
	if !w.viaOrigin && w.repo != nil {
		repoID, apiURL = w.repo.RepoID, w.repo.APIURL
	}
	if !w.viaOrigin && (repoID == "" || apiURL == "") && !w.Opts.DryRun {
		w.brainPending = true
		w.record(stepBrain, StatusPending, "built locally; connect pending")
		w.Out.Warn("Could not resolve the repo id for %s yet; connect later with %s.", w.repoRef, ui.Code("entire brain connect"))
		return nil
	}
	cmd := "entire brain connect"
	if repoID != "" {
		cmd += " --repo-id " + repoID + " --api-url " + apiURL
	}
	w.announce(cmd)
	if err := w.Client.BrainConnect(ctx, repoID, apiURL); err != nil {
		w.brainPending = true
		w.record(stepBrain, StatusPending, "built locally; connect failed: "+errSummary(err))
		w.Out.Warn("Connecting failed (the repo may still be provisioning): %v", errSummary(err))
		w.Out.Info("Retry with %s.", ui.Code("entire brain connect"))
		return nil
	}
	w.done("Connected to the hosted brain for %s", ui.Bold(w.repoRef))
	detail := "built + connected"
	if backfill {
		detail += " (backfill on)"
	}
	w.record(stepBrain, StatusDone, detail)
	return nil
}
