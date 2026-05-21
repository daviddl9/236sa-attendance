package handlers

import (
	"testing"
)

func TestDetectColumns_CallupFile(t *testing.T) {
	// Simulates the callup file layout: row 0 blank, row 1 has headers
	rows := [][]string{
		{}, // Row 1: blank
		{"S/N", "NRIC Last 5", "Rank", "Full Name", "", "", "", "", "", "", "", "", "", "", "", "Bty", "2026 Call up"},
		{"1", "1234A", "3SG", "John Doe", "", "", "", "", "", "", "", "", "", "", "", "Alpha", "MAIN BODY"},
	}

	colMap := detectColumns(rows)
	if colMap == nil {
		t.Fatal("expected column mapping, got nil")
	}

	if colMap.HeaderRow != 1 {
		t.Errorf("HeaderRow = %d, want 1", colMap.HeaderRow)
	}
	if colMap.NRICLast5 != 1 {
		t.Errorf("NRICLast5 = %d, want 1", colMap.NRICLast5)
	}
	if colMap.Rank != 2 {
		t.Errorf("Rank = %d, want 2", colMap.Rank)
	}
	if colMap.FullName != 3 {
		t.Errorf("FullName = %d, want 3", colMap.FullName)
	}
	if colMap.Battery != 15 {
		t.Errorf("Battery = %d, want 15", colMap.Battery)
	}
	if colMap.CallupCol != 16 {
		t.Errorf("CallupCol = %d, want 16", colMap.CallupCol)
	}
}

func TestDetectColumns_SimpleFormat(t *testing.T) {
	// The simple 4-column format used in the original UI
	rows := [][]string{
		{"Full Name", "Rank", "Battery", "NRIC Last 5"},
		{"Jane Smith", "CPL", "Bravo", "5678B"},
	}

	colMap := detectColumns(rows)
	if colMap == nil {
		t.Fatal("expected column mapping, got nil")
	}

	if colMap.HeaderRow != 0 {
		t.Errorf("HeaderRow = %d, want 0", colMap.HeaderRow)
	}
	if colMap.FullName != 0 {
		t.Errorf("FullName = %d, want 0", colMap.FullName)
	}
	if colMap.Rank != 1 {
		t.Errorf("Rank = %d, want 1", colMap.Rank)
	}
	if colMap.Battery != 2 {
		t.Errorf("Battery = %d, want 2", colMap.Battery)
	}
	if colMap.NRICLast5 != 3 {
		t.Errorf("NRICLast5 = %d, want 3", colMap.NRICLast5)
	}
	if colMap.CallupCol != -1 {
		t.Errorf("CallupCol = %d, want -1", colMap.CallupCol)
	}
}

func TestDetectColumns_NoHeaders(t *testing.T) {
	// Data without recognizable headers — should return nil for fallback
	rows := [][]string{
		{"John Doe", "3SG", "Alpha", "1234A"},
		{"Jane Smith", "CPL", "Bravo", "5678B"},
	}

	colMap := detectColumns(rows)
	if colMap != nil {
		t.Errorf("expected nil for unrecognized headers, got %+v", colMap)
	}
}

func TestDetectColumns_EmptyRows(t *testing.T) {
	rows := [][]string{}
	colMap := detectColumns(rows)
	if colMap != nil {
		t.Errorf("expected nil for empty rows, got %+v", colMap)
	}
}

func TestParseEndRow(t *testing.T) {
	tests := []struct {
		dim  string
		want int
	}{
		{"A1:P433", 433},
		{"A1:A1", 1},
		{"A1:Z100", 100},
		{"", 0},
		{"A1", 0},
	}

	for _, tt := range tests {
		t.Run(tt.dim, func(t *testing.T) {
			got := parseEndRow(tt.dim)
			if got != tt.want {
				t.Errorf("parseEndRow(%q) = %d, want %d", tt.dim, got, tt.want)
			}
		})
	}
}

func TestCellValue(t *testing.T) {
	row := []string{"  hello  ", "world", ""}

	tests := []struct {
		name string
		idx  int
		want string
	}{
		{"trims whitespace", 0, "hello"},
		{"normal value", 1, "world"},
		{"empty string", 2, ""},
		{"out of bounds", 5, ""},
		{"negative index", -1, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cellValue(row, tt.idx)
			if got != tt.want {
				t.Errorf("cellValue(row, %d) = %q, want %q", tt.idx, got, tt.want)
			}
		})
	}
}

func TestBulkNRICLast5ValidationRule(t *testing.T) {
	tests := []struct {
		value string
		want  string
		ok    bool
	}{
		{value: "1234A", want: "1234A", ok: true},
		{value: "1234a", want: "1234A", ok: true},
		{value: "12345", ok: false},
		{value: "123A4", ok: false},
		{value: "1234@", ok: false},
		{value: "1234AB", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			got, ok := normalizeNRICLast5(tt.value)
			if ok != tt.ok {
				t.Fatalf("normalizeNRICLast5(%q) ok = %v, want %v", tt.value, ok, tt.ok)
			}
			if got != tt.want {
				t.Fatalf("normalizeNRICLast5(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}
