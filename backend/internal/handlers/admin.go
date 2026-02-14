package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/database"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/models"
	"github.com/xuri/excelize/v2"
	"golang.org/x/crypto/bcrypt"
)

type AdminHandler struct {
	db *database.DB
}

func NewAdminHandler(db *database.DB) *AdminHandler {
	return &AdminHandler{db: db}
}

type BulkUploadResponse struct {
	Success int           `json:"success"`
	Failed  int           `json:"failed"`
	Skipped int           `json:"skipped"`
	Errors  []string      `json:"errors,omitempty"`
	Message string        `json:"message,omitempty"`
	Users   []models.User `json:"users,omitempty"`
}

type BulkUploadRow struct {
	FullName  string `json:"fullName"`
	Rank      string `json:"rank"`
	Battery   string `json:"battery"`
	NRICLast5 string `json:"nricLast5"`
}

// columnMapping holds detected column indices for each required field.
type columnMapping struct {
	FullName  int
	Rank      int
	Battery   int
	NRICLast5 int
	CallupCol int // -1 if not found
	HeaderRow int // 0-indexed row where headers were found
}

// findDataSheet returns the sheet name with the most rows using sheet dimensions
// (avoids reading all cell data for every sheet).
func findDataSheet(xlFile *excelize.File) string {
	sheets := xlFile.GetSheetList()
	if len(sheets) == 0 {
		return ""
	}

	bestSheet := sheets[0]
	bestRows := 0

	for _, name := range sheets {
		dim, err := xlFile.GetSheetDimension(name)
		if err != nil || dim == "" {
			continue
		}
		rowCount := parseEndRow(dim)
		if rowCount > bestRows {
			bestRows = rowCount
			bestSheet = name
		}
	}

	return bestSheet
}

// parseEndRow extracts the row number from a sheet dimension string like "A1:P433".
func parseEndRow(dim string) int {
	parts := strings.Split(dim, ":")
	if len(parts) != 2 {
		return 0
	}
	ref := parts[1]
	i := len(ref) - 1
	for i >= 0 && ref[i] >= '0' && ref[i] <= '9' {
		i--
	}
	n, _ := strconv.Atoi(ref[i+1:])
	return n
}

// detectColumns scans the first few rows for recognizable column headers.
// Returns nil if no headers are detected.
func detectColumns(rows [][]string) *columnMapping {
	maxScan := 5
	if len(rows) < maxScan {
		maxScan = len(rows)
	}

	for i := 0; i < maxScan; i++ {
		row := rows[i]
		mapping := columnMapping{
			FullName:  -1,
			Rank:      -1,
			Battery:   -1,
			NRICLast5: -1,
			CallupCol: -1,
			HeaderRow: i,
		}

		for j, cell := range row {
			lower := strings.ToLower(strings.TrimSpace(cell))
			if lower == "" {
				continue
			}
			switch {
			case mapping.FullName == -1 && (lower == "full name" || lower == "name" || lower == "fullname"):
				mapping.FullName = j
			case mapping.Rank == -1 && lower == "rank":
				mapping.Rank = j
			case mapping.Battery == -1 && (lower == "battery" || lower == "bty"):
				mapping.Battery = j
			case mapping.NRICLast5 == -1 && strings.Contains(lower, "nric"):
				mapping.NRICLast5 = j
			case mapping.CallupCol == -1 && (strings.Contains(lower, "call up") || strings.Contains(lower, "callup")):
				mapping.CallupCol = j
			}
		}

		if mapping.FullName >= 0 && mapping.Rank >= 0 && mapping.Battery >= 0 && mapping.NRICLast5 >= 0 {
			return &mapping
		}
	}

	return nil
}

// cellValue safely retrieves a trimmed cell value by index.
func cellValue(row []string, idx int) string {
	if idx < 0 || idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[idx])
}

// BulkUploadUsers handles Excel file upload for bulk user creation
func (h *AdminHandler) BulkUploadUsers(w http.ResponseWriter, r *http.Request) {
	// Parse multipart form
	err := r.ParseMultipartForm(10 << 20) // 10 MB max
	if err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "No file uploaded", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Validate file type
	if !strings.HasSuffix(header.Filename, ".xlsx") && !strings.HasSuffix(header.Filename, ".xls") {
		http.Error(w, "Invalid file type. Only Excel files (.xlsx, .xls) are allowed", http.StatusBadRequest)
		return
	}

	// Read file into memory
	fileBytes, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "Failed to read file", http.StatusInternalServerError)
		return
	}

	// Open Excel file
	xlFile, err := excelize.OpenReader(strings.NewReader(string(fileBytes)))
	if err != nil {
		http.Error(w, "Failed to open Excel file", http.StatusBadRequest)
		return
	}
	defer func() {
		_ = xlFile.Close()
	}()

	// Find the sheet with the most data (handles multi-sheet callup files)
	sheetName := findDataSheet(xlFile)
	if sheetName == "" {
		http.Error(w, "No valid data sheet found in Excel file", http.StatusBadRequest)
		return
	}

	rows, err := xlFile.GetRows(sheetName)
	if err != nil {
		http.Error(w, "Failed to read Excel rows", http.StatusInternalServerError)
		return
	}

	if len(rows) < 2 {
		http.Error(w, "Excel file must have at least a header row and one data row", http.StatusBadRequest)
		return
	}

	// Detect column mapping from headers
	colMap := detectColumns(rows)

	var nameIdx, rankIdx, batteryIdx, nricIdx, callupIdx, dataStartRow int
	if colMap != nil {
		nameIdx = colMap.FullName
		rankIdx = colMap.Rank
		batteryIdx = colMap.Battery
		nricIdx = colMap.NRICLast5
		callupIdx = colMap.CallupCol
		dataStartRow = colMap.HeaderRow + 1
	} else {
		// Fallback: assume first row is header, columns are [0,1,2,3]
		nameIdx = 0
		rankIdx = 1
		batteryIdx = 2
		nricIdx = 3
		callupIdx = -1
		dataStartRow = 1
	}

	dataRows := rows[dataStartRow:]

	// Phase 1: Validate all rows and collect valid ones
	type validatedRow struct {
		rowNum int
		data   BulkUploadRow
	}

	var validRows []validatedRow
	var failedCount int
	var skippedCount int
	var errors []string

	for i, row := range dataRows {
		rowNum := dataStartRow + i + 1 // Excel is 1-indexed

		fullName := cellValue(row, nameIdx)
		rank := cellValue(row, rankIdx)
		battery := cellValue(row, batteryIdx)
		nricLast5 := cellValue(row, nricIdx)

		// Skip completely empty rows
		if fullName == "" && rank == "" && battery == "" && nricLast5 == "" {
			continue
		}

		// Filter by callup status if column detected
		if callupIdx >= 0 {
			callupStatus := strings.ToUpper(cellValue(row, callupIdx))
			if strings.Contains(callupStatus, "NO CALL UP") || callupStatus == "" {
				skippedCount++
				continue
			}
		}

		uploadRow := BulkUploadRow{
			FullName:  fullName,
			Rank:      rank,
			Battery:   battery,
			NRICLast5: nricLast5,
		}

		// Validate required fields
		if uploadRow.FullName == "" || uploadRow.Rank == "" ||
			uploadRow.Battery == "" || uploadRow.NRICLast5 == "" {
			errors = append(errors, fmt.Sprintf("Row %d: Missing required fields", rowNum))
			failedCount++
			continue
		}

		// Validate rank
		validRank := false
		for _, r := range models.ValidRanks {
			if r == uploadRow.Rank {
				validRank = true
				break
			}
		}
		if !validRank {
			errors = append(errors, fmt.Sprintf("Row %d: Invalid rank '%s'", rowNum, uploadRow.Rank))
			failedCount++
			continue
		}

		// Validate battery
		if uploadRow.Battery != models.BatteryHQ && uploadRow.Battery != models.BatteryAlpha && uploadRow.Battery != models.BatteryBravo {
			errors = append(errors, fmt.Sprintf("Row %d: Invalid battery '%s' (must be HQ, Alpha, or Bravo)", rowNum, uploadRow.Battery))
			failedCount++
			continue
		}

		// Validate NRIC Last 5 format (5 characters)
		if len(uploadRow.NRICLast5) != 5 {
			errors = append(errors, fmt.Sprintf("Row %d: Invalid NRIC Last 5 '%s' (must be exactly 5 characters)", rowNum, uploadRow.NRICLast5))
			failedCount++
			continue
		}

		validRows = append(validRows, validatedRow{rowNum: rowNum, data: uploadRow})
	}

	// Phase 2: Hash passwords in parallel (bcrypt is ~100ms per call)
	hashedPasswords := make([]string, len(validRows))
	hashErrors := make([]error, len(validRows))

	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.NumCPU())

	for i, vr := range validRows {
		wg.Add(1)
		go func(idx int, password string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			hash, err := bcrypt.GenerateFromPassword([]byte(password), 4)
			if err != nil {
				hashErrors[idx] = err
				return
			}
			hashedPasswords[idx] = string(hash)
		}(i, vr.data.NRICLast5)
	}
	wg.Wait()

	// Phase 3: Insert into database
	ctx := context.Background()
	var successCount int
	var createdUsers []models.User

	for i, vr := range validRows {
		if hashErrors[i] != nil {
			errors = append(errors, fmt.Sprintf("Row %d: Failed to hash password", vr.rowNum))
			failedCount++
			continue
		}

		userID := generateID()
		now := time.Now()

		tx, err := h.db.Pool.Begin(ctx)
		if err != nil {
			errors = append(errors, fmt.Sprintf("Row %d: Failed to start transaction", vr.rowNum))
			failedCount++
			continue
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO "user" (
				id, "full_name", rank, battery, "nric_last5", password,
				"createdAt", "updatedAt"
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, userID, vr.data.FullName, vr.data.Rank,
			vr.data.Battery, vr.data.NRICLast5, hashedPasswords[i], now, now)

		if err != nil {
			_ = tx.Rollback(ctx)
			if strings.Contains(err.Error(), "unique constraint") || strings.Contains(err.Error(), "duplicate") {
				errors = append(errors, fmt.Sprintf("Row %d: User '%s' already exists", vr.rowNum, vr.data.FullName))
			} else {
				errors = append(errors, fmt.Sprintf("Row %d: Failed to create user: %v", vr.rowNum, err))
			}
			failedCount++
			continue
		}

		if err := tx.Commit(ctx); err != nil {
			errors = append(errors, fmt.Sprintf("Row %d: Failed to commit transaction", vr.rowNum))
			failedCount++
			continue
		}

		fn := vr.data.FullName
		rk := vr.data.Rank
		bt := vr.data.Battery
		nr := vr.data.NRICLast5

		createdUsers = append(createdUsers, models.User{
			ID:        userID,
			FullName:  &fn,
			Rank:      &rk,
			Battery:   &bt,
			NRICLast5: &nr,
			CreatedAt: now,
			UpdatedAt: now,
		})

		successCount++
	}

	var message string
	if skippedCount > 0 {
		message = fmt.Sprintf("%d personnel skipped (not called up)", skippedCount)
	}

	response := BulkUploadResponse{
		Success: successCount,
		Failed:  failedCount,
		Skipped: skippedCount,
		Errors:  errors,
		Message: message,
		Users:   createdUsers,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode bulk upload response: %v", err)
	}
}

// BulkCreateUsers handles JSON-based bulk user creation (frontend parses Excel client-side).
func (h *AdminHandler) BulkCreateUsers(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Users []BulkUploadRow `json:"users"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	if len(req.Users) == 0 {
		http.Error(w, "No users provided", http.StatusBadRequest)
		return
	}

	// Phase 1: Validate all rows
	type validatedRow struct {
		idx  int
		data BulkUploadRow
	}

	var validRows []validatedRow
	var failedCount int
	var errors []string

	for i, u := range req.Users {
		rowNum := i + 1

		u.FullName = strings.TrimSpace(u.FullName)
		u.Rank = strings.TrimSpace(u.Rank)
		u.Battery = strings.TrimSpace(u.Battery)
		u.NRICLast5 = strings.TrimSpace(u.NRICLast5)

		if u.FullName == "" || u.Rank == "" || u.Battery == "" || u.NRICLast5 == "" {
			errors = append(errors, fmt.Sprintf("Row %d: Missing required fields", rowNum))
			failedCount++
			continue
		}

		validRank := false
		for _, r := range models.ValidRanks {
			if r == u.Rank {
				validRank = true
				break
			}
		}
		if !validRank {
			errors = append(errors, fmt.Sprintf("Row %d: Invalid rank '%s'", rowNum, u.Rank))
			failedCount++
			continue
		}

		if u.Battery != models.BatteryHQ && u.Battery != models.BatteryAlpha && u.Battery != models.BatteryBravo {
			errors = append(errors, fmt.Sprintf("Row %d: Invalid battery '%s' (must be HQ, Alpha, or Bravo)", rowNum, u.Battery))
			failedCount++
			continue
		}

		if len(u.NRICLast5) != 5 {
			errors = append(errors, fmt.Sprintf("Row %d: Invalid NRIC Last 5 '%s' (must be exactly 5 characters)", rowNum, u.NRICLast5))
			failedCount++
			continue
		}

		validRows = append(validRows, validatedRow{idx: i, data: u})
	}

	// Phase 2: Hash passwords in parallel
	hashedPasswords := make([]string, len(validRows))
	hashErrors := make([]error, len(validRows))

	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.NumCPU())

	for i, vr := range validRows {
		wg.Add(1)
		go func(idx int, password string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			hash, err := bcrypt.GenerateFromPassword([]byte(password), 4)
			if err != nil {
				hashErrors[idx] = err
				return
			}
			hashedPasswords[idx] = string(hash)
		}(i, vr.data.NRICLast5)
	}
	wg.Wait()

	// Phase 3: Insert into database
	ctx := context.Background()
	var successCount int
	var createdUsers []models.User

	for i, vr := range validRows {
		if hashErrors[i] != nil {
			errors = append(errors, fmt.Sprintf("Row %d: Failed to hash password", vr.idx+1))
			failedCount++
			continue
		}

		userID := generateID()
		now := time.Now()

		tx, err := h.db.Pool.Begin(ctx)
		if err != nil {
			errors = append(errors, fmt.Sprintf("Row %d: Failed to start transaction", vr.idx+1))
			failedCount++
			continue
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO "user" (
				id, "full_name", rank, battery, "nric_last5", password,
				"createdAt", "updatedAt"
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, userID, vr.data.FullName, vr.data.Rank,
			vr.data.Battery, vr.data.NRICLast5, hashedPasswords[i], now, now)

		if err != nil {
			_ = tx.Rollback(ctx)
			if strings.Contains(err.Error(), "unique constraint") || strings.Contains(err.Error(), "duplicate") {
				errors = append(errors, fmt.Sprintf("Row %d: User '%s' already exists", vr.idx+1, vr.data.FullName))
			} else {
				errors = append(errors, fmt.Sprintf("Row %d: Failed to create user: %v", vr.idx+1, err))
			}
			failedCount++
			continue
		}

		if err := tx.Commit(ctx); err != nil {
			errors = append(errors, fmt.Sprintf("Row %d: Failed to commit transaction", vr.idx+1))
			failedCount++
			continue
		}

		fn := vr.data.FullName
		rk := vr.data.Rank
		bt := vr.data.Battery
		nr := vr.data.NRICLast5

		createdUsers = append(createdUsers, models.User{
			ID:        userID,
			FullName:  &fn,
			Rank:      &rk,
			Battery:   &bt,
			NRICLast5: &nr,
			CreatedAt: now,
			UpdatedAt: now,
		})

		successCount++
	}

	response := BulkUploadResponse{
		Success: successCount,
		Failed:  failedCount,
		Errors:  errors,
		Users:   createdUsers,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode bulk create response: %v", err)
	}
}
