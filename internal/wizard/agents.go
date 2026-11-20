package wizard

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/entireio/entire-init/internal/detect"
	"github.com/entireio/entire-init/internal/hostcli"
	"github.com/entireio/entire-init/internal/ui"
)

const stepAgents = "Agents & session capture"

func (w *Wizard) stepAgents(ctx context.Context) error {
	if !w.isRepo {
		w.record(stepAgents, StatusSkipped, "no git repository")
		w.Out.Info("Skipped: no git repository.")
		return nil
	}
	st, err := w.Client.Status(ctx)
	if err != nil {
		w.record(stepAgents, StatusFailed, errSummary(err))
		w.Out.Fail("Could not read Entire status: %v", errSummary(err))
		return nil
	}
	w.enabled = st.Enabled
	installed := map[string]bool{}
	for _, d := range st.Agents {
		if a, ok := detect.AgentByDisplayName(d); ok {
			installed[a.Name] = true
			w.agents = append(w.agents, a.Name)
		}
	}
	if st.Enabled {
		w.Out.OK("Entire is enabled %s", ui.Muted("(agents: "+orNone(st.Agents)+")"))
	} else {
		w.Out.Info("Entire is not enabled in this repository yet.")
	}

	// Build the choice list with the reason each agent is suggested.
	detections := detect.DetectAgents(w.root)
	var opts []ui.Option
	var defaults []string
	for _, d := range detections {
		label := d.Agent.DisplayName
		switch {
		case installed[d.Agent.Name]:
			label += ui.Muted("  (already enabled)")
			defaults = append(defaults, d.Agent.Name)
		case d.Binary != "":
			label += ui.Muted("  (" + d.Binary + " found on PATH)")
			defaults = append(defaults, d.Agent.Name)
		case d.Marker != "":
			label += ui.Muted("  (" + d.Marker + " in repo)")
			defaults = append(defaults, d.Agent.Name)
		}
		opts = append(opts, ui.Option{Label: label, Value: d.Agent.Name})
	}
	if len(defaults) == 0 {
		defaults = []string{"claude-code"}
	}

	var selected []string
	if w.Opts.Agents != nil {
		for _, name := range w.Opts.Agents {
			if _, ok := detect.AgentByName(name); !ok {
				return fmt.Errorf("unknown agent %q (supported: %s)", name, supportedAgents())
			}
		}
		selected = append(selected, w.Opts.Agents...)
	} else {
		selected, err = w.P.MultiSelect(
			"Which agents should Entire capture sessions for?",
			"Entire installs hooks so every session becomes a searchable checkpoint. Space to toggle, Enter to confirm.",
			opts, defaults)
		if err != nil {
			return err
		}
	}

	var toAdd []string
	for _, name := range selected {
		if !installed[name] && !slices.Contains(toAdd, name) {
			toAdd = append(toAdd, name)
		}
	}
	for name := range installed {
		if !slices.Contains(selected, name) {
			w.Out.Info("%s stays enabled (remove it with %s).", name, ui.Code("entire agent remove "+name))
		}
	}

	if len(toAdd) == 0 {
		if st.Enabled {
			w.record(stepAgents, StatusAlready, strings.Join(w.agents, ", "))
			w.Out.OK("Nothing to add.")
		} else {
			w.record(stepAgents, StatusSkipped, "no agent selected; Entire not enabled")
			w.Out.Warn("No agent selected, so Entire was not enabled.")
		}
		return nil
	}

	var added, failed []string
	rest := toAdd
	if !st.Enabled {
		importHistory := w.Opts.ImportHistory
		if !importHistory && w.Opts.Agents == nil {
			importHistory, err = w.P.Confirm("Import the last 30 days of agent history?",
				"Turns your recent sessions into checkpoints so search, recap and Brain have something to work with from day one.", false)
			if err != nil {
				return err
			}
		}
		first := toAdd[0]
		rest = toAdd[1:]
		c := hostcli.EnableOptions{Agent: first, ImportHistory: importHistory}
		cmd := "entire enable --agent " + first
		if importHistory {
			cmd += " --import-history"
		}
		w.announce(cmd)
		if err := w.Client.Enable(ctx, c); err != nil {
			w.Out.Fail("Enabling Entire failed: %v", errSummary(err))
			w.record(stepAgents, StatusFailed, errSummary(err))
			return nil
		}
		w.enabled = true
		added = append(added, first)
	}
	for _, name := range rest {
		w.announce("entire agent add " + name)
		if err := w.Client.AgentAdd(ctx, name); err != nil {
			w.Out.Fail("Adding %s failed: %v", name, errSummary(err))
			failed = append(failed, name)
			continue
		}
		added = append(added, name)
	}
	w.agents = append(w.agents, added...)

	if len(added) > 0 {
		w.done("Session capture enabled for %s", strings.Join(added, ", "))
	}
	switch {
	case len(failed) > 0:
		w.record(stepAgents, StatusFailed, "failed: "+strings.Join(failed, ", "))
	default:
		w.record(stepAgents, StatusDone, strings.Join(w.agents, ", "))
	}
	return nil
}

func orNone(xs []string) string {
	if len(xs) == 0 {
		return "none"
	}
	return strings.Join(xs, ", ")
}

func supportedAgents() string {
	names := make([]string, len(detect.Agents))
	for i, a := range detect.Agents {
		names[i] = a.Name
	}
	return strings.Join(names, ", ")
}
