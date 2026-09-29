package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// CheckpointWAL runs a WAL checkpoint to flush WAL pages to the main DB file.
func CheckpointWAL(ctx context.Context, db *sql.DB) error {
	var busy, logSize, checkpointed int
	err := db.QueryRowContext(ctx, "PRAGMA wal_checkpoint(PASSIVE);").Scan(&busy, &logSize, &checkpointed)
	if err != nil {
		return fmt.Errorf("failed to checkpoint WAL: %w", err)
	}
	return nil
}

// RunIntegrityCheck checks SQLite database health using PRAGMA quick_check.
func RunIntegrityCheck(ctx context.Context, db *sql.DB) error {
	var result string
	err := db.QueryRowContext(ctx, "PRAGMA quick_check;").Scan(&result)
	if err != nil {
		return fmt.Errorf("quick_check query failed: %w", err)
	}
	if result != "ok" {
		// Run deep integrity check to gather details
		var deepResult string
		_ = db.QueryRowContext(ctx, "PRAGMA integrity_check(5);").Scan(&deepResult)
		return fmt.Errorf("database integrity issue detected: %s (details: %s)", result, deepResult)
	}
	return nil
}

// Optimize executes PRAGMA optimize to update SQLite query planner statistics.
func Optimize(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, "PRAGMA optimize;")
	if err != nil {
		return fmt.Errorf("failed to optimize sqlite: %w", err)
	}
	return nil
}

// StartMaintenanceWorker runs periodic background self-healing and maintenance routines for 24/7 reliability.
func StartMaintenanceWorker(ctx context.Context, db *sql.DB, interval time.Duration, logger *slog.Logger) {
	if interval < 1*time.Minute {
		interval = 15 * time.Minute
	}

	logger.Info("starting database 24/7 maintenance worker", slog.Duration("interval", interval))

	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				logger.Info("stopping database maintenance worker")
				return
			case <-ticker.C:
				runMaintenanceCycle(ctx, db, logger)
			}
		}
	}()
}

func runMaintenanceCycle(ctx context.Context, db *sql.DB, logger *slog.Logger) {
	cycleCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// 1. Connection ping
	if err := db.PingContext(cycleCtx); err != nil {
		logger.Error("database ping failed during maintenance", slog.String("error", err.Error()))
		return
	}

	// 2. Health & Integrity check
	if err := RunIntegrityCheck(cycleCtx, db); err != nil {
		logger.Error("CRITICAL: database integrity warning during maintenance", slog.String("error", err.Error()))
	}

	// 3. Flush WAL log (keeps db file compact and prevents unbounded WAL growth)
	if err := CheckpointWAL(cycleCtx, db); err != nil {
		logger.Warn("wal checkpoint failed", slog.String("error", err.Error()))
	}

	// 4. Optimize query planner stats
	if err := Optimize(cycleCtx, db); err != nil {
		logger.Warn("database optimize pragma failed", slog.String("error", err.Error()))
	}

	logger.Debug("database maintenance cycle completed successfully")
}

// EnsureHealthy verifies connection and integrity, returning a structured error if unhealthy.
func EnsureHealthy(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return errors.New("database instance is nil")
	}
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("database unreachable: %w", err)
	}
	if err := RunIntegrityCheck(ctx, db); err != nil {
		return fmt.Errorf("database corrupted or degraded: %w", err)
	}
	return nil
}
