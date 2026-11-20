package hostcli

import (
	"context"
	"errors"
	"testing"
)

func TestDecodeFirstJSONIgnoresTrailingBanner(t *testing.T) {
	var v struct {
		LoggedIn bool `json:"logged_in"`
	}
	in := []byte("{\"logged_in\":true}\n\nUpdate available! 0.13.4 -> 0.14.0\n")
	if err := DecodeFirstJSON(in, &v); err != nil || !v.LoggedIn {
		t.Fatalf("got %v, %+v", err, v)
	}
	if err := DecodeFirstJSON([]byte("oops {}"), &v); err == nil {
		t.Fatal("leading garbage should fail")
	}
}

type stubRunner struct {
	out  string
	err  error
	last Cmd
}

func (s *stubRunner) Run(_ context.Context, c Cmd) (Result, error) {
	s.last = c
	return Result{Stdout: []byte(s.out)}, s.err
}

func TestPluginInstalled(t *testing.T) {
	r := &stubRunner{out: "Name:        brain\nInstalled:   v0.5.1 (from https://x)\nRequires:    graph\n"}
	c := &Client{R: r}
	ok, ver, err := c.PluginInstalled(context.Background(), "brain")
	if err != nil || !ok || ver != "v0.5.1" {
		t.Fatalf("got %v %q %v", ok, ver, err)
	}
	r.out = "Name: investigate\nInstalled:   no\n"
	if ok, _, _ := c.PluginInstalled(context.Background(), "investigate"); ok {
		t.Fatal("expected not installed")
	}
}

func TestRepoViewNotLinked(t *testing.T) {
	r := &stubRunner{err: &ExitError{ExitCode: 1, Stderr: "remote \"origin\" not found; pass /gh/<owner>/<repo>"}}
	c := &Client{R: r}
	if _, err := c.RepoView(context.Background(), ""); !errors.Is(err, ErrRepoNotLinked) {
		t.Fatalf("err = %v, want ErrRepoNotLinked", err)
	}
}

func TestDryRunnerSkipsMutations(t *testing.T) {
	inner := &stubRunner{out: "{}"}
	var buf testBuf
	d := DryRunner{Inner: inner, Out: &buf}
	_, _ = d.Run(context.Background(), Cmd{Name: "entire", Args: []string{"enable", "--agent", "claude-code"}, Mutates: true})
	if inner.last.Name != "" {
		t.Fatal("mutating command reached the inner runner")
	}
	if buf.s != "  [dry-run] entire enable --agent claude-code\n" {
		t.Fatalf("output %q", buf.s)
	}
	_, _ = d.Run(context.Background(), Cmd{Name: "entire", Args: []string{"status", "--json"}})
	if inner.last.Name != "entire" {
		t.Fatal("read-only command should run")
	}
}

func TestBrainSetupFlags(t *testing.T) {
	r := &stubRunner{}
	c := &Client{R: r}
	_ = c.BrainSetup(context.Background(), BrainSetupOptions{})
	if got := r.last.String(); got != "entire brain setup --no-backfill --no-daemon" {
		t.Fatalf("got %q", got)
	}
	_ = c.BrainSetup(context.Background(), BrainSetupOptions{Backfill: true, SupportsYes: true})
	if got := r.last.String(); got != "entire brain setup --backfill --daemon --yes" {
		t.Fatalf("got %q", got)
	}
}

type testBuf struct{ s string }

func (b *testBuf) Write(p []byte) (int, error) { b.s += string(p); return len(p), nil }
