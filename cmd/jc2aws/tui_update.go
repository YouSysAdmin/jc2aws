package main

import (
	"cmp"
	"context"
	"slices"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/viper"

	"github.com/yousysadmin/jc2aws/internal/config"
	"github.com/yousysadmin/jc2aws/pkg"
	"github.com/yousysadmin/jc2aws/pkg/update"
)

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// The early return below keeps the message away from the component
		// delegation block, so the new size has to be pushed explicitly.
		m.resizeComponents()
		return m, nil

	case updateCheckMsg:
		if msg.latestVersion != "" {
			m.updateVersion = msg.latestVersion
			// The banner arrives asynchronously and costs the component rows.
			m.resizeComponents()
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
		m.credResult = new(msg.cred)
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
			m.compType = compSpinner
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
		if i := slices.IndexFunc(accounts, func(a config.Account) bool { return a.Name == item.name }); i >= 0 {
			m.account = &accounts[i]
		}
		m.refreshProvider()
		m.setStepValueWithSource(stepAccount, item.name, sourceInteractive)
		m.preResolveSteps()
		m.advanceStep()

	case stepRole:
		if m.account != nil {
			roles := m.account.Roles
			if i := slices.IndexFunc(roles, func(r config.Role) bool { return r.Name == item.name }); i >= 0 {
				m.values[stepRole] = roles[i].Arn
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
	case stepCLIProfile:
		m.values[stepCLIProfile] = val
		m.setStepValueWithSource(stepCLIProfile, val, sourceInteractive)
	case stepMFA:
		m.values[stepMFA] = val
		m.setStepValueWithSource(stepMFA, "(set)", sourceInteractive)
	}
	m.advanceStep()
}

func (m tuiModel) restart() tuiModel {
	nm := newTuiModel(m.appCfg)
	// Preserve terminal dimensions so the layout doesn't shrink to defaults.
	// newTuiModel already ran initStep at the default size, so the component
	// has to be resized after the copy.
	nm.width = m.width
	nm.height = m.height
	nm.resizeComponents()
	// Preserve update check result across restarts.
	nm.updateVersion = m.updateVersion
	return nm
}

func (m tuiModel) fetchCredentials() tea.Cmd {
	return func() tea.Msg {
		email := cmp.Or(resolveString(keyEmail, m.account), m.values[stepEmail])
		password := cmp.Or(resolveString(keyPassword, m.account), m.values[stepPassword])
		idpURL := cmp.Or(resolveString(keyIdpURL, m.account), m.values[stepIdpURL])
		mfa := cmp.Or(resolveString(keyMFA, m.account), m.values[stepMFA])
		principalARN := cmp.Or(resolveString(keyPrincipalARN, m.account), m.values[stepPrincipalARN])
		roleARN := cmp.Or(viper.GetString(keyRoleARN), m.values[stepRole])
		region := cmp.Or(resolveString(keyRegion, m.account), m.values[stepRegion])
		duration := resolveDuration(m.account)

		cred, err := getCredentials(context.Background(), credentialRequest{
			Provider:     m.provider,
			Email:        email,
			Password:     password,
			IdpURL:       idpURL,
			MFA:          mfa,
			PrincipalARN: principalARN,
			RoleARN:      roleARN,
			Region:       region,
			Duration:     duration,
		})
		return credentialResultMsg{cred: cred, err: err}
	}
}

func (m tuiModel) writeOutput() tea.Cmd {
	cred := m.credResult
	format := m.resolveOutputFormat()
	profileName := cmp.Or(resolveString(keyCLIProfile, m.account), m.values[stepCLIProfile])

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
			err := outputCredentials(m.provider, *cred, format, profileName)
			return outputResultMsg{err: err}
		}
	}
}

// ---------------------------------------------------------------------------
// Update check
// ---------------------------------------------------------------------------

func checkForUpdate() tea.Cmd {
	return func() tea.Msg {
		result := update.CheckLatestVersion(context.Background(), pkg.Version)
		if result.Err != nil || result.LatestVersion == "" {
			return updateCheckMsg{}
		}
		return updateCheckMsg{latestVersion: result.LatestVersion}
	}
}
