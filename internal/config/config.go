package config

import (
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config aggregates all application configuration parameters.
type Config struct {
	Server      ServerConfig
	Database    DatabaseConfig
	Auth        AuthConfig
	SMTP        SMTPConfig
	Logging     LoggingConfig
	Maintenance MaintenanceConfig
}

type ServerConfig struct {
	Port            string
	BaseURL         string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
}

type DatabaseConfig struct {
	Path            string
	BusyTimeoutMs   int
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

type AuthConfig struct {
	TenantID     string
	ClientID     string
	ClientSecret string
	RedirectURL  string
	GraphUserURL string
	AdminEmails  map[string]bool
}

type SMTPConfig struct {
	Host       string
	Port       int
	Username   string
	Password   string
	FromEmail  string
	ReplyEmail string
	MaxRetries int
	RetryDelay time.Duration
}

type LoggingConfig struct {
	Level  slog.Level
	Format string // "json" or "text"
}

type MaintenanceConfig struct {
	Enabled  bool
	Interval time.Duration
}

// Load loads configuration from environment variables with sensible defaults.
func Load() *Config {
	baseURL := strings.TrimRight(getEnv("BASE_URL", "http://localhost:8080"), "/")

	cfg := &Config{
		Server: ServerConfig{
			Port:            getEnv("PORT", "8080"),
			BaseURL:         baseURL,
			ReadTimeout:     getEnvDuration("HTTP_READ_TIMEOUT", 15*time.Second),
			WriteTimeout:    getEnvDuration("HTTP_WRITE_TIMEOUT", 30*time.Second),
			IdleTimeout:     getEnvDuration("HTTP_IDLE_TIMEOUT", 60*time.Second),
			ShutdownTimeout: getEnvDuration("SHUTDOWN_TIMEOUT", 10*time.Second),
		},
		Database: DatabaseConfig{
			Path:            getEnv("DB_PATH", "/app/data/signatures.db"),
			BusyTimeoutMs:   getEnvAsInt("DB_BUSY_TIMEOUT_MS", 5000),
			MaxOpenConns:    getEnvAsInt("DB_MAX_OPEN_CONNS", 10),
			MaxIdleConns:    getEnvAsInt("DB_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime: getEnvDuration("DB_CONN_MAX_LIFETIME", 1*time.Hour),
		},
		Auth: AuthConfig{
			TenantID:     getEnv("AZURE_TENANT_ID", "common"),
			ClientID:     getEnv("AZURE_CLIENT_ID", ""),
			ClientSecret: getEnv("AZURE_CLIENT_SECRET", ""),
			RedirectURL:  baseURL + "/auth/callback",
			GraphUserURL: getEnv("GRAPH_USER_URL", "https://graph.microsoft.com/v1.0/me?$select=id,displayName,mail,userPrincipalName,department,jobTitle,officeLocation"),
			AdminEmails:  parseListToMap(getEnv("ADMIN_EMAILS", "")),
		},
		SMTP: SMTPConfig{
			Host:       getEnv("SMTP_HOST", "smtp.office365.com"),
			Port:       getEnvAsInt("SMTP_PORT", 587),
			Username:   getEnv("SMTP_USERNAME", ""),
			Password:   getEnv("SMTP_PASSWORD", ""),
			FromEmail:  getEnv("SMTP_FROM_EMAIL", ""),
			ReplyEmail: getEnv("SMTP_REPLY_EMAIL", ""),
			MaxRetries: getEnvAsInt("SMTP_MAX_RETRIES", 3),
			RetryDelay: getEnvDuration("SMTP_RETRY_DELAY", 2*time.Second),
		},
		Logging: LoggingConfig{
			Level:  parseLogLevel(getEnv("LOG_LEVEL", "INFO")),
			Format: strings.ToLower(getEnv("LOG_FORMAT", "text")),
		},
		Maintenance: MaintenanceConfig{
			Enabled:  getEnvBool("MAINTENANCE_ENABLED", true),
			Interval: getEnvDuration("MAINTENANCE_INTERVAL", 15*time.Minute),
		},
	}

	return cfg
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvAsInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	if val := os.Getenv(key); val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			return b
		}
	}
	return defaultVal
}

func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	}
	return defaultVal
}

func parseListToMap(csv string) map[string]bool {
	m := make(map[string]bool)
	parts := strings.Split(csv, ",")
	for _, p := range parts {
		clean := strings.ToLower(strings.TrimSpace(p))
		if clean != "" {
			m[clean] = true
		}
	}
	return m
}

func parseLogLevel(level string) slog.Level {
	switch strings.ToUpper(strings.TrimSpace(level)) {
	case "DEBUG":
		return slog.LevelDebug
	case "WARN", "WARNING":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
