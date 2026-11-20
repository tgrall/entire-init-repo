// Package detect inspects the local machine and repository.
package detect

import (
	"os"
	"os/exec"
	"path/filepath"
)

// Agent is a coding agent Entire can capture sessions for.
type Agent struct {
	Name        string   // `entire agent add` name
	DisplayName string   // as shown by `entire status --json`
	Binaries    []string // CLI executables that indicate it is installed
	RepoMarkers []string // files/dirs in the repo that indicate it is used
}

// Agents is the set of built-in agents supported by the Entire CLI, in the
// order they are offered. Binary names mirror the CLI's own launch logic.
var Agents = []Agent{
	{Name: "claude-code", DisplayName: "Claude Code", Binaries: []string{"claude"}, RepoMarkers: []string{".claude", "CLAUDE.md"}},
	{Name: "codex", DisplayName: "Codex", Binaries: []string{"codex"}, RepoMarkers: []string{".codex"}},
	{Name: "cursor", DisplayName: "Cursor", Binaries: []string{"cursor-agent", "cursor"}, RepoMarkers: []string{".cursor"}},
	{Name: "copilot-cli", DisplayName: "Copilot CLI", Binaries: []string{"copilot"}, RepoMarkers: nil},
	{Name: "opencode", DisplayName: "OpenCode", Binaries: []string{"opencode"}, RepoMarkers: []string{".opencode", "opencode.json"}},
	{Name: "factoryai-droid", DisplayName: "Factory AI Droid", Binaries: []string{"droid"}, RepoMarkers: []string{".factory"}},
	{Name: "pi", DisplayName: "Pi", Binaries: []string{"pi"}, RepoMarkers: []string{".pi"}},
	{Name: "antigravity", DisplayName: "Antigravity", Binaries: []string{"agy"}, RepoMarkers: nil},
}

// AgentByName returns the agent with the given `entire agent add` name.
func AgentByName(name string) (Agent, bool) {
	for _, a := range Agents {
		if a.Name == name {
			return a, true
		}
	}
	return Agent{}, false
}

// AgentByDisplayName maps a `entire status` display name back to an agent.
func AgentByDisplayName(display string) (Agent, bool) {
	for _, a := range Agents {
		if a.DisplayName == display {
			return a, true
		}
	}
	return Agent{}, false
}

// LookPath is exec.LookPath, swappable in tests.
var LookPath = exec.LookPath

// Detection explains why an agent was pre-selected.
type Detection struct {
	Agent  Agent
	Binary string // binary found on PATH, if any
	Marker string // repo marker found, if any
}

// Found reports whether there is any signal for this agent.
func (d Detection) Found() bool { return d.Binary != "" || d.Marker != "" }

// DetectAgents checks PATH and the repo for every known agent.
func DetectAgents(repoRoot string) []Detection {
	out := make([]Detection, 0, len(Agents))
	for _, a := range Agents {
		d := Detection{Agent: a}
		for _, b := range a.Binaries {
			if _, err := LookPath(b); err == nil {
				d.Binary = b
				break
			}
		}
		if repoRoot != "" {
			for _, m := range a.RepoMarkers {
				if _, err := os.Stat(filepath.Join(repoRoot, m)); err == nil {
					d.Marker = m
					break
				}
			}
		}
		out = append(out, d)
	}
	return out
}
