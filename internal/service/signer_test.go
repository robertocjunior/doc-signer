package service

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"doc-signer/internal/config"
	"doc-signer/internal/database"
)

type dummyEmailSender struct {
	sentCount int
}

func (d *dummyEmailSender) SendReceipt(ctx context.Context, toEmail, toName, docName, hashID, dateStr string) error {
	d.sentCount++
	return nil
}

func TestSignerServiceHashGeneration(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewSignerService(nil, nil, logger)

	fixedTime := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	hash1 := svc.GenerateHash("docA", "user@empresa.com", fixedTime, "127.0.0.1", "curl")
	hash2 := svc.GenerateHash("docA", "user@empresa.com", fixedTime, "127.0.0.1", "curl")
	hash3 := svc.GenerateHash("docB", "user@empresa.com", fixedTime, "127.0.0.1", "curl")

	if hash1 != hash2 {
		t.Errorf("expected deterministic hash output, got %s and %s", hash1, hash2)
	}
	if hash1 == hash3 {
		t.Errorf("expected different hash for different doc, got same: %s", hash1)
	}
	if len(hash1) != 64 {
		t.Errorf("expected 64 character hex SHA-256 string, got %d", len(hash1))
	}
}

func TestSignerServiceSignAndDuplicate(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := database.Open(config.DatabaseConfig{Path: ":memory:"}, logger)
	if err != nil {
		t.Fatalf("failed to open memory db: %v", err)
	}
	ctx := context.Background()
	_ = database.Migrate(ctx, db, logger)
	repo := database.NewRepository(db)

	emailSender := &dummyEmailSender{}
	svc := NewSignerService(repo, emailSender, logger)

	req := SignRequest{
		Doc:            "Termo-Uso.pdf",
		UserEmail:      "maria@empresa.com",
		UserName:       "Maria Silva",
		Department:     "RH",
		JobTitle:       "Analista",
		OfficeLocation: "Filial",
		IP:             "10.0.0.1",
		UserAgent:      "Mozilla",
	}

	// 1. Initial sign
	record, err := svc.SignDocument(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error signing document: %v", err)
	}
	if record == nil || record.ID == 0 {
		t.Fatalf("expected valid record with non-zero ID")
	}

	// 2. Duplicate sign attempt: must be idempotent and return existing record
	duplicateRecord, err := svc.SignDocument(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error on duplicate sign: %v", err)
	}
	if duplicateRecord.ID != record.ID {
		t.Errorf("expected duplicate sign to return existing record ID %d, got %d", record.ID, duplicateRecord.ID)
	}
	if duplicateRecord.HashID != record.HashID {
		t.Errorf("expected matching hash ID for duplicate sign")
	}
}
