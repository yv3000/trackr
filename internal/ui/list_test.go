package ui

import (
	"testing"

	"trackr/internal/model"
)

func sampleRows() []Row {
	return []Row{
		{Header: true, Text: "PACKAGE MANAGERS"},
		{Separator: true},
		{Text: "pip      requests   2.0", Item: &model.Item{Name: "requests"}},
		{Text: "pip      numpy      1.0", Item: &model.Item{Name: "numpy"}},
		{Text: "pip      numpydoc   1.0", Item: &model.Item{Name: "numpydoc"}},
	}
}

func TestNavPositionsSkipsHeadersAndSeparators(t *testing.T) {
	m := newListModel("t", sampleRows(), false)
	np := m.navPositions()
	// Only the 3 item rows (positions 2,3,4) are navigable.
	want := []int{2, 3, 4}
	if len(np) != len(want) {
		t.Fatalf("navPositions = %v, want %v", np, want)
	}
	for i := range want {
		if np[i] != want[i] {
			t.Fatalf("navPositions = %v, want %v", np, want)
		}
	}
	// Initial cursor must land on the first navigable row, never a header.
	if m.cursorPos != 2 {
		t.Errorf("initial cursorPos = %d, want 2 (first item row)", m.cursorPos)
	}
}

func TestApplyFilterMatchesText(t *testing.T) {
	m := newListModel("t", sampleRows(), true)
	m.searchQuery = "numpy"
	m.applyFilter()
	// "numpy" and "numpydoc" rows match (indices 3 and 4); header/sep excluded.
	if len(m.filtered) != 2 {
		t.Fatalf("filtered len = %d (%v), want 2", len(m.filtered), m.filtered)
	}
	if m.filtered[0] != 3 || m.filtered[1] != 4 {
		t.Errorf("filtered = %v, want [3 4]", m.filtered)
	}
	// After filtering, cursor resets to the first visible match.
	if m.cursorPos != 0 {
		t.Errorf("cursorPos after filter = %d, want 0", m.cursorPos)
	}

	// Clearing the query removes the filter.
	m.searchQuery = ""
	m.applyFilter()
	if m.filtered != nil {
		t.Errorf("expected nil filter after clearing query, got %v", m.filtered)
	}
}

func TestMoveCursorReadOnlySkipsDecorative(t *testing.T) {
	m := newListModel("t", sampleRows(), false) // read-only list
	if m.cursorPos != 2 {
		t.Fatalf("start cursorPos = %d, want 2", m.cursorPos)
	}
	m.moveCursor(1)
	if m.cursorPos != 3 {
		t.Errorf("after down cursorPos = %d, want 3", m.cursorPos)
	}
	m.moveCursor(-1)
	if m.cursorPos != 2 {
		t.Errorf("after up cursorPos = %d, want 2", m.cursorPos)
	}
	// Cannot move above the first navigable row.
	m.moveCursor(-1)
	if m.cursorPos != 2 {
		t.Errorf("cursor went past top: got %d, want 2", m.cursorPos)
	}
}
