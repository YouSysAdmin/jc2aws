package main

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

func (m tuiModel) View() string {
	if m.quitting || m.done {
		return ""
	}

	sidebar := m.viewSidebar()
	content := m.viewContent()

	// Make content panel fill remaining width
	contentWidth := max(m.width-sidebarWidth-4, 30) // border + padding
	panelHeight := max(m.height-2, 0)

	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		sidebarStyle.Height(panelHeight).Render(sidebar),
		contentStyle.Width(contentWidth).Height(panelHeight).MaxHeight(panelHeight).Render(content),
	)
}

func (m tuiModel) viewSidebar() string {
	var b strings.Builder

	// Print Logo
	//  ╦╔═╗╔═╗╔═╗╦ ╦╔═╗
	//  ║║  ╔═╝╠═╣║║║╚═╗
	//╚═╝╚═╝╚══╩ ╩╚╩╝╚═╝
	b.WriteString(titleStyle.Render("    ╦╔═╗╔═╗╔═╗╦ ╦╔═╗\n    ║║  ╔═╝╠═╣║║║╚═╗\n  ╚═╝╚═╝╚══╩ ╩╚╩╝╚═╝") + "\n\n")

	for _, s := range m.steps {
		// Hide steps that were pre-set via config/flags/env
		if s.source == sourcePreset {
			continue
		}

		var line string

		if s.value != "" && m.current != s.id {
			// Completed interactive step
			line = doneStyle.Render("  \u2713 " + s.title)
		} else if m.current == s.id {
			// Active step
			line = activeStyle.Render("  \u25b8 " + s.title)
		} else {
			// Pending step
			line = mutedStyle.Render("  \u00b7 " + s.title)
		}

		b.WriteString(line + "\n")
	}

	// Status
	b.WriteString("\n")
	if m.current == stepFetching {
		b.WriteString("  " + m.spinner.View() + " Fetching...\n")
	} else if m.current == stepDone {
		if m.credErr != nil {
			b.WriteString("  " + errorStyle.Render("\u2717 Failed") + "\n")
		} else if m.outputErr != nil {
			b.WriteString("  " + warnStyle.Render("\u26a0 Output error") + "\n")
		} else if m.outputDone || m.credResult != nil {
			b.WriteString("  " + doneStyle.Render("\u2713 Done") + "\n")
		}
	}

	b.WriteString("\n" + hintStyle.Render("  ctrl+c quit"))

	return b.String()
}

// viewBanner renders the update banner and the notice shown above the active
// component, or "" when there is nothing to show. It is a separate function so
// that contentBox can measure exactly what viewContent will print.
func (m tuiModel) viewBanner() string {
	var banner string
	if m.updateVersion != "" {
		banner = updateBannerStyle.Render(
			"\u2191 Update available: v"+m.updateVersion+" \u2014 run: jc2aws --update",
		) + "\n\n"
	}
	if m.notice != "" {
		banner += warnStyle.Render("\u26a0 "+m.notice) + "\n\n"
	}
	return banner
}

// contentBox returns the text area available to the active component: the
// content pane less its padding and less whatever the banner occupies.
func (m tuiModel) contentBox() (width, height int) {
	width = max(m.width-sidebarWidth-4, 30) - 4 // contentStyle Padding(1, 2)
	height = max(m.height-2, 0) - 2

	if banner := m.viewBanner(); banner != "" {
		// The banner ends in a blank line that the component's first row does
		// not occupy, so discount one from its measured height.
		height -= lipgloss.Height(banner) - 1
	}

	return max(width, listMinWidth), max(height, 0)
}

// resizeComponents pushes the current content box into the active component.
// Call it after anything that changes the terminal size or the chrome above the
// component.
func (m *tuiModel) resizeComponents() {
	if m.compType != compSelect {
		return
	}
	width, height := m.contentBox()
	m.selectComp = m.selectComp.WithSize(width, height)
}

func (m tuiModel) viewContent() string {
	banner := m.viewBanner()

	switch m.compType {
	case "select":
		return banner + m.selectComp.View()
	case "input":
		return banner + m.inputComp.View()
	case "choice":
		if m.current == stepConfirm {
			return banner + m.viewSummary() + "\n" + m.choiceComp.View()
		}
		// Done state: show result + summary + menu
		return banner + m.viewDoneResult() + "\n" + m.viewSummary() + "\n" + m.choiceComp.View()
	case "spinner":
		return banner + "\n" + m.spinner.View() + " Authenticating with JumpCloud...\n\n" +
			hintStyle.Render("This may take a few seconds")
	case "await-key":
		return banner + m.viewDoneResult() + "\n" + m.viewSummary() + "\n" +
			hintStyle.Render("press any key to continue  esc restart")
	}

	return ""
}

func (m tuiModel) viewSummary() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Summary") + "\n")
	b.WriteString(mutedStyle.Render(strings.Repeat("\u2500", 40)) + "\n")

	region := cmp.Or(resolveString(keyRegion, m.account), m.values[stepRegion])
	email := cmp.Or(resolveString(keyEmail, m.account), m.values[stepEmail])
	idpURL := cmp.Or(resolveString(keyIdpURL, m.account), m.values[stepIdpURL])
	principalARN := cmp.Or(resolveString(keyPrincipalARN, m.account), m.values[stepPrincipalARN])
	duration := resolveDuration(m.account)

	rows := []struct {
		label  string
		value  string
		source string
	}{
		{"Account", m.stepDisplay(stepAccount), m.stepSource(stepAccount)},
		{"Provider", m.providerDisplayName(), ""},
		{"Role", m.stepDisplay(stepRole), m.stepSource(stepRole)},
		{"Region", region, m.stepSource(stepRegion)},
		{"Email", email, m.stepSource(stepEmail)},
		{"Password", "\u2022\u2022\u2022\u2022\u2022\u2022\u2022\u2022", m.stepSource(stepPassword)},
		{"IDP URL", idpURL, m.stepSource(stepIdpURL)},
		{m.providerARNLabel(), truncateARN(principalARN), m.stepSource(stepPrincipalARN)},
		{"Output Format", m.resolveOutputFormat(), m.stepSource(stepOutputFormat)},
		{"Duration", fmt.Sprintf("%ds", duration), ""},
	}

	for _, r := range rows {
		if r.value == "" || r.value == "(n/a)" || r.value == "(no config)" {
			continue
		}
		line := detailLabelStyle.Render(r.label+":") + " " + detailValueStyle.Render(r.value)
		if r.source == sourcePreset {
			line += " " + mutedStyle.Render("(config)")
		}
		b.WriteString(line + "\n")
	}

	b.WriteString(mutedStyle.Render(strings.Repeat("\u2500", 40)) + "\n")

	return b.String()
}

// doneErrorMaxRows caps a provider error on the done screen. STS errors are
// unbounded, and the pane is clipped to the terminal, so an untrimmed one would
// push the "Run again / Quit" menu off the bottom and look like a hang.
const doneErrorMaxRows = 6

// trimError wraps err to width and cuts it to at most doneErrorMaxRows rows,
// marking the cut so the reader knows there is more.
func (m tuiModel) trimError(err error) string {
	width, _ := m.contentBox()
	wrapped := errorStyle.Width(width).Render(err.Error())

	rows := strings.Split(wrapped, "\n")
	if len(rows) <= doneErrorMaxRows {
		return wrapped
	}
	return strings.Join(rows[:doneErrorMaxRows], "\n") + "\n" + mutedStyle.Render("  \u2026")
}

func (m tuiModel) viewDoneResult() string {
	if m.credErr != nil {
		return errorBannerStyle.Render("\u2717 Failed to obtain credentials") + "\n\n" +
			m.trimError(m.credErr) + "\n"
	}
	if m.outputErr != nil {
		return successBannerStyle.Render("\u2713 Credentials obtained") + "\n\n" +
			errorBannerStyle.Render("\u26a0 Output error: ") + m.trimError(m.outputErr) + "\n"
	}
	if m.outputDone {
		format := m.resolveOutputFormat()
		var details strings.Builder

		switch {
		case format == "shell":
			details.WriteString(successBannerStyle.Render("\u2713 Credentials obtained \u2014 shell will launch on exit") + "\n\n")
		default:
			details.WriteString(successBannerStyle.Render("\u2713 Credentials saved successfully") + "\n\n")
			details.WriteString(detailLabelStyle.Render("Format:") + " " + highlightStyle.Render(format) + "\n")
		}

		details.WriteString(detailLabelStyle.Render("Region:") + " " + highlightStyle.Render(m.credResult.Region) + "\n")
		if m.credResult.Expiration != nil {
			details.WriteString(detailLabelStyle.Render("Expires:") + " " + highlightStyle.Render(m.credResult.Expiration.Local().Format("15:04:05 MST")) + "\n")
		}
		return details.String()
	}
	return ""
}

// accountInfoText builds the plain-text (no styling) account summary printed to
// the normal terminal after the TUI exits. Never includes secret material.
func (m tuiModel) accountInfoText() string {
	region := cmp.Or(m.credResult.Region, resolveString(keyRegion, m.account), m.values[stepRegion])

	var b strings.Builder
	b.WriteString("Logged in successfully\n")
	writeKV(&b, "Account", m.stepDisplay(stepAccount))
	writeKV(&b, "Provider", m.providerDisplayName())
	writeKV(&b, "Role", m.stepDisplay(stepRole))
	writeKV(&b, "Region", region)
	if m.credResult.Expiration != nil {
		exp := m.credResult.Expiration.Local()
		writeKV(&b, "Expires", fmt.Sprintf("%s (in %s)",
			exp.Format("2006-01-02 15:04:05 MST"),
			time.Until(exp).Round(time.Second)))
	}
	return b.String()
}

// writeKV writes an aligned "Label: value" line, skipping empty/placeholder values.
func writeKV(b *strings.Builder, label, value string) {
	if value == "" || value == "(n/a)" || value == "(no config)" {
		return
	}
	fmt.Fprintf(b, "  %-8s %s\n", label+":", value)
}

func (m tuiModel) stepDisplay(id stepID) string {
	if i := slices.IndexFunc(m.steps, func(s stepMeta) bool { return s.id == id }); i >= 0 {
		return m.steps[i].value
	}
	return ""
}

func (m tuiModel) stepSource(id stepID) string {
	if i := slices.IndexFunc(m.steps, func(s stepMeta) bool { return s.id == id }); i >= 0 {
		return m.steps[i].source
	}
	return ""
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func truncateARN(s string) string {
	if r := []rune(s); len(r) > 30 {
		return "..." + string(r[len(r)-27:])
	}
	return s
}
