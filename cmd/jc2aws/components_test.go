package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// ---------------------------------------------------------------------------
// selectModel tests
// ---------------------------------------------------------------------------

func TestNewSelectModel(t *testing.T) {
	items := []selectItem{
		{name: "alpha", description: "first"},
		{name: "beta", description: "second"},
		{name: "gamma", description: "third"},
	}
	m := newSelectModel("Pick one:", items)

	if m.label != "Pick one:" {
		t.Errorf("expected label 'Pick one:', got %q", m.label)
	}
	if len(m.items) != 3 {
		t.Errorf("expected 3 items, got %d", len(m.items))
	}
	if len(m.filtered) != 3 {
		t.Errorf("expected 3 filtered indices, got %d", len(m.filtered))
	}
	if m.cursor != 0 {
		t.Errorf("expected cursor at 0, got %d", m.cursor)
	}
	if m.chosen != -1 {
		t.Errorf("expected chosen -1, got %d", m.chosen)
	}
}

func TestSelectModelNavigation(t *testing.T) {
	items := []selectItem{
		{name: "a"}, {name: "b"}, {name: "c"},
	}
	m := newSelectModel("Test:", items)

	// Move down
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 1 {
		t.Errorf("expected cursor 1 after down, got %d", m.cursor)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 2 {
		t.Errorf("expected cursor 2 after second down, got %d", m.cursor)
	}

	// Don't go past end
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 2 {
		t.Errorf("expected cursor to stay at 2, got %d", m.cursor)
	}

	// Move up
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.cursor != 1 {
		t.Errorf("expected cursor 1 after up, got %d", m.cursor)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.cursor != 0 {
		t.Errorf("expected cursor 0, got %d", m.cursor)
	}

	// Don't go before start
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.cursor != 0 {
		t.Errorf("expected cursor to stay at 0, got %d", m.cursor)
	}
}

func TestSelectModelFilter(t *testing.T) {
	items := []selectItem{
		{name: "us-east-1"},
		{name: "us-west-2"},
		{name: "eu-west-1"},
		{name: "ap-northeast-1"},
	}
	m := newSelectModel("Region:", items)

	// Type "us"
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})

	if m.filter != "us" {
		t.Errorf("expected filter 'us', got %q", m.filter)
	}
	if len(m.filtered) != 2 {
		t.Errorf("expected 2 filtered items for 'us', got %d", len(m.filtered))
	}

	// Backspace
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if m.filter != "u" {
		t.Errorf("expected filter 'u' after backspace, got %q", m.filter)
	}

	// Clear filter entirely
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if m.filter != "" {
		t.Errorf("expected empty filter, got %q", m.filter)
	}
	if len(m.filtered) != 4 {
		t.Errorf("expected all 4 items after clearing filter, got %d", len(m.filtered))
	}
}

func TestSelectModelEnterSelection(t *testing.T) {
	items := []selectItem{
		{name: "one"}, {name: "two"}, {name: "three"},
	}
	m := newSelectModel("Pick:", items)

	// Move to "two" and press enter
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	item, ok := m.Selected()
	if !ok {
		t.Fatal("expected a selection after enter")
	}
	if item.name != "two" {
		t.Errorf("expected 'two', got %q", item.name)
	}
}

func TestSelectModelSelectedBeforeChoosing(t *testing.T) {
	m := newSelectModel("Test:", []selectItem{{name: "a"}})
	_, ok := m.Selected()
	if ok {
		t.Error("expected no selection before pressing enter")
	}
}

func TestSelectModelView(t *testing.T) {
	items := []selectItem{
		{name: "alpha", description: "first item"},
	}
	m := newSelectModel("Pick:", items)
	v := m.View()

	if !strings.Contains(v, "Pick:") {
		t.Error("view should contain the label")
	}
	if !strings.Contains(v, "alpha") {
		t.Error("view should contain item name")
	}
	if !strings.Contains(v, "first item") {
		t.Error("view should contain item description in details")
	}
}

func TestSelectModelFilterCursorClamp(t *testing.T) {
	items := []selectItem{
		{name: "alpha"}, {name: "beta"}, {name: "gamma"},
	}
	m := newSelectModel("Test:", items)

	// Move cursor to last item
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 2 {
		t.Fatalf("expected cursor at 2, got %d", m.cursor)
	}

	// Filter to just "alpha" — cursor should clamp to 0
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})

	if m.cursor >= len(m.filtered) {
		t.Errorf("cursor %d should be < filtered length %d", m.cursor, len(m.filtered))
	}
}

// ---------------------------------------------------------------------------
// inputModel tests
// ---------------------------------------------------------------------------

func TestNewInputModel(t *testing.T) {
	m := newInputModel("Email", false, nil)

	if m.label != "Email" {
		t.Errorf("expected label 'Email', got %q", m.label)
	}
	if m.isMasked {
		t.Error("expected isMasked=false")
	}
	if m.submitted {
		t.Error("expected submitted=false initially")
	}
}

func TestNewInputModelMasked(t *testing.T) {
	m := newInputModel("Password", true, nil)

	if !m.isMasked {
		t.Error("expected isMasked=true for password input")
	}
}

func TestInputModelSubmitNoValidator(t *testing.T) {
	m := newInputModel("Name", false, nil)

	// Type "hello"
	for _, r := range "hello" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Submit
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if !m.IsSubmitted() {
		t.Error("expected submitted=true after enter")
	}
	if m.Value() != "hello" {
		t.Errorf("expected 'hello', got %q", m.Value())
	}
}

func TestInputModelSubmitWithValidatorFail(t *testing.T) {
	validator := func(s string) error {
		if len(s) < 5 {
			return &validationError{"too short"}
		}
		return nil
	}
	m := newInputModel("Input", false, validator)

	// Type "hi" (too short)
	for _, r := range "hi" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Try submit — should fail
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.IsSubmitted() {
		t.Error("expected submitted=false when validation fails")
	}
	if m.err == "" {
		t.Error("expected error message to be set")
	}
}

func TestInputModelSubmitWithValidatorPass(t *testing.T) {
	validator := func(s string) error {
		if len(s) < 3 {
			return &validationError{"too short"}
		}
		return nil
	}
	m := newInputModel("Input", false, validator)

	// Type "hello"
	for _, r := range "hello" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if !m.IsSubmitted() {
		t.Error("expected submitted=true after valid input")
	}
}

func TestInputModelView(t *testing.T) {
	m := newInputModel("Email", false, nil)
	v := m.View()

	if !strings.Contains(v, "Email") {
		t.Error("view should contain the label")
	}
}

func TestInputModelErrorClearsOnTyping(t *testing.T) {
	validator := func(s string) error {
		if s == "" {
			return &validationError{"required"}
		}
		return nil
	}
	m := newInputModel("Field", false, validator)

	// Submit empty — should set error
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.err == "" {
		t.Fatal("expected error after submitting empty")
	}

	// Type a character — error should clear
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if m.err != "" {
		t.Error("expected error to clear after typing")
	}
}

// validationError is a simple error type for testing.
type validationError struct{ msg string }

func (e *validationError) Error() string { return e.msg }

// ---------------------------------------------------------------------------
// choiceModel tests
// ---------------------------------------------------------------------------

func TestNewChoiceModel(t *testing.T) {
	m := newChoiceModel("What next?", []string{"Run again", "Quit"})

	if m.title != "What next?" {
		t.Errorf("expected title 'What next?', got %q", m.title)
	}
	if len(m.choices) != 2 {
		t.Errorf("expected 2 choices, got %d", len(m.choices))
	}
	if m.cursor != 0 {
		t.Errorf("expected cursor 0, got %d", m.cursor)
	}
	if m.IsChosen() {
		t.Error("expected IsChosen()=false initially")
	}
	if m.ChosenIndex() != -1 {
		t.Errorf("expected ChosenIndex()=-1, got %d", m.ChosenIndex())
	}
}

func TestChoiceModelNavigation(t *testing.T) {
	m := newChoiceModel("Title", []string{"A", "B", "C"})

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 1 {
		t.Errorf("expected cursor 1, got %d", m.cursor)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 2 {
		t.Errorf("expected cursor 2, got %d", m.cursor)
	}

	// Don't go past end
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 2 {
		t.Errorf("expected cursor to stay at 2, got %d", m.cursor)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.cursor != 1 {
		t.Errorf("expected cursor 1 after up, got %d", m.cursor)
	}

	// Arrow key navigation (up)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.cursor != 0 {
		t.Errorf("expected cursor 0 after up, got %d", m.cursor)
	}

	// Arrow key navigation (down)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 1 {
		t.Errorf("expected cursor 1 after down, got %d", m.cursor)
	}
}

func TestChoiceModelSelection(t *testing.T) {
	m := newChoiceModel("Pick:", []string{"Yes", "No"})

	// Move to "No" and select
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if !m.IsChosen() {
		t.Error("expected IsChosen()=true after enter")
	}
	if m.ChosenIndex() != 1 {
		t.Errorf("expected ChosenIndex()=1, got %d", m.ChosenIndex())
	}
}

func TestChoiceModelView(t *testing.T) {
	m := newChoiceModel("What next?", []string{"Run again", "Quit"})
	v := m.View()

	if !strings.Contains(v, "What next?") {
		t.Error("view should contain the title")
	}
	if !strings.Contains(v, "Run again") {
		t.Error("view should contain first choice")
	}
	if !strings.Contains(v, "Quit") {
		t.Error("view should contain second choice")
	}
}
