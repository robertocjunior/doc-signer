package email

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"doc-signer/internal/config"
)

func TestBuildReceiptMessage(t *testing.T) {
	from := "Assinador <nfe@empresa.com>"
	reply := "nfe@empresa.com"
	to := "colaborador@empresa.com"
	name := "Colaborador Teste"
	doc := "Regulamento-Interno.pdf"
	hash := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	date := "29/09/2026 às 10:00:00"

	subject, msg := BuildReceiptMessage(from, reply, to, name, doc, hash, date)

	if !strings.Contains(subject, "Regulamento-Interno.pdf") {
		t.Errorf("subject should contain doc name, got: %s", subject)
	}
	if !strings.Contains(msg, "To: colaborador@empresa.com") {
		t.Errorf("msg should contain recipient email")
	}
	if !strings.Contains(msg, hash) {
		t.Errorf("msg should contain hash")
	}
}

func TestSenderIncompleteConfig(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	sender := NewSender(config.SMTPConfig{}, logger)

	// Incomplete config should gracefully return nil without crashing
	err := sender.SendReceipt(context.Background(), "test@empresa.com", "Test", "Doc.pdf", "hash", "date")
	if err != nil {
		t.Errorf("expected no error when SMTP is unconfigured, got: %v", err)
	}
}
