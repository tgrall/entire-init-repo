package detect

import "testing"

func TestParseGitHubRemote(t *testing.T) {
	cases := []struct{ url, owner, repo string }{
		{"https://github.com/entireio/cli.git", "entireio", "cli"},
		{"https://github.com/entireio/cli", "entireio", "cli"},
		{"git@github.com:entireio/entire-brain.git", "entireio", "entire-brain"},
		{"ssh://git@github.com/o/r.js.git", "o", "r.js"},
		{"https://user:tok@github.com/o/r.git", "o", "r"},
	}
	for _, c := range cases {
		o, r, ok := ParseGitHubRemote(c.url)
		if !ok || o != c.owner || r != c.repo {
			t.Errorf("ParseGitHubRemote(%q) = %q, %q, %v", c.url, o, r, ok)
		}
	}
	for _, bad := range []string{"https://gitlab.com/o/r.git", "entire://h/et/p/r", ""} {
		if _, _, ok := ParseGitHubRemote(bad); ok {
			t.Errorf("ParseGitHubRemote(%q) should not match", bad)
		}
	}
}

func TestSanitizeName(t *testing.T) {
	cases := map[string]string{"My_App": "my-app", "entire-cli-setup": "entire-cli-setup", "--x--": "x", "!!!": "my-repo"}
	for in, want := range cases {
		if got := SanitizeName(in); got != want {
			t.Errorf("SanitizeName(%q) = %q, want %q", in, got, want)
		}
	}
}
