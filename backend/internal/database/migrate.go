package database

import (
	"database/sql"
	"fmt"
	"log"
	"path/filepath"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// RunMigrations runs database migrations using Goose
func RunMigrations(databaseURL, migrationsDir string) error {
	if databaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}

	if migrationsDir == "" {
		migrationsDir = "./migrations"
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("Failed to close database: %v", err)
		}
	}()

	if err := db.Ping(); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("failed to set dialect: %w", err)
	}

	// Resolve absolute path for migrations directory
	absPath, err := filepath.Abs(migrationsDir)
	if err != nil {
		return fmt.Errorf("failed to resolve migrations path: %w", err)
	}

	log.Printf("Running database migrations from %s...", absPath)

	if err := goose.Up(db, absPath); err != nil {
		log.Printf("Migration error: %v", err)
		return fmt.Errorf("migration failed: %w", err)
	}

	log.Println("Database migrations completed successfully")
	return nil
}
