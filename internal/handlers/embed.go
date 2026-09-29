package handlers

import (
	"fmt"
	"html/template"
	"net/http"

	"doc-signer/internal/models"
	"doc-signer/internal/service"
)

type EmbedHandler struct {
	signerSvc service.SignerService
	tmpl      *template.Template
}

func NewEmbedHandler(signerSvc service.SignerService, tmpl *template.Template) *EmbedHandler {
	return &EmbedHandler{
		signerSvc: signerSvc,
		tmpl:      tmpl,
	}
}

func (h *EmbedHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	doc := extractDocParam(r)
	if doc == "" {
		http.Error(w, "Identificador de documento 'doc' não informado.", http.StatusBadRequest)
		return
	}

	userEmail := getCookieValue(r, "user_email")
	userName := getCookieValue(r, "user_name")
	userDept := getCookieValue(r, "user_department")
	isLogged := userEmail != ""

	var signedRecord *models.SignatureRecord
	if isLogged {
		var err error
		signedRecord, err = h.signerSvc.GetSignature(r.Context(), doc, userEmail)
		if err != nil {
			// Non-fatal, just log and render as unsigned
			signedRecord = nil
		}
	}

	data := map[string]interface{}{
		"Doc":            doc,
		"IsLogged":       isLogged,
		"UserEmail":      userEmail,
		"UserName":       userName,
		"UserDepartment": userDept,
		"Record":         signedRecord,
		"IsSigned":       signedRecord != nil,
		"LoginURL":       fmt.Sprintf("/auth/login?doc=%s&return=/assinar%%3Fdoc%%3D%s", doc, doc),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.tmpl.Execute(w, data)
}
