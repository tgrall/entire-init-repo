package detect

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// RepoRoot returns the git worktree root. The Entire CLI exports
// ENTIRE_REPO_ROOT to plugins run inside a repo; when invoked directly as
// `entire-init-repo` we fall back to git.
func RepoRoot(ctx context.Context, cwd string) (string, bool) {
	if r := os.Getenv("ENTIRE_REPO_ROOT"); r != "" {
		return r, true
	}
	out, err := gitOutput(ctx, cwd, "rev-parse", "--show-toplevel")
	if err != nil || out == "" {
		return "", false
	}
	return out, true
}

// HasCommits reports whether HEAD resolves to a commit.
func HasCommits(ctx context.Context, dir string) bool {
	_, err := gitOutput(ctx, dir, "rev-parse", "--verify", "--quiet", "HEAD")
	return err == nil
}

// CurrentBranch returns the checked-out branch name (works on an unborn
// branch too).
func CurrentBranch(ctx context.Context, dir string) string {
	out, err := gitOutput(ctx, dir, "symbolic-ref", "--short", "HEAD")
	if err != nil {
		return ""
	}
	return out
}

// RemoteURL returns the URL of a git remote, or "" if it does not exist.
func RemoteURL(ctx context.Context, dir, remote string) string {
	out, err := gitOutput(ctx, dir, "remote", "get-url", remote)
	if err != nil {
		return ""
	}
	return out
}

// DirName is the repository directory's base name, the default repo name.
func DirName(dir string) string {
	return filepath.Base(filepath.Clean(dir))
}

var githubURL = regexp.MustCompile(`^(?:https?://(?:[^@/]+@)?github\.com/|git@github\.com:|ssh://git@github\.com/)([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+?)(?:\.git)?/?$`)

// ParseGitHubRemote extracts owner/repo from a GitHub remote URL.
func ParseGitHubRemote(url string) (owner, repo string, ok bool) {
	m := githubURL.FindStringSubmatch(strings.TrimSpace(url))
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}

// IsEntireRemote reports whether a remote URL points at Entire.
func IsEntireRemote(url string) bool {
	return strings.HasPrefix(strings.TrimSpace(url), "entire://")
}

var invalidRepoChars = regexp.MustCompile(`[^a-z0-9-]+`)

// SanitizeName turns a directory name into a valid Entire repo/project name
// candidate: lowercase, [a-z0-9-], no leading/trailing dashes.
func SanitizeName(s string) string {
	s = strings.ToLower(s)
	s = invalidRepoChars.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "my-repo"
	}
	return s
}

func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}
