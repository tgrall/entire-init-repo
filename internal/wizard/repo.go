package wizard

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/entireio/entire-init/internal/detect"
	"github.com/entireio/entire-init/internal/hostcli"
	"github.com/entireio/entire-init/internal/ui"
)

const stepRepo = "Entire repository"

const (
	modeMirror = "mirror"
	modeCreate = "create"
	modeSkip   = "skip"

	newProjectValue = "\x00new"
)

func (w *Wizard) stepRepo(ctx context.Context) error {
	if !w.isRepo {
		w.record(stepRepo, StatusSkipped, "no git repository")
		w.Out.Info("Skipped: no git repository.")
		return nil
	}
	if !w.loggedIn() {
		w.record(stepRepo, StatusSkipped, "not logged in")
		w.Out.Info("Skipped: log in first (%s).", ui.Code("entire login"))
		return nil
	}

	// Already linked? `repo view` resolves via origin; also honour an
	// `entire` remote that a previous run (or the user) added.
	if done, err := w.detectLinkedRepo(ctx); err != nil || done {
		return err
	}

	mode := w.Opts.RepoMode
	if mode == "" {
		opts := []ui.Option{}
		def := modeCreate
		if w.githubOrigin != "" {
			def = modeMirror
			opts = append(opts, ui.Option{Label: "Mirror " + w.githubOrigin + " from GitHub (recommended)", Value: modeMirror})
			opts = append(opts, ui.Option{Label: "Create a new Entire repository instead", Value: modeCreate})
		} else {
			opts = append(opts, ui.Option{Label: "Create a new Entire repository (recommended)", Value: modeCreate})
			opts = append(opts, ui.Option{Label: "Mirror a GitHub repository…", Value: modeMirror})
		}
		opts = append(opts, ui.Option{Label: "Skip for now", Value: modeSkip})
		var err error
		mode, err = w.P.Select("Link this repository to Entire",
			"An Entire repository hosts your code with its sessions, checkpoints, trails and Brain.", opts, def)
		if err != nil {
			return err
		}
	}

	switch mode {
	case modeMirror:
		return w.mirrorRepo(ctx)
	case modeCreate:
		return w.createRepo(ctx)
	default:
		w.record(stepRepo, StatusSkipped, "skipped by choice")
		w.Out.Info("Skipped.")
		return nil
	}
}

func (w *Wizard) detectLinkedRepo(ctx context.Context) (bool, error) {
	if origin := detect.RemoteURL(ctx, w.root, "origin"); origin != "" {
		v, err := w.Client.RepoView(ctx, "")
		switch {
		case err == nil:
			w.setRepo(&v, v.Repo, true)
			w.Out.OK("Already linked to %s %s", ui.Bold(v.Repo), ui.Muted("("+placementsSummary(v)+")"))
			w.record(stepRepo, StatusAlready, v.Repo)
			return true, nil
		case !errors.Is(err, hostcli.ErrRepoNotLinked):
			w.Out.Warn("Could not check the origin remote: %v", errSummary(err))
		}
	}
	if url := detect.RemoteURL(ctx, w.root, "entire"); detect.IsEntireRemote(url) {
		if ref := refFromEntireURL(url); ref != "" {
			if v, err := w.Client.RepoView(ctx, ref); err == nil {
				w.setRepo(&v, ref, false)
				w.Out.OK("Already linked to %s via the %s remote", ui.Bold(v.Repo), ui.Code("entire"))
				w.record(stepRepo, StatusAlready, v.Repo)
				return true, nil
			}
		}
	}
	return false, nil
}

func (w *Wizard) setRepo(v *hostcli.RepoView, ref string, viaOrigin bool) {
	w.repo = v
	w.repoRef = ref
	w.viaOrigin = viaOrigin
}

// ---- mirror -------------------------------------------------------------------

var ownerRepoRE = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

func (w *Wizard) mirrorRepo(ctx context.Context) error {
	ownerRepo := w.githubOrigin
	if ownerRepo == "" {
		var err error
		ownerRepo, err = w.P.Input("GitHub repository to mirror (owner/repo)", "", "", func(s string) error {
			if !ownerRepoRE.MatchString(strings.TrimSpace(s)) {
				return errors.New("use the form owner/repo")
			}
			return nil
		})
		if err != nil {
			if w.Opts.Yes {
				w.record(stepRepo, StatusSkipped, "no GitHub origin to mirror")
				w.Out.Warn("No GitHub origin remote to mirror.")
				return nil
			}
			return err
		}
		ownerRepo = strings.TrimSpace(ownerRepo)
	}
	ref := "/gh/" + ownerRepo

	entry, err := w.Client.MirrorLookup(ctx, ownerRepo)
	if err != nil {
		w.record(stepRepo, StatusFailed, errSummary(err))
		w.Out.Fail("Could not look up %s: %v", ownerRepo, errSummary(err))
		return nil
	}
	switch {
	case entry == nil:
		w.record(stepRepo, StatusFailed, ownerRepo+" is not visible to Entire")
		w.Out.Fail("%s is not visible to your Entire login.", ownerRepo)
		w.Out.Info("Install the Entire GitHub App on %s, then check %s.", strings.Split(ownerRepo, "/")[0], ui.Code("entire repo mirror list --available"))
		return nil
	case entry.Status == "owner-only":
		w.record(stepRepo, StatusFailed, "GitHub App lacks access to "+ownerRepo)
		w.Out.Fail("The Entire GitHub App cannot access %s yet.", ownerRepo)
		w.Out.Info("Grant it access to the repository in GitHub, then re-run %s.", ui.Code("entire init"))
		return nil
	case entry.Status != "available":
		w.Out.OK("%s is already mirrored %s", ui.Bold(ref), ui.Muted("(status: "+entry.Status+")"))
		w.finishLink(ctx, ref)
		w.record(stepRepo, StatusAlready, ref+" ("+entry.Status+")")
		return nil
	}

	cluster, err := w.pickCluster(ctx)
	if err != nil {
		return err
	}
	var clusters []string
	if cluster != "" {
		clusters = []string{cluster}
	}
	cmd := "entire repo mirror add " + ref
	if cluster != "" {
		cmd += " --cluster " + cluster
	}
	w.announce(cmd)
	if err := w.Client.MirrorAdd(ctx, ownerRepo, clusters); err != nil {
		w.record(stepRepo, StatusFailed, errSummary(err))
		w.Out.Fail("Mirroring failed: %v", errSummary(err))
		return nil
	}
	w.done("Mirrored %s", ui.Bold(ref))
	w.finishLink(ctx, ref)
	w.record(stepRepo, StatusDone, "mirrored "+ref)
	return nil
}

// finishLink re-resolves the repo to capture repoId/apiUrl for Brain.
func (w *Wizard) finishLink(ctx context.Context, ref string) {
	viaOrigin := strings.HasPrefix(ref, "/gh/") && strings.EqualFold(ref, "/gh/"+w.githubOrigin)
	if w.Opts.DryRun {
		w.repoRef, w.viaOrigin = ref, viaOrigin
		return
	}
	v, err := w.Client.RepoView(ctx, ref)
	if err != nil {
		w.repoRef, w.viaOrigin = ref, viaOrigin
		w.Out.Warn("Could not read back %s yet: %v", ref, errSummary(err))
		return
	}
	w.setRepo(&v, ref, viaOrigin)
}

func (w *Wizard) pickCluster(ctx context.Context) (string, error) {
	if w.Opts.Cluster != "" {
		return w.Opts.Cluster, nil
	}
	clusters, err := w.Client.Clusters(ctx)
	if err != nil || len(clusters) == 0 {
		w.Out.Warn("Could not list clusters; using the CLI default.")
		return "", nil
	}
	def := clusters[0].Host
	for _, c := range clusters {
		if c.Jurisdiction == w.auth.Jurisdiction {
			def = c.Host
			if c.IsDefault {
				break
			}
		}
	}
	opts := make([]ui.Option, len(clusters))
	for i, c := range clusters {
		opts[i] = ui.Option{Label: fmt.Sprintf("%s  %s", strings.ToUpper(c.Jurisdiction), ui.Muted(c.Host)), Value: c.Host}
	}
	return w.P.Select("Which region should host the mirror?",
		"Defaults to the region of your Entire account ("+w.auth.Jurisdiction+").", opts, def)
}

// ---- create -------------------------------------------------------------------

func (w *Wizard) createRepo(ctx context.Context) error {
	project, err := w.pickProject(ctx)
	if err != nil {
		return err
	}
	if project == "" {
		return nil // pickProject recorded the failure
	}

	name := w.Opts.RepoName
	if name == "" {
		name, err = w.P.Input("Repository name", "Lowercase letters, digits and dashes.",
			detect.SanitizeName(detect.DirName(w.root)), validateName)
		if err != nil {
			return err
		}
	}
	visibility := w.Opts.Visibility
	if visibility == "" {
		visibility, err = w.P.Select("Visibility", "", []ui.Option{
			{Label: "Private", Value: "private"},
			{Label: "Public", Value: "public"},
		}, "private")
		if err != nil {
			return err
		}
	}

	w.announce(fmt.Sprintf("entire repo create %s --project %s --visibility %s", name, project, visibility))
	created, err := w.Client.RepoCreate(ctx, name, project, visibility)
	if err != nil {
		w.record(stepRepo, StatusFailed, errSummary(err))
		w.Out.Fail("Creating the repository failed: %v", errSummary(err))
		return nil
	}
	ref := created.Path
	w.done("Created %s", ui.Bold(ref))

	// Wire a git remote. Prefer origin when there is none; never clobber a
	// GitHub (or other) origin.
	remote, override := "origin", false
	if detect.RemoteURL(ctx, w.root, "origin") != "" {
		remote = "entire"
		override = detect.RemoteURL(ctx, w.root, "entire") != ""
	}
	cmd := fmt.Sprintf("entire repo remote add %s %s", remote, ref)
	if override {
		cmd += " --override"
	}
	w.announce(cmd)
	if err := w.Client.RemoteAdd(ctx, remote, ref, override); err != nil {
		w.record(stepRepo, StatusFailed, "created "+ref+" but adding the remote failed")
		w.Out.Fail("Adding the %s remote failed: %v", remote, errSummary(err))
		return nil
	}
	w.done("Remote %s → %s", ui.Code(remote), ref)

	pushed := false
	if w.hasCommits {
		branch := detect.CurrentBranch(ctx, w.root)
		if branch == "" {
			branch = "HEAD"
		}
		w.announce(fmt.Sprintf("git push -u %s %s", remote, branch))
		if _, err := w.Client.Git(ctx, hostcli.Stream, true, "push", "-u", remote, branch); err != nil {
			w.Out.Warn("Push failed: %v", errSummary(err))
			w.Out.Info("Push later with %s.", ui.Code("git push -u "+remote+" "+branch))
		} else {
			pushed = true
			w.done("Pushed %s", branch)
		}
	}

	w.repoRef, w.viaOrigin = ref, remote == "origin"
	if !w.Opts.DryRun {
		if v, err := w.Client.RepoView(ctx, ref); err == nil {
			w.setRepo(&v, ref, remote == "origin")
		}
	}
	detail := "created " + ref
	if !pushed && w.hasCommits && !w.Opts.DryRun {
		detail += " (not pushed)"
	}
	w.record(stepRepo, StatusDone, detail)
	return nil
}

func (w *Wizard) pickProject(ctx context.Context) (string, error) {
	if w.Opts.Project != "" {
		return w.Opts.Project, nil
	}
	projects, err := w.Client.Projects(ctx)
	if err != nil {
		w.Out.Warn("Could not list projects: %v", errSummary(err))
	}
	var opts []ui.Option
	def := newProjectValue
	for _, p := range projects {
		// Skip read-only projects and system projects such as the
		// "gh:mirrors:<id>" containers that hold GitHub mirrors.
		if !p.Capabilities.CanCreateRepository || strings.Contains(p.Name, ":") {
			continue
		}
		owner := p.OwnerName
		if p.OwnerType == "account" {
			owner = "personal"
		}
		opts = append(opts, ui.Option{Label: fmt.Sprintf("%s  %s", p.Name, ui.Muted(owner+" · "+p.Region)), Value: p.Name})
		if def == newProjectValue && p.OwnerType == "account" {
			def = p.Name
		}
	}
	opts = append(opts, ui.Option{Label: "+ Create a new project", Value: newProjectValue})

	choice, err := w.P.Select("Which project should own the repository?",
		"Projects group repositories, trails and access.", opts, def)
	if err != nil {
		return "", err
	}
	if choice != newProjectValue {
		return choice, nil
	}

	name, err := w.P.Input("New project name", "3-32 lowercase letters, digits and dashes; unique across Entire.",
		detect.SanitizeName(handle(w.auth.User)+"-"+detect.DirName(w.root)), validateName)
	if err != nil {
		return "", err
	}
	owner := w.auth.User
	w.announce(fmt.Sprintf("entire project create %s --owner %s --owner-type account --region %s", name, owner, w.auth.Jurisdiction))
	p, err := w.Client.ProjectCreate(ctx, name, owner, "account", w.auth.Jurisdiction)
	if err != nil {
		w.record(stepRepo, StatusFailed, errSummary(err))
		w.Out.Fail("Creating project %s failed: %v", name, errSummary(err))
		return "", nil
	}
	w.done("Created project %s", ui.Bold(p.Name))
	return p.Name, nil
}

var nameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,30}[a-z0-9]$`)

func validateName(s string) error {
	if !nameRE.MatchString(s) {
		return errors.New("use 3-32 lowercase letters, digits or dashes (not at either end)")
	}
	return nil
}

// refFromEntireURL turns entire://host/et/p/r into /et/p/r.
func refFromEntireURL(url string) string {
	rest := strings.TrimPrefix(strings.TrimSpace(url), "entire://")
	i := strings.Index(rest, "/")
	if i < 0 {
		return ""
	}
	return strings.TrimSuffix(rest[i:], ".git")
}

func placementsSummary(v hostcli.RepoView) string {
	var js []string
	for _, p := range v.Placements {
		js = append(js, p.Jurisdiction)
	}
	s := v.Status
	if len(js) > 0 {
		s += " in " + strings.Join(js, ", ")
	}
	return s
}
