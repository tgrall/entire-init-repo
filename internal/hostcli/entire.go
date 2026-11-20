package hostcli

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// EntireBinary is the host CLI. It is resolved from PATH; the CLI puts its
// managed plugin dir first on PATH but does not tell plugins its own path.
const EntireBinary = "entire"

// Client wraps the `entire` subcommands the onboarding uses.
type Client struct {
	R   Runner
	Dir string // repository root; every command runs here
}

func (c *Client) cmd(mode Mode, mutates bool, args ...string) Cmd {
	return Cmd{Name: EntireBinary, Args: args, Dir: c.Dir, Mode: mode, Mutates: mutates}
}

func (c *Client) captureJSON(ctx context.Context, target any, args ...string) error {
	res, err := c.R.Run(ctx, c.cmd(Capture, false, args...))
	if err != nil {
		return err
	}
	if err := DecodeFirstJSON(res.Stdout, target); err != nil {
		return fmt.Errorf("entire %s: parse JSON: %w", strings.Join(args, " "), err)
	}
	return nil
}

// Git runs a git command in the repo.
func (c *Client) Git(ctx context.Context, mode Mode, mutates bool, args ...string) (Result, error) {
	return c.R.Run(ctx, Cmd{Name: "git", Args: args, Dir: c.Dir, Mode: mode, Mutates: mutates})
}

// Entire runs an arbitrary `entire` command.
func (c *Client) Entire(ctx context.Context, mode Mode, mutates bool, args ...string) (Result, error) {
	return c.R.Run(ctx, c.cmd(mode, mutates, args...))
}

// ---- auth -----------------------------------------------------------------

// AuthStatus is `entire auth status --json`.
type AuthStatus struct {
	LoggedIn     bool   `json:"logged_in"`
	User         string `json:"user"`
	DisplayName  string `json:"display_name"`
	Jurisdiction string `json:"jurisdiction"`
	Server       string `json:"server"`
	Context      string `json:"context"`
	Error        string `json:"error"`
}

// AuthStatus reports whether the user is logged in. A logged-out user is
// exit 0 with logged_in=false; a non-zero exit still carries a JSON error.
func (c *Client) AuthStatus(ctx context.Context) (AuthStatus, error) {
	var s AuthStatus
	res, err := c.R.Run(ctx, c.cmd(Capture, false, "auth", "status", "--json"))
	if decErr := DecodeFirstJSON(res.Stdout, &s); decErr != nil {
		if err != nil {
			return s, err
		}
		return s, fmt.Errorf("entire auth status: parse JSON: %w", decErr)
	}
	if err != nil && s.Error == "" {
		return s, err
	}
	return s, nil
}

// Login runs `entire login` attached to the terminal (browser or device flow).
func (c *Client) Login(ctx context.Context) error {
	_, err := c.R.Run(ctx, c.cmd(Interactive, true, "login"))
	return err
}

// ---- enable / agents --------------------------------------------------------

// Status is `entire status --json`. It always exits 0; "not set up" and "not
// a git repository" arrive in Error.
type Status struct {
	Enabled bool     `json:"enabled"`
	Agents  []string `json:"agents"` // display names, e.g. "Claude Code"
	Error   string   `json:"error"`
}

// Status returns whether Entire is enabled in the repo and which agents.
func (c *Client) Status(ctx context.Context) (Status, error) {
	var s Status
	err := c.captureJSON(ctx, &s, "status", "--json")
	return s, err
}

// EnableOptions are the non-interactive `entire enable` flags we use.
type EnableOptions struct {
	Agent         string
	ImportHistory bool
}

// Enable runs `entire enable --agent <a>`. --agent alone disables every
// prompt; we deliberately never pass --yes because it selects ALL agents.
func (c *Client) Enable(ctx context.Context, o EnableOptions) error {
	args := []string{"enable", "--agent", o.Agent}
	if o.ImportHistory {
		args = append(args, "--import-history")
	}
	_, err := c.R.Run(ctx, c.cmd(Stream, true, args...))
	return err
}

// AgentAdd runs `entire agent add <name>` (never prompts).
func (c *Client) AgentAdd(ctx context.Context, name string) error {
	_, err := c.R.Run(ctx, c.cmd(Stream, true, "agent", "add", name))
	return err
}

// ---- control plane ----------------------------------------------------------

// Placement is one cluster copy of a repo.
type Placement struct {
	Cluster      string `json:"cluster"`
	ClusterSlug  string `json:"clusterSlug"`
	Jurisdiction string `json:"jurisdiction"`
	Status       string `json:"status"`
	CloneURL     string `json:"cloneUrl"`
}

// RepoView is `entire repo view --json`.
type RepoView struct {
	Repo       string      `json:"repo"`
	Private    bool        `json:"private"`
	Status     string      `json:"status"`
	Placements []Placement `json:"placements"`
	RepoID     string      `json:"repoId"`
	APIURL     string      `json:"apiUrl"`
}

// ErrRepoNotLinked means `entire repo view` could not resolve the current
// checkout to an Entire repo (no origin, or a GitHub origin not mirrored).
var ErrRepoNotLinked = errors.New("repository is not linked to Entire")

// RepoView resolves ref (or, when empty, the checkout's origin) to an Entire
// repo. A missing/unmirrored origin returns ErrRepoNotLinked with the CLI's
// own explanation wrapped.
func (c *Client) RepoView(ctx context.Context, ref string) (RepoView, error) {
	args := []string{"repo", "view"}
	if ref != "" {
		args = append(args, ref)
	}
	args = append(args, "--json")
	var v RepoView
	err := c.captureJSON(ctx, &v, args...)
	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		msg := exitErr.Stderr
		if strings.Contains(msg, "not found") || strings.Contains(msg, "no repo matching") ||
			strings.Contains(msg, "not visible") || strings.Contains(msg, "unsupported") {
			return v, fmt.Errorf("%w: %s", ErrRepoNotLinked, strings.TrimSpace(lastLines(msg, 1)))
		}
	}
	return v, err
}

// Cluster is one row of `entire cluster list --json`.
type Cluster struct {
	Host         string `json:"host"`
	Slug         string `json:"slug"`
	Jurisdiction string `json:"jurisdiction"`
	IsDefault    bool   `json:"isDefault"`
	APIURL       string `json:"apiUrl"`
}

// Clusters lists the clusters the user can place repos on.
func (c *Client) Clusters(ctx context.Context) ([]Cluster, error) {
	var cs []Cluster
	err := c.captureJSON(ctx, &cs, "cluster", "list", "--json")
	return cs, err
}

// Project is one row of `entire project list --json`.
type Project struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	OwnerName    string `json:"ownerName"`
	OwnerType    string `json:"ownerType"`
	Region       string `json:"region"`
	Capabilities struct {
		CanCreateRepository bool `json:"canCreateRepository"`
	} `json:"capabilities"`
}

// Projects lists projects visible to the user.
func (c *Client) Projects(ctx context.Context) ([]Project, error) {
	var ps []Project
	err := c.captureJSON(ctx, &ps, "project", "list", "--json")
	return ps, err
}

// ProjectCreate creates a project owned by the user's personal account.
func (c *Client) ProjectCreate(ctx context.Context, name, owner, ownerType, region string) (Project, error) {
	args := []string{"project", "create", name, "--owner", owner, "--owner-type", ownerType, "--json"}
	if region != "" {
		args = append(args, "--region", region)
	}
	var p Project
	res, err := c.R.Run(ctx, c.cmd(Capture, true, args...))
	if err != nil {
		return p, err
	}
	if len(res.Stdout) == 0 { // dry-run
		p.Name = name
		return p, nil
	}
	if err := DecodeFirstJSON(res.Stdout, &p); err != nil {
		return p, fmt.Errorf("entire project create: parse JSON: %w", err)
	}
	return p, nil
}

// CreatedRepo is the subset of `entire repo create --json` we use.
type CreatedRepo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Path        string `json:"path"` // /et/<project>/<name>
	ClusterHost string `json:"clusterHost"`
	Remote      string `json:"remote"` // entire://host/et/<project>/<name>
}

// RepoCreate creates a native Entire repo and waits for it to be ready.
func (c *Client) RepoCreate(ctx context.Context, name, project, visibility string) (CreatedRepo, error) {
	args := []string{"repo", "create", name, "--project", project, "--json"}
	if visibility != "" {
		args = append(args, "--visibility", visibility)
	}
	var r CreatedRepo
	res, err := c.R.Run(ctx, c.cmd(Capture, true, args...))
	if err != nil {
		return r, err
	}
	if len(res.Stdout) == 0 { // dry-run
		r.Name, r.Path = name, "/et/"+project+"/"+name
		return r, nil
	}
	if err := DecodeFirstJSON(res.Stdout, &r); err != nil {
		return r, fmt.Errorf("entire repo create: parse JSON: %w", err)
	}
	if r.Path == "" {
		r.Path = "/et/" + project + "/" + name
	}
	return r, nil
}

// RemoteAdd points a git remote at an Entire repo.
func (c *Client) RemoteAdd(ctx context.Context, remote, ref string, override bool) error {
	args := []string{"repo", "remote", "add", remote}
	if ref != "" {
		args = append(args, ref)
	}
	if override {
		args = append(args, "--override")
	}
	_, err := c.R.Run(ctx, c.cmd(Stream, true, args...))
	return err
}

// MirrorEntry is one row of `entire repo mirror list --json`. Status is
// "available" / "owner-only" for candidates, or the clone status
// (ready/processing/failed/suspended/mixed) for already-mirrored repos.
type MirrorEntry struct {
	Repo    string `json:"repo"`
	Private bool   `json:"private"`
	Status  string `json:"status"`
	Access  string `json:"access"`
}

// MirrorLookup finds owner/repo in the GitHub mirror directory. It returns
// (nil, nil) when the repo is not visible at all.
func (c *Client) MirrorLookup(ctx context.Context, ownerRepo string) (*MirrorEntry, error) {
	var es []MirrorEntry
	if err := c.captureJSON(ctx, &es, "repo", "mirror", "list", "--name", ownerRepo, "--json", "--all", "--no-pager"); err != nil {
		return nil, err
	}
	want := "/gh/" + strings.ToLower(ownerRepo)
	for i := range es {
		if strings.ToLower(es[i].Repo) == want {
			return &es[i], nil
		}
	}
	return nil, nil
}

// MirrorAdd onboards a GitHub repo onto the given clusters. Idempotent for
// the same (repo, cluster).
func (c *Client) MirrorAdd(ctx context.Context, ownerRepo string, clusters []string) error {
	args := []string{"repo", "mirror", "add", "/gh/" + ownerRepo}
	if len(clusters) > 0 {
		args = append(args, "--cluster", strings.Join(clusters, ","))
	}
	_, err := c.R.Run(ctx, c.cmd(Stream, true, args...))
	return err
}

// ---- plugins / brain --------------------------------------------------------

// PluginInstalled reports whether `entire plugin info <name>` shows it
// installed. ok=false with err=nil means "known but not installed".
func (c *Client) PluginInstalled(ctx context.Context, name string) (installed bool, version string, err error) {
	res, err := c.R.Run(ctx, c.cmd(Capture, false, "plugin", "info", name))
	if err != nil {
		return false, "", err
	}
	for _, line := range strings.Split(string(res.Stdout), "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(k) != "Installed" {
			continue
		}
		v = strings.TrimSpace(v)
		if v == "" || v == "no" {
			return false, "", nil
		}
		ver, _, _ := strings.Cut(v, " ")
		return true, ver, nil
	}
	return false, "", nil
}

// PluginInstall installs a plugin (and its dependencies) from the index.
func (c *Client) PluginInstall(ctx context.Context, name string) error {
	_, err := c.R.Run(ctx, c.cmd(Stream, true, "plugin", "install", name, "--yes"))
	return err
}

// BrainSetupOptions selects how much `entire brain setup` does.
type BrainSetupOptions struct {
	// Backfill allows the token-spending background distillation and the
	// watcher daemon. When false, --no-backfill --no-daemon spends nothing.
	Backfill bool
	// SupportsYes is true when the installed brain has a --yes flag (needed
	// to consent to backfill without a terminal).
	SupportsYes bool
}

// BrainSetup builds the repository brain.
func (c *Client) BrainSetup(ctx context.Context, o BrainSetupOptions) error {
	args := []string{"brain", "setup"}
	if o.Backfill {
		args = append(args, "--backfill", "--daemon")
		if o.SupportsYes {
			args = append(args, "--yes")
		}
	} else {
		args = append(args, "--no-backfill", "--no-daemon")
	}
	_, err := c.R.Run(ctx, c.cmd(Stream, true, args...))
	return err
}

// BrainSetupSupportsYes checks `entire brain setup --help` for --yes.
func (c *Client) BrainSetupSupportsYes(ctx context.Context) bool {
	res, err := c.R.Run(ctx, c.cmd(Capture, false, "brain", "setup", "--help"))
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(res.Stdout), "\n") {
		if f := strings.Fields(line); len(f) > 0 && f[0] == "--yes" {
			return true
		}
	}
	return false
}

// BrainConnect binds the repo to its hosted brain. repoID/apiURL are needed
// only when the Entire remote is not `origin`.
func (c *Client) BrainConnect(ctx context.Context, repoID, apiURL string) error {
	args := []string{"brain", "connect"}
	if repoID != "" && apiURL != "" {
		args = append(args, "--repo-id", repoID, "--api-url", apiURL)
	}
	_, err := c.R.Run(ctx, c.cmd(Stream, true, args...))
	return err
}

// BrainStatus is the subset of `entire brain status --json` we use.
type BrainStatus struct {
	Brain struct {
		ManifestState string `json:"manifest_state"`
	} `json:"brain"`
	Sources map[string]bool `json:"sources"`
	Hosted  struct {
		State     string `json:"state"`
		RepoState string `json:"repo_state"`
	} `json:"hosted"`
}

// Built reports whether a queryable brain already exists for the repo.
func (b BrainStatus) Built() bool {
	if b.Brain.ManifestState == "" || b.Brain.ManifestState == "absent" {
		return false
	}
	for _, ok := range b.Sources {
		if ok {
			return true
		}
	}
	return false
}

// Connected reports whether the repo is bound to its hosted brain.
func (b BrainStatus) Connected() bool {
	return b.Hosted.RepoState == "connected" || b.Hosted.State == "connected"
}

// BrainStatus reads the repo's brain state. Requires the brain plugin.
func (c *Client) BrainStatus(ctx context.Context) (BrainStatus, error) {
	var s BrainStatus
	err := c.captureJSON(ctx, &s, "brain", "status", "--json")
	return s, err
}
