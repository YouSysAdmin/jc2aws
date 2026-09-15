package main

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/viper"

	"github.com/yousysadmin/jc2aws/internal/cloud"
	"github.com/yousysadmin/jc2aws/internal/cloud/providers"
	"github.com/yousysadmin/jc2aws/internal/config"
)

// ---------------------------------------------------------------------------
// Messages
// ---------------------------------------------------------------------------

type credentialResultMsg struct {
	cred cloud.Credentials
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

// compKind identifies the component active for the current step.
type compKind string

const (
	compNone     compKind = ""
	compSelect   compKind = "select"
	compInput    compKind = "input"
	compChoice   compKind = "choice"
	compSpinner  compKind = "spinner"
	compAwaitKey compKind = "await-key"
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
	compType compKind

	// notice is a non-fatal warning shown above the active component
	// (e.g. a preset account or role name that was not found).
	notice string

	// provider is the cloud provider for the selected account. It is never nil
	// after newTuiModel, and is re-resolved whenever the account changes.
	provider cloud.Provider

	// Result
	credResult *cloud.Credentials
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

	m.refreshProvider()
	m.preResolveSteps()
	m.initStep()
	return m
}

// refreshProvider re-resolves the cloud provider after the selected account
// changes. An unknown or unimplemented provider is surfaced as a notice and
// falls back to the default provider, so the wizard stays usable.
func (m *tuiModel) refreshProvider() {
	p, err := resolveProvider(m.account)
	if err != nil {
		m.notice = err.Error()
		p, _ = providers.Get(cloud.DefaultName)
	}
	m.provider = p
	m.retitleForProvider()
}

// providerDisplayName returns the selected provider's display name, or an empty
// string when no provider is resolved yet, which the summary then skips.
func (m tuiModel) providerDisplayName() string {
	if m.provider == nil {
		return ""
	}
	return m.provider.Info().DisplayName
}

// providerARNLabel names the identity-provider field in the selected vendor's
// own vocabulary.
func (m tuiModel) providerARNLabel() string {
	if m.provider == nil {
		return "Identity Provider ARN"
	}
	return m.provider.Info().ProviderARNLabel
}

// retitleForProvider rewrites the sidebar titles that name a provider-specific
// concept, so the wizard speaks the selected vendor's vocabulary.
func (m *tuiModel) retitleForProvider() {
	if m.provider == nil {
		return
	}

	info := m.provider.Info()
	for i := range m.steps {
		if m.steps[i].id == stepPrincipalARN {
			m.steps[i].title = info.ProviderARNLabel
		}
	}
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
	if m.compType == compInput {
		return m.inputComp.Init()
	}
	if m.compType == compSpinner {
		return m.spinner.Tick
	}
	return nil
}

// ---------------------------------------------------------------------------
// Step initialization: decide what component to show, or auto-skip
// ---------------------------------------------------------------------------

func (m *tuiModel) initStep() {
	cfg := m.appCfg
	m.notice = ""

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
				m.account = new(acc)
				m.refreshProvider()
				m.setStepValueWithSource(stepAccount, acc.Name, sourcePreset)
				m.preResolveSteps()
				m.advanceStep()
				return
			}
			m.notice = fmt.Sprintf("account %q not found in config — select one manually", accountName)
		}
		m.selectComp = buildAccountSelect(cfg.config.GetAccounts())
		m.compType = compSelect

	case stepRole:
		if roleARN := viper.GetString(keyRoleARN); roleARN != "" {
			m.setStepValueWithSource(stepRole, roleARN, sourcePreset)
			m.advanceStep()
			return
		}
		if roleName := viper.GetString(keyRoleName); roleName != "" && m.account != nil {
			role, err := m.account.FindRoleByName(roleName)
			if err == nil {
				m.values[stepRole] = role.Arn
				m.setStepValueWithSource(stepRole, role.Name, sourcePreset)
				m.advanceStep()
				return
			}
			m.notice = fmt.Sprintf("role %q not found in account %q — select one manually", roleName, m.account.Name)
		}
		if m.account != nil && len(m.account.Roles) > 0 {
			m.selectComp = buildRoleSelect(*m.account)
			m.compType = compSelect
		} else {
			m.inputComp = buildRoleARNInput(m.provider)
			m.compType = compInput
		}

	case stepRegion:
		if val := resolveString(keyRegion, m.account); val != "" {
			m.setStepValueWithSource(stepRegion, val, sourcePreset)
			m.advanceStep()
			return
		}
		regions := regionListForAccount(m.account, m.provider)
		m.selectComp = buildRegionSelect(regions)
		m.compType = compSelect

	case stepEmail:
		val := cmp.Or(resolveString(keyEmail, m.account), m.values[stepEmail])
		if val != "" {
			m.setStepValueWithSource(stepEmail, val, sourcePreset)
			m.advanceStep()
			return
		}
		m.inputComp = buildEmailInput()
		m.compType = compInput

	case stepPassword:
		val := cmp.Or(resolveString(keyPassword, m.account), m.values[stepPassword])
		if val != "" {
			m.setStepValueWithSource(stepPassword, "(set)", sourcePreset)
			m.advanceStep()
			return
		}
		m.inputComp = buildPasswordInput()
		m.compType = compInput

	case stepIdpURL:
		val := cmp.Or(resolveString(keyIdpURL, m.account), m.values[stepIdpURL])
		if val != "" {
			m.setStepValueWithSource(stepIdpURL, val, sourcePreset)
			m.advanceStep()
			return
		}
		m.inputComp = buildIdpURLInput()
		m.compType = compInput

	case stepPrincipalARN:
		val := cmp.Or(resolveString(keyPrincipalARN, m.account), m.values[stepPrincipalARN])
		if val != "" {
			m.setStepValueWithSource(stepPrincipalARN, truncateARN(val), sourcePreset)
			m.advanceStep()
			return
		}
		m.inputComp = buildPrincipalARNInput(m.provider)
		m.compType = compInput

	case stepOutputFormat:
		if viper.IsSet(keyOutputFormat) {
			m.setStepValueWithSource(stepOutputFormat, viper.GetString(keyOutputFormat), sourcePreset)
			m.advanceStep()
			return
		}
		m.selectComp = buildOutputFormatSelect(m.provider)
		m.compType = compSelect

	case stepCLIProfile:
		format := m.resolveOutputFormat()
		if format != "cli" && format != "cli-stdout" {
			m.setStepValueWithSource(stepCLIProfile, "(n/a)", sourcePreset)
			m.advanceStep()
			return
		}
		val := cmp.Or(resolveString(keyCLIProfile, m.account), m.values[stepCLIProfile])
		if val != "" {
			m.setStepValueWithSource(stepCLIProfile, val, sourcePreset)
			m.advanceStep()
			return
		}
		m.inputComp = buildCLIProfileInput()
		m.compType = compInput

	case stepMFA:
		val := cmp.Or(resolveString(keyMFA, m.account), m.values[stepMFA])
		if val != "" {
			m.setStepValueWithSource(stepMFA, "(set)", sourcePreset)
			m.advanceStep()
			return
		}
		m.inputComp = buildMFAInput()
		m.compType = compInput

	case stepConfirm:
		m.choiceComp = newChoiceModel("Review and confirm", []string{"Confirm", "Restart"})
		m.compType = compChoice

	case stepFetching:
		m.compType = compSpinner

	case stepDone:
		// Always show errors to the user — never auto-exit on failure.
		if m.credErr != nil || m.outputErr != nil {
			m.choiceComp = newChoiceModel("What next?", []string{"Run again", "Quit"})
			m.compType = compChoice
			return
		}

		format := m.resolveOutputFormat()
		switch format {
		case "shell":
			// Shell launches post-TUI; show result and wait for any key.
			m.compType = compAwaitKey
		case "cli-stdout", "env-stdout":
			// Stdout formats: immediately quit; output prints post-TUI.
			m.done = true
			m.compType = compNone
		default:
			// File-based formats (cli, env): behavior depends on tui_done_action config.
			switch viper.GetString(keyTUIDoneAction) {
			case "menu":
				m.choiceComp = newChoiceModel("What next?", []string{"Run again", "Quit"})
				m.compType = compChoice
			case "wait":
				m.compType = compAwaitKey
			default: // "exit" or empty
				m.done = true
				m.compType = compNone
			}
		}
	}
}

func (m *tuiModel) setStepValueWithSource(id stepID, display, source string) {
	if i := slices.IndexFunc(m.steps, func(s stepMeta) bool { return s.id == id }); i >= 0 {
		m.steps[i].value = display
		m.steps[i].source = source
	}
}

func (m *tuiModel) advanceStep() {
	m.current = min(m.current+1, stepDone)
	m.compType = compNone
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
		if role, err := acc.FindRoleByName(roleName); err == nil {
			m.setStepValueWithSource(stepRole, role.Name, sourcePreset)
		}
	}

	// Region
	if resolveString(keyRegion, acc) != "" {
		m.setStepValueWithSource(stepRegion, resolveString(keyRegion, acc), sourcePreset)
	}

	// Email
	if cmp.Or(resolveString(keyEmail, acc), m.values[stepEmail]) != "" {
		m.setStepValueWithSource(stepEmail, cmp.Or(resolveString(keyEmail, acc), m.values[stepEmail]), sourcePreset)
	}

	// Password
	if cmp.Or(resolveString(keyPassword, acc), m.values[stepPassword]) != "" {
		m.setStepValueWithSource(stepPassword, "(set)", sourcePreset)
	}

	// IDP URL
	if cmp.Or(resolveString(keyIdpURL, acc), m.values[stepIdpURL]) != "" {
		m.setStepValueWithSource(stepIdpURL, cmp.Or(resolveString(keyIdpURL, acc), m.values[stepIdpURL]), sourcePreset)
	}

	// Principal ARN
	if val := cmp.Or(resolveString(keyPrincipalARN, acc), m.values[stepPrincipalARN]); val != "" {
		m.setStepValueWithSource(stepPrincipalARN, truncateARN(val), sourcePreset)
	}

	// Output Format
	if viper.IsSet(keyOutputFormat) {
		m.setStepValueWithSource(stepOutputFormat, viper.GetString(keyOutputFormat), sourcePreset)
	}

	// AWS CLI Profile
	format := m.resolveOutputFormat()
	if format != "cli" && format != "cli-stdout" {
		m.setStepValueWithSource(stepCLIProfile, "(n/a)", sourcePreset)
	} else if cmp.Or(resolveString(keyCLIProfile, acc), m.values[stepCLIProfile]) != "" {
		m.setStepValueWithSource(stepCLIProfile, cmp.Or(resolveString(keyCLIProfile, acc), m.values[stepCLIProfile]), sourcePreset)
	}

	// MFA
	if cmp.Or(resolveString(keyMFA, acc), m.values[stepMFA]) != "" {
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
	// Prefer the interactive value, falling back to the Viper flag default.
	return cmp.Or(m.values[stepOutputFormat], viper.GetString(keyOutputFormat))
}
