package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"doc-signer/internal/service"
)

type SignHandler struct {
	signerSvc service.SignerService
	logger    *slog.Logger
}

func NewSignHandler(signerSvc service.SignerService, logger *slog.Logger) *SignHandler {
	return &SignHandler{
		signerSvc: signerSvc,
		logger:    logger,
	}
}

func (h *SignHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		return
	}

	doc := strings.TrimSpace(r.FormValue("doc"))
	if doc == "" {
		http.Error(w, "Documento não informado", http.StatusBadRequest)
		return
	}

	userEmail := getCookieValue(r, "user_email")
	if userEmail == "" {
		http.Redirect(w, r, fmt.Sprintf("/auth/login?doc=%s&return=/assinar%%3Fdoc%%3D%s", doc, doc), http.StatusSeeOther)
		return
	}

	req := service.SignRequest{
		Doc:            doc,
		UserEmail:      userEmail,
		UserName:       getCookieValue(r, "user_name"),
		Department:     getCookieValue(r, "user_department"),
		JobTitle:       getCookieValue(r, "user_job"),
		OfficeLocation: getCookieValue(r, "user_location"),
		IP:             extractClientIP(r),
		UserAgent:      r.UserAgent(),
	}

	_, err := h.signerSvc.SignDocument(r.Context(), req)
	if err != nil {
		h.logger.Error("error registering signature",
			slog.String("doc", doc),
			slog.String("email", userEmail),
			slog.String("error", err.Error()),
		)
		http.Error(w, "Erro ao registrar confirmação: "+err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/assinar?doc="+doc, http.StatusSeeOther)
}
