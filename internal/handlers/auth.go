package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"doc-signer/internal/auth"
)

type AuthHandler struct {
	authSvc auth.Service
	logger  *slog.Logger
}

func NewAuthHandler(authSvc auth.Service, logger *slog.Logger) *AuthHandler {
	return &AuthHandler{
		authSvc: authSvc,
		logger:  logger,
	}
}

func (h *AuthHandler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	doc := extractDocParam(r)
	returnURL := r.URL.Query().Get("return")
	if returnURL == "" {
		returnURL = "/assinar?doc=" + doc
	}

	state := fmt.Sprintf("%s|%s", doc, returnURL)
	redirectURL := h.authSvc.GetAuthURL(state)
	http.Redirect(w, r, redirectURL, http.StatusTemporaryRedirect)
}

func (h *AuthHandler) HandleCallback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")

	if code == "" {
		http.Error(w, "Código de autorização não recebido da Microsoft", http.StatusBadRequest)
		return
	}

	token, err := h.authSvc.Exchange(r.Context(), code)
	if err != nil {
		h.logger.Error("failed to exchange oauth code", slog.String("error", err.Error()))
		http.Error(w, "Falha na troca de token com o Azure: "+err.Error(), http.StatusInternalServerError)
		return
	}

	profile, err := h.authSvc.GetProfile(r.Context(), token)
	if err != nil {
		h.logger.Error("failed to fetch user profile from Microsoft Graph", slog.String("error", err.Error()))
		http.Error(w, "Falha ao consultar perfil no Microsoft Graph: "+err.Error(), http.StatusInternalServerError)
		return
	}

	effectiveEmail := strings.ToLower(strings.TrimSpace(profile.EffectiveEmail()))

	setSessionCookie(w, "user_email", effectiveEmail)
	setSessionCookie(w, "user_name", profile.DisplayName)
	setSessionCookie(w, "user_department", profile.Department)
	setSessionCookie(w, "user_job", profile.JobTitle)
	setSessionCookie(w, "user_location", profile.OfficeLocation)

	h.logger.Info("user authenticated successfully via Microsoft Entra ID",
		slog.String("email", effectiveEmail),
		slog.String("name", profile.DisplayName),
	)

	parts := strings.Split(state, "|")
	returnURL := "/assinar"
	if len(parts) >= 2 && parts[1] != "" {
		returnURL = parts[1]
	}

	http.Redirect(w, r, returnURL, http.StatusSeeOther)
}
