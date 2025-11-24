package database

import (
	"context"
	"fmt"
	"log"

	"golang.org/x/crypto/bcrypt"
)

// SeedAdminUser creates or updates the admin user with proper password hash
func SeedAdminUser(db *DB) error {
	ctx := context.Background()

	adminID := "00000000000000000000000000000000"
	adminFullName := "admin"
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte("236SAadmin!"), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash admin password: %w", err)
	}

	// Check if admin user already exists
	var existingID string
	err = db.Pool.QueryRow(ctx, `SELECT id FROM "user" WHERE id = $1`, adminID).Scan(&existingID)
	if err == nil {
		// Admin exists, update password hash
		log.Println("Admin user exists, updating password hash...")
		_, err = db.Pool.Exec(ctx, `
			UPDATE "user" 
			SET password = $1, "is_superadmin" = true, "updatedAt" = NOW()
			WHERE id = $2
		`, string(hashedPassword), adminID)
		if err != nil {
			return fmt.Errorf("failed to update admin password: %w", err)
		}
		log.Println("Admin password hash updated successfully")
		return nil
	}

	// Admin doesn't exist, create it
	log.Println("Creating admin user...")
	_, err = db.Pool.Exec(ctx, `
		INSERT INTO "user" (
			id, "full_name", "is_superadmin", password, "createdAt", "updatedAt"
		)
		VALUES ($1, $2, $3, $4, NOW(), NOW())
	`, adminID, adminFullName, true, string(hashedPassword))
	if err != nil {
		return fmt.Errorf("failed to create admin user: %w", err)
	}

	log.Println("Admin user seeded successfully")
	return nil
}
