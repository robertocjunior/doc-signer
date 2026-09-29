package database

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"doc-signer/internal/config"
	"doc-signer/internal/models"
)

func setupTestDB(t *testing.T) Repository {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := config.DatabaseConfig{
		Path:          ":memory:",
		BusyTimeoutMs: 5000,
		MaxOpenConns:  1,
		MaxIdleConns:  1,
	}

	db, err := Open(cfg, logger)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	ctx := context.Background()
	if err := Migrate(ctx, db, logger); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	if err := RunIntegrityCheck(ctx, db); err != nil {
		t.Fatalf("integrity check failed on new db: %v", err)
	}

	return NewRepository(db)
}

func TestRepositoryCreateAndGet(t *testing.T) {
	repo := setupTestDB(t)
	ctx := context.Background()

	now := time.Now().Truncate(time.Second)
	rec := &models.SignatureRecord{
		HashID:         "test-hash-12345",
		DocumentName:   "Politica-Seguranca.pdf",
		UserName:       "Roberto Junior",
		UserEmail:      "roberto@empresa.com",
		Department:     "TI",
		JobTitle:       "Desenvolvedor",
		OfficeLocation: "Matriz",
		SignedAt:       now,
		UserAgent:      "Mozilla/5.0",
		IPAddress:      "192.168.1.100",
	}

	if err := repo.Create(ctx, rec); err != nil {
		t.Fatalf("failed to create signature: %v", err)
	}
	if rec.ID == 0 {
		t.Fatalf("expected non-zero ID after creation")
	}

	// Fetch by doc and email
	found, err := repo.GetByDocAndEmail(ctx, "Politica-Seguranca.pdf", "roberto@empresa.com")
	if err != nil {
		t.Fatalf("failed to get signature: %v", err)
	}
	if found == nil {
		t.Fatalf("expected to find signature, got nil")
	}
	if found.HashID != "test-hash-12345" {
		t.Errorf("expected hash test-hash-12345, got %s", found.HashID)
	}
	if found.UserName != "Roberto Junior" {
		t.Errorf("expected user name Roberto Junior, got %s", found.UserName)
	}

	// Fetch non-existing
	missing, err := repo.GetByDocAndEmail(ctx, "Outro.pdf", "roberto@empresa.com")
	if err != nil {
		t.Fatalf("unexpected error fetching missing record: %v", err)
	}
	if missing != nil {
		t.Errorf("expected nil for missing signature, got %+v", missing)
	}

	// Fetch by hash
	byHash, err := repo.GetByHash(ctx, "test-hash-12345")
	if err != nil {
		t.Fatalf("failed to get signature by hash: %v", err)
	}
	if byHash == nil || byHash.UserEmail != "roberto@empresa.com" {
		t.Errorf("expected byHash to match email, got %+v", byHash)
	}
}

func TestRepositoryListAndCount(t *testing.T) {
	repo := setupTestDB(t)
	ctx := context.Background()

	_ = repo.Create(ctx, &models.SignatureRecord{
		HashID:       "hash-1",
		DocumentName: "DocA.pdf",
		UserName:     "Alice Smith",
		UserEmail:    "alice@empresa.com",
		Department:   "RH",
		SignedAt:     time.Now(),
	})

	_ = repo.Create(ctx, &models.SignatureRecord{
		HashID:       "hash-2",
		DocumentName: "DocB.pdf",
		UserName:     "Bob Jones",
		UserEmail:    "bob@empresa.com",
		Department:   "Financeiro",
		SignedAt:     time.Now(),
	})

	// Total count
	count, err := repo.Count(ctx, models.FilterCriteria{})
	if err != nil {
		t.Fatalf("failed to count: %v", err)
	}
	if count != 2 {
		t.Errorf("expected count 2, got %d", count)
	}

	// Filter by document
	docs, err := repo.List(ctx, models.FilterCriteria{Document: "DocA"})
	if err != nil {
		t.Fatalf("failed to list by doc: %v", err)
	}
	if len(docs) != 1 || docs[0].UserName != "Alice Smith" {
		t.Errorf("unexpected list result: %+v", docs)
	}

	// Filter by dept
	deptList, err := repo.List(ctx, models.FilterCriteria{Dept: "Financeiro"})
	if err != nil {
		t.Fatalf("failed to list by dept: %v", err)
	}
	if len(deptList) != 1 || deptList[0].UserName != "Bob Jones" {
		t.Errorf("unexpected dept list result: %+v", deptList)
	}
}
