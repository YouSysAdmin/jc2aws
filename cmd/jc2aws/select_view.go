package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Scrollbar glyphs.
const (
	scrollThumb = "█"
	scrollTrack = "│"
)

// truncateCell shortens s to at most width display cells, marking the cut with
// an ellipsis.
//
// Unlike truncateARN it keeps the leading characters, which is what
// distinguishes account and region names from one another.
func truncateCell(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	if width == 1 {
		return "…"
	}

	r := []rune(s)
	for len(r) > 0 && lipgloss.Width(string(r))+1 > width {
		r = r[:len(r)-1]
	}

	return string(r) + "…"
}

// View renders the select step: a label, a filter line, the item columns with
// their scrollbar, a window counter, the item details, and the key hints.
func (m selectModel) View() string {
	// Defensive: correct even if a resize site were ever missed. The clamp runs
	// on this copy and is discarded.
	m = m.clampScroll()
	l := m.layout()

	var header strings.Builder
	header.WriteString(promptLabelStyle.Render(m.label) + "\n")
	if m.filter != "" {
		header.WriteString(mutedStyle.Render("Filter: ") + inputStyle.Render(m.filter) + "\n")
	} else {
		header.WriteString(mutedStyle.Render("Type to filter...") + "\n")
	}
	header.WriteString("\n")

	body := lipgloss.JoinHorizontal(lipgloss.Top, m.viewColumns(l), m.viewScrollbar(l))

	// The rule spans the content it separates, so it does not dangle into the
	// rows reserved for the counter.
	if l.detailsWidth > 0 {
		details := lipgloss.NewStyle().Width(l.detailsWidth).Render(m.viewDetails(l.detailsWidth, false))
		body = lipgloss.JoinHorizontal(lipgloss.Top,
			body,
			m.viewDivider(max(lipgloss.Height(body), lipgloss.Height(details))),
			details,
		)
	}

	main := lipgloss.JoinVertical(lipgloss.Left, body, "", m.viewCounter(l))
	if l.detailRows > 0 {
		main = lipgloss.JoinVertical(lipgloss.Left, main, m.viewDetails(l.listWidth, true))
	}

	return header.String() + main + "\n\n" + hintStyle.Render(m.hint(l)) + "\n"
}

// hint returns the key hints, advertising the column keys only when there is
// more than one column: promising keys that do nothing is worse than silence.
func (m selectModel) hint(l selectLayout) string {
	if l.cols > 1 {
		return "↑/↓/←/→ navigate  enter select  type to filter  esc restart"
	}
	return "↑/↓ navigate  enter select  type to filter  esc restart"
}

// viewDivider renders the vertical rule between the list and details panes. It
// mirrors the sidebar's right border, so the wizard has one way of separating
// panes rather than two.
func (m selectModel) viewDivider(height int) string {
	height = max(height, 1)

	rule := " " + mutedStyle.Render(scrollTrack) + " "
	rows := make([]string, height)
	for i := range rows {
		rows[i] = rule
	}

	return strings.Join(rows, "\n")
}

// viewColumns renders the visible window as column-major blocks: filled top to
// bottom, then left to right.
func (m selectModel) viewColumns(l selectLayout) string {
	if len(m.filtered) == 0 {
		return mutedStyle.Render("  no matches")
	}

	end := min(m.offset+l.capacity, len(m.filtered))
	colStyle := lipgloss.NewStyle().Width(l.cellWidth)

	blocks := make([]string, 0, l.cols*2)
	for c := range l.cols {
		var b strings.Builder
		for r := range l.rows {
			if r > 0 {
				b.WriteString("\n")
			}
			// Column-major: down, then across.
			if i := m.offset + gridIndex(r, c, l.rows); i < end {
				b.WriteString(m.viewRow(i, l.cellWidth))
			}
		}
		if c > 0 {
			blocks = append(blocks, strings.Repeat(" ", selectColGap))
		}
		blocks = append(blocks, colStyle.Render(b.String()))
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, blocks...)
}

// viewRow renders one item cell. The two-cell cursor prefix is part of every
// cell, so highlighting an item never shifts a column boundary.
func (m selectModel) viewRow(i, width int) string {
	// Truncation happens here, on the way to the screen, never in the model:
	// handleSelectResult matches the chosen item by its full name.
	item := m.items[m.filtered[i]]
	name := truncateCell(item.name, width-2)

	if i == m.cursor {
		return cursorStyle.Render("> ") + selectedItemStyle.Render(name)
	}
	return "  " + normalItemStyle.Render(name)
}

// viewScrollbar renders the scroll indicator beside the columns. The gutter is
// reserved even when nothing overflows, so the columns never shift sideways
// when a filter removes the overflow.
func (m selectModel) viewScrollbar(l selectLayout) string {
	total := len(m.filtered)
	if total <= l.capacity || l.rows <= 0 {
		return lipgloss.NewStyle().Width(selectScrollGutter).Render("")
	}

	thumb := min(max(1, l.rows*l.capacity/total), l.rows)
	travel := l.rows - thumb
	maxOffset := total - l.capacity

	// Map offset onto the thumb's travel, not onto the whole track: using the
	// track would leave the thumb short of the bottom at maximum scroll.
	top := 0
	if travel > 0 && maxOffset > 0 {
		top = (m.offset*travel + maxOffset/2) / maxOffset
	}
	top = min(max(top, 0), travel)

	var b strings.Builder
	for r := range l.rows {
		if r > 0 {
			b.WriteString("\n")
		}
		b.WriteString(" ")
		if r >= top && r < top+thumb {
			b.WriteString(cursorStyle.Render(scrollThumb))
		} else {
			b.WriteString(mutedStyle.Render(scrollTrack))
		}
	}

	return b.String()
}

// viewCounter reports the visible window over the total. The row is reserved
// even when there is nothing to say, so the layout never jumps by a row the
// moment scrolling starts.
func (m selectModel) viewCounter(l selectLayout) string {
	total := len(m.filtered)
	shown := min(m.offset+l.capacity, total) - m.offset

	var s string
	switch {
	case total == 0 && m.filter != "":
		s = fmt.Sprintf("no matches for %q", m.filter)
	case total == 0:
		s = ""
	case shown < total && m.filter != "":
		s = fmt.Sprintf("showing %d-%d of %d matched (%d total)",
			m.offset+1, m.offset+shown, total, len(m.items))
	case shown < total:
		s = fmt.Sprintf("showing %d-%d of %d", m.offset+1, m.offset+shown, total)
	case m.filter != "" && total < len(m.items):
		s = fmt.Sprintf("%d of %d match", total, len(m.items))
	default:
		s = ""
	}
	if s == "" {
		return ""
	}

	return mutedStyle.Render("  " + s)
}

// viewDetails renders the highlighted item's information.
//
// Stacked mode sits below the list on a narrow terminal, so its values are
// truncated to one line to keep the reserved row count exact. The side pane is
// independent of the list, so there values may wrap freely.
func (m selectModel) viewDetails(width int, stacked bool) string {
	if len(m.filtered) == 0 || m.cursor >= len(m.filtered) {
		return ""
	}
	item := m.items[m.filtered[m.cursor]]
	if item.description == "" && len(item.details) == 0 {
		return ""
	}

	labelWidth := lipgloss.Width(detailLabelStyle.Render(""))
	valueWidth := max(width-labelWidth-1, 8)

	var b strings.Builder
	if stacked {
		b.WriteString("\n" + mutedStyle.Render(strings.Repeat("─", max(width, 1))) + "\n")
	} else {
		b.WriteString(titleStyle.Render(truncateCell(item.name, width)) + "\n\n")
	}

	writeRow := func(key, value string) {
		var rendered string
		if stacked {
			rendered = detailValueStyle.Render(truncateCell(value, valueWidth))
		} else {
			rendered = detailValueStyle.Width(valueWidth).Render(value)
		}
		// JoinHorizontal keeps a wrapped value hanging under itself rather than
		// running back under the label.
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top,
			detailLabelStyle.Render(key+":"), " ", rendered) + "\n")
	}

	if item.description != "" {
		writeRow("Description", item.description)
	}
	for _, d := range item.details {
		writeRow(d.key, d.value)
	}

	return b.String()
}
