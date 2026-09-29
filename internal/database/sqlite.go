package database

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"doc-signer/internal/config"

	_ "github.com/mattn/go-sqlite3"
)

// Open initializes and configures a SQLite connection with production-grade settings for 24/7 reliability.
func Open(cfg config.DatabaseConfig, logger *slog.Logger) (*sql.DB, error) {
	// Ensure parent directory exists for file-based SQLite databases
	if cfg.Path != ":memory:" && !strings.HasPrefix(cfg.Path, "file:") {
		dir := filepath.Dir(cfg.Path)
		if dir != "." && dir != "" {
			if err := os.MkdirAll(dir, 0750); err != nil {
				return nil, fmt.Errorf("failed to create database directory %s: %w", dir, err)
			}
		}
	}

	// Build connection string with pragmas
	dsn := cfg.Path
	params := url.Values{}
	params.Set("_journal_mode", "WAL")
	params.Set("_busy_timeout", fmt.Sprintf("%d", cfg.BusyTimeoutMs))
	params.Set("_synchronous", "NORMAL")
	params.Set("_foreign_keys", "ON")
	params.Set("_cache_size", "-20000") // 20MB in-memory cache

	separator := "?"
	if strings.Contains(dsn, "?") {
		separator = "&"
	}
	dsn = fmt.Sprintf("%s%s%s", dsn, separator, params.Encode())

	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	// Concurrency tuning for SQLite WAL mode
	// SQLite supports multiple readers concurrently. Setting MaxOpenConns allows concurrent reads
	// while busy_timeout prevents lock contention during writes.
	maxOpen := cfg.MaxOpenConns
	if maxOpen <= 0 {
		maxOpen = 10
	}
	maxIdle := cfg.MaxIdleConns
	if maxIdle <= 0 {
		maxIdle = 5
	}

	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	// Verify connectivity
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to ping sqlite database: %w", err)
	}

	logger.Info("connected to sqlite database",
		slog.String("path", cfg.Path),
		slog.Int("max_open_conns", maxOpen),
		slog.Int("max_idle_conns", maxIdle),
	)

	return db, nil
}

// Migrate executes schema migrations idempotently.
func Migrate(ctx context.Context, db *sql.DB, logger *slog.Logger) error {
	migrationQuery := `
	CREATE TABLE IF NOT EXISTS signatures (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		hash_id TEXT UNIQUE NOT NULL,
		document_name TEXT NOT NULL,
		user_name TEXT NOT NULL,
		user_email TEXT NOT NULL,
		department TEXT,
		job_title TEXT,
		office_location TEXT,
		signed_at DATETIME NOT NULL,
		user_agent TEXT,
		ip_address TEXT
	);

	CREATE INDEX IF NOT EXISTS idx_signatures_doc_email ON signatures(document_name, user_email);
	CREATE INDEX IF NOT EXISTS idx_signatures_hash ON signatures(hash_id);
	CREATE INDEX IF NOT EXISTS idx_signatures_signed_at ON signatures(signed_at DESC);
	`

	if _, err := db.ExecContext(ctx, migrationQuery); err != nil {
		return fmt.Errorf("failed to run database migrations: %w", err)
	}

	logger.Info("database migrations executed successfully")
	return nil
}
