package main

import (
	"database/sql"
	_ "embed"
	"fmt"
	"log"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

var db *sql.DB

func initDB(dbPath string) error {
	// Ensure parent directory exists.
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create db directory: %w", err)
	}

	var err error
	db, err = sql.Open("sqlite", dbPath+"?_pragma=journal_mode(wal)&_pragma=foreign_keys(on)&_pragma=busy_timeout(5000)")
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}

	// Execute schema.
	if _, err := db.Exec(schemaSQL); err != nil {
		return fmt.Errorf("execute schema: %w", err)
	}

	// Migrate: add columns that may not exist in older databases.
	migrations := []string{
		`ALTER TABLE schedule_tasks ADD COLUMN all_sort INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE schedule_tasks ADD COLUMN all_sort_completed INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE schedule_tasks ADD COLUMN all_sort_time INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE schedule_tasks ADD COLUMN recurrence_id TEXT NOT NULL DEFAULT ''`,
	}
	for _, m := range migrations {
		db.Exec(m) // Ignore "duplicate column" errors for already-migrated DBs.
	}

	log.Printf("Database initialized at %s", dbPath)
	return nil
}

func createUser(email, passwordHash, username string) error {
	uid := nextID()
	_, err := db.Exec(
		`INSERT INTO users (id, email, password_hash, username) VALUES (?, ?, ?, ?)
		 ON CONFLICT(email) DO UPDATE SET password_hash=excluded.password_hash, username=excluded.username, updated_at=CURRENT_TIMESTAMP`,
		uid, email, passwordHash, username,
	)
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	log.Printf("User created/updated: %s (id=%d)", email, uid)
	return nil
}

func lookupUserByEmail(email string) (int64, string, string, error) {
	var id int64
	var hash, username string
	err := db.QueryRow(`SELECT id, password_hash, username FROM users WHERE email = ?`, email).Scan(&id, &hash, &username)
	if err != nil {
		return 0, "", "", err
	}
	return id, hash, username, nil
}

func lookupUserByID(userID int64) (string, string, string, error) {
	var email, hash, username string
	err := db.QueryRow(`SELECT email, password_hash, username FROM users WHERE id = ?`, userID).Scan(&email, &hash, &username)
	if err != nil {
		return "", "", "", err
	}
	return email, hash, username, nil
}
