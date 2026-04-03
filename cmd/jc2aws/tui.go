package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/viper"

	"github.com/yousysadmin/jc2aws/internal/aws"
	"github.com/yousysadmin/jc2aws/internal/config"
	"github.com/yousysadmin/jc2aws/pkg"
	"github.com/yousysadmin/jc2aws/pkg/update"
)

// ---------------------------------------------------------------------------
// Messages
// ---------------------------------------------------------------------------

type credentialResultMsg struct {
	cred aws.AwsSamlOutput
	err  error
}

type outputResultMsg struct {
	err error
}

type updateCheckMsg struct {
	latestVersion string
}

// ---------------------------------------------------------------------------
// Choice indices for the confirm and done menus
// ---------------------------------------------------------------------------

const (
	confirmChoiceConfirm = 0
	confirmChoiceRestart = 1

	doneChoiceRunAgain = 0
	doneChoiceQuit     = 1
)

// ---------------------------------------------------------------------------
// Main TUI model
// ---------------------------------------------------------------------------

type tuiModel struct {
	// Application state from CLI flags / config
	appCfg *appConfig

	// Wizard state
	steps   []stepMeta
	current stepID

	// Resolved account (nil until selected)
	account *config.Account

	// Collected values
	values map[stepID]string

	// Active component (only one at a time)
	selectComp selectModel
	inputComp  inputModel
	choiceComp choiceModel
	spinner    spinner.Model

	// Component type active for current step
	compType string // "select", "input", "choice", "spinner", "await-key", ""

	// Result
	credResult *aws.AwsSamlOutput
	credErr    error
	outputErr  error
	outputDone bool

	// Terminal size
	width  int
	height int

	// quitting means the user aborted via ctrl+c (no further action).
	// done means the user finished normally via the done-menu "Quit" choice.
	quitting bool
	done     bool

	// Update check result
	updateVersion string // non-empty if a newer version is available
}

func newTuiModel(cfg *appConfig) tuiModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = spinnerStyle

	m := tuiModel{
		appCfg:  cfg,
		steps:   allStepMeta(),
		current: stepAccount,
		values:  make(map[stepID]string),
		spinner: sp,
		width:   80,
		height:  24,
	}

	m.preResolveSteps()
	m.initStep()
	return m
}

func (m tuiModel) Init() tea.Cmd {
	cmds := []tea.Cmd{
		tea.WindowSize(),
		m.initCmd(),
	}
	if !viper.GetBool(keyNoUpdateCheck) {
		cmds = append(cmds, checkForUpdate())
	}
	return tea.Batch(cmds...)
}

func (m tuiModel) initCmd() tea.Cmd {
	if m.compType == "input" {
		return m.inputComp.Init()
	}
	if m.compType == "spinner" {
		return m.spinner.Tick
	}
	return nil
}

// ---------------------------------------------------------------------------
// Step initialization: decide what component to show, or auto-skip
// ---------------------------------------------------------------------------

func (m *tuiModel) initStep() {
	cfg := m.appCfg

	switch m.current {
	case stepAccount:
		if len(cfg.config.Accounts) == 0 {
			m.setStepValueWithSource(stepAccount, "(no config)", sourcePreset)
			m.advanceStep()
			return
		}
		if accountName := viper.GetString(keyAccount); accountName != "" {
			acc, err := cfg.config.FindAccountByName(accountName)
			if err == nil {
				m.account = &acc
				m.setStepValueWithSource(stepAccount, acc.Name, sourcePreset)
				m.preResolveSteps()
				m.advanceStep()
				return
			}
		}
		m.selectComp = buildAccountSelect(cfg.config.GetAccounts())
		m.compType = "select"

	case stepRole:
		if roleARN := viper.GetString(keyRoleARN); roleARN != "" {
			m.setStepValueWithSource(stepRole, roleARN, sourcePreset)
			m.advanceStep()
			return
		}
		if roleName := viper.GetString(keyRoleName); roleName != "" && m.account != nil {
			role, err := m.account.FindAWSRoleArnByName(roleName)
			if err == nil {
				m.values[stepRole] = role.Arn
				m.setStepValueWithSource(stepRole, role.Name, sourcePreset)
				m.advanceStep()
				return
			}
		}
		if m.account != nil && len(m.account.AWSRoleArns) > 0 {
			m.selectComp = buildRoleSelect(*m.account)
			m.compType = "select"
		} else {
			m.inputComp = buildRoleARNInput()
			m.compType = "input"
		}

	case stepRegion:
		if val := resolveString(keyRegion, m.account); val != "" {
			m.setStepValueWithSource(stepRegion, val, sourcePreset)
			m.advanceStep()
			return
		}
		regions := regionListForAccount(m.account)
		m.selectComp = buildRegionSelect(regions)
		m.compType = "select"

	case stepEmail:
		val := firstNonEmpty(resolveString(keyEmail, m.account), m.values[stepEmail])
		if val != "" {
			m.setStepValueWithSource(stepEmail, val, sourcePreset)
			m.advanceStep()
			return
		}
		m.inputComp = buildEmailInput()
		m.compType = "input"

	case stepPassword:
		val := firstNonEmpty(resolveString(keyPassword, m.account), m.values[stepPassword])
		if val != "" {
			m.setStepValueWithSource(stepPassword, "(set)", sourcePreset)
			m.advanceStep()
			return
		}
		m.inputComp = buildPasswordInput()
		m.compType = "input"

	case stepIdpURL:
		val := firstNonEmpty(resolveString(keyIdpURL, m.account), m.values[stepIdpURL])
		if val != "" {
			m.setStepValueWithSource(stepIdpURL, val, sourcePreset)
			m.advanceStep()
			return
		}
		m.inputComp = buildIdpURLInput()
		m.compType = "input"

	case stepPrincipalARN:
		val := firstNonEmpty(resolveString(keyPrincipalARN, m.account), m.values[stepPrincipalARN])
		if val != "" {
			m.setStepValueWithSource(stepPrincipalARN, truncateARN(val), sourcePreset)
			m.advanceStep()
			return
		}
		m.inputComp = buildPrincipalARNInput()
		m.compType = "input"

	case stepOutputFormat:
		if viper.IsSet(keyOutputFormat) {
			m.setStepValueWithSource(stepOutputFormat, viper.GetString(keyOutputFormat), sourcePreset)
			m.advanceStep()
			return
		}
		m.selectComp = buildOutputFormatSelect()
		m.compType = "select"

	case stepAwsCliProfile:
		format := m.resolveOutputFormat()
		if format != "cli" && format != "cli-stdout" {
			m.setStepValueWithSource(stepAwsCliProfile, "(n/a)", sourcePreset)
			m.advanceStep()
			return
		}
		val := firstNonEmpty(resolveString(keyAwsCliProfile, m.account), m.values[stepAwsCliProfile])
		if val != "" {
			m.setStepValueWithSource(stepAwsCliProfile, val, sourcePreset)
			m.advanceStep()
			return
		}
		m.inputComp = buildAwsCliProfileInput()
		m.compType = "input"

	case stepMFA:
		val := firstNonEmpty(resolveString(keyMFA, m.account), m.values[stepMFA])
		if val != "" {
			m.setStepValueWithSource(stepMFA, "(set)", sourcePreset)
			m.advanceStep()
			return
		}
		m.inputComp = buildMFAInput()
		m.compType = "input"

	case stepConfirm:
		m.choiceComp = newChoiceModel("Review and confirm", []string{"Confirm", "Restart"})
		m.compType = "choice"

	case stepFetching:
		m.compType = "spinner"

	case stepDone:
		// Always show errors to the user — never auto-exit on failure.
		if m.credErr != nil || m.outputErr != nil {
			m.choiceComp = newChoiceModel("What next?", []string{"Run again", "Quit"})
			m.compType = "choice"
			return
		}

		format := m.resolveOutputFormat()
		switch format {
		case "shell":
			// Shell launches post-TUI; show result and wait for any key.
			m.compType = "await-key"
		case "cli-stdout", "env-stdout":
			// Stdout formats: immediately quit; output prints post-TUI.
			m.done = true
			m.compType = ""
		default:
			// File-based formats (cli, env): behavior depends on tui_done_action config.
			switch viper.GetString(keyTUIDoneAction) {
			case "menu":
				m.choiceComp = newChoiceModel("What next?", []string{"Run again", "Quit"})
				m.compType = "choice"
			case "wait":
				m.compType = "await-key"
			default: // "exit" or empty
				m.done = true
				m.compType = ""
			}
		}
	}
}

func (m *tuiModel) setStepValueWithSource(id stepID, display, source string) {
	for i, s := range m.steps {
		if s.id == id {
			m.steps[i].value = display
			m.steps[i].source = source
			break
		}
	}
}

func (m *tuiModel) advanceStep() {
	next := m.current + 1
	if next > stepDone {
		next = stepDone
	}
	m.current = next
	m.compType = ""
	m.initStep()
}

// preResolveSteps marks steps that will be auto-skipped as sourcePreset so the
// sidebar hides them immediately instead of showing them as pending steps until
// the wizard sequentially walks through each one.
// This is purely a display optimization — initStep() still performs the actual
// skip logic and is the source of truth.
func (m *tuiModel) preResolveSteps() {
	acc := m.account

	// Role
	if viper.GetString(keyRoleARN) != "" {
		m.setStepValueWithSource(stepRole, viper.GetString(keyRoleARN), sourcePreset)
	} else if roleName := viper.GetString(keyRoleName); roleName != "" && acc != nil {
		if role, err := acc.FindAWSRoleArnByName(roleName); err == nil {
			m.setStepValueWithSource(stepRole, role.Name, sourcePreset)
		}
	}

	// Region
	if resolveString(keyRegion, acc) != "" {
		m.setStepValueWithSource(stepRegion, resolveString(keyRegion, acc), sourcePreset)
	}

	// Email
	if firstNonEmpty(resolveString(keyEmail, acc), m.values[stepEmail]) != "" {
		m.setStepValueWithSource(stepEmail, firstNonEmpty(resolveString(keyEmail, acc), m.values[stepEmail]), sourcePreset)
	}

	// Password
	if firstNonEmpty(resolveString(keyPassword, acc), m.values[stepPassword]) != "" {
		m.setStepValueWithSource(stepPassword, "(set)", sourcePreset)
	}

	// IDP URL
	if firstNonEmpty(resolveString(keyIdpURL, acc), m.values[stepIdpURL]) != "" {
		m.setStepValueWithSource(stepIdpURL, firstNonEmpty(resolveString(keyIdpURL, acc), m.values[stepIdpURL]), sourcePreset)
	}

	// Principal ARN
	if val := firstNonEmpty(resolveString(keyPrincipalARN, acc), m.values[stepPrincipalARN]); val != "" {
		m.setStepValueWithSource(stepPrincipalARN, truncateARN(val), sourcePreset)
	}

	// Output Format
	if viper.IsSet(keyOutputFormat) {
		m.setStepValueWithSource(stepOutputFormat, viper.GetString(keyOutputFormat), sourcePreset)
	}

	// AWS CLI Profile
	format := m.resolveOutputFormat()
	if format != "cli" && format != "cli-stdout" {
		m.setStepValueWithSource(stepAwsCliProfile, "(n/a)", sourcePreset)
	} else if firstNonEmpty(resolveString(keyAwsCliProfile, acc), m.values[stepAwsCliProfile]) != "" {
		m.setStepValueWithSource(stepAwsCliProfile, firstNonEmpty(resolveString(keyAwsCliProfile, acc), m.values[stepAwsCliProfile]), sourcePreset)
	}

	// MFA
	if firstNonEmpty(resolveString(keyMFA, acc), m.values[stepMFA]) != "" {
		m.setStepValueWithSource(stepMFA, "(set)", sourcePreset)
	}
}

// resolveOutputFormat returns the effective output format.
// When the flag was not explicitly set (via flag, env, or config), prefer the
// interactive value over the Viper default ("cli") so that the user's TUI
// selection takes effect.
func (m tuiModel) resolveOutputFormat() string {
	if viper.IsSet(keyOutputFormat) {
		return viper.GetString(keyOutputFormat)
	}
	if v := m.values[stepOutputFormat]; v != "" {
		return v
	}
	return viper.GetString(keyOutputFormat) // fall back to default
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case updateCheckMsg:
		if msg.latestVersion != "" {
			m.updateVersion = msg.latestVersion
		}
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			m.quitting = true
			return m, tea.Quit
		}
		// ESC restarts the wizard from any interactive step (not during fetch).
		if msg.String() == "esc" {
			switch m.compType {
			case "select", "input", "choice", "await-key":
				nm := m.restart()
				return nm, nm.initCmd()
			}
		}

	case credentialResultMsg:
		if msg.err != nil {
			m.credErr = msg.err
			m.current = stepDone
			m.initStep()
			return m, nil
		}
		m.credResult = &msg.cred
		// Write output immediately inside the TUI
		return m, m.writeOutput()

	case outputResultMsg:
		m.outputDone = true
		if msg.err != nil {
			m.outputErr = msg.err
		}
		m.current = stepDone
		m.initStep()
		if m.done {
			return m, tea.Quit
		}
		return m, nil
	}

	// Delegate to active component
	var cmd tea.Cmd

	switch m.compType {
	case "select":
		m.selectComp, cmd = m.selectComp.Update(msg)
		if item, ok := m.selectComp.Selected(); ok {
			m.handleSelectResult(item)
			return m, m.initCmd()
		}
		return m, cmd

	case "input":
		m.inputComp, cmd = m.inputComp.Update(msg)
		if m.inputComp.IsSubmitted() {
			m.handleInputResult(m.inputComp.Value())
			return m, m.initCmd()
		}
		return m, cmd

	case "choice":
		m.choiceComp, cmd = m.choiceComp.Update(msg)
		if m.choiceComp.IsChosen() {
			return m.handleChoiceResult()
		}
		return m, cmd

	case "spinner":
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case "await-key":
		// Any non-esc keypress exits (esc is handled above as restart).
		if _, ok := msg.(tea.KeyMsg); ok {
			m.done = true
			return m, tea.Quit
		}
		return m, nil
	}

	return m, nil
}

func (m tuiModel) handleChoiceResult() (tea.Model, tea.Cmd) {
	idx := m.choiceComp.ChosenIndex()

	switch m.current {
	case stepConfirm:
		switch idx {
		case confirmChoiceConfirm:
			m.current = stepFetching
			m.compType = "spinner"
			return m, tea.Batch(m.spinner.Tick, m.fetchCredentials())
		case confirmChoiceRestart:
			nm := m.restart()
			return nm, nm.initCmd()
		}

	case stepDone:
		switch idx {
		case doneChoiceRunAgain:
			nm := m.restart()
			return nm, nm.initCmd()
		case doneChoiceQuit:
			m.done = true
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m *tuiModel) handleSelectResult(item selectItem) {
	switch m.current {
	case stepAccount:
		accounts := m.appCfg.config.GetAccounts()
		for i := range accounts {
			if accounts[i].Name == item.name {
				m.account = &accounts[i]
				break
			}
		}
		m.setStepValueWithSource(stepAccount, item.name, sourceInteractive)
		m.preResolveSteps()
		m.advanceStep()

	case stepRole:
		if m.account != nil {
			for _, r := range m.account.AWSRoleArns {
				if r.Name == item.name {
					m.values[stepRole] = r.Arn
					break
				}
			}
		}
		m.setStepValueWithSource(stepRole, item.name, sourceInteractive)
		m.advanceStep()

	case stepRegion:
		m.values[stepRegion] = item.name
		m.setStepValueWithSource(stepRegion, item.name, sourceInteractive)
		m.advanceStep()

	case stepOutputFormat:
		m.values[stepOutputFormat] = item.name
		m.setStepValueWithSource(stepOutputFormat, item.name, sourceInteractive)
		m.advanceStep()
	}
}

func (m *tuiModel) handleInputResult(val string) {
	switch m.current {
	case stepRole:
		m.values[stepRole] = val
		m.setStepValueWithSource(stepRole, truncateARN(val), sourceInteractive)
	case stepEmail:
		m.values[stepEmail] = val
		m.setStepValueWithSource(stepEmail, val, sourceInteractive)
	case stepPassword:
		m.values[stepPassword] = val
		m.setStepValueWithSource(stepPassword, "(set)", sourceInteractive)
	case stepIdpURL:
		m.values[stepIdpURL] = val
		m.setStepValueWithSource(stepIdpURL, val, sourceInteractive)
	case stepPrincipalARN:
		m.values[stepPrincipalARN] = val
		m.setStepValueWithSource(stepPrincipalARN, truncateARN(val), sourceInteractive)
	case stepAwsCliProfile:
		m.values[stepAwsCliProfile] = val
		m.setStepValueWithSource(stepAwsCliProfile, val, sourceInteractive)
	case stepMFA:
		m.values[stepMFA] = val
		m.setStepValueWithSource(stepMFA, "(set)", sourceInteractive)
	}
	m.advanceStep()
}

func (m tuiModel) restart() tuiModel {
	nm := newTuiModel(m.appCfg)
	// Preserve terminal dimensions so the layout doesn't shrink to defaults.
	nm.width = m.width
	nm.height = m.height
	// Preserve update check result across restarts.
	nm.updateVersion = m.updateVersion
	return nm
}

func (m tuiModel) fetchCredentials() tea.Cmd {
	return func() tea.Msg {
		email := firstNonEmpty(resolveString(keyEmail, m.account), m.values[stepEmail])
		password := firstNonEmpty(resolveString(keyPassword, m.account), m.values[stepPassword])
		idpURL := firstNonEmpty(resolveString(keyIdpURL, m.account), m.values[stepIdpURL])
		mfa := firstNonEmpty(resolveString(keyMFA, m.account), m.values[stepMFA])
		principalARN := firstNonEmpty(resolveString(keyPrincipalARN, m.account), m.values[stepPrincipalARN])
		roleARN := firstNonEmpty(viper.GetString(keyRoleARN), m.values[stepRole])
		region := firstNonEmpty(resolveString(keyRegion, m.account), m.values[stepRegion])
		duration := resolveDuration(m.account)

		cred, err := getCredentials(email, password, idpURL, mfa, principalARN, roleARN, region, duration)
		return credentialResultMsg{cred: cred, err: err}
	}
}

func (m tuiModel) writeOutput() tea.Cmd {
	cred := m.credResult
	format := m.resolveOutputFormat()
	profileName := firstNonEmpty(resolveString(keyAwsCliProfile, m.account), m.values[stepAwsCliProfile])

	return func() tea.Msg {
		switch {
		case format == "shell":
			// Shell launch happens after TUI exits; nothing to write now.
			return outputResultMsg{}

		case format == "cli-stdout" || format == "env-stdout":
			// Always defer stdout output to post-TUI (real stdout).
			return outputResultMsg{}

		default:
			// File-based formats (cli, env): write immediately.
			err := outputCredentials(*cred, format, profileName)
			return outputResultMsg{err: err}
		}
	}
}

// ---------------------------------------------------------------------------
// Update check
// ---------------------------------------------------------------------------

func checkForUpdate() tea.Cmd {
	return func() tea.Msg {
		result := update.CheckLatestVersion(pkg.Version)
		if result.Err != nil || result.LatestVersion == "" {
			return updateCheckMsg{}
		}
		return updateCheckMsg{latestVersion: result.LatestVersion}
	}
}

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
	contentWidth := m.width - sidebarWidth - 4 // border + padding
	if contentWidth < 30 {
		contentWidth = 30
	}

	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		sidebarStyle.Height(m.height-2).Render(sidebar),
		contentStyle.Width(contentWidth).Height(m.height-2).Render(content),
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

func (m tuiModel) viewContent() string {
	var banner string
	if m.updateVersion != "" {
		banner = updateBannerStyle.Render(
			"\u2191 Update available: v"+m.updateVersion+" \u2014 run: jc2aws --update",
		) + "\n\n"
	}

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

	region := firstNonEmpty(resolveString(keyRegion, m.account), m.values[stepRegion])
	email := firstNonEmpty(resolveString(keyEmail, m.account), m.values[stepEmail])
	idpURL := firstNonEmpty(resolveString(keyIdpURL, m.account), m.values[stepIdpURL])
	principalARN := firstNonEmpty(resolveString(keyPrincipalARN, m.account), m.values[stepPrincipalARN])
	duration := resolveDuration(m.account)

	rows := []struct {
		label  string
		value  string
		source string
	}{
		{"Account", m.stepDisplay(stepAccount), m.stepSource(stepAccount)},
		{"Role", m.stepDisplay(stepRole), m.stepSource(stepRole)},
		{"Region", region, m.stepSource(stepRegion)},
		{"Email", email, m.stepSource(stepEmail)},
		{"Password", "\u2022\u2022\u2022\u2022\u2022\u2022\u2022\u2022", m.stepSource(stepPassword)},
		{"IDP URL", idpURL, m.stepSource(stepIdpURL)},
		{"Principal ARN", truncateARN(principalARN), m.stepSource(stepPrincipalARN)},
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

func (m tuiModel) viewDoneResult() string {
	if m.credErr != nil {
		return errorBannerStyle.Render("\u2717 Failed to obtain credentials") + "\n\n" +
			errorStyle.Render(m.credErr.Error()) + "\n"
	}
	if m.outputErr != nil {
		return successBannerStyle.Render("\u2713 Credentials obtained") + "\n\n" +
			errorBannerStyle.Render("\u26a0 Output error: "+m.outputErr.Error()) + "\n"
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

func (m tuiModel) stepDisplay(id stepID) string {
	for _, s := range m.steps {
		if s.id == id {
			return s.value
		}
	}
	return ""
}

func (m tuiModel) stepSource(id stepID) string {
	for _, s := range m.steps {
		if s.id == id {
			return s.source
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func truncateARN(s string) string {
	if len(s) > 30 {
		return "..." + s[len(s)-27:]
	}
	return s
}
