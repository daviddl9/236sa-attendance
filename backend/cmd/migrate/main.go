package main

import (
	"context"
	"database/sql"
	"flag"
	"log"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/services/deeplink"
)

func main() {
	flag.Parse()
	args := flag.Args()

	if len(args) < 1 {
		flag.Usage()
		os.Exit(1)
	}

	command := args[0]

	dbString := os.Getenv("DATABASE_URL")
	if dbString == "" {
		log.Fatal("DATABASE_URL environment variable is required")
	}

	db, err := sql.Open("pgx", dbString)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("Failed to close database: %v", err)
		}
	}()

	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}

	dir := "./migrations"
	if len(args) > 1 {
		dir = args[1]
	}

	if err := goose.SetDialect("postgres"); err != nil {
		log.Fatalf("Failed to set dialect: %v", err)
	}

	ctx := context.Background()
	if err := goose.RunContext(ctx, command, db, dir, args[2:]...); err != nil {
		log.Fatalf("Migration failed: %v", err)
	}

	if err := deeplink.BackfillActiveSessionCodes(ctx, db); err != nil {
		log.Fatalf("Deep-link backfill failed: %v", err)
	}
}
