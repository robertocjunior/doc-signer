package models

import (
	"encoding/xml"
	"time"
)

// UserProfile represents the user data returned by Microsoft Graph API.
type UserProfile struct {
	ID                string `json:"id"`
	DisplayName       string `json:"displayName"`
	Mail              string `json:"mail"`
	UserPrincipalName string `json:"userPrincipalName"`
	Department        string `json:"department"`
	JobTitle          string `json:"jobTitle"`
	OfficeLocation    string `json:"officeLocation"`
}

// EffectiveEmail returns the best available email for the user.
func (u *UserProfile) EffectiveEmail() string {
	if u.Mail != "" {
		return u.Mail
	}
	return u.UserPrincipalName
}

// SignatureRecord represents a completed electronic document confirmation.
type SignatureRecord struct {
	ID             int64     `xml:"id" json:"id"`
	HashID         string    `xml:"hash_id" json:"hash_id"`
	DocumentName   string    `xml:"document_name" json:"document_name"`
	UserName       string    `xml:"user_name" json:"user_name"`
	UserEmail      string    `xml:"user_email" json:"user_email"`
	Department     string    `xml:"department" json:"department"`
	JobTitle       string    `xml:"job_title" json:"job_title"`
	OfficeLocation string    `xml:"office_location" json:"office_location"`
	SignedAt       time.Time `xml:"signed_at" json:"signed_at"`
	UserAgent      string    `xml:"user_agent" json:"user_agent"`
	IPAddress      string    `xml:"ip_address" json:"ip_address"`
}

// XMLSignatures wraps multiple signature records for XML export.
type XMLSignatures struct {
	XMLName xml.Name          `xml:"assinaturas"`
	Records []SignatureRecord `xml:"assinatura"`
}

// FilterCriteria defines filtering options for querying signature records.
type FilterCriteria struct {
	Document string
	User     string
	Dept     string
	Job      string
	Hash     string
	IP       string
}

// HealthResponse represents system health and liveness state for 24/7 monitoring.
type HealthResponse struct {
	Status    string            `json:"status"`
	Uptime    string            `json:"uptime"`
	Timestamp time.Time         `json:"timestamp"`
	Checks    map[string]string `json:"checks"`
}
