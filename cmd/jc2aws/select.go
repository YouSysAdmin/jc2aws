package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"
)

// ---------------------------------------------------------------------------
// selectModel - a filterable selection list
// ---------------------------------------------------------------------------

// Geometry constants for the select component.
const (
	// selectChromeRows is the number of rows View spends on everything that is
	// not an item: label, filter line, two separators, the counter row, and a
	// blank plus the hint line.
	selectChromeRows = 7
	// selectMinRows keeps the list usable on a terminal too short for the chrome.
	selectMinRows = 3
	// selectMaxColumns caps the fan-out: past three columns the eye-scan cost of
	// column-major order outweighs the density.
	selectMaxColumns = 3
	// selectMinCellWidth is the narrowest an item cell may be.
	selectMinCellWidth = 12
	// selectColGap separates adjacent item columns.
	selectColGap = 2
	// selectScrollGutter is reserved for the scrollbar whether or not it is
	// drawn, so the columns never shift when scrolling starts.
	selectScrollGutter = 2
	// detailsMinWidth derives from detailLabelStyle.Width(16) plus a space and
	// enough room for a value to be worth reading.
	detailsMinWidth = 34
	// detailsMaxWidth stops the details pane from dominating a very wide window.
	detailsMaxWidth = 52
	// listMinWidth is the narrowest list pane worth keeping a details pane beside.
	listMinWidth = 24
	// paneGap separates the list pane from the details pane: a space, a
	// vertical rule, and a space.
	paneGap = 3
	// detailRowsMax caps the stacked details block on narrow terminals.
	detailRowsMax = 10

	// Fallbacks for a model that has not been given a size yet.
	defaultSelectWidth  = 76
	defaultSelectHeight = 20
)

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
	// cursor is an index into filtered. It is never a (row, col) pair and never
	// an index into a per-column sub-slice: the grid is only a rendering
	// projection, so chosen can be derived from it directly.
	cursor int
	// offset is the index into filtered of the first visible item. It is always
	// a multiple of the current layout's scroll unit.
	offset int
	filter string
	chosen int // -1 until chosen

	// width and height are the usable text area of the content pane, net of
	// padding and any banner.
	width  int
	height int
}

// identityIndices returns [0, 1, ..., n-1].
func identityIndices(n int) []int {
	indices := make([]int, n)
	for i := range indices {
		indices[i] = i
	}
	return indices
}

func newSelectModel(label string, items []selectItem) selectModel {
	return selectModel{
		label:    label,
		items:    items,
		filtered: identityIndices(len(items)),
		cursor:   0,
		chosen:   -1,
	}
}

// WithSize returns a copy of the model laid out for the given usable text area.
// Width and height exclude the surrounding pane padding and any banner rows.
func (m selectModel) WithSize(width, height int) selectModel {
	m.width = width
	m.height = height
	return m.clampScroll()
}

// Size returns the usable text area the model was last given.
func (m selectModel) Size() (width, height int) {
	return m.width, m.height
}

// ---------------------------------------------------------------------------
// Geometry
// ---------------------------------------------------------------------------

// selectLayout is the derived geometry of the select panes for the current size
// and filtered item set. It is recomputed on demand and never stored, because it
// depends on the filter, which changes on every keystroke.
type selectLayout struct {
	rows         int // item rows per column
	cols         int // number of item columns
	cellWidth    int // width of one item cell, including the 2-cell cursor prefix
	capacity     int // rows*cols: items visible at once
	scrollUnit   int // items scrolled per step: a whole column, or 1 when single-column
	listWidth    int // width of the list pane
	detailsWidth int // width of the details pane; 0 when it does not fit
	detailRows   int // rows reserved for a details block below the list; 0 when the pane is used
}

// gridPos returns the on-screen position of a window-relative index in a
// column-major grid filled top-to-bottom then left-to-right.
func gridPos(index, rows int) (row, col int) {
	if rows < 1 {
		rows = 1
	}
	return index % rows, index / rows
}

// gridIndex returns the window-relative index at the given column-major position.
func gridIndex(row, col, rows int) int {
	if rows < 1 {
		rows = 1
	}
	return col*rows + row
}

// columnCount returns how many columns n items need at the given column height.
func columnCount(n, rows int) int {
	if n <= 0 {
		return 0
	}
	if rows < 1 {
		rows = 1
	}
	return (n + rows - 1) / rows
}

// alignDown rounds v down to a multiple of unit.
func alignDown(v, unit int) int {
	if unit <= 1 {
		return v
	}
	return (v / unit) * unit
}

// alignUp rounds v up to a multiple of unit.
func alignUp(v, unit int) int {
	if unit <= 1 {
		return v
	}
	return ((v + unit - 1) / unit) * unit
}

// hasDetails reports whether any item carries information worth a details pane.
func (m selectModel) hasDetails() bool {
	for _, it := range m.items {
		if it.description != "" || len(it.details) > 0 {
			return true
		}
	}
	return false
}

// detailRowsReserve returns a stable height for the stacked details block: the
// worst case across every item, so the list height never depends on which item
// happens to be highlighted.
func (m selectModel) detailRowsReserve() int {
	worst := 0
	for _, it := range m.items {
		n := len(it.details)
		if it.description != "" {
			n++
		}
		worst = max(worst, n)
	}
	if worst == 0 {
		return 0
	}
	return min(worst+2, detailRowsMax) // +2 for the blank line and the rule
}

// layout derives the pane geometry for the current size and filtered set.
func (m selectModel) layout() selectLayout {
	w, h := m.width, m.height
	if w <= 0 {
		w = defaultSelectWidth
	}
	if h <= 0 {
		h = defaultSelectHeight
	}
	n := len(m.filtered)

	detailsWidth, detailRows := 0, 0
	if m.hasDetails() {
		d := min(max(w/3, detailsMinWidth), detailsMaxWidth)
		if w-d-paneGap >= listMinWidth {
			detailsWidth = d
		} else {
			detailRows = m.detailRowsReserve()
		}
	}
	listWidth := w
	if detailsWidth > 0 {
		listWidth = w - detailsWidth - paneGap
	}

	rows := max(h-selectChromeRows-detailRows, selectMinRows)

	// Measured across every item, not just the visible ones: deriving it from
	// the filter would make the column width — and with it the details pane's
	// position — jump around as the user types.
	longest := 0
	for _, it := range m.items {
		longest = max(longest, lipgloss.Width(it.name))
	}
	inner := max(listWidth-selectScrollGutter, selectMinCellWidth)
	cell := min(max(longest+2, selectMinCellWidth), inner) // +2 for the cursor prefix

	maxCols := min(max(1, (inner+selectColGap)/(cell+selectColGap)), selectMaxColumns)
	cols := min(max(columnCount(n, rows), 1), maxCols)
	if n > 0 {
		// Never taller than the chosen column count actually requires.
		rows = min(rows, max(1, columnCount(n, cols)))
	}

	unit := 1
	if cols > 1 {
		unit = rows
	}

	return selectLayout{
		rows:         rows,
		cols:         cols,
		cellWidth:    cell,
		capacity:     rows * cols,
		scrollUnit:   unit,
		listWidth:    listWidth,
		detailsWidth: detailsWidth,
		detailRows:   detailRows,
	}
}

// clampScroll keeps the cursor in range and the visible window aligned around it.
// It is the single place those invariants are established.
func (m selectModel) clampScroll() selectModel {
	n := len(m.filtered)
	if n == 0 {
		m.cursor = 0
		m.offset = 0
		return m
	}

	m.cursor = min(max(m.cursor, 0), n-1)

	l := m.layout()
	unit := l.scrollUnit
	maxOffset := alignUp(max(0, n-l.capacity), unit)

	m.offset = alignDown(min(max(m.offset, 0), maxOffset), unit)
	if m.cursor < m.offset {
		m.offset = alignDown(m.cursor, unit)
	}
	if m.cursor >= m.offset+l.capacity {
		m.offset = alignUp(m.cursor-l.capacity+1, unit)
	}
	m.offset = min(max(m.offset, 0), maxOffset)

	return m
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func (m selectModel) Init() tea.Cmd { return nil }

func (m selectModel) Update(msg tea.Msg) (selectModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		l := m.layout()
		n := len(m.filtered)

		switch msg.String() {
		case "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down":
			if m.cursor < n-1 {
				m.cursor++
			}
		case "left":
			// Every column to the left is full by construction.
			if m.cursor >= l.rows {
				m.cursor -= l.rows
			}
		case "right":
			// True exactly when at least one item exists in a later column; the
			// min then lands on the target cell, or on the bottom of a short
			// last column.
			if n > 0 && m.cursor/l.rows < (n-1)/l.rows {
				m.cursor = min(m.cursor+l.rows, n-1)
			}
		case "home":
			m.cursor = 0
		case "end":
			m.cursor = max(0, n-1)
		case "pgup":
			m.cursor = max(m.cursor-l.capacity, 0)
		case "pgdown":
			m.cursor = min(m.cursor+l.capacity, max(0, n-1))
		case "enter":
			if n > 0 {
				m.chosen = m.filtered[m.cursor]
			}
		case "backspace":
			if len(m.filter) > 0 {
				m.filter = m.filter[:len(m.filter)-1]
				m = m.applyFilter()
			}
		default:
			// Any single-character key extends the filter. This is why no bare
			// letter or digit can ever become a navigation binding: h/j/k/l,
			// g/G, q, / and space are permanently unavailable here. Named keys
			// (up, pgdown, ...) are multi-character and so are safe.
			if len(msg.String()) == 1 {
				m.filter += msg.String()
				m = m.applyFilter()
			}
		}
	}

	return m.clampScroll(), nil
}

func (m selectModel) applyFilter() selectModel {
	if m.filter == "" {
		m.filtered = identityIndices(len(m.items))
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
	// A new filter always shows its results from the top.
	m.offset = 0

	return m.clampScroll()
}

func (m selectModel) Selected() (selectItem, bool) {
	if m.chosen < 0 || m.chosen >= len(m.items) {
		return selectItem{}, false
	}
	return m.items[m.chosen], true
}
