package main

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// newTestSelect builds a sized model with n uniformly named items.
func newTestSelect(t *testing.T, n, width, height int) selectModel {
	t.Helper()

	items := make([]selectItem, 0, n)
	for i := range n {
		items = append(items, selectItem{name: fmt.Sprintf("item-%02d", i)})
	}
	return newSelectModel("Select:", items).WithSize(width, height)
}

// key sends a named key to the model.
func key(m selectModel, name string) selectModel {
	var k tea.KeyMsg
	switch name {
	case "up":
		k = tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		k = tea.KeyMsg{Type: tea.KeyDown}
	case "left":
		k = tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		k = tea.KeyMsg{Type: tea.KeyRight}
	case "home":
		k = tea.KeyMsg{Type: tea.KeyHome}
	case "end":
		k = tea.KeyMsg{Type: tea.KeyEnd}
	case "pgup":
		k = tea.KeyMsg{Type: tea.KeyPgUp}
	case "pgdown":
		k = tea.KeyMsg{Type: tea.KeyPgDown}
	case "enter":
		k = tea.KeyMsg{Type: tea.KeyEnter}
	case "backspace":
		k = tea.KeyMsg{Type: tea.KeyBackspace}
	default:
		k = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(name)}
	}
	m, _ = m.Update(k)
	return m
}

// assertSelectInvariants checks the properties that must hold after every Update.
func assertSelectInvariants(t *testing.T, m selectModel, context string) {
	t.Helper()

	l := m.layout()
	n := len(m.filtered)

	if l.rows < 1 || l.cols < 1 {
		t.Errorf("%s: rows=%d cols=%d, both must be >= 1", context, l.rows, l.cols)
	}
	if m.cursor < 0 {
		t.Errorf("%s: cursor %d is negative", context, m.cursor)
	}
	if n == 0 {
		if m.cursor != 0 || m.offset != 0 {
			t.Errorf("%s: empty list must have cursor=0 offset=0, got %d/%d", context, m.cursor, m.offset)
		}
		return
	}
	if m.cursor >= n {
		t.Errorf("%s: cursor %d out of range for %d items", context, m.cursor, n)
	}
	if m.offset < 0 {
		t.Errorf("%s: offset %d is negative", context, m.offset)
	}
	if m.offset%l.scrollUnit != 0 {
		t.Errorf("%s: offset %d is not a multiple of the scroll unit %d", context, m.offset, l.scrollUnit)
	}
	if m.cursor < m.offset || m.cursor >= m.offset+l.capacity {
		t.Errorf("%s: cursor %d outside window [%d, %d)", context, m.cursor, m.offset, m.offset+l.capacity)
	}
	if m.chosen != -1 && (m.chosen < 0 || m.chosen >= len(m.items)) {
		t.Errorf("%s: chosen %d out of range", context, m.chosen)
	}
}

func TestGridPos(t *testing.T) {
	tests := []struct {
		name             string
		index, rows      int
		wantRow, wantCol int
	}{
		{name: "first cell", index: 0, rows: 10, wantRow: 0, wantCol: 0},
		{name: "bottom of first column", index: 9, rows: 10, wantRow: 9, wantCol: 0},
		{name: "top of second column", index: 10, rows: 10, wantRow: 0, wantCol: 1},
		{name: "bottom of second column", index: 19, rows: 10, wantRow: 9, wantCol: 1},
		{name: "third column", index: 20, rows: 10, wantRow: 0, wantCol: 2},
		{name: "single row per column", index: 5, rows: 1, wantRow: 0, wantCol: 5},
		{name: "zero rows is treated as one", index: 3, rows: 0, wantRow: 0, wantCol: 3},
		{name: "negative rows is treated as one", index: 3, rows: -4, wantRow: 0, wantCol: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row, col := gridPos(tt.index, tt.rows)
			if row != tt.wantRow || col != tt.wantCol {
				t.Errorf("gridPos(%d, %d) = (%d, %d), want (%d, %d)",
					tt.index, tt.rows, row, col, tt.wantRow, tt.wantCol)
			}
		})
	}
}

func TestGridIndexRoundTrips(t *testing.T) {
	for _, rows := range []int{1, 3, 10} {
		for i := range 41 {
			row, col := gridPos(i, rows)
			if got := gridIndex(row, col, rows); got != i {
				t.Errorf("rows=%d: gridIndex(gridPos(%d)) = %d", rows, i, got)
			}
		}
	}
}

func TestColumnCount(t *testing.T) {
	tests := []struct {
		name    string
		n, rows int
		want    int
	}{
		{name: "empty", n: 0, rows: 10, want: 0},
		{name: "one item", n: 1, rows: 10, want: 1},
		{name: "exactly one column", n: 10, rows: 10, want: 1},
		{name: "one over", n: 11, rows: 10, want: 2},
		{name: "four columns", n: 33, rows: 10, want: 4},
		{name: "single row", n: 33, rows: 1, want: 33},
		{name: "zero rows is guarded", n: 5, rows: 0, want: 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := columnCount(tt.n, tt.rows); got != tt.want {
				t.Errorf("columnCount(%d, %d) = %d, want %d", tt.n, tt.rows, got, tt.want)
			}
		})
	}
}

func TestAlignUpDown(t *testing.T) {
	tests := []struct {
		name             string
		v, unit          int
		wantDown, wantUp int
	}{
		{name: "already aligned", v: 20, unit: 10, wantDown: 20, wantUp: 20},
		{name: "misaligned", v: 7, unit: 10, wantDown: 0, wantUp: 10},
		{name: "unit one is identity", v: 7, unit: 1, wantDown: 7, wantUp: 7},
		{name: "unit zero is identity", v: 7, unit: 0, wantDown: 7, wantUp: 7},
		{name: "zero value", v: 0, unit: 10, wantDown: 0, wantUp: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := alignDown(tt.v, tt.unit); got != tt.wantDown {
				t.Errorf("alignDown(%d, %d) = %d, want %d", tt.v, tt.unit, got, tt.wantDown)
			}
			if got := alignUp(tt.v, tt.unit); got != tt.wantUp {
				t.Errorf("alignUp(%d, %d) = %d, want %d", tt.v, tt.unit, got, tt.wantUp)
			}
		})
	}
}

func TestLayoutRowsGrowWithHeight(t *testing.T) {
	// The bug this whole change is about: rows was a constant, not a function
	// of the terminal height.
	short := newTestSelect(t, 60, 100, 24).layout()
	tall := newTestSelect(t, 60, 100, 60).layout()

	if tall.rows <= short.rows {
		t.Errorf("rows did not grow with height: %d rows at h=24, %d at h=60", short.rows, tall.rows)
	}
	if short.rows < selectMinRows {
		t.Errorf("rows %d fell below the floor %d", short.rows, selectMinRows)
	}
}

func TestLayoutColumnsOnlyWhenNeeded(t *testing.T) {
	tests := []struct {
		name          string
		n, width, hei int
		wantCols      int
	}{
		{name: "fits in one column on a wide terminal", n: 18, width: 160, hei: 60, wantCols: 1},
		{name: "overflows into columns when short", n: 40, width: 160, hei: 20, wantCols: 3},
		{name: "two columns when that is enough", n: 14, width: 160, hei: 14, wantCols: 2},
		{name: "narrow terminal fits fewer columns", n: 40, width: 30, hei: 20, wantCols: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := newTestSelect(t, tt.n, tt.width, tt.hei).layout()
			if l.cols != tt.wantCols {
				t.Errorf("cols = %d, want %d (rows=%d capacity=%d)", l.cols, tt.wantCols, l.rows, l.capacity)
			}
		})
	}
}

func TestLayoutLongNamesCapColumns(t *testing.T) {
	// Width caps the column count independently of how badly the list overflows:
	// with realistic account names a narrow pane holds only one column.
	items := make([]selectItem, 0, 40)
	for i := range 40 {
		items = append(items, selectItem{name: fmt.Sprintf("spotlight-prod-china-%02d", i)})
	}
	m := newSelectModel("Select:", items).WithSize(30, 20)

	if l := m.layout(); l.cols != 1 {
		t.Errorf("cols = %d, want 1 (cellWidth=%d listWidth=%d)", l.cols, l.cellWidth, l.listWidth)
	}
}

func TestLayoutZeroSizeIsUsable(t *testing.T) {
	// A model that never received a size still has to render something sane:
	// several existing tests build one directly.
	l := newSelectModel("x", []selectItem{{name: "a"}}).layout()
	if l.rows < 1 || l.cols < 1 || l.capacity < 1 {
		t.Errorf("zero-size layout is unusable: %+v", l)
	}
}

func TestSelectModelGridNavigation(t *testing.T) {
	// 40 items at 160x20 lays out as 3 columns of 7 (capacity 21).
	base := newTestSelect(t, 40, 160, 20)
	l := base.layout()
	if l.cols < 2 {
		t.Fatalf("fixture must be multi-column, got cols=%d rows=%d", l.cols, l.rows)
	}
	rows := l.rows

	tests := []struct {
		name  string
		start int
		keys  []string
		want  int
	}{
		{name: "down moves one", start: 0, keys: []string{"down"}, want: 1},
		{name: "down at bottom of a column enters the next", start: rows - 1, keys: []string{"down"}, want: rows},
		{name: "up at top of a column returns to the previous", start: rows, keys: []string{"up"}, want: rows - 1},
		{name: "right moves one column", start: 2, keys: []string{"right"}, want: 2 + rows},
		{name: "left undoes right", start: 2, keys: []string{"right", "left"}, want: 2},
		{name: "left in the first column is a no-op", start: 2, keys: []string{"left"}, want: 2},
		{name: "up at the first item is a no-op", start: 0, keys: []string{"up"}, want: 0},
		{name: "down at the last item is a no-op", start: 39, keys: []string{"down"}, want: 39},
		{name: "home jumps to the first", start: 25, keys: []string{"home"}, want: 0},
		{name: "end jumps to the last", start: 3, keys: []string{"end"}, want: 39},
		{name: "right from the last column is a no-op", start: 39, keys: []string{"right"}, want: 39},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := base
			m.cursor = tt.start
			m = m.clampScroll()
			for _, k := range tt.keys {
				m = key(m, k)
				assertSelectInvariants(t, m, tt.name)
			}
			if m.cursor != tt.want {
				t.Errorf("cursor = %d, want %d (rows=%d)", m.cursor, tt.want, rows)
			}
		})
	}
}

func TestSelectModelRightLandsOnShortLastColumn(t *testing.T) {
	// A ragged last column: right must land on its bottom rather than dead-end.
	m := newTestSelect(t, 23, 160, 20)
	l := m.layout()
	if l.cols < 2 {
		t.Fatalf("fixture must be multi-column, got cols=%d", l.cols)
	}

	// Stand in a row that does not exist in the last column.
	last := 22
	m.cursor = last - (last % l.rows) - 1 // bottom of the previous column
	if m.cursor < 0 {
		t.Skip("fixture geometry leaves no previous column")
	}
	m = m.clampScroll()

	before := m.cursor
	m = key(m, "right")
	if m.cursor <= before {
		t.Errorf("right did not advance: %d -> %d", before, m.cursor)
	}
	if m.cursor > last {
		t.Errorf("right overshot the last item: %d > %d", m.cursor, last)
	}
	assertSelectInvariants(t, m, "ragged right")
}

func TestSelectModelSingleColumnNavigationUnchanged(t *testing.T) {
	// Small lists must behave exactly as before: left/right do nothing.
	m := newTestSelect(t, 5, 160, 60)
	if l := m.layout(); l.cols != 1 {
		t.Fatalf("fixture must be single-column, got cols=%d", l.cols)
	}

	m = key(m, "down")
	m = key(m, "down")
	if m.cursor != 2 {
		t.Errorf("cursor = %d, want 2", m.cursor)
	}
	for _, k := range []string{"left", "right"} {
		before := m.cursor
		if m = key(m, k); m.cursor != before {
			t.Errorf("%s moved the cursor in a single-column list: %d -> %d", k, before, m.cursor)
		}
	}
}

func TestSelectModelChosenUnaffectedByScroll(t *testing.T) {
	// The obvious bug when adding an offset is indexing filtered[offset+cursor].
	m := newTestSelect(t, 40, 160, 20)
	m = key(m, "end")
	if m.offset == 0 {
		t.Fatal("fixture must have scrolled")
	}

	m = key(m, "enter")
	item, ok := m.Selected()
	if !ok {
		t.Fatal("Selected() returned nothing after enter")
	}
	if want := "item-39"; item.name != want {
		t.Errorf("Selected() = %q, want %q (offset=%d cursor=%d)", item.name, want, m.offset, m.cursor)
	}
}

func TestSelectModelInvariantsUnderKeySequence(t *testing.T) {
	geometries := []struct {
		name          string
		n, width, hei int
	}{
		{name: "multi-column overflow", n: 60, width: 160, hei: 20},
		{name: "single column", n: 18, width: 100, hei: 60},
		{name: "tiny terminal", n: 30, width: 30, hei: 8},
		{name: "empty list", n: 0, width: 100, hei: 30},
	}
	script := []string{
		"down", "down", "right", "down", "end", "up", "left", "pgdown", "pgup",
		"i", "t", "e", "m", "down", "right", "backspace", "backspace", "home",
		"down", "left", "end", "backspace", "backspace", "up", "pgdown",
	}

	for _, g := range geometries {
		t.Run(g.name, func(t *testing.T) {
			m := newTestSelect(t, g.n, g.width, g.hei)
			assertSelectInvariants(t, m, "initial")
			for i, k := range script {
				m = key(m, k)
				assertSelectInvariants(t, m, fmt.Sprintf("after %d:%s", i, k))
			}
		})
	}
}

func TestSelectModelFilterResetsOffset(t *testing.T) {
	m := newTestSelect(t, 60, 160, 20)
	m = key(m, "end")
	if m.offset == 0 {
		t.Fatal("fixture must have scrolled")
	}

	m = key(m, "5") // matches item-05, item-15, ...
	if m.offset != 0 {
		t.Errorf("offset = %d, want 0 after filtering", m.offset)
	}
	if m.cursor >= len(m.filtered) {
		t.Errorf("cursor %d out of range for %d matches", m.cursor, len(m.filtered))
	}
	assertSelectInvariants(t, m, "after filter")
}

func TestSelectModelResizeKeepsCursor(t *testing.T) {
	m := newTestSelect(t, 60, 160, 20)
	m = key(m, "end")
	want := m.cursor

	for _, size := range [][2]int{{100, 60}, {40, 10}, {160, 20}} {
		m = m.WithSize(size[0], size[1])
		if m.cursor != want {
			t.Errorf("resize to %dx%d moved the cursor: %d, want %d", size[0], size[1], m.cursor, want)
		}
		assertSelectInvariants(t, m, fmt.Sprintf("resized to %dx%d", size[0], size[1]))
	}
}

func TestSelectModelZeroSizeDoesNotPanic(t *testing.T) {
	for _, size := range [][2]int{{0, 0}, {-5, -5}, {1, 1}} {
		m := newTestSelect(t, 10, size[0], size[1])
		for _, k := range []string{"down", "up", "left", "right", "home", "end", "pgup", "pgdown"} {
			m = key(m, k)
			assertSelectInvariants(t, m, fmt.Sprintf("%dx%d after %s", size[0], size[1], k))
		}
		if m.View() == "" {
			t.Errorf("View() at %dx%d rendered nothing", size[0], size[1])
		}
	}
}

func TestSelectModelEmptyListIsSafe(t *testing.T) {
	m := newTestSelect(t, 0, 100, 30)
	for _, k := range []string{"down", "up", "left", "right", "end", "enter"} {
		m = key(m, k)
	}
	if _, ok := m.Selected(); ok {
		t.Error("Selected() returned an item from an empty list")
	}
	assertSelectInvariants(t, m, "empty")
}
