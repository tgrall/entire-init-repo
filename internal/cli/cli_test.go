package cli

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeEntire is a stand-in `entire` binary: it appends its argv to $LOG and
// prints fixture output for the read-only commands the wizard parses.
const fakeEntire = `#!/bin/sh
echo "$*" >> "$FAKE_ENTIRE_LOG"
case "$*" in
  "auth status --json") echo '{"logged_in":true,"user":"github:octo","jurisdiction":"eu"}' ;;
  "status --json") echo '{"enabled":false,"agents":null,"error":"not set up"}' ;;
  "repo view /et/"*)
    if [ -f "$FAKE_STATE/created" ]; then
      echo '{"repo":"/et/demo/my-app","status":"ready","repoId":"R1","apiUrl":"https://eu.api"}'
    else
      echo 'repo /et/demo/my-app not found or not shared with you' >&2; exit 1
    fi ;;
  "repo create"*) touch "$FAKE_STATE/created"; echo '{"path":"/et/demo/my-app"}'; echo; echo 'Update available! 0.13.4 -> 0.14.0' ;;
  "repo remote add"*) git remote add "$4" "$FAKE_BARE_REMOTE" ;;
  "plugin info brain") printf 'Name: brain\nInstalled:   v0.5.1 (from x)\n' ;;
esac
exit 0
`

func TestEndToEndWithFakeEntire(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fake")
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "entire"), []byte(fakeEntire), 0o755); err != nil {
		t.Fatal(err)
	}
	logFile := filepath.Join(t.TempDir(), "calls.log")
	// The fake `repo remote add` points the remote at a local bare repo so
	// the wizard's real `git push` never leaves the machine.
	bare := filepath.Join(t.TempDir(), "remote.git")
	if out, err := exec.Command("git", "init", "-q", "--bare", bare).CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %v %s", err, out)
	}
	repo := filepath.Join(t.TempDir(), "my-app")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"-c", "user.name=t", "-c", "user.email=t@x", "commit", "-q", "--allow-empty", "-m", "init"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}

	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_ENTIRE_LOG", logFile)
	t.Setenv("FAKE_BARE_REMOTE", bare)
	t.Setenv("FAKE_STATE", t.TempDir())
	t.Setenv("ENTIRE_REPO_ROOT", "")
	t.Setenv("ENTIRE_TEST_TTY", "0")
	t.Chdir(repo)

	var stdout, stderr bytes.Buffer
	code := run(context.Background(), "test", []string{"--yes", "--agents", "claude-code", "--project", "demo"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	logged, _ := os.ReadFile(logFile)
	got := strings.TrimSpace(string(logged))
	want := strings.Join([]string{
		"auth status --json",
		"status --json",
		"enable --agent claude-code",
		"repo view /et/demo/my-app --json",
		"repo create my-app --project demo --json --visibility private",
		"repo remote add origin /et/demo/my-app",
		"repo view /et/demo/my-app --json",
		"plugin info brain",
		"brain status --json",
		"brain setup --no-backfill --no-daemon",
		"brain connect",
	}, "\n")
	if out, err := exec.Command("git", "--git-dir", bare, "rev-parse", "--verify", "main").CombinedOutput(); err != nil {
		t.Errorf("main was not pushed to the remote: %v %s\n%s", err, out, stdout.String())
	}
	if got != want {
		t.Fatalf("entire calls:\n%s\n\nwant:\n%s\n\nstdout:\n%s", got, want, stdout.String())
	}
}

func TestNoTerminalWithoutYesFails(t *testing.T) {
	t.Setenv("ENTIRE_TEST_TTY", "0")
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), "test", nil, &stdout, &stderr)
	if code != exitUsage || !strings.Contains(stderr.String(), "--yes") {
		t.Fatalf("exit %d, stderr %q", code, stderr.String())
	}
}

func TestVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), "v1.2.3", []string{"version"}, &stdout, &stderr); code != 0 || strings.TrimSpace(stdout.String()) != "v1.2.3" {
		t.Fatalf("exit %d out %q", code, stdout.String())
	}
}

func TestBadRepoFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), "x", []string{"--repo", "nope"}, &stdout, &stderr); code != exitUsage {
		t.Fatalf("exit %d", code)
	}
}
