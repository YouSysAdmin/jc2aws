package main

import "github.com/charmbracelet/lipgloss"

var (
	// Colors
	colorPrimary   = lipgloss.Color("205") // pink/magenta
	colorSuccess   = lipgloss.Color("42")  // green
	colorMuted     = lipgloss.Color("241") // gray
	colorError     = lipgloss.Color("196") // red
	colorHighlight = lipgloss.Color("39")  // blue
	colorCyan      = lipgloss.Color("86")  // cyan
	colorWarn      = lipgloss.Color("214") // orange/yellow

	// Text styles
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary)

	activeStyle = lipgloss.NewStyle().
			Foreground(colorPrimary).
			Bold(true)

	doneStyle = lipgloss.NewStyle().
			Foreground(colorSuccess)

	mutedStyle = lipgloss.NewStyle().
			Foreground(colorMuted)

	errorStyle = lipgloss.NewStyle().
			Foreground(colorError).
			Bold(true)

	highlightStyle = lipgloss.NewStyle().
			Foreground(colorHighlight)

	cyanStyle = lipgloss.NewStyle().
			Foreground(colorCyan)

	warnStyle = lipgloss.NewStyle().
			Foreground(colorWarn)

	// Layout styles
	sidebarWidth = 36

	sidebarStyle = lipgloss.NewStyle().
			Width(sidebarWidth).
			BorderRight(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(colorMuted).
			Padding(1, 2)

	contentStyle = lipgloss.NewStyle().
			Padding(1, 2)

	// Component styles
	cursorStyle = lipgloss.NewStyle().
			Foreground(colorPrimary)

	selectedItemStyle = lipgloss.NewStyle().
				Foreground(colorCyan).
				Bold(true)

	normalItemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	detailLabelStyle = lipgloss.NewStyle().
				Foreground(colorMuted).
				Width(16)

	detailValueStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252"))

	spinnerStyle = lipgloss.NewStyle().
			Foreground(colorPrimary)

	promptLabelStyle = lipgloss.NewStyle().
				Foreground(colorHighlight).
				Bold(true)

	inputStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	validationErrStyle = lipgloss.NewStyle().
				Foreground(colorError).
				MarginTop(1)

	// Error banner for done-screen failures
	errorBannerStyle = lipgloss.NewStyle().
				Foreground(colorError).
				Bold(true).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorError).
				Padding(0, 1)

	// Success banner
	successBannerStyle = lipgloss.NewStyle().
				Foreground(colorSuccess).
				Bold(true).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorSuccess).
				Padding(0, 1)

	// Update banner
	updateBannerStyle = lipgloss.NewStyle().
				Foreground(colorSuccess).
				Bold(true).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorSuccess).
				Padding(0, 1)

	// Hint style for keybinding help
	hintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("238"))
)
