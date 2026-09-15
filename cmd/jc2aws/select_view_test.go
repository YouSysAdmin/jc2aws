package main

import (
	"fmt"
	"strings"
	"testing"
)

// newDetailedSelect builds a sized model whose items all carry details.
func newDetailedSelect(t *testing.T, n, width, height int) selectModel {
	t.Helper()

	items := make([]selectItem, 0, n)
	for i := range n {
		items = append(items, selectItem{
			name:        fmt.Sprintf("account-%02d", i),
			description: "Production account",
			details: []detailPair{
				{"Provider", "AWS"},
				{"Roles", "admin, read-only"},
				{"Regions", "ca-central-1, us-east-1"},
			},
		})
	}
	return newSelectModel("Select account:", items).WithSize(width, height)
}

func TestTruncateCell(t *testing.T) {
	tests := []struct {
		name  string
		s     string
		width int
		want  string
	}{
		{name: "fits", s: "abc", width: 10, want: "abc"},
		{name: "exact fit", s: "abcde", width: 5, want: "abcde"},
		{name: "truncates", s: "abcdefgh", width: 5, want: "abcd…"},
		{name: "keeps the leading characters", s: "production-eu-west", width: 8, want: "product…"},
		{name: "ellipsis counts toward the width", s: "abcdefgh", width: 3, want: "ab…"},
		{name: "width one", s: "abc", width: 1, want: "…"},
		{name: "width zero", s: "abc", width: 0, want: ""},
		{name: "negative width", s: "abc", width: -3, want: ""},
		{name: "empty input", s: "", width: 5, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := truncateCell(tt.s, tt.width); got != tt.want {
				t.Errorf("truncateCell(%q, %d) = %q, want %q", tt.s, tt.width, got, tt.want)
			}
		})
	}
}

func TestSelectModelViewCounter(t *testing.T) {
	tests := []struct {
		name            string
		n, width, hei   int
		wantContains    []string
		wantNotContains []string
	}{
		{
			name: "everything fits, no counter",
			n:    10, width: 160, hei: 60,
			wantNotContains: []string{"showing"},
		},
		{
			name: "overflow reports the window, not the filter ratio",
			n:    60, width: 60, hei: 20,
			wantContains: []string{"showing 1-"},
			// The old footer printed len(filtered)/len(items), which read as
			// "all 60 shown" while a fraction of them were on screen.
			wantNotContains: []string{"60/60", "items"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := newTestSelect(t, tt.n, tt.width, tt.hei).View()
			for _, want := range tt.wantContains {
				if !strings.Contains(v, want) {
					t.Errorf("view does not contain %q\n%s", want, v)
				}
			}
			for _, unwanted := range tt.wantNotContains {
				if strings.Contains(v, unwanted) {
					t.Errorf("view unexpectedly contains %q\n%s", unwanted, v)
				}
			}
		})
	}
}

func TestSelectModelViewCounterCountsTheWindow(t *testing.T) {
	m := newTestSelect(t, 60, 60, 20)
	l := m.layout()
	if l.capacity >= 60 {
		t.Fatalf("fixture must overflow, capacity=%d", l.capacity)
	}

	want := fmt.Sprintf("showing 1-%d of 60", l.capacity)
	if v := m.View(); !strings.Contains(v, want) {
		t.Errorf("view does not contain %q\n%s", want, v)
	}
}

func TestSelectModelViewScrollbarOnlyWhenOverflowing(t *testing.T) {
	tests := []struct {
		name          string
		n, width, hei int
		wantThumb     bool
	}{
		{name: "fits", n: 10, width: 160, hei: 60, wantThumb: false},
		{name: "overflows", n: 60, width: 60, hei: 20, wantThumb: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := newTestSelect(t, tt.n, tt.width, tt.hei).View()
			if got := strings.Contains(v, scrollThumb); got != tt.wantThumb {
				t.Errorf("scrollbar present = %v, want %v\n%s", got, tt.wantThumb, v)
			}
		})
	}
}

func TestSelectModelViewScrollbarReachesBothEnds(t *testing.T) {
	m := newTestSelect(t, 60, 60, 20)
	l := m.layout()

	// At the top the thumb must start on the first row.
	rows := strings.Split(m.View(), "\n")
	if !strings.Contains(rows[3], scrollThumb) {
		t.Errorf("thumb is not flush with the top:\n%s", m.View())
	}

	// At the bottom it must reach the last row of the track.
	m = key(m, "end")
	rows = strings.Split(m.View(), "\n")
	if !strings.Contains(rows[3+l.rows-1], scrollThumb) {
		t.Errorf("thumb is not flush with the bottom:\n%s", m.View())
	}
}

func TestSelectModelViewHintMentionsColumnsOnlyWhenMultiColumn(t *testing.T) {
	single := newTestSelect(t, 5, 160, 60)
	if l := single.layout(); l.cols != 1 {
		t.Fatalf("fixture must be single-column, got %d", l.cols)
	}
	if v := single.View(); strings.Contains(v, "←/→") {
		t.Error("single-column view advertises column keys that do nothing")
	}

	multi := newTestSelect(t, 40, 160, 20)
	if l := multi.layout(); l.cols < 2 {
		t.Fatalf("fixture must be multi-column, got %d", l.cols)
	}
	if v := multi.View(); !strings.Contains(v, "←/→") {
		t.Error("multi-column view does not advertise the column keys")
	}
}

func TestSelectModelViewAllItemsReachable(t *testing.T) {
	m := newTestSelect(t, 60, 60, 20)

	if v := m.View(); !strings.Contains(v, "item-00") {
		t.Errorf("first item not visible at the top\n%s", v)
	}
	m = key(m, "end")
	if v := m.View(); !strings.Contains(v, "item-59") {
		t.Errorf("last item not reachable via end\n%s", v)
	}
	m = key(m, "home")
	if v := m.View(); !strings.Contains(v, "item-00") {
		t.Errorf("first item not reachable via home\n%s", v)
	}
}

func TestSelectModelViewEmptyFilterIsExplained(t *testing.T) {
	m := newTestSelect(t, 10, 100, 30)
	m = key(m, "z") // matches nothing

	v := m.View()
	if !strings.Contains(v, "no matches") {
		t.Errorf("an over-restrictive filter renders an unexplained void\n%s", v)
	}
}

func TestSelectModelViewDetailsPlacement(t *testing.T) {
	// Wide: details sit beside the list, so the label shares a line with an item.
	wide := newDetailedSelect(t, 10, 120, 40)
	if l := wide.layout(); l.detailsWidth == 0 {
		t.Fatalf("fixture must use the side pane, got detailsWidth=0 (width=%d)", l.listWidth)
	}
	sideBySide := false
	for _, line := range strings.Split(wide.View(), "\n") {
		if strings.Contains(line, "account-0") && strings.Contains(line, "Provider:") {
			sideBySide = true
			break
		}
	}
	if !sideBySide {
		t.Errorf("details are not beside the list\n%s", wide.View())
	}

	// Narrow: details fall back under the list, never on an item's line.
	narrow := newDetailedSelect(t, 10, 40, 40)
	if l := narrow.layout(); l.detailsWidth != 0 {
		t.Fatalf("fixture must stack, got detailsWidth=%d", l.detailsWidth)
	}
	for _, line := range strings.Split(narrow.View(), "\n") {
		if strings.Contains(line, "account-0") && strings.Contains(line, "Provider:") {
			t.Errorf("narrow view put details beside the list\n%s", narrow.View())
		}
	}
	if !strings.Contains(narrow.View(), "Provider:") {
		t.Errorf("narrow view dropped the details entirely\n%s", narrow.View())
	}
}

func TestSelectModelListHeightIndependentOfHighlightedItem(t *testing.T) {
	// The reason details moved out of the list flow: an item with many details
	// must not shrink the list.
	items := []selectItem{
		{name: "bare"},
		{name: "rich", description: "x", details: []detailPair{
			{"a", "1"}, {"b", "2"}, {"c", "3"}, {"d", "4"}, {"e", "5"},
		}},
	}
	m := newSelectModel("Select:", items).WithSize(120, 40)

	first := m.layout().rows
	m = key(m, "down")
	if second := m.layout().rows; second != first {
		t.Errorf("list height changed with the highlighted item: %d -> %d", first, second)
	}
}

func TestSelectModelViewWithoutSizeStillRenders(t *testing.T) {
	// Several existing tests build a model and call View without a size.
	m := newSelectModel("Select:", []selectItem{
		{name: "first item", description: "the first one"},
	})

	v := m.View()
	for _, want := range []string{"Select:", "first item", "the first one"} {
		if !strings.Contains(v, want) {
			t.Errorf("unsized view is missing %q\n%s", want, v)
		}
	}
}

func TestSelectModelViewDividesThePanes(t *testing.T) {
	// A fits-in-one-screen fixture, so any vertical rule in the output is the
	// pane divider rather than the scrollbar track.
	wide := newDetailedSelect(t, 20, 120, 40)
	l := wide.layout()
	if l.detailsWidth == 0 {
		t.Fatalf("fixture must use the side pane")
	}
	if l.capacity < 20 {
		t.Fatalf("fixture must not overflow, capacity=%d", l.capacity)
	}

	v := wide.View()
	ruleRows := 0
	for _, row := range strings.Split(v, "\n") {
		if strings.Contains(row, scrollTrack) {
			ruleRows++
		}
	}
	if ruleRows == 0 {
		t.Fatalf("no rule between the list and the details pane\n%s", v)
	}
	// The list is the taller pane here, so the rule spans exactly it: it must
	// neither stop short nor dangle into the rows reserved for the counter.
	if ruleRows != l.rows {
		t.Errorf("divider spans %d rows, want %d (the list height)\n%s", ruleRows, l.rows, v)
	}

	// Stacked mode has no second pane, so no rule either.
	narrow := newDetailedSelect(t, 6, 40, 40)
	if narrow.layout().detailsWidth != 0 {
		t.Fatalf("fixture must stack")
	}
	if strings.Contains(narrow.View(), scrollTrack) {
		t.Errorf("stacked view drew a pane divider\n%s", narrow.View())
	}
}
