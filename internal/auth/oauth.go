package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"doc-signer/internal/config"
	"doc-signer/internal/models"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/microsoft"
)

// Service defines authentication and authorization operations.
type Service interface {
	GetAuthURL(state string) string
	Exchange(ctx context.Context, code string) (*oauth2.Token, error)
	GetProfile(ctx context.Context, token *oauth2.Token) (*models.UserProfile, error)
	IsAdmin(email string) bool
}

type authService struct {
	oauthConfig  *oauth2.Config
	graphUserURL string
	adminEmails  map[string]bool
}

// NewService creates an authentication service instance.
func NewService(cfg config.AuthConfig) Service {
	endpoint := microsoft.AzureADEndpoint(cfg.TenantID)

	oauthCfg := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Scopes:       []string{"openid", "profile", "email", "User.Read"},
		Endpoint:     endpoint,
	}

	return &authService{
		oauthConfig:  oauthCfg,
		graphUserURL: cfg.GraphUserURL,
		adminEmails:  cfg.AdminEmails,
	}
}

func (s *authService) GetAuthURL(state string) string {
	return s.oauthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline)
}

func (s *authService) Exchange(ctx context.Context, code string) (*oauth2.Token, error) {
	if strings.TrimSpace(code) == "" {
		return nil, errors.New("authorization code cannot be empty")
	}
	token, err := s.oauthConfig.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange authorization code: %w", err)
	}
	return token, nil
}

func (s *authService) GetProfile(ctx context.Context, token *oauth2.Token) (*models.UserProfile, error) {
	if token == nil {
		return nil, errors.New("oauth token cannot be nil")
	}

	client := s.oauthConfig.Client(ctx, token)
	resp, err := client.Get(s.graphUserURL)
	if err != nil {
		return nil, fmt.Errorf("failed to query Microsoft Graph API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("microsoft Graph returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var profile models.UserProfile
	if err := json.Unmarshal(body, &profile); err != nil {
		return nil, fmt.Errorf("failed to decode profile json: %w", err)
	}

	return &profile, nil
}

func (s *authService) IsAdmin(email string) bool {
	clean := strings.ToLower(strings.TrimSpace(email))
	if clean == "" {
		return false
	}
	return s.adminEmails[clean]
}
