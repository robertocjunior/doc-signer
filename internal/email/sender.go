package email

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net/smtp"
	"strings"
	"time"

	"doc-signer/internal/config"
)

// Sender defines operations for sending emails.
type Sender interface {
	SendReceipt(ctx context.Context, toEmail, toName, docName, hashID, dateStr string) error
}

type office365Sender struct {
	cfg    config.SMTPConfig
	logger *slog.Logger
}

// NewSender creates an email sender configured for Office 365 / standard SMTP.
func NewSender(cfg config.SMTPConfig, logger *slog.Logger) Sender {
	return &office365Sender{
		cfg:    cfg,
		logger: logger,
	}
}

// loginAuth implements the AUTH LOGIN mechanism required by Office 365 and some corporate SMTP relays.
type loginAuth struct {
	username, password string
}

func newLoginAuth(username, password string) smtp.Auth {
	return &loginAuth{username: username, password: password}
}

func (a *loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	return "LOGIN", []byte{}, nil
}

func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if more {
		prompt := strings.ToLower(string(fromServer))
		switch {
		case strings.Contains(prompt, "username"):
			return []byte(a.username), nil
		case strings.Contains(prompt, "password"):
			return []byte(a.password), nil
		default:
			return nil, errors.New("unknown SMTP auth prompt: " + string(fromServer))
		}
	}
	return nil, nil
}

func BuildReceiptMessage(from, replyTo, toEmail, toName, docName, hashID, dateStr string) (string, string) {
	subject := fmt.Sprintf("Confirmação de Leitura: %s", docName)
	body := fmt.Sprintf("Olá, %s.\r\n\r\nConfirmamos que você leu e aceitou eletronicamente o documento corporativo:\r\n\r\nDocumento: %s\r\nData e Hora: %s\r\nCódigo Único (Hash): %s\r\n\r\nEste e-mail é enviado automaticamente, favor não responder.\r\n", toName, docName, dateStr, hashID)

	msg := "From: " + from + "\r\n" +
		"To: " + toEmail + "\r\n" +
		"Reply-To: " + replyTo + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n\r\n" +
		body

	return subject, msg
}

func (s *office365Sender) SendReceipt(ctx context.Context, toEmail, toName, docName, hashID, dateStr string) error {
	if s.cfg.Host == "" || s.cfg.Username == "" || s.cfg.Password == "" {
		s.logger.Warn("SMTP configuration incomplete; skipping email dispatch",
			slog.String("to", toEmail),
			slog.String("doc", docName),
		)
		return nil
	}

	_, msg := BuildReceiptMessage(s.cfg.FromEmail, s.cfg.ReplyEmail, toEmail, toName, docName, hashID, dateStr)
	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)

	var lastErr error
	maxRetries := s.cfg.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 1
	}

	for attempt := 1; attempt <= maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		err := s.dialAndSend(addr, toEmail, msg)
		if err == nil {
			s.logger.Info("receipt email sent successfully",
				slog.String("to", toEmail),
				slog.String("doc", docName),
				slog.String("hash", hashID),
				slog.Int("attempt", attempt),
			)
			return nil
		}

		lastErr = err
		s.logger.Warn("temporary error sending email, retrying...",
			slog.String("to", toEmail),
			slog.Int("attempt", attempt),
			slog.Int("max_attempts", maxRetries),
			slog.String("error", err.Error()),
		)

		if attempt < maxRetries {
			delay := s.cfg.RetryDelay * time.Duration(attempt)
			time.Sleep(delay)
		}
	}

	s.logger.Error("failed to send receipt email after retries",
		slog.String("to", toEmail),
		slog.String("doc", docName),
		slog.String("error", lastErr.Error()),
	)
	return lastErr
}

func (s *office365Sender) dialAndSend(addr, toEmail, msg string) error {
	tlsConfig := &tls.Config{
		ServerName:         s.cfg.Host,
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: false,
	}

	c, err := smtp.Dial(addr)
	if err != nil {
		return fmt.Errorf("smtp dial failed (%s): %w", addr, err)
	}
	defer c.Close()

	if ok, _ := c.Extension("STARTTLS"); ok {
		if err = c.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("starttls negotiation failed: %w", err)
		}
	} else {
		return errors.New("server does not support STARTTLS")
	}

	auth := newLoginAuth(s.cfg.Username, s.cfg.Password)
	if err = c.Auth(auth); err != nil {
		return fmt.Errorf("login authentication failed: %w", err)
	}

	if err = c.Mail(s.cfg.Username); err != nil {
		return fmt.Errorf("mail from failed: %w", err)
	}

	if err = c.Rcpt(toEmail); err != nil {
		return fmt.Errorf("rcpt to failed: %w", err)
	}

	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("data command failed: %w", err)
	}

	if _, err = w.Write([]byte(msg)); err != nil {
		return fmt.Errorf("failed to write message content: %w", err)
	}

	if err = w.Close(); err != nil {
		return fmt.Errorf("failed to close data writer: %w", err)
	}

	return c.Quit()
}
