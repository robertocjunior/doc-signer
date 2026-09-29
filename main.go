package main

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"database/sql"
	"embed"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/smtp"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/xuri/excelize/v2"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/microsoft"
)

//go:embed templates/*
var templateFS embed.FS

var (
	embedTmpl *template.Template
	adminTmpl *template.Template
)

type Config struct {
	BaseURL      string
	Port         string
	AdminEmails  map[string]bool
	OAuth2Config *oauth2.Config
	GraphUserURL string
	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	SMTPPassword string
	SMTPFrom     string
	SMTPReply    string
	DBPath       string
}

type UserProfile struct {
	ID                string `json:"id"`
	DisplayName       string `json:"displayName"`
	Mail              string `json:"mail"`
	UserPrincipalName string `json:"userPrincipalName"`
	Department        string `json:"department"`
	JobTitle          string `json:"jobTitle"`
	OfficeLocation    string `json:"officeLocation"`
}

type SignatureRecord struct {
	ID             int64     `xml:"id"`
	HashID         string    `xml:"hash_id"`
	DocumentName   string    `xml:"document_name"`
	UserName       string    `xml:"user_name"`
	UserEmail      string    `xml:"user_email"`
	Department     string    `xml:"department"`
	JobTitle       string    `xml:"job_title"`
	OfficeLocation string    `xml:"office_location"`
	SignedAt       time.Time `xml:"signed_at"`
	UserAgent      string    `xml:"user_agent"`
	IPAddress      string    `xml:"ip_address"`
}

type XMLSignatures struct {
	XMLName xml.Name          `xml:"assinaturas"`
	Records []SignatureRecord `xml:"assinatura"`
}

var (
	cfg Config
	db  *sql.DB
)

func init() {
	cfg = Config{
		BaseURL:      getEnv("BASE_URL", "http://localhost:8080"),
		Port:         getEnv("PORT", "8080"),
		AdminEmails:  parseListToMap(getEnv("ADMIN_EMAILS", "")),
		GraphUserURL: "https://graph.microsoft.com/v1.0/me?$select=id,displayName,mail,userPrincipalName,department,jobTitle,officeLocation",
		SMTPHost:     getEnv("SMTP_HOST", "smtp.office365.com"),
		SMTPPort:     getEnvAsInt("SMTP_PORT", 587),
		SMTPUsername: getEnv("SMTP_USERNAME", ""),
		SMTPPassword: getEnv("SMTP_PASSWORD", ""),
		SMTPFrom:     getEnv("SMTP_FROM_EMAIL", ""),
		SMTPReply:    getEnv("SMTP_REPLY_EMAIL", ""),
		DBPath:       getEnv("DB_PATH", "/app/data/signatures.db"),
	}

	cfg.OAuth2Config = &oauth2.Config{
		ClientID:     getEnv("AZURE_CLIENT_ID", ""),
		ClientSecret: getEnv("AZURE_CLIENT_SECRET", ""),
		RedirectURL:  strings.TrimRight(cfg.BaseURL, "/") + "/auth/callback",
		Scopes:       []string{"openid", "profile", "email", "User.Read"},
		Endpoint:     microsoft.AzureADEndpoint(getEnv("AZURE_TENANT_ID", "common")),
	}

	var err error
	embedTmpl, err = template.ParseFS(templateFS, "templates/embed.html")
	if err != nil {
		log.Fatalf("Erro ao carregar templates/embed.html: %v", err)
	}

	adminTmpl, err = template.ParseFS(templateFS, "templates/admin.html")
	if err != nil {
		log.Fatalf("Erro ao carregar templates/admin.html: %v", err)
	}
}

func main() {
	var err error
	db, err = sql.Open("sqlite3", cfg.DBPath+"?_journal_mode=WAL")
	if err != nil {
		log.Fatalf("Falha ao abrir SQLite: %v", err)
	}
	defer db.Close()

	createTableQuery := `
	CREATE TABLE IF NOT EXISTS signatures (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		hash_id TEXT UNIQUE NOT NULL,
		document_name TEXT NOT NULL,
		user_name TEXT NOT NULL,
		user_email TEXT NOT NULL,
		department TEXT,
		job_title TEXT,
		office_location TEXT,
		signed_at DATETIME NOT NULL,
		user_agent TEXT,
		ip_address TEXT
	);
	CREATE INDEX IF NOT EXISTS idx_doc_email ON signatures(document_name, user_email);
	`
	if _, err := db.Exec(createTableQuery); err != nil {
		log.Fatalf("Erro ao migrar tabela: %v", err)
	}

	http.HandleFunc("/assinar", handleSignEmbed)
	http.HandleFunc("/auth/login", handleLogin)
	http.HandleFunc("/auth/callback", handleCallback)
	http.HandleFunc("/confirmar", handleConfirmSign)
	http.HandleFunc("/admin", handleAdmin)
	http.HandleFunc("/admin/export", handleAdminExport)

	log.Printf("Servidor de Confirmações iniciado na porta :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, nil); err != nil {
		log.Fatalf("Erro HTTP: %v", err)
	}
}

func extractDocParam(r *http.Request) string {
	doc := r.URL.Query().Get("doc")
	if doc == "" {
		raw := r.URL.RawQuery
		if idx := strings.Index(raw, "doc="); idx != -1 {
			sub := raw[idx+4:]
			if ampHex := strings.Index(sub, "&"); ampHex != -1 {
				doc = sub[:ampHex]
			} else {
				doc = sub
			}
		}
	}
	return strings.TrimSpace(doc)
}

func handleSignEmbed(w http.ResponseWriter, r *http.Request) {
	doc := extractDocParam(r)
	if doc == "" {
		http.Error(w, "Identificador de documento 'doc' não informado.", http.StatusBadRequest)
		return
	}

	emailCookie, _ := r.Cookie("user_email")
	nameCookie, _ := r.Cookie("user_name")
	deptCookie, _ := r.Cookie("user_department")
	isLogged := emailCookie != nil && emailCookie.Value != ""

	var signedRecord *SignatureRecord
	if isLogged {
		signedRecord = getExistingSignature(doc, emailCookie.Value)
	}

	data := map[string]interface{}{
		"Doc":            doc,
		"IsLogged":       isLogged,
		"UserEmail":      valFromCookie(emailCookie),
		"UserName":       valFromCookie(nameCookie),
		"UserDepartment": valFromCookie(deptCookie),
		"Record":         signedRecord,
		"IsSigned":       signedRecord != nil,
		"LoginURL":       fmt.Sprintf("/auth/login?doc=%s&return=/assinar%%3Fdoc%%3D%s", doc, doc),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = embedTmpl.Execute(w, data)
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	doc := extractDocParam(r)
	returnURL := r.URL.Query().Get("return")
	if returnURL == "" {
		returnURL = "/assinar?doc=" + doc
	}

	state := fmt.Sprintf("%s|%s", doc, returnURL)
	url := cfg.OAuth2Config.AuthCodeURL(state, oauth2.AccessTypeOffline)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

func handleCallback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")

	if code == "" {
		http.Error(w, "Código de autorização não recebido da Microsoft", http.StatusBadRequest)
		return
	}

	token, err := cfg.OAuth2Config.Exchange(r.Context(), code)
	if err != nil {
		http.Error(w, "Falha na troca de token com o Azure: "+err.Error(), http.StatusInternalServerError)
		return
	}

	client := cfg.OAuth2Config.Client(r.Context(), token)
	resp, err := client.Get(cfg.GraphUserURL)
	if err != nil {
		http.Error(w, "Falha ao consultar perfil no Microsoft Graph: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var profile UserProfile
	if err := json.Unmarshal(body, &profile); err != nil {
		http.Error(w, "Erro ao decodificar JSON do Microsoft Graph", http.StatusInternalServerError)
		return
	}

	email := profile.Mail
	if email == "" {
		email = profile.UserPrincipalName
	}
	email = strings.ToLower(strings.TrimSpace(email))

	setSessionCookie(w, "user_email", email)
	setSessionCookie(w, "user_name", profile.DisplayName)
	setSessionCookie(w, "user_department", profile.Department)
	setSessionCookie(w, "user_job", profile.JobTitle)
	setSessionCookie(w, "user_location", profile.OfficeLocation)

	parts := strings.Split(state, "|")
	returnURL := "/assinar"
	if len(parts) >= 2 && parts[1] != "" {
		returnURL = parts[1]
	}

	http.Redirect(w, r, returnURL, http.StatusSeeOther)
}

func handleConfirmSign(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		return
	}

	doc := r.FormValue("doc")
	if doc == "" {
		http.Error(w, "Documento não informado", http.StatusBadRequest)
		return
	}

	emailCookie, _ := r.Cookie("user_email")
	if emailCookie == nil || emailCookie.Value == "" {
		http.Redirect(w, r, fmt.Sprintf("/auth/login?doc=%s&return=/assinar%%3Fdoc%%3D%s", doc, doc), http.StatusSeeOther)
		return
	}

	userEmail := strings.ToLower(emailCookie.Value)
	existing := getExistingSignature(doc, userEmail)
	if existing != nil {
		http.Redirect(w, r, "/assinar?doc="+doc, http.StatusSeeOther)
		return
	}

	userName := valFromCookieName(r, "user_name")
	department := valFromCookieName(r, "user_department")
	jobTitle := valFromCookieName(r, "user_job")
	officeLocation := valFromCookieName(r, "user_location")
	now := time.Now()
	ip := getIP(r)
	ua := r.UserAgent()

	rawSeed := fmt.Sprintf("%s|%s|%s|%s|%s", doc, userEmail, now.UTC().Format(time.RFC3339Nano), ip, ua)
	hasher := sha256.New()
	hasher.Write([]byte(rawSeed))
	hashID := hex.EncodeToString(hasher.Sum(nil))

	query := `
	INSERT INTO signatures (hash_id, document_name, user_name, user_email, department, job_title, office_location, signed_at, user_agent, ip_address)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := db.Exec(query, hashID, doc, userName, userEmail, department, jobTitle, officeLocation, now, ua, ip)
	if err != nil {
		http.Error(w, "Erro ao registrar confirmação: "+err.Error(), http.StatusInternalServerError)
		return
	}

	go sendReceiptEmail(userEmail, userName, doc, hashID, now.Format("02/01/2006 às 15:04:05"))

	http.Redirect(w, r, "/assinar?doc="+doc, http.StatusSeeOther)
}

func checkAdminAuth(w http.ResponseWriter, r *http.Request) (string, bool) {
	emailCookie, _ := r.Cookie("user_email")
	if emailCookie == nil || emailCookie.Value == "" {
		http.Redirect(w, r, "/auth/login?return="+url.QueryEscape(r.RequestURI), http.StatusSeeOther)
		return "", false
	}

	userEmail := strings.ToLower(emailCookie.Value)
	if !cfg.AdminEmails[userEmail] {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("Acesso restrito: Seu e-mail (" + userEmail + ") não está autorizado."))
		return "", false
	}
	return userEmail, true
}

func querySignatures(r *http.Request) ([]SignatureRecord, error) {
	doc := strings.TrimSpace(r.URL.Query().Get("doc"))
	user := strings.TrimSpace(r.URL.Query().Get("user"))
	dept := strings.TrimSpace(r.URL.Query().Get("dept"))
	job := strings.TrimSpace(r.URL.Query().Get("job"))
	hash := strings.TrimSpace(r.URL.Query().Get("hash"))
	ip := strings.TrimSpace(r.URL.Query().Get("ip"))

	query := `
		SELECT id, hash_id, document_name, user_name, user_email,
		       COALESCE(department, ''), COALESCE(job_title, ''), COALESCE(office_location, ''),
		       signed_at, user_agent, ip_address
		FROM signatures
		WHERE 1=1`
	var params []interface{}

	if doc != "" {
		query += " AND document_name LIKE ?"
		params = append(params, "%"+doc+"%")
	}
	if user != "" {
		query += " AND (user_email LIKE ? OR user_name LIKE ?)"
		params = append(params, "%"+user+"%", "%"+user+"%")
	}
	if dept != "" {
		query += " AND department LIKE ?"
		params = append(params, "%"+dept+"%")
	}
	if job != "" {
		query += " AND job_title LIKE ?"
		params = append(params, "%"+job+"%")
	}
	if hash != "" {
		query += " AND hash_id LIKE ?"
		params = append(params, "%"+hash+"%")
	}
	if ip != "" {
		query += " AND ip_address LIKE ?"
		params = append(params, "%"+ip+"%")
	}

	query += " ORDER BY signed_at DESC"

	rows, err := db.Query(query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []SignatureRecord
	for rows.Next() {
		var rec SignatureRecord
		if err := rows.Scan(&rec.ID, &rec.HashID, &rec.DocumentName, &rec.UserName, &rec.UserEmail, &rec.Department, &rec.JobTitle, &rec.OfficeLocation, &rec.SignedAt, &rec.UserAgent, &rec.IPAddress); err == nil {
			records = append(records, rec)
		}
	}
	return records, nil
}

func handleAdmin(w http.ResponseWriter, r *http.Request) {
	userEmail, ok := checkAdminAuth(w, r)
	if !ok {
		return
	}

	records, err := querySignatures(r)
	if err != nil {
		http.Error(w, "Erro ao buscar registros: "+err.Error(), http.StatusInternalServerError)
		return
	}

	q := r.URL.Query()
	q.Del("format")

	data := map[string]interface{}{
		"UserEmail":   userEmail,
		"Records":     records,
		"FilterDoc":   r.URL.Query().Get("doc"),
		"FilterUser":  r.URL.Query().Get("user"),
		"FilterDept":  r.URL.Query().Get("dept"),
		"FilterJob":   r.URL.Query().Get("job"),
		"FilterHash":  r.URL.Query().Get("hash"),
		"FilterIP":    r.URL.Query().Get("ip"),
		"QueryString": q.Encode(),
		"Count":       len(records),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = adminTmpl.Execute(w, data)
}

func handleAdminExport(w http.ResponseWriter, r *http.Request) {
	_, ok := checkAdminAuth(w, r)
	if !ok {
		return
	}

	records, err := querySignatures(r)
	if err != nil {
		http.Error(w, "Erro ao gerar exportação: "+err.Error(), http.StatusInternalServerError)
		return
	}

	format := strings.ToLower(r.URL.Query().Get("format"))
	timestamp := time.Now().Format("20060102_150405")

	switch format {
	case "csv":
		filename := fmt.Sprintf("assinaturas_%s.csv", timestamp)
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))

		w.Write([]byte{0xEF, 0xBB, 0xBF}) // BOM UTF-8

		writer := csv.NewWriter(w)
		writer.Comma = ';'
		_ = writer.Write([]string{"ID", "Data/Hora", "Documento", "Colaborador", "Email", "Setor", "Cargo", "Localizacao", "Hash", "IP", "User Agent"})

		for _, rec := range records {
			_ = writer.Write([]string{
				strconv.FormatInt(rec.ID, 10),
				rec.SignedAt.Format("02/01/2006 15:04:05"),
				rec.DocumentName,
				rec.UserName,
				rec.UserEmail,
				rec.Department,
				rec.JobTitle,
				rec.OfficeLocation,
				rec.HashID,
				rec.IPAddress,
				rec.UserAgent,
			})
		}
		writer.Flush()

	case "xml":
		filename := fmt.Sprintf("assinaturas_%s.xml", timestamp)
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))

		data := XMLSignatures{Records: records}
		w.Write([]byte(xml.Header))
		enc := xml.NewEncoder(w)
		enc.Indent("", "  ")
		_ = enc.Encode(data)

	case "xlsx":
		filename := fmt.Sprintf("assinaturas_%s.xlsx", timestamp)
		f := excelize.NewFile()
		defer f.Close()

		sheet := "Assinaturas"
		f.SetSheetName("Sheet1", sheet)

		headers := []string{"ID", "Data / Hora", "Documento", "Colaborador", "E-mail", "Setor", "Cargo", "Localização", "Hash ID", "IP Origem", "Navegador / SO"}
		for i, h := range headers {
			cell, _ := excelize.CoordinatesToCellName(i+1, 1)
			f.SetCellValue(sheet, cell, h)
		}

		styleHeader, _ := f.NewStyle(&excelize.Style{
			Font:      &excelize.Font{Bold: true, Color: "FFFFFF"},
			Fill:      excelize.Fill{Type: "pattern", Color: []string{"0078D4"}, Pattern: 1},
			Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		})
		_ = f.SetRowStyle(sheet, 1, 1, styleHeader)

		for rowIdx, rec := range records {
			rNum := rowIdx + 2
			f.SetCellValue(sheet, fmt.Sprintf("A%d", rNum), rec.ID)
			f.SetCellValue(sheet, fmt.Sprintf("B%d", rNum), rec.SignedAt.Format("02/01/2006 15:04:05"))
			f.SetCellValue(sheet, fmt.Sprintf("C%d", rNum), rec.DocumentName)
			f.SetCellValue(sheet, fmt.Sprintf("D%d", rNum), rec.UserName)
			f.SetCellValue(sheet, fmt.Sprintf("E%d", rNum), rec.UserEmail)
			f.SetCellValue(sheet, fmt.Sprintf("F%d", rNum), rec.Department)
			f.SetCellValue(sheet, fmt.Sprintf("G%d", rNum), rec.JobTitle)
			f.SetCellValue(sheet, fmt.Sprintf("H%d", rNum), rec.OfficeLocation)
			f.SetCellValue(sheet, fmt.Sprintf("I%d", rNum), rec.HashID)
			f.SetCellValue(sheet, fmt.Sprintf("J%d", rNum), rec.IPAddress)
			f.SetCellValue(sheet, fmt.Sprintf("K%d", rNum), rec.UserAgent)
		}

		_ = f.SetColWidth(sheet, "B", "B", 20)
		_ = f.SetColWidth(sheet, "C", "C", 22)
		_ = f.SetColWidth(sheet, "D", "E", 32)
		_ = f.SetColWidth(sheet, "F", "G", 26)
		_ = f.SetColWidth(sheet, "I", "I", 36)

		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))

		var buf bytes.Buffer
		_ = f.Write(&buf)
		w.Write(buf.Bytes())

	default:
		http.Error(w, "Formato não suportado. Use xlsx, csv ou xml.", http.StatusBadRequest)
	}
}

func getExistingSignature(doc, email string) *SignatureRecord {
	query := `SELECT id, hash_id, document_name, user_name, user_email, COALESCE(department, ''), signed_at FROM signatures WHERE document_name = ? AND user_email = ? LIMIT 1`
	row := db.QueryRow(query, doc, strings.ToLower(email))

	var rec SignatureRecord
	err := row.Scan(&rec.ID, &rec.HashID, &rec.DocumentName, &rec.UserName, &rec.UserEmail, &rec.Department, &rec.SignedAt)
	if err != nil {
		return nil
	}
	return &rec
}

// Implementação do mecanismo AUTH LOGIN requerido pelo Office 365
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
			return nil, errors.New("prompt de autenticação desconhecido: " + string(fromServer))
		}
	}
	return nil, nil
}

func sendReceiptEmail(toEmail string, toName string, docName string, hashID string, dateStr string) {
	subject := fmt.Sprintf("Confirmação de Leitura: %s", docName)
	body := fmt.Sprintf("Olá, %s.\r\n\r\nConfirmamos que você leu e aceitou eletronicamente o documento corporativo:\r\n\r\nDocumento: %s\r\nData e Hora: %s\r\nCódigo Único (Hash): %s\r\n\r\nEste e-mail é enviado automaticamente, favor não responder.\r\nGrupo Lipetral\r\n", toName, docName, dateStr, hashID)

	msg := "From: " + cfg.SMTPFrom + "\r\n" +
		"To: " + toEmail + "\r\n" +
		"Reply-To: " + cfg.SMTPReply + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n\r\n" +
		body

	addr := fmt.Sprintf("%s:%d", cfg.SMTPHost, cfg.SMTPPort)

	// Conexão e Upgrade com STARTTLS explícito com TLS 1.2+
	tlsConfig := &tls.Config{
		ServerName:         cfg.SMTPHost,
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: false,
	}

	c, err := smtp.Dial(addr)
	if err != nil {
		log.Printf("[SMTP] Erro ao conectar no Office 365 (%s): %v", addr, err)
		return
	}
	defer c.Close()

	if ok, _ := c.Extension("STARTTLS"); ok {
		if err = c.StartTLS(tlsConfig); err != nil {
			log.Printf("[SMTP] Falha STARTTLS: %v", err)
			return
		}
	} else {
		log.Printf("[SMTP] Servidor não suporta STARTTLS")
		return
	}

	// Autenticação LOGIN (compatível com Office 365)
	auth := newLoginAuth(cfg.SMTPUsername, cfg.SMTPPassword)
	if err = c.Auth(auth); err != nil {
		log.Printf("[SMTP] Falha de autenticação LOGIN: %v", err)
		return
	}

	if err = c.Mail(cfg.SMTPUsername); err != nil {
		log.Printf("[SMTP] Erro MAIL FROM: %v", err)
		return
	}

	if err = c.Rcpt(toEmail); err != nil {
		log.Printf("[SMTP] Erro RCPT TO (%s): %v", toEmail, err)
		return
	}

	w, err := c.Data()
	if err != nil {
		log.Printf("[SMTP] Erro no comando DATA: %v", err)
		return
	}

	if _, err = w.Write([]byte(msg)); err != nil {
		log.Printf("[SMTP] Erro enviando corpo do e-mail: %v", err)
		return
	}

	_ = w.Close()
	_ = c.Quit()
	log.Printf("[SMTP] Comprovante enviado com sucesso para %s (Doc: %s)", toEmail, docName)
}

func setSessionCookie(w http.ResponseWriter, name, value string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(24 * time.Hour * 30),
	})
}

func valFromCookie(c *http.Cookie) string {
	if c != nil {
		return c.Value
	}
	return ""
}

func valFromCookieName(r *http.Request, name string) string {
	c, err := r.Cookie(name)
	if err == nil {
		return c.Value
	}
	return ""
}

func getIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	return r.RemoteAddr
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