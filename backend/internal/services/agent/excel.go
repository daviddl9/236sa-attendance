package agent

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"
)

const excelBatchSize = 80

// excelToBatches opens an XLSX/XLSM file, finds the largest sheet, and
// returns a slice of markdown-table strings. Each string contains the
// header row plus at most excelBatchSize data rows. Keeping batches
// small prevents Claude from hitting its output-token limit when rosters
// have hundreds of rows.
func excelToBatches(contents []byte) ([]string, error) {
	xl, err := excelize.OpenReader(bytes.NewReader(contents))
	if err != nil {
		return nil, fmt.Errorf("agent: open xlsx: %w", err)
	}
	defer xl.Close()

	sheet := largestSheet(xl)
	if sheet == "" {
		return nil, ErrNoRowsDetected
	}

	rows, err := xl.GetRows(sheet)
	if err != nil {
		return nil, fmt.Errorf("agent: read xlsx rows: %w", err)
	}

	// Drop leading empty rows so the first non-empty row becomes the header.
	for len(rows) > 0 && isEmptyRow(rows[0]) {
		rows = rows[1:]
	}
	if len(rows) == 0 {
		return nil, ErrNoRowsDetected
	}

	header := rows[0]
	data := rows[1:]

	maxCols := len(header)
	for _, row := range data {
		if len(row) > maxCols {
			maxCols = len(row)
		}
	}

	// Produce a markdown header block (header + separator).
	headerMD := renderRow(header, maxCols) + "\n" + separatorRow(maxCols) + "\n"

	// Split data rows into batches.
	var batches []string
	for start := 0; start < len(data); start += excelBatchSize {
		end := start + excelBatchSize
		if end > len(data) {
			end = len(data)
		}
		var b strings.Builder
		b.WriteString(headerMD)
		for _, row := range data[start:end] {
			b.WriteString(renderRow(row, maxCols))
			b.WriteString("\n")
		}
		batches = append(batches, b.String())
	}

	if len(batches) == 0 {
		return nil, ErrNoRowsDetected
	}
	return batches, nil
}

func renderRow(row []string, maxCols int) string {
	var b strings.Builder
	b.WriteString("|")
	for c := 0; c < maxCols; c++ {
		cell := ""
		if c < len(row) {
			cell = strings.ReplaceAll(row[c], "|", "\\|")
			cell = strings.ReplaceAll(cell, "\n", " ")
		}
		b.WriteString(" ")
		b.WriteString(cell)
		b.WriteString(" |")
	}
	return b.String()
}

func separatorRow(maxCols int) string {
	var b strings.Builder
	b.WriteString("|")
	for c := 0; c < maxCols; c++ {
		b.WriteString(" --- |")
	}
	return b.String()
}

// isEmptyRow returns true when every cell in the row is blank.
func isEmptyRow(row []string) bool {
	for _, cell := range row {
		if strings.TrimSpace(cell) != "" {
			return false
		}
	}
	return true
}

// largestSheet returns the sheet name with the most rows.
func largestSheet(xl *excelize.File) string {
	sheets := xl.GetSheetList()
	best := ""
	bestRows := 0
	for _, name := range sheets {
		rows, err := xl.GetRows(name)
		if err != nil {
			continue
		}
		if len(rows) > bestRows {
			best = name
			bestRows = len(rows)
		}
	}
	return best
}
