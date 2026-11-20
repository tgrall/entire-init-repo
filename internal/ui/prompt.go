// Package ui holds the wizard's prompts and terminal output styling.
package ui

import (
	"errors"
	"io"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// ErrAborted is returned when the user cancels a prompt (Ctrl+C / Esc).
var ErrAborted = errors.New("aborted")

// Option is one choice in a select.
type Option struct {
	Label string
	Value string
}

// Prompter asks the user questions. The wizard talks only to this interface
// so it can run with real huh forms, with defaults (--yes), or with a script
// in tests.
type Prompter interface {
	Confirm(title, description string, def bool) (bool, error)
	Select(title, description string, opts []Option, def string) (string, error)
	MultiSelect(title, description string, opts []Option, defs []string) ([]string, error)
	Input(title, description, def string, validate func(string) error) (string, error)
}

// HuhPrompter renders prompts with huh, themed like the Entire CLI.
type HuhPrompter struct {
	Accessible bool
	Output     io.Writer // optional; huh defaults to stdout
}

// label strips styling in accessible mode, where screen readers would read
// the escape sequences and huh prints labels verbatim.
func (p HuhPrompter) label(s string) string {
	if p.Accessible {
		return ansi.Strip(s)
	}
	return s
}

func (p HuhPrompter) run(field huh.Field) error {
	f := huh.NewForm(huh.NewGroup(field)).
		WithTheme(Theme()).
		WithAccessible(p.Accessible).
		WithShowHelp(false)
	if p.Output != nil {
		f = f.WithOutput(p.Output)
	}
	if err := f.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return ErrAborted
		}
		return err
	}
	return nil
}

// Confirm implements Prompter.
func (p HuhPrompter) Confirm(title, description string, def bool) (bool, error) {
	v := def
	err := p.run(huh.NewConfirm().Title(title).Description(description).Affirmative("Yes").Negative("No").Value(&v))
	return v, err
}

// Select implements Prompter.
func (p HuhPrompter) Select(title, description string, opts []Option, def string) (string, error) {
	v := def
	hopts := make([]huh.Option[string], len(opts))
	for i, o := range opts {
		hopts[i] = huh.NewOption(p.label(o.Label), o.Value)
	}
	err := p.run(huh.NewSelect[string]().Title(title).Description(description).Options(hopts...).Value(&v))
	return v, err
}

// MultiSelect implements Prompter.
func (p HuhPrompter) MultiSelect(title, description string, opts []Option, defs []string) ([]string, error) {
	v := append([]string(nil), defs...)
	hopts := make([]huh.Option[string], len(opts))
	for i, o := range opts {
		hopts[i] = huh.NewOption(p.label(o.Label), o.Value)
	}
	err := p.run(huh.NewMultiSelect[string]().Title(title).Description(description).Options(hopts...).Height(len(opts) + 2).Value(&v))
	return v, err
}

// Input implements Prompter.
func (p HuhPrompter) Input(title, description, def string, validate func(string) error) (string, error) {
	v := def
	in := huh.NewInput().Title(title).Description(description).Value(&v)
	if validate != nil {
		in = in.Validate(validate)
	}
	err := p.run(in)
	return v, err
}

// DefaultsPrompter answers every question with its default. Used for --yes
// and for flags that pre-answer a step.
type DefaultsPrompter struct{}

// Confirm implements Prompter.
func (DefaultsPrompter) Confirm(_, _ string, def bool) (bool, error) { return def, nil }

// Select implements Prompter.
func (DefaultsPrompter) Select(_, _ string, _ []Option, def string) (string, error) { return def, nil }

// MultiSelect implements Prompter.
func (DefaultsPrompter) MultiSelect(_, _ string, _ []Option, defs []string) ([]string, error) {
	return defs, nil
}

// Input implements Prompter.
func (DefaultsPrompter) Input(_, _, def string, validate func(string) error) (string, error) {
	if validate != nil {
		if err := validate(def); err != nil {
			return "", err
		}
	}
	return def, nil
}

// Theme mirrors the Entire CLI's uiform theme: base16 colours so prompts
// follow the terminal palette, with magenta (ANSI 5) as the accent.
func Theme() huh.Theme {
	return huh.ThemeFunc(func(isDark bool) *huh.Styles {
		t := huh.ThemeBase16(isDark)
		accent := lipgloss.Color(colorAccent)
		t.Focused.Title = t.Focused.Title.UnsetForeground()
		t.Group.Title = t.Group.Title.UnsetForeground()
		t.Focused.UnselectedOption = t.Focused.UnselectedOption.UnsetForeground()
		t.Blurred.Title = t.Blurred.Title.UnsetForeground()
		t.Blurred.UnselectedOption = t.Blurred.UnselectedOption.UnsetForeground()
		t.Focused.SelectSelector = t.Focused.SelectSelector.Foreground(accent)
		t.Focused.MultiSelectSelector = t.Focused.MultiSelectSelector.Foreground(accent)
		t.Focused.SelectedOption = t.Focused.SelectedOption.Foreground(accent)
		t.Focused.SelectedPrefix = t.Focused.SelectedPrefix.Foreground(accent)
		t.Focused.BlurredButton = t.Focused.BlurredButton.UnsetForeground().UnsetBackground()
		t.Blurred.BlurredButton = t.Blurred.BlurredButton.UnsetForeground().UnsetBackground()
		return t
	})
}
