package main

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// ---------------------------------------------------------------------------
// inputModel - text input with validation
// ---------------------------------------------------------------------------

type inputModel struct {
	label     string
	input     textinput.Model
	validator func(string) error
	err       string
	submitted bool
}

func newInputModel(label string, masked bool, validator func(string) error) inputModel {
	ti := textinput.New()
	ti.Placeholder = label
	ti.Focus()
	ti.CharLimit = 256
	ti.Width = 50

	if masked {
		ti.EchoMode = textinput.EchoPassword
		ti.EchoCharacter = '*'
	}

	return inputModel{
		label:     label,
		input:     ti,
		validator: validator,
	}
}

func (m inputModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m inputModel) Update(msg tea.Msg) (inputModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			val := m.input.Value()
			if m.validator != nil {
				if err := m.validator(val); err != nil {
					m.err = err.Error()
					return m, nil
				}
			}
			m.submitted = true
			m.err = ""
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.err = ""
	return m, cmd
}

func (m inputModel) View() string {
	var b strings.Builder

	b.WriteString(promptLabelStyle.Render(m.label) + "\n\n")
	b.WriteString(m.input.View() + "\n")

	if m.err != "" {
		b.WriteString(validationErrStyle.Render("\u2717 "+m.err) + "\n")
	}

	b.WriteString("\n" + hintStyle.Render("enter submit  esc restart") + "\n")

	return b.String()
}

func (m inputModel) Value() string {
	return m.input.Value()
}

func (m inputModel) IsSubmitted() bool {
	return m.submitted
}

// ---------------------------------------------------------------------------
// choiceModel generic choice selector (confirm, done menu, ...)
// ---------------------------------------------------------------------------

type choiceModel struct {
	title   string
	cursor  int
	choices []string
	chosen  int // -1 until chosen
}

func newChoiceModel(title string, choices []string) choiceModel {
	return choiceModel{
		title:   title,
		choices: choices,
		chosen:  -1,
	}
}

func (m choiceModel) Update(msg tea.Msg) (choiceModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down":
			if m.cursor < len(m.choices)-1 {
				m.cursor++
			}
		case "enter":
			m.chosen = m.cursor
		}
	}
	return m, nil
}

func (m choiceModel) View() string {
	var b strings.Builder

	b.WriteString(promptLabelStyle.Render(m.title) + "\n\n")

	for i, c := range m.choices {
		if i == m.cursor {
			b.WriteString(cursorStyle.Render("> ") + selectedItemStyle.Render(c) + "\n")
		} else {
			b.WriteString("  " + normalItemStyle.Render(c) + "\n")
		}
	}

	b.WriteString("\n" + hintStyle.Render("\u2191/\u2193 navigate  enter select  esc reset") + "\n")

	return b.String()
}

// ChosenIndex returns the index of the chosen item, or -1 if nothing chosen yet.
func (m choiceModel) ChosenIndex() int {
	return m.chosen
}

// IsChosen returns true if the user has made a selection.
func (m choiceModel) IsChosen() bool {
	return m.chosen >= 0
}
