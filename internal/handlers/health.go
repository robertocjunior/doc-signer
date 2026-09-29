package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"doc-signer/internal/database"
	"doc-signer/internal/models"
)

type HealthHandler struct {
	db        *sql.DB
	startTime time.Time
}

func NewHealthHandler(db *sql.DB, startTime time.Time) *HealthHandler {
	return &HealthHandler{
		db:        db,
		startTime: startTime,
	}
}

func (h *HealthHandler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	checks := make(map[string]string)
	isHealthy := true

	// Check database connection
	if err := h.db.PingContext(ctx); err != nil {
		checks["database"] = "failed: " + err.Error()
		isHealthy = false
	} else {
		checks["database"] = "ok"
	}

	// Check database integrity
	if err := database.RunIntegrityCheck(ctx, h.db); err != nil {
		checks["integrity"] = "warning: " + err.Error()
		// If integrity fails, still report status
		isHealthy = false
	} else {
		checks["integrity"] = "ok"
	}

	status := "healthy"
	httpStatus := http.StatusOK
	if !isHealthy {
		status = "degraded"
		httpStatus = http.StatusServiceUnavailable
	}

	resp := models.HealthResponse{
		Status:    status,
		Uptime:    time.Since(h.startTime).Truncate(time.Second).String(),
		Timestamp: time.Now().UTC(),
		Checks:    checks,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *HealthHandler) HandleLive(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"alive"}`))
}
