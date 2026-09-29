package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"doc-signer/internal/models"
)

// Repository defines data access operations for signatures.
type Repository interface {
	Create(ctx context.Context, rec *models.SignatureRecord) error
	GetByDocAndEmail(ctx context.Context, doc, email string) (*models.SignatureRecord, error)
	GetByHash(ctx context.Context, hash string) (*models.SignatureRecord, error)
	List(ctx context.Context, filter models.FilterCriteria) ([]models.SignatureRecord, error)
	Count(ctx context.Context, filter models.FilterCriteria) (int, error)
	Ping(ctx context.Context) error
}

type sqliteRepository struct {
	db *sql.DB
}

// NewRepository creates a new SQLite-backed repository.
func NewRepository(db *sql.DB) Repository {
	return &sqliteRepository{db: db}
}

func (r *sqliteRepository) Ping(ctx context.Context) error {
	return r.db.PingContext(ctx)
}

func (r *sqliteRepository) Create(ctx context.Context, rec *models.SignatureRecord) error {
	query := `
	INSERT INTO signatures (
		hash_id, document_name, user_name, user_email, department, job_title, office_location, signed_at, user_agent, ip_address
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	res, err := r.db.ExecContext(
		ctx,
		query,
		rec.HashID,
		rec.DocumentName,
		rec.UserName,
		strings.ToLower(strings.TrimSpace(rec.UserEmail)),
		rec.Department,
		rec.JobTitle,
		rec.OfficeLocation,
		rec.SignedAt,
		rec.UserAgent,
		rec.IPAddress,
	)
	if err != nil {
		return fmt.Errorf("failed to insert signature record: %w", err)
	}

	id, err := res.LastInsertId()
	if err == nil {
		rec.ID = id
	}
	return nil
}

func (r *sqliteRepository) GetByDocAndEmail(ctx context.Context, doc, email string) (*models.SignatureRecord, error) {
	query := `
	SELECT id, hash_id, document_name, user_name, user_email,
	       COALESCE(department, ''), COALESCE(job_title, ''), COALESCE(office_location, ''),
	       signed_at, user_agent, ip_address
	FROM signatures
	WHERE document_name = ? AND user_email = ?
	LIMIT 1
	`
	row := r.db.QueryRowContext(ctx, query, doc, strings.ToLower(strings.TrimSpace(email)))

	var rec models.SignatureRecord
	err := row.Scan(
		&rec.ID,
		&rec.HashID,
		&rec.DocumentName,
		&rec.UserName,
		&rec.UserEmail,
		&rec.Department,
		&rec.JobTitle,
		&rec.OfficeLocation,
		&rec.SignedAt,
		&rec.UserAgent,
		&rec.IPAddress,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to fetch signature: %w", err)
	}

	return &rec, nil
}

func (r *sqliteRepository) GetByHash(ctx context.Context, hash string) (*models.SignatureRecord, error) {
	query := `
	SELECT id, hash_id, document_name, user_name, user_email,
	       COALESCE(department, ''), COALESCE(job_title, ''), COALESCE(office_location, ''),
	       signed_at, user_agent, ip_address
	FROM signatures
	WHERE hash_id = ?
	LIMIT 1
	`
	row := r.db.QueryRowContext(ctx, query, strings.TrimSpace(hash))

	var rec models.SignatureRecord
	err := row.Scan(
		&rec.ID,
		&rec.HashID,
		&rec.DocumentName,
		&rec.UserName,
		&rec.UserEmail,
		&rec.Department,
		&rec.JobTitle,
		&rec.OfficeLocation,
		&rec.SignedAt,
		&rec.UserAgent,
		&rec.IPAddress,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to fetch signature by hash: %w", err)
	}

	return &rec, nil
}

func (r *sqliteRepository) buildFilterQuery(baseSQL string, filter models.FilterCriteria) (string, []interface{}) {
	var sb strings.Builder
	sb.WriteString(baseSQL)
	sb.WriteString(" WHERE 1=1")

	var args []interface{}

	if doc := strings.TrimSpace(filter.Document); doc != "" {
		sb.WriteString(" AND document_name LIKE ?")
		args = append(args, "%"+doc+"%")
	}
	if user := strings.TrimSpace(filter.User); user != "" {
		sb.WriteString(" AND (user_email LIKE ? OR user_name LIKE ?)")
		args = append(args, "%"+user+"%", "%"+user+"%")
	}
	if dept := strings.TrimSpace(filter.Dept); dept != "" {
		sb.WriteString(" AND department LIKE ?")
		args = append(args, "%"+dept+"%")
	}
	if job := strings.TrimSpace(filter.Job); job != "" {
		sb.WriteString(" AND job_title LIKE ?")
		args = append(args, "%"+job+"%")
	}
	if hash := strings.TrimSpace(filter.Hash); hash != "" {
		sb.WriteString(" AND hash_id LIKE ?")
		args = append(args, "%"+hash+"%")
	}
	if ip := strings.TrimSpace(filter.IP); ip != "" {
		sb.WriteString(" AND ip_address LIKE ?")
		args = append(args, "%"+ip+"%")
	}

	return sb.String(), args
}

func (r *sqliteRepository) List(ctx context.Context, filter models.FilterCriteria) ([]models.SignatureRecord, error) {
	baseQuery := `
	SELECT id, hash_id, document_name, user_name, user_email,
	       COALESCE(department, ''), COALESCE(job_title, ''), COALESCE(office_location, ''),
	       signed_at, user_agent, ip_address
	FROM signatures
	`
	query, args := r.buildFilterQuery(baseQuery, filter)
	query += " ORDER BY signed_at DESC"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list signatures: %w", err)
	}
	defer rows.Close()

	records := make([]models.SignatureRecord, 0)
	for rows.Next() {
		var rec models.SignatureRecord
		if err := rows.Scan(
			&rec.ID,
			&rec.HashID,
			&rec.DocumentName,
			&rec.UserName,
			&rec.UserEmail,
			&rec.Department,
			&rec.JobTitle,
			&rec.OfficeLocation,
			&rec.SignedAt,
			&rec.UserAgent,
			&rec.IPAddress,
		); err != nil {
			return nil, fmt.Errorf("failed to scan signature row: %w", err)
		}
		records = append(records, rec)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during row iteration: %w", err)
	}

	return records, nil
}

func (r *sqliteRepository) Count(ctx context.Context, filter models.FilterCriteria) (int, error) {
	baseQuery := `SELECT COUNT(*) FROM signatures`
	query, args := r.buildFilterQuery(baseQuery, filter)

	var count int
	err := r.db.QueryRowContext(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count signatures: %w", err)
	}
	return count, nil
}
