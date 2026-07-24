// Package style centralizes git-multi-tool's retro-terminal, Charm-flavored look:
// a bright, high-contrast palette, a banner/logo, and reusable lipgloss
// styles so every command feels like part of the same toolkit.
package style

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// Palette mirrors the colors Charm tools (gum, soft-serve, huh) are known
// for: hot fuchsia, electric indigo, minty green, and warning coral, all
// against a cream/charcoal adaptive foreground.
var (
	Fuchsia = lipgloss.Color("#F780E2")
	Indigo  = lipgloss.AdaptiveColor{Light: "#5A56E0", Dark: "#7571F9"}
	Cream   = lipgloss.AdaptiveColor{Light: "#FFFDF5", Dark: "#FFFDF5"}
	Mint    = lipgloss.AdaptiveColor{Light: "#02BA84", Dark: "#02BF87"}
	Coral   = lipgloss.AdaptiveColor{Light: "#FF4672", Dark: "#ED567A"}
	Amber   = lipgloss.Color("#FFB300")
	Gray    = lipgloss.AdaptiveColor{Light: "#767676", Dark: "#9E9E9E"}
	Subtle  = lipgloss.AdaptiveColor{Light: "#B2B2B2", Dark: "#4A4A4A"}
)

var (
	Title = lipgloss.NewStyle().Bold(true).Foreground(Indigo)

	Banner = lipgloss.NewStyle().
		Bold(true).
		Foreground(Cream).
		Background(Fuchsia).
		Padding(0, 2)

	Tagline = lipgloss.NewStyle().Foreground(Gray).Italic(true)

	Success = lipgloss.NewStyle().Bold(true).Foreground(Mint)
	Warning = lipgloss.NewStyle().Bold(true).Foreground(Amber)
	Danger  = lipgloss.NewStyle().Bold(true).Foreground(Coral)
	Info    = lipgloss.NewStyle().Foreground(Indigo)
	Muted   = lipgloss.NewStyle().Foreground(Subtle)

	Hash = lipgloss.NewStyle().Foreground(Amber)
	Ok   = lipgloss.NewStyle().Foreground(Mint).SetString("✔")
	Err  = lipgloss.NewStyle().Foreground(Coral).SetString("✘")
	Bolt = lipgloss.NewStyle().Foreground(Fuchsia).SetString("⚡")

	Box = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(Indigo).
		Padding(1, 2)
)

// Logo returns the git-multi-tool wordmark banner used at the top of interactive
// flows.
func Logo() string {
	logo := Banner.Render(" ⚒ gmt ")
	tag := Tagline.Render("forging tidy git history, one hammer swing at a time")
	return logo + "\n" + tag
}

// Heading renders a section heading with a little lightning bolt bullet,
// matching the playful-but-tidy Charm aesthetic.
func Heading(s string) string {
	return Bolt.String() + " " + Title.Render(s)
}

// SuccessLine renders a checkmark-prefixed success message.
func SuccessLine(format string, a ...any) string {
	return Ok.String() + " " + Success.Render(fmt.Sprintf(format, a...))
}

// ErrLine renders a cross-prefixed error message.
func ErrLine(format string, a ...any) string {
	return Err.String() + " " + Danger.Render(fmt.Sprintf(format, a...))
}

// WarnLine renders a warning message.
func WarnLine(format string, a ...any) string {
	return Warning.Render("⚠ " + fmt.Sprintf(format, a...))
}

// ShortHash truncates and colors a commit hash for display.
func ShortHash(h string) string {
	if len(h) > 7 {
		h = h[:7]
	}
	return Hash.Render(h)
}

// Rule draws a horizontal divider sized to width, in the muted subtle color.
func Rule(width int) string {
	if width <= 0 {
		width = 40
	}
	return Muted.Render(strings.Repeat("─", width))
}

// Theme returns the huh form theme used across git-multi-tool, based on Charm's
// own theme so prompts feel native to the rest of the ecosystem while still
// being tuned to our palette.
func Theme() *huh.Theme {
	t := huh.ThemeCharm()
	t.Focused.Title = t.Focused.Title.Foreground(Indigo)
	t.Focused.SelectedOption = t.Focused.SelectedOption.Foreground(Mint)
	t.Focused.FocusedButton = t.Focused.FocusedButton.Foreground(Cream).Background(Fuchsia)
	return t
}
