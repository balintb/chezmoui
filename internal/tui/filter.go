package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) beginFilter() (Model, tea.Cmd) {
	m.filtering = true
	m.filterInput.SetValue(m.filter)
	m.filterInput.CursorEnd()
	m.filterInput.Focus()
	return m, nil
}

func (m Model) handleFilterKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.filtering = false
		m.filterInput.Blur()
		m.filter = ""
		m.filterInput.SetValue("")
		m.recomputeVisible()
		m.clampCursor()
		return m, nil
	case tea.KeyEnter:
		m.filtering = false
		m.filterInput.Blur()
		m.filter = m.filterInput.Value()
		m.recomputeVisible()
		m.clampCursor()
		return m, nil
	}

	var cmd tea.Cmd
	m.filterInput, cmd = m.filterInput.Update(msg)
	m.filter = m.filterInput.Value()
	m.recomputeVisible()
	m.clampCursor()
	return m, cmd
}

func (m *Model) clampCursor() {
	if m.cursor >= len(m.visibleIdxs) {
		m.cursor = max(0, len(m.visibleIdxs)-1)
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}
