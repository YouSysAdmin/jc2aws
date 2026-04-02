package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// ---------------------------------------------------------------------------
// selectModel - a filterable selection list (replaces promptui.Select)
// ---------------------------------------------------------------------------

// detailPair is an ordered key-value pair for the item information panel.
type detailPair struct {
	key   string
	value string
}

type selectItem struct {
	name        string
	description string
	details     []detailPair
}

type selectModel struct {
	label    string
	items    []selectItem
	filtered []int
	cursor   int
	filter   string
	chosen   int // -1 until chosen
}

func newSelectModel(label string, items []selectItem) selectModel {
	indices := make([]int, len(items))
	for i := range items {
		indices[i] = i
	}
	return selectModel{
		label:    label,
		items:    items,
		filtered: indices,
		cursor:   0,
		chosen:   -1,
	}
}

func (m selectModel) Init() tea.Cmd { return nil }

func (m selectModel) Update(msg tea.Msg) (selectModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down":
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
			}
		case "enter":
			if len(m.filtered) > 0 {
				m.chosen = m.filtered[m.cursor]
			}
		case "backspace":
			if len(m.filter) > 0 {
				m.filter = m.filter[:len(m.filter)-1]
				m = m.applyFilter()
			}
		default:
			if len(msg.String()) == 1 {
				m.filter += msg.String()
				m = m.applyFilter()
			}
		}
	}
	return m, nil
}

func (m selectModel) applyFilter() selectModel {
	if m.filter == "" {
		indices := make([]int, len(m.items))
		for i := range m.items {
			indices[i] = i
		}
		m.filtered = indices
	} else {
		var filtered []int
		needle := strings.ToLower(m.filter)
		for i, item := range m.items {
			if strings.Contains(strings.ToLower(item.name), needle) {
				filtered = append(filtered, i)
			}
		}
		m.filtered = filtered
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
	return m
}

func (m selectModel) View() string {
	var b strings.Builder

	b.WriteString(promptLabelStyle.Render(m.label) + "\n")

	if m.filter != "" {
		b.WriteString(mutedStyle.Render("Filter: ") + inputStyle.Render(m.filter) + "\n")
	} else {
		b.WriteString(mutedStyle.Render("Type to filter...") + "\n")
	}
	b.WriteString("\n")

	// Show list items
	visible := m.filtered
	maxVisible := 12
	startIdx := 0
	if m.cursor >= maxVisible {
		startIdx = m.cursor - maxVisible + 1
	}
	endIdx := min(startIdx+maxVisible, len(visible))

	for i := startIdx; i < endIdx; i++ {
		idx := visible[i]
		item := m.items[idx]
		if i == m.cursor {
			b.WriteString(cursorStyle.Render("> ") + selectedItemStyle.Render(item.name) + "\n")
		} else {
			b.WriteString("  " + normalItemStyle.Render(item.name) + "\n")
		}
	}

	if len(visible) > maxVisible {
		b.WriteString(mutedStyle.Render(fmt.Sprintf("\n  ... %d/%d items", len(visible), len(m.items))) + "\n")
	}

	// Show details for current item
	if len(visible) > 0 && m.cursor < len(visible) {
		idx := visible[m.cursor]
		item := m.items[idx]
		if item.description != "" || len(item.details) > 0 {
			b.WriteString("\n" + mutedStyle.Render(strings.Repeat("\u2500", 40)) + "\n")
			if item.description != "" {
				b.WriteString(detailLabelStyle.Render("Description:") + " " + detailValueStyle.Render(item.description) + "\n")
			}
			for _, d := range item.details {
				b.WriteString(detailLabelStyle.Render(d.key+":") + " " + detailValueStyle.Render(d.value) + "\n")
			}
		}
	}

	// Keybinding hints
	b.WriteString("\n" + hintStyle.Render("\u2191/\u2193 navigate  enter select  type to filter  esc restart") + "\n")

	return b.String()
}

func (m selectModel) Selected() (selectItem, bool) {
	if m.chosen < 0 || m.chosen >= len(m.items) {
		return selectItem{}, false
	}
	return m.items[m.chosen], true
}

// ---------------------------------------------------------------------------
// inputModel - text input with validation
// ---------------------------------------------------------------------------

type inputModel struct {
	label     string
	input     textinput.Model
	validator func(string) error
	err       string
	submitted bool
	isMasked  bool
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
		isMasked:  masked,
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
