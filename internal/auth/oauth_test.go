package auth

import (
	"strings"
	"testing"

	"doc-signer/internal/config"
)

func TestAuthService(t *testing.T) {
	cfg := config.AuthConfig{
		TenantID:     "common",
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		RedirectURL:  "http://localhost:8080/auth/callback",
		AdminEmails: map[string]bool{
			"admin@empresa.com": true,
		},
	}

	service := NewService(cfg)

	// Test GetAuthURL
	url := service.GetAuthURL("doc123|/assinar?doc=doc123")
	if !strings.Contains(url, "client_id=test-client-id") {
		t.Errorf("expected client_id in auth url, got: %s", url)
	}
	if !strings.Contains(url, "state=doc123") {
		t.Errorf("expected state in auth url, got: %s", url)
	}

	// Test IsAdmin
	if !service.IsAdmin("admin@empresa.com") {
		t.Errorf("expected admin@empresa.com to be admin")
	}
	if !service.IsAdmin("ADMIN@EMPRESA.COM") {
		t.Errorf("expected case insensitive admin match")
	}
	if service.IsAdmin("user@empresa.com") {
		t.Errorf("expected user@empresa.com to not be admin")
	}
}
