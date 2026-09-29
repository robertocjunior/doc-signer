package handlers

import (
	"html/template"
	"net/http"
	"net/url"
	"strings"

	"doc-signer/internal/auth"
	"doc-signer/internal/database"
	"doc-signer/internal/models"
)

type AdminHandler struct {
	repo    database.Repository
	authSvc auth.Service
	tmpl    *template.Template
}

func NewAdminHandler(repo database.Repository, authSvc auth.Service, tmpl *template.Template) *AdminHandler {
	return &AdminHandler{
		repo:    repo,
		authSvc: authSvc,
		tmpl:    tmpl,
	}
}

func (h *AdminHandler) checkAdminAuth(w http.ResponseWriter, r *http.Request) (string, bool) {
	email := getCookieValue(r, "user_email")
	if email == "" {
		http.Redirect(w, r, "/auth/login?return="+url.QueryEscape(r.RequestURI), http.StatusSeeOther)
		return "", false
	}

	userEmail := strings.ToLower(email)
	if !h.authSvc.IsAdmin(userEmail) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("Acesso restrito: Seu e-mail (" + userEmail + ") não está autorizado."))
		return "", false
	}

	return userEmail, true
}

func extractFilterCriteria(r *http.Request) models.FilterCriteria {
	return models.FilterCriteria{
		Document: strings.TrimSpace(r.URL.Query().Get("doc")),
		User:     strings.TrimSpace(r.URL.Query().Get("user")),
		Dept:     strings.TrimSpace(r.URL.Query().Get("dept")),
		Job:      strings.TrimSpace(r.URL.Query().Get("job")),
		Hash:     strings.TrimSpace(r.URL.Query().Get("hash")),
		IP:       strings.TrimSpace(r.URL.Query().Get("ip")),
	}
}

func (h *AdminHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	userEmail, ok := h.checkAdminAuth(w, r)
	if !ok {
		return
	}

	filter := extractFilterCriteria(r)
	records, err := h.repo.List(r.Context(), filter)
	if err != nil {
		http.Error(w, "Erro ao buscar registros: "+err.Error(), http.StatusInternalServerError)
		return
	}

	q := r.URL.Query()
	q.Del("format")

	data := map[string]interface{}{
		"UserEmail":   userEmail,
		"Records":     records,
		"FilterDoc":   filter.Document,
		"FilterUser":  filter.User,
		"FilterDept":  filter.Dept,
		"FilterJob":   filter.Job,
		"FilterHash":  filter.Hash,
		"FilterIP":    filter.IP,
		"QueryString": q.Encode(),
		"Count":       len(records),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.tmpl.Execute(w, data)
}
