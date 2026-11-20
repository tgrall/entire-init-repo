package wizard

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/entireio/entire-init/internal/detect"
	"github.com/entireio/entire-init/internal/hostcli"
	"github.com/entireio/entire-init/internal/ui"
)

// fakeRunner answers `entire` commands from a table keyed by the joined
// argument prefix and records every command it receives. git commands that
// mutate (push) are recorded and succeed without running.
type fakeRunner struct {
	responses map[string]response
	calls     []hostcli.Cmd
}

type response struct {
	stdout string
	stderr string
	exit   int
}

func (f *fakeRunner) Run(_ context.Context, c hostcli.Cmd) (hostcli.Result, error) {
	f.calls = append(f.calls, c)
	key := c.Name + " " + strings.Join(c.Args, " ")
	best := ""
	for k := range f.responses {
		if strings.HasPrefix(key, k) && len(k) > len(best) {
			best = k
		}
	}
	r, ok := f.responses[best]
	if !ok {
		return hostcli.Result{}, nil
	}
	res := hostcli.Result{Stdout: []byte(r.stdout), Stderr: []byte(r.stderr), ExitCode: r.exit}
	if r.exit != 0 {
		return res, &hostcli.ExitError{Cmd: c, ExitCode: r.exit, Stderr: r.stderr}
	}
	return res, nil
}

func (f *fakeRunner) mutating() []string {
	var out []string
	for _, c := range f.calls {
		if c.Mutates {
			out = append(out, c.String())
		}
	}
	return out
}

// scriptedPrompter returns defaults unless a title substring is overridden.
type scriptedPrompter struct {
	ui.DefaultsPrompter
	confirm map[string]bool
	sel     map[string]string
	multi   map[string][]string
	asked   []string
}

func (s *scriptedPrompter) Confirm(title, d string, def bool) (bool, error) {
	s.asked = append(s.asked, title)
	for k, v := range s.confirm {
		if strings.Contains(title, k) {
			return v, nil
		}
	}
	return def, nil
}

func (s *scriptedPrompter) Select(title, d string, o []ui.Option, def string) (string, error) {
	s.asked = append(s.asked, title)
	for k, v := range s.sel {
		if strings.Contains(title, k) {
			return v, nil
		}
	}
	return def, nil
}

func (s *scriptedPrompter) MultiSelect(title, d string, o []ui.Option, defs []string) ([]string, error) {
	s.asked = append(s.asked, title)
	for k, v := range s.multi {
		if strings.Contains(title, k) {
			return v, nil
		}
	}
	return defs, nil
}

const (
	authLoggedIn  = `{"logged_in":true,"user":"github:octo","display_name":"Octo Cat","jurisdiction":"eu"}`
	authLoggedOut = `{"logged_in":false}`
	statusOff     = `{"enabled":false,"agents":null,"active_sessions":null,"error":"not set up"}`
	statusClaude  = `{"enabled":true,"agents":["Claude Code"]}`
	clusters      = `[{"host":"aws-us-east-2.entire.io","slug":"aws-us-east-2","jurisdiction":"us","isDefault":true},
	                  {"host":"aws-eu-central-1.entire.io","slug":"aws-eu-central-1","jurisdiction":"eu","isDefault":true}]`
	projects = `[{"name":"org-proj","ownerName":"acme","ownerType":"org","region":"us","capabilities":{"canCreateRepository":true}},
	             {"name":"octo-proj","ownerName":"octo","ownerType":"account","region":"eu","capabilities":{"canCreateRepository":true}},
	             {"name":"ro-proj","ownerName":"x","ownerType":"org","region":"eu","capabilities":{"canCreateRepository":false}}]`
	brainInfoInstalled = "Name:        brain\nInstalled:   v0.5.1 (from https://github.com/entireio/entire-brain)\n"
	noOrigin           = "remote \"origin\" not found; pass /gh/<owner>/<repo> or /et/<project>/<repo> explicitly\n"
)

// newRepo makes a temp git repo with one commit and an optional origin.
func newRepo(t *testing.T, origin string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "my-app")
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(cmd.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if err := exec.Command("mkdir", "-p", dir).Run(); err != nil {
		t.Fatal(err)
	}
	git("init", "-q", "-b", "main")
	git("commit", "-q", "--allow-empty", "-m", "init")
	if origin != "" {
		git("remote", "add", "origin", origin)
	}
	return dir
}

func setup(t *testing.T, dir string, f *fakeRunner, p ui.Prompter, o Options) (*Wizard, *bytes.Buffer) {
	t.Helper()
	t.Setenv("ENTIRE_REPO_ROOT", "")
	onPath := map[string]bool{"claude": true}
	old := detect.LookPath
	detect.LookPath = func(b string) (string, error) {
		if onPath[b] {
			return "/usr/bin/" + b, nil
		}
		return "", errors.New("not found")
	}
	t.Cleanup(func() { detect.LookPath = old; CanLogin = true })
	var buf bytes.Buffer
	var r hostcli.Runner = f
	if o.DryRun {
		r = hostcli.DryRunner{Inner: f, Out: &buf}
	}
	return &Wizard{
		Opts:   o,
		Client: &hostcli.Client{R: r, Dir: dir},
		P:      p,
		Out:    ui.Printer{W: &buf},
		Cwd:    dir,
	}, &buf
}

func assertCalls(t *testing.T, got, want []string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("mutating commands mismatch\n got:\n  %s\nwant:\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

func statusOf(w *Wizard, step string) string {
	for _, r := range w.Results() {
		if r.Step == step {
			return r.Status
		}
	}
	return ""
}

func TestFreshRepoCreatePath(t *testing.T) {
	dir := newRepo(t, "")
	f := &fakeRunner{responses: map[string]response{
		"entire auth status":       {stdout: authLoggedIn},
		"entire status":            {stdout: statusOff},
		"entire project list":      {stdout: projects},
		"entire repo create":       {stdout: `{"id":"r1","name":"my-app","path":"/et/octo-proj/my-app","remote":"entire://aws-eu-central-1.entire.io/et/octo-proj/my-app"}`},
		"entire repo view /et/":    {stdout: `{"repo":"/et/octo-proj/my-app","status":"ready","repoId":"R1","apiUrl":"https://eu.api"}`},
		"entire plugin info brain": {stdout: brainInfoInstalled},
	}}
	w, out := setup(t, dir, f, ui.DefaultsPrompter{}, Options{Yes: true})
	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v\n%s", err, out)
	}
	assertCalls(t, f.mutating(), []string{
		"entire enable --agent claude-code",
		"entire repo create my-app --project octo-proj --json --visibility private",
		"entire repo remote add origin /et/octo-proj/my-app",
		"git push -u origin main",
		"entire brain setup --no-backfill --no-daemon",
		"entire brain connect",
	})
	if !strings.Contains(out.String(), "entire trail create") {
		t.Errorf("summary should suggest trails:\n%s", out)
	}
}

func TestGitHubOriginMirrorPath(t *testing.T) {
	dir := newRepo(t, "https://github.com/acme/widgets.git")
	f := &fakeRunner{responses: map[string]response{
		"entire auth status":      {stdout: authLoggedIn},
		"entire status":           {stdout: statusOff},
		"entire repo view --json": {stderr: `no repo matching "/gh/acme/widgets" visible from your login`, exit: 1},
		"entire repo mirror list": {stdout: `[{"repo":"/gh/acme/widgets","private":true,"status":"available","access":"admin"}]`},
		"entire cluster list":     {stdout: clusters},
		"entire repo view /gh/":   {stdout: `{"repo":"/gh/acme/widgets","status":"processing","repoId":"R2","apiUrl":"https://eu.api"}`},
	}}
	no := false
	p := &scriptedPrompter{multi: map[string][]string{"agents": {"claude-code", "codex"}}}
	w, out := setup(t, dir, f, p, Options{Brain: &no})
	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v\n%s", err, out)
	}
	assertCalls(t, f.mutating(), []string{
		"entire enable --agent claude-code",
		"entire agent add codex",
		"entire repo mirror add /gh/acme/widgets --cluster aws-eu-central-1.entire.io",
	})
	if got := statusOf(w, stepBrain); got != StatusSkipped {
		t.Errorf("brain status = %q, want skipped", got)
	}
}

func TestAlreadySetUpIsNoop(t *testing.T) {
	dir := newRepo(t, "https://github.com/acme/widgets.git")
	f := &fakeRunner{responses: map[string]response{
		"entire auth status":      {stdout: authLoggedIn},
		"entire status":           {stdout: statusClaude},
		"entire repo view --json": {stdout: `{"repo":"/gh/acme/widgets","status":"ready","placements":[{"jurisdiction":"eu"}],"repoId":"R2","apiUrl":"https://eu.api"}`},
	}}
	p := &scriptedPrompter{confirm: map[string]bool{"Brain": false}}
	w, out := setup(t, dir, f, p, Options{})
	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v\n%s", err, out)
	}
	assertCalls(t, f.mutating(), nil)
	for _, step := range []string{stepAuth, stepAgents, stepRepo} {
		if got := statusOf(w, step); got != StatusAlready {
			t.Errorf("%s status = %q, want already", step, got)
		}
	}
}

func TestDryRunRunsNoMutations(t *testing.T) {
	dir := newRepo(t, "")
	f := &fakeRunner{responses: map[string]response{
		"entire auth status":       {stdout: authLoggedIn},
		"entire status":            {stdout: statusOff},
		"entire project list":      {stdout: projects},
		"entire plugin info brain": {stdout: "Name: brain\nInstalled:   no\n"},
	}}
	w, out := setup(t, dir, f, ui.DefaultsPrompter{}, Options{Yes: true, DryRun: true})
	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v\n%s", err, out)
	}
	assertCalls(t, f.mutating(), nil)
	for _, want := range []string{
		"[dry-run] entire enable --agent claude-code",
		"[dry-run] entire repo create my-app --project octo-proj",
		"[dry-run] entire plugin install brain --yes",
		"[dry-run] entire brain setup --no-backfill --no-daemon",
		"[dry-run] entire brain connect",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("dry-run output missing %q\n%s", want, out)
		}
	}
}

func TestNotLoggedInWithoutTerminal(t *testing.T) {
	dir := newRepo(t, "")
	f := &fakeRunner{responses: map[string]response{
		"entire auth status":       {stdout: authLoggedOut},
		"entire status":            {stdout: statusClaude},
		"entire plugin info brain": {stdout: brainInfoInstalled},
		"entire repo view":         {stderr: noOrigin, exit: 1},
	}}
	w, out := setup(t, dir, f, ui.DefaultsPrompter{}, Options{Yes: true})
	CanLogin = false
	err := w.Run(context.Background())
	if !errors.Is(err, ErrFailed) {
		t.Fatalf("Run err = %v, want ErrFailed\n%s", err, out)
	}
	if got := statusOf(w, stepRepo); got != StatusSkipped {
		t.Errorf("repo status = %q, want skipped", got)
	}
	// Brain still builds locally; connect is left as a next step.
	assertCalls(t, f.mutating(), []string{"entire brain setup --no-backfill --no-daemon"})
	if got := statusOf(w, stepBrain); got != StatusPending {
		t.Errorf("brain status = %q, want pending", got)
	}
	if !strings.Contains(out.String(), "entire login") {
		t.Errorf("summary should tell the user to log in\n%s", out)
	}
}

func TestCreateKeepsGitHubOriginAndUsesEntireRemote(t *testing.T) {
	dir := newRepo(t, "git@github.com:acme/widgets.git")
	f := &fakeRunner{responses: map[string]response{
		"entire auth status":       {stdout: authLoggedIn},
		"entire status":            {stdout: statusClaude},
		"entire repo view --json":  {stderr: `no repo matching "/gh/acme/widgets"`, exit: 1},
		"entire repo create":       {stdout: `{"path":"/et/octo-proj/widgets"}`},
		"entire repo view /et/":    {stdout: `{"repo":"/et/octo-proj/widgets","status":"ready","repoId":"R3","apiUrl":"https://eu.api"}`},
		"entire plugin info brain": {stdout: brainInfoInstalled},
	}}
	yes := true
	w, out := setup(t, dir, f, ui.DefaultsPrompter{}, Options{Yes: true, RepoMode: "create", Project: "octo-proj", RepoName: "widgets", Brain: &yes})
	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v\n%s", err, out)
	}
	assertCalls(t, f.mutating(), []string{
		"entire repo create widgets --project octo-proj --json --visibility private",
		"entire repo remote add entire /et/octo-proj/widgets",
		"git push -u entire main",
		"entire brain setup --no-backfill --no-daemon",
		"entire brain connect --repo-id R3 --api-url https://eu.api",
	})
}

func TestBrainAlreadyBuiltAndConnectedIsSkipped(t *testing.T) {
	dir := newRepo(t, "https://github.com/acme/widgets.git")
	f := &fakeRunner{responses: map[string]response{
		"entire auth status":       {stdout: authLoggedIn},
		"entire status":            {stdout: statusClaude},
		"entire repo view --json":  {stdout: `{"repo":"/gh/acme/widgets","status":"ready","repoId":"R2","apiUrl":"https://eu.api"}`},
		"entire plugin info brain": {stdout: brainInfoInstalled},
		"entire brain status":      {stdout: `{"brain":{"manifest_state":"present"},"sources":{"seed":true},"hosted":{"state":"connected","repo_state":"connected"}}`},
	}}
	w, out := setup(t, dir, f, ui.DefaultsPrompter{}, Options{Yes: true})
	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v\n%s", err, out)
	}
	assertCalls(t, f.mutating(), nil)
	if got := statusOf(w, stepBrain); got != StatusAlready {
		t.Errorf("brain status = %q, want already", got)
	}
}

func TestBrainBuiltButNotConnectedOnlyConnects(t *testing.T) {
	dir := newRepo(t, "https://github.com/acme/widgets.git")
	f := &fakeRunner{responses: map[string]response{
		"entire auth status":       {stdout: authLoggedIn},
		"entire status":            {stdout: statusClaude},
		"entire repo view --json":  {stdout: `{"repo":"/gh/acme/widgets","status":"ready","repoId":"R2","apiUrl":"https://eu.api"}`},
		"entire plugin info brain": {stdout: brainInfoInstalled},
		"entire brain status":      {stdout: `{"brain":{"manifest_state":"present"},"sources":{"history":true},"hosted":{"state":"disconnected"}}`},
	}}
	w, out := setup(t, dir, f, ui.DefaultsPrompter{}, Options{Yes: true})
	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v\n%s", err, out)
	}
	assertCalls(t, f.mutating(), []string{"entire brain connect"})
}
