package handlers

import (
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"doc-signer/internal/auth"
	"doc-signer/internal/database"
	"doc-signer/internal/models"

	"github.com/xuri/excelize/v2"
)

type ExportHandler struct {
	repo    database.Repository
	authSvc auth.Service
}

func NewExportHandler(repo database.Repository, authSvc auth.Service) *ExportHandler {
	return &ExportHandler{
		repo:    repo,
		authSvc: authSvc,
	}
}

func (h *ExportHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	email := getCookieValue(r, "user_email")
	if email == "" || !h.authSvc.IsAdmin(strings.ToLower(email)) {
		http.Error(w, "Acesso restrito para administradores.", http.StatusForbidden)
		return
	}

	filter := extractFilterCriteria(r)
	records, err := h.repo.List(r.Context(), filter)
	if err != nil {
		http.Error(w, "Erro ao gerar exportação: "+err.Error(), http.StatusInternalServerError)
		return
	}

	format := strings.ToLower(r.URL.Query().Get("format"))
	timestamp := time.Now().Format("20060102_150405")

	switch format {
	case "csv":
		h.exportCSV(w, records, timestamp)
	case "xml":
		h.exportXML(w, records, timestamp)
	case "xlsx":
		h.exportXLSX(w, records, timestamp)
	default:
		http.Error(w, "Formato não suportado. Use xlsx, csv ou xml.", http.StatusBadRequest)
	}
}

func (h *ExportHandler) exportCSV(w http.ResponseWriter, records []models.SignatureRecord, timestamp string) {
	filename := fmt.Sprintf("assinaturas_%s.csv", timestamp)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))

	// Write UTF-8 BOM so Excel opens accents correctly
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})

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
}

func (h *ExportHandler) exportXML(w http.ResponseWriter, records []models.SignatureRecord, timestamp string) {
	filename := fmt.Sprintf("assinaturas_%s.xml", timestamp)
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))

	data := models.XMLSignatures{Records: records}
	_, _ = w.Write([]byte(xml.Header))
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	_ = enc.Encode(data)
}

func (h *ExportHandler) exportXLSX(w http.ResponseWriter, records []models.SignatureRecord, timestamp string) {
	filename := fmt.Sprintf("assinaturas_%s.xlsx", timestamp)
	f := excelize.NewFile()
	defer f.Close()

	sheet := "Assinaturas"
	f.SetSheetName("Sheet1", sheet)

	headers := []string{"ID", "Data / Hora", "Documento", "Colaborador", "E-mail", "Setor", "Cargo", "Localização", "Hash ID", "IP Origem", "Navegador / SO"}
	for i, head := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheet, cell, head)
	}

	styleHeader, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"0078D4"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	_ = f.SetRowStyle(sheet, 1, 1, styleHeader)

	for rowIdx, rec := range records {
		rNum := rowIdx + 2
		_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", rNum), rec.ID)
		_ = f.SetCellValue(sheet, fmt.Sprintf("B%d", rNum), rec.SignedAt.Format("02/01/2006 15:04:05"))
		_ = f.SetCellValue(sheet, fmt.Sprintf("C%d", rNum), rec.DocumentName)
		_ = f.SetCellValue(sheet, fmt.Sprintf("D%d", rNum), rec.UserName)
		_ = f.SetCellValue(sheet, fmt.Sprintf("E%d", rNum), rec.UserEmail)
		_ = f.SetCellValue(sheet, fmt.Sprintf("F%d", rNum), rec.Department)
		_ = f.SetCellValue(sheet, fmt.Sprintf("G%d", rNum), rec.JobTitle)
		_ = f.SetCellValue(sheet, fmt.Sprintf("H%d", rNum), rec.OfficeLocation)
		_ = f.SetCellValue(sheet, fmt.Sprintf("I%d", rNum), rec.HashID)
		_ = f.SetCellValue(sheet, fmt.Sprintf("J%d", rNum), rec.IPAddress)
		_ = f.SetCellValue(sheet, fmt.Sprintf("K%d", rNum), rec.UserAgent)
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
	_, _ = w.Write(buf.Bytes())
}
