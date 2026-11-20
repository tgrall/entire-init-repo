package wizard

import (
	"context"
	"fmt"

	"github.com/entireio/entire-init-repo/internal/ui"
)

const stepAuth = "Entire account"

// CanLogin is false when there is no terminal for `entire login`.
var CanLogin = true

func (w *Wizard) stepAuth(ctx context.Context) error {
	st, err := w.Client.AuthStatus(ctx)
	if err != nil {
		w.record(stepAuth, StatusFailed, errSummary(err))
		w.Out.Fail("Could not check login status: %v", errSummary(err))
		return nil
	}
	w.auth = st
	if st.LoggedIn {
		w.Out.OK("Logged in as %s %s", ui.Bold(who(st.DisplayName, st.User)), ui.Muted("("+st.Jurisdiction+")"))
		w.record(stepAuth, StatusAlready, who(st.DisplayName, st.User))
		return nil
	}

	w.Out.Warn("You are not logged in to Entire.")
	if !CanLogin {
		w.record(stepAuth, StatusFailed, "not logged in")
		w.Out.Fail("Run %s in a terminal, then re-run %s.", ui.Code("entire login"), ui.Code("entire init-repo"))
		return nil
	}
	yes, err := w.P.Confirm("Log in to Entire now?", "Needed to create or mirror a repository and to connect Brain.", true)
	if err != nil {
		return err
	}
	if !yes {
		w.record(stepAuth, StatusSkipped, "not logged in")
		return nil
	}
	w.announce("entire login")
	if err := w.Client.Login(ctx); err != nil {
		w.record(stepAuth, StatusFailed, errSummary(err))
		w.Out.Fail("Login failed: %v", errSummary(err))
		return nil
	}
	if w.Opts.DryRun {
		w.record(stepAuth, StatusDone, "would log in")
		return nil
	}
	st, err = w.Client.AuthStatus(ctx)
	if err != nil || !st.LoggedIn {
		w.record(stepAuth, StatusFailed, "still not logged in")
		w.Out.Fail("Still not logged in.")
		return nil
	}
	w.auth = st
	w.Out.OK("Logged in as %s", ui.Bold(who(st.DisplayName, st.User)))
	w.record(stepAuth, StatusDone, who(st.DisplayName, st.User))
	return nil
}

func who(display, user string) string {
	if display != "" && user != "" {
		return fmt.Sprintf("%s (%s)", display, user)
	}
	if user != "" {
		return user
	}
	return display
}
