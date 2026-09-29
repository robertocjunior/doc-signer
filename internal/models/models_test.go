package models

import (
	"encoding/xml"
	"strings"
	"testing"
	"time"
)

func TestUserProfileEffectiveEmail(t *testing.T) {
	u1 := UserProfile{Mail: "user@example.com", UserPrincipalName: "upn@example.com"}
	if u1.EffectiveEmail() != "user@example.com" {
		t.Fatalf("expected user@example.com, got %s", u1.EffectiveEmail())
	}

	u2 := UserProfile{Mail: "", UserPrincipalName: "upn@example.com"}
	if u2.EffectiveEmail() != "upn@example.com" {
		t.Fatalf("expected upn@example.com, got %s", u2.EffectiveEmail())
	}
}

func TestXMLSignaturesSerialization(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	data := XMLSignatures{
		Records: []SignatureRecord{
			{
				ID:           1,
				HashID:       "abcd1234ef",
				DocumentName: "Politica-TI.pdf",
				UserName:     "Roberto Silva",
				UserEmail:    "roberto@example.com",
				SignedAt:     now,
			},
		},
	}

	output, err := xml.MarshalIndent(data, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal xml: %v", err)
	}

	str := string(output)
	if !strings.Contains(str, "<assinaturas>") || !strings.Contains(str, "<document_name>Politica-TI.pdf</document_name>") {
		t.Fatalf("unexpected xml content: %s", str)
	}
}
