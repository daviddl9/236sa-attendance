package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
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
	Errors  []string      `json:"errors,omitempty"`
	Users   []models.User `json:"users,omitempty"`
}

type BulkUploadRow struct {
	FullName  string `json:"fullName"`
	Rank      string `json:"rank"`
	Battery   string `json:"battery"`
	NRICLast4 string `json:"nricLast4"`
	DOB       string `json:"dob"` // DDMMYY format
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
	defer func() {
		if err := file.Close(); err != nil {
			http.Error(w, "Failed to close file", http.StatusInternalServerError)
		}
	}()

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
		_ = xlFile.Close() // Ignore close error during cleanup
	}()

	// Get first sheet
	sheetName := xlFile.GetSheetName(0)
	rows, err := xlFile.GetRows(sheetName)
	if err != nil {
		http.Error(w, "Failed to read Excel rows", http.StatusInternalServerError)
		return
	}

	if len(rows) < 2 {
		http.Error(w, "Excel file must have at least a header row and one data row", http.StatusBadRequest)
		return
	}

	// Skip header row
	rows = rows[1:]

	ctx := context.Background()
	var successCount int
	var failedCount int
	var errors []string
	var createdUsers []models.User

	// Process each row
	for i, row := range rows {
		rowNum := i + 2 // +2 because we skipped header and Excel is 1-indexed

		// Validate row has enough columns (Full Name, Rank, Battery, NRIC Last 4, DOB)
		if len(row) < 5 {
			errors = append(errors, fmt.Sprintf("Row %d: Insufficient columns (expected 5)", rowNum))
			failedCount++
			continue
		}

		uploadRow := BulkUploadRow{
			FullName:  strings.TrimSpace(row[0]),
			Rank:      strings.TrimSpace(row[1]),
			Battery:   strings.TrimSpace(row[2]),
			NRICLast4: strings.TrimSpace(row[3]),
			DOB:       strings.TrimSpace(row[4]),
		}

		// Validate required fields
		if uploadRow.FullName == "" || uploadRow.Rank == "" ||
			uploadRow.Battery == "" || uploadRow.NRICLast4 == "" || uploadRow.DOB == "" {
			errors = append(errors, fmt.Sprintf("Row %d: Missing required fields", rowNum))
			failedCount++
			continue
		}

		// Validate rank
		validRank := false
		for _, rank := range models.ValidRanks {
			if rank == uploadRow.Rank {
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

		// Validate DOB format (DDMMYY, 6 characters)
		if len(uploadRow.DOB) != 6 {
			errors = append(errors, fmt.Sprintf("Row %d: Invalid DOB format '%s' (must be DDMMYY)", rowNum, uploadRow.DOB))
			failedCount++
			continue
		}

		// Generate password: NRIC last 4 + DOB
		password := uploadRow.NRICLast4 + uploadRow.DOB
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			errors = append(errors, fmt.Sprintf("Row %d: Failed to hash password", rowNum))
			failedCount++
			continue
		}

		// Generate user ID
		userID := generateID()
		now := time.Now()

		// Start transaction
		tx, err := h.db.Pool.Begin(ctx)
		if err != nil {
			errors = append(errors, fmt.Sprintf("Row %d: Failed to start transaction", rowNum))
			failedCount++
			continue
		}

		// Create user with password
		_, err = tx.Exec(ctx, `
			INSERT INTO "user" (
				id, "full_name", rank, battery, "nric_last4", dob, password,
				"createdAt", "updatedAt"
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		`, userID, uploadRow.FullName, uploadRow.Rank,
			uploadRow.Battery, uploadRow.NRICLast4, uploadRow.DOB, string(hashedPassword), now, now)

		if err != nil {
			_ = tx.Rollback(ctx) // Rollback on error, ignore rollback error
			if strings.Contains(err.Error(), "unique constraint") || strings.Contains(err.Error(), "duplicate") {
				errors = append(errors, fmt.Sprintf("Row %d: User '%s' already exists", rowNum, uploadRow.FullName))
			} else {
				errors = append(errors, fmt.Sprintf("Row %d: Failed to create user: %v", rowNum, err))
			}
			failedCount++
			continue
		}

		// Commit transaction
		if err := tx.Commit(ctx); err != nil {
			errors = append(errors, fmt.Sprintf("Row %d: Failed to commit transaction", rowNum))
			failedCount++
			continue
		}

		// Success - add to created users
		fullName := uploadRow.FullName
		rank := uploadRow.Rank
		battery := uploadRow.Battery
		nricLast4 := uploadRow.NRICLast4
		dob := uploadRow.DOB

		createdUsers = append(createdUsers, models.User{
			ID:        userID,
			FullName:  &fullName,
			Rank:      &rank,
			Battery:   &battery,
			NRICLast4: &nricLast4,
			DOB:       &dob,
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
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}
