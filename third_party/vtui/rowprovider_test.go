package vtui

import (
	"fmt"
	"testing"
)

type millionRowProvider struct {
	requestedRows map[int]bool
	count         int
}

func (m *millionRowProvider) RowCount() int {
	return m.count
}

func (m *millionRowProvider) Row(index int) []string {
	if m.requestedRows == nil {
		m.requestedRows = make(map[int]bool)
	}
	m.requestedRows[index] = true
	return []string{fmt.Sprintf("Item %d", index), fmt.Sprintf("%d KB", index%1024)}
}

func TestTable_MillionRowProvider(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(40, 10)

	provider := &millionRowProvider{count: 1_000_000}
	cols := []TableColumn{
		{Title: "Name", Width: 20},
		{Title: "Size", Width: 10},
	}

	tbl := NewTable(0, 0, 35, 10, cols)
	tbl.SetRowProvider(provider)

	// 1. Initial display: should only request the visible rows (top 10 rows)
	tbl.Show(scr)

	if len(provider.requestedRows) > 20 {
		t.Errorf("Expected at most 20 requested rows on initial draw, got %d", len(provider.requestedRows))
	}
	if !provider.requestedRows[0] {
		t.Error("Row 0 was not requested")
	}
	if provider.requestedRows[500_000] {
		t.Error("Row 500,000 should not be requested during initial draw")
	}

	// 2. Scroll far down (e.g. to row 50,000)
	provider.requestedRows = make(map[int]bool)
	tbl.SetSelectPos(50_000)
	tbl.Show(scr)

	if len(provider.requestedRows) > 20 {
		t.Errorf("Expected at most 20 requested rows after scrolling, got %d", len(provider.requestedRows))
	}
	if !provider.requestedRows[50_000] {
		t.Error("Row 50,000 was not requested after scrolling")
	}
	if provider.requestedRows[0] {
		t.Error("Row 0 should not be requested when scrolled to 50,000")
	}
}

func TestListBox_RowProvider(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(30, 8)

	provider := &millionRowProvider{count: 500_000}
	lb := NewListBox(0, 0, 25, 8, nil)
	lb.SetRowProvider(provider)

	lb.Show(scr)

	if len(provider.requestedRows) > 15 {
		t.Errorf("Expected at most 15 rows requested by ListBox, got %d", len(provider.requestedRows))
	}
	if !provider.requestedRows[0] {
		t.Error("ListBox did not request row 0")
	}
}
