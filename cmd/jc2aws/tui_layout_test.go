package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yousysadmin/jc2aws/internal/config"
)

// layoutTestAccounts returns more accounts than any sane terminal has rows, so
// the list geometry is driven by the available height rather than by the item
// count. layout() caps rows at what the items actually need, so a small fixture
// would make a height assertion meaningless.
func layoutTestAccounts() []config.Account {
	accounts := make([]config.Account, 0, 40)
	for i := range 40 {
		accounts = append(accounts, config.Account{
			Name:        fmt.Sprintf("account-%02d", i),
			Description: "Test account",
			Regions:     []string{"us-east-1"},
		})
	}
	return accounts
}

func TestContentBoxShrinksWithTheBanner(t *testing.T) {
	resetViper()

	base := tuiModel{appCfg: newTestConfig(nil), width: 120, height: 40}
	_, plain := base.contentBox()

	withNotice := base
	withNotice.notice = "account \"x\" not found in config"
	_, noticed := withNotice.contentBox()
	if noticed >= plain {
		t.Errorf("notice did not reduce the box: %d -> %d", plain, noticed)
	}

	withUpdate := base
	withUpdate.updateVersion = "9.9.9"
	_, updated := withUpdate.contentBox()
	if updated >= plain {
		t.Errorf("update banner did not reduce the box: %d -> %d", plain, updated)
	}
}

func TestContentBoxNeverNegative(t *testing.T) {
	for _, size := range [][2]int{{0, 0}, {10, 1}, {200, 2}} {
		m := tuiModel{appCfg: newTestConfig(nil), width: size[0], height: size[1]}
		w, h := m.contentBox()
		if w < 0 || h < 0 {
			t.Errorf("contentBox at %dx%d = (%d, %d)", size[0], size[1], w, h)
		}
	}
}

func TestUpdate_WindowSizeMsgPropagatesToSelect(t *testing.T) {
	resetViper()

	m := newTuiModel(newTestConfig(layoutTestAccounts()))
	if m.compType != compSelect {
		t.Fatalf("expected the account select, got %q", m.compType)
	}
	_, before := m.selectComp.Size()

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 60})
	rm, ok := updated.(tuiModel)
	if !ok {
		t.Fatal("Update did not return a tuiModel")
	}

	_, after := rm.selectComp.Size()
	if after <= before {
		t.Errorf("select height did not grow with the terminal: %d -> %d", before, after)
	}
	if rows := rm.selectComp.layout().rows; rows <= 12 {
		t.Errorf("a 60-row terminal still yields only %d list rows", rows)
	}
}

func TestRestart_PropagatesSizeToSelect(t *testing.T) {
	resetViper()

	m := newTuiModel(newTestConfig(layoutTestAccounts()))
	m.width, m.height = 200, 60
	m.resizeComponents()
	want := m.selectComp.layout().rows

	nm := m.restart()
	if nm.width != 200 || nm.height != 60 {
		t.Fatalf("restart lost the terminal size: %dx%d", nm.width, nm.height)
	}
	if got := nm.selectComp.layout().rows; got != want {
		t.Errorf("restart reset the list geometry: %d rows, want %d", got, want)
	}
}

func TestInitStep_AllSelectBuildersReceiveSize(t *testing.T) {
	accounts := layoutTestAccounts()
	accounts[0].Roles = []config.Role{
		{Name: "admin", Arn: "arn:aws:iam::000000000000:role/admin"},
		{Name: "read-only", Arn: "arn:aws:iam::000000000000:role/ro"},
	}

	tests := []struct {
		name  string
		setup func(m *tuiModel)
	}{
		{
			name:  "account",
			setup: func(m *tuiModel) { m.current = stepAccount },
		},
		{
			name: "role",
			setup: func(m *tuiModel) {
				m.account = &accounts[0]
				m.current = stepRole
			},
		},
		{
			name: "region",
			setup: func(m *tuiModel) {
				m.account = &accounts[0]
				m.current = stepRegion
			},
		},
		{
			name:  "output format",
			setup: func(m *tuiModel) { m.current = stepOutputFormat },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetViper()

			m := newTuiModel(newTestConfig(accounts))
			m.width, m.height = 200, 60
			tt.setup(&m)
			m.initStep()

			if m.compType != compSelect {
				t.Skipf("step did not build a select (compType %q)", m.compType)
			}
			w, h := m.selectComp.Size()
			if w <= 0 || h <= 0 {
				t.Errorf("select was built unsized: %dx%d", w, h)
			}
			// Only the account step has enough items for the height to bind;
			// layout() caps rows at what the items need.
			if tt.name == "account" {
				if rows := m.selectComp.layout().rows; rows <= 12 {
					t.Errorf("select on a 60-row terminal has only %d rows", rows)
				}
			}
		})
	}
}

func TestResizeComponentsIgnoresNonSelectSteps(t *testing.T) {
	resetViper()

	m := newTuiModel(newTestConfig(nil))
	m.compType = compInput
	m.width, m.height = 200, 60

	// Must not panic and must leave the select alone.
	m.resizeComponents()
	if w, h := m.selectComp.Size(); w != 0 || h != 0 {
		t.Errorf("an input step resized the select: %dx%d", w, h)
	}
}

func TestTrimErrorBoundsTheDoneScreen(t *testing.T) {
	resetViper()

	m := tuiModel{appCfg: newTestConfig(nil), width: 100, height: 40}
	long := errors.New(strings.Repeat("a very long provider error message. ", 60))

	rows := strings.Count(m.trimError(long), "\n") + 1
	if rows > doneErrorMaxRows+1 {
		t.Errorf("trimError produced %d rows, want at most %d", rows, doneErrorMaxRows+1)
	}

	short := errors.New("boom")
	if got := m.trimError(short); !strings.Contains(got, "boom") {
		t.Errorf("trimError dropped a short message: %q", got)
	}
	if strings.Contains(m.trimError(short), "…") {
		t.Error("trimError marked a short message as cut")
	}
}
