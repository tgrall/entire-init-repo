package ui

import (
	"fmt"
	"io"
	"strings"

	"charm.land/lipgloss/v2"
)

// Base16 palette slots, matching the Entire CLI's palette package.
const (
	colorAccent = "5" // magenta
	colorGreen  = "2"
	colorYellow = "3"
	colorRed    = "1"
	colorMuted  = "8"
)

var (
	styleAccent = lipgloss.NewStyle().Foreground(lipgloss.Color(colorAccent)).Bold(true)
	styleOK     = lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen))
	styleWarn   = lipgloss.NewStyle().Foreground(lipgloss.Color(colorYellow))
	styleFail   = lipgloss.NewStyle().Foreground(lipgloss.Color(colorRed))
	styleMuted  = lipgloss.NewStyle().Foreground(lipgloss.Color(colorMuted))
	styleBold   = lipgloss.NewStyle().Bold(true)
	styleCode   = lipgloss.NewStyle().Foreground(lipgloss.Color(colorAccent))
)

// Printer writes styled progress output.
type Printer struct {
	W io.Writer
}

func (p Printer) println(s string) { _, _ = lipgloss.Fprintln(p.W, s) }

// Line prints a pre-styled line, downsampling colour for non-terminals.
func (p Printer) Line(s string) { p.println(s) }

// Banner prints the wizard's title.
func (p Printer) Banner(title, subtitle string) {
	p.println("")
	p.println(styleAccent.Render("◆ " + title))
	if subtitle != "" {
		p.println(styleMuted.Render("  " + subtitle))
	}
}

// Step prints a numbered section header.
func (p Printer) Step(n, total int, title string) {
	p.println("")
	p.println(styleBold.Render(fmt.Sprintf("%s %s", styleAccent.Render(fmt.Sprintf("[%d/%d]", n, total)), title)))
}

// OK prints a success line.
func (p Printer) OK(format string, a ...any) {
	p.println("  " + styleOK.Render("✓") + " " + fmt.Sprintf(format, a...))
}

// Info prints a neutral line.
func (p Printer) Info(format string, a ...any) {
	p.println("  " + styleMuted.Render("•") + " " + fmt.Sprintf(format, a...))
}

// Warn prints a warning line.
func (p Printer) Warn(format string, a ...any) {
	p.println("  " + styleWarn.Render("!") + " " + fmt.Sprintf(format, a...))
}

// Fail prints an error line.
func (p Printer) Fail(format string, a ...any) {
	p.println("  " + styleFail.Render("✗") + " " + fmt.Sprintf(format, a...))
}

// Run announces a command about to be executed.
func (p Printer) Run(cmd string) {
	p.println("  " + styleMuted.Render("$ "+cmd))
}

// Code styles an inline command.
func Code(s string) string { return styleCode.Render(s) }

// Muted styles secondary text.
func Muted(s string) string { return styleMuted.Render(s) }

// Bold styles emphasised text.
func Bold(s string) string { return styleBold.Render(s) }

// Status glyphs for the summary.
func Glyph(status string) string {
	switch status {
	case "done":
		return styleOK.Render("✓")
	case "already":
		return styleOK.Render("✓")
	case "skipped":
		return styleMuted.Render("–")
	case "failed":
		return styleFail.Render("✗")
	case "planned":
		return styleAccent.Render("→")
	case "pending":
		return styleWarn.Render("…")
	default:
		return " "
	}
}

// Pad right-pads s to width w (by rune count of the unstyled string).
func Pad(s string, w int) string {
	n := lipgloss.Width(s)
	if n >= w {
		return s
	}
	return s + strings.Repeat(" ", w-n)
}
