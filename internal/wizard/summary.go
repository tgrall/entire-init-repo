package wizard

import (
	"fmt"
	"strings"

	"github.com/entireio/entire-init/internal/detect"
	"github.com/entireio/entire-init/internal/ui"
)

func (w *Wizard) printSummary() {
	out := w.Out
	out.Banner("Summary", "")
	width := 0
	for _, r := range w.results {
		if len(r.Step) > width {
			width = len(r.Step)
		}
	}
	for _, r := range w.results {
		label := r.Status
		switch label {
		case StatusAlready:
			label = "ready"
		case StatusPlanned:
			label = "planned"
		}
		out.Line(fmt.Sprintf("  %s %s  %s  %s", ui.Glyph(r.Status), ui.Pad(r.Step, width), ui.Pad(label, 8), ui.Muted(r.Detail)))
	}

	steps := w.nextSteps()
	if len(steps) == 0 {
		return
	}
	out.Banner("Next steps", "")
	for i, s := range steps {
		out.Line(fmt.Sprintf("  %d. %s", i+1, s))
	}
	out.Line("")
}

func (w *Wizard) nextSteps() []string {
	var s []string
	status := map[string]string{}
	for _, r := range w.results {
		status[r.Step] = r.Status
	}

	if !w.loggedIn() && !w.Opts.DryRun {
		s = append(s, "Log in: "+ui.Code("entire login")+", then re-run "+ui.Code("entire init")+".")
	}
	if w.Opts.DryRun {
		s = append(s, "Run "+ui.Code("entire init")+" without --dry-run to apply these changes.")
	}

	// Agents & sessions.
	if len(w.agents) > 0 {
		var bins []string
		for _, name := range w.agents {
			if a, ok := detect.AgentByName(name); ok && len(a.Binaries) > 0 {
				bins = append(bins, ui.Code(a.Binaries[0]))
			}
		}
		line := "Start an agent session"
		if len(bins) > 0 {
			line += " (" + strings.Join(bins, ", ") + ")"
		}
		line += ". Entire captures it automatically. Review with " + ui.Code("entire session list") + " and " + ui.Code("entire recap") + "."
		s = append(s, line)
		s = append(s, "Commit the config Entire generated ("+ui.Code(".entire/")+" and agent hook files) so teammates get session capture too: "+ui.Code("git status")+".")
	}

	// Repo.
	if status[stepRepo] == StatusSkipped && w.loggedIn() {
		s = append(s, "Put the repo on Entire: "+ui.Code("entire repo mirror add /gh/<owner>/<repo>")+" or "+ui.Code("entire repo create <name> --project <project>")+".")
	}

	// Brain.
	switch {
	case w.brainPending:
		s = append(s, "Connect Brain to Entire: "+ui.Code("entire brain connect")+".")
	case status[stepBrain] == StatusSkipped:
		s = append(s, "Add Brain later: "+ui.Code("entire brain setup --no-backfill --no-daemon")+" then "+ui.Code("entire brain connect")+".")
	}
	if w.brainSetup {
		s = append(s, "Brief your agent before a task: "+ui.Code(`entire brain brief "<what you are about to do>"`)+".")
	}

	// Trails.
	if w.repoRef != "" {
		s = append(s, "Plan work as a trail: "+ui.Code(`entire trail create --title "My first trail" --checkout`)+", then follow it with "+ui.Code("entire trail list --mine")+".")
		s = append(s, "Summarize what your agents shipped: "+ui.Code("entire dispatch")+".")
	}
	return s
}
