package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	maxExpenseAttachmentBytes        = 8 << 20
	maxExpenseAttachmentRequestBytes = 32 << 20
	maxExpenseAttachmentCount        = 10
)

var errInvalidExpenseAttachment = errors.New("invalid expense attachment")

type expenseAttachment struct {
	ID          uint64 `gorm:"primaryKey"`
	UserID      uint64
	ExpenseID   uint64
	FileName    string
	ContentType string
	SizeBytes   int64
	SHA256      string
	FileData    []byte `gorm:"column:file_data"`
	CreatedAt   time.Time
	RemovedAt   *time.Time
}

type expenseAttachmentUpload struct {
	FileName    string
	ContentType string
	Data        []byte
	SHA256      string
}

type expenseAttachmentView struct {
	ID          uint64
	FileName    string
	DownloadURL string
	SizeLabel   string
}

func parseExpenseAttachment(name string, data []byte) (expenseAttachmentUpload, error) {
	name = safeExpenseInvoiceFileName(name)
	if name == "" || len(data) == 0 || len(data) > maxExpenseAttachmentBytes {
		return expenseAttachmentUpload{}, errInvalidExpenseAttachment
	}
	ext := strings.ToLower(filepath.Ext(name))
	detected := http.DetectContentType(data[:min(len(data), 512)])
	contentType := ""
	switch ext {
	case ".pdf":
		if detected == "application/pdf" && bytes.HasPrefix(data, []byte("%PDF-")) {
			contentType = "application/pdf"
		}
	case ".jpg", ".jpeg":
		if detected == "image/jpeg" {
			contentType = detected
		}
	case ".png":
		if detected == "image/png" {
			contentType = detected
		}
	case ".webp":
		if detected == "image/webp" {
			contentType = detected
		}
	case ".docx", ".xlsx", ".pptx":
		if validExpenseOfficeArchive(ext, data) {
			contentType = map[string]string{
				".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
				".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
				".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
			}[ext]
		}
	}
	if contentType == "" {
		return expenseAttachmentUpload{}, errInvalidExpenseAttachment
	}
	digest := sha256.Sum256(data)
	return expenseAttachmentUpload{FileName: name, ContentType: contentType, Data: data, SHA256: hex.EncodeToString(digest[:])}, nil
}

func validExpenseOfficeArchive(ext string, data []byte) bool {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return false
	}
	want := map[string]string{".docx": "word/document.xml", ".xlsx": "xl/workbook.xml", ".pptx": "ppt/presentation.xml"}[ext]
	wantType := map[string]string{".docx": "wordprocessingml.document.main+xml", ".xlsx": "spreadsheetml.sheet.main+xml", ".pptx": "presentationml.presentation.main+xml"}[ext]
	hasTypes, hasDocument := false, false
	for _, file := range reader.File {
		if file.Name == "[Content_Types].xml" {
			if file.UncompressedSize64 > 1<<20 {
				return false
			}
			stream, err := file.Open()
			if err != nil {
				return false
			}
			content, readErr := io.ReadAll(io.LimitReader(stream, 1<<20+1))
			_ = stream.Close()
			hasTypes = readErr == nil && bytes.Contains(content, []byte(wantType))
		}
		if file.Name == want {
			hasDocument = true
		}
	}
	return hasTypes && hasDocument
}

func expenseAttachmentUploads(r *http.Request) ([]expenseAttachmentUpload, error) {
	if r.MultipartForm == nil {
		return nil, nil
	}
	files := r.MultipartForm.File["attachments"]
	if len(files) > maxExpenseAttachmentCount {
		return nil, errInvalidExpenseAttachment
	}
	uploads := make([]expenseAttachmentUpload, 0, len(files))
	for _, header := range files {
		if header.Size > maxExpenseAttachmentBytes {
			return nil, errInvalidExpenseAttachment
		}
		file, err := header.Open()
		if err != nil {
			return nil, errInvalidExpenseAttachment
		}
		data, readErr := io.ReadAll(io.LimitReader(file, maxExpenseAttachmentBytes+1))
		_ = file.Close()
		if readErr != nil {
			return nil, errInvalidExpenseAttachment
		}
		upload, err := parseExpenseAttachment(header.Filename, data)
		if err != nil {
			return nil, err
		}
		uploads = append(uploads, upload)
	}
	return uploads, nil
}

func saveExpenseAttachments(tx *gorm.DB, userID, expenseID uint64, uploads []expenseAttachmentUpload) error {
	for _, upload := range uploads {
		row := expenseAttachment{UserID: userID, ExpenseID: expenseID, FileName: upload.FileName, ContentType: upload.ContentType, SizeBytes: int64(len(upload.Data)), SHA256: upload.SHA256, FileData: upload.Data}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

func listExpenseAttachments(ctx context.Context, db *gorm.DB, userID uint64, expenseIDs []uint64) ([]expenseAttachment, error) {
	if len(expenseIDs) == 0 {
		return nil, nil
	}
	var rows []expenseAttachment
	err := db.WithContext(ctx).Select("id", "user_id", "expense_id", "file_name", "content_type", "size_bytes", "sha256", "created_at").Where("user_id = ? AND expense_id IN ? AND removed_at IS NULL", userID, expenseIDs).Order("expense_id ASC, id DESC").Find(&rows).Error
	return rows, err
}

func attachmentView(row expenseAttachment) expenseAttachmentView {
	return expenseAttachmentView{ID: row.ID, FileName: row.FileName, DownloadURL: fmt.Sprintf("/expenses/files/%d", row.ID), SizeLabel: fmt.Sprintf("%.1f MB", float64(row.SizeBytes)/(1<<20))}
}

func expenseAttachmentPostURL(expenseID uint64, period, status, search, sortValue string) string {
	query := url.Values{"expense_id": {strconv.FormatUint(expenseID, 10)}, "period": {validatedPeriodValue(period)}}
	if status == "all" || status == "invoice_linked" || status == "invoice_missing" {
		query.Set("status", status)
	}
	if search != "" {
		query.Set("search", search)
	}
	if sortValue != "" {
		query.Set("sort", sortValue)
	}
	return "/expenses/files?" + query.Encode()
}

func (a *app) handleExpenseAttachmentFile(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	userID, ok := a.currentUserID(r)
	if !ok || a.db == nil {
		http.Error(w, "database session required", http.StatusServiceUnavailable)
		return
	}
	id, err := strconv.ParseUint(strings.TrimPrefix(r.URL.Path, "/expenses/files/"), 10, 64)
	if err != nil || id == 0 {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.Method == http.MethodPost {
		var row expenseAttachment
		if err := a.db.WithContext(r.Context()).Where("id = ? AND user_id = ? AND removed_at IS NULL", id, userID).First(&row).Error; err != nil {
			http.NotFound(w, r)
			return
		}
		now := time.Now().UTC()
		if err := a.db.WithContext(r.Context()).Model(&expenseAttachment{}).Where("id = ? AND user_id = ? AND removed_at IS NULL", id, userID).Update("removed_at", now).Error; err != nil {
			http.Error(w, "could not remove file", http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, expenseInvoiceActionURL(strconv.FormatUint(row.ExpenseID, 10), r.URL.Query().Get("period"), r.URL.Query().Get("status"), r.URL.Query().Get("search"), r.URL.Query().Get("sort"))+"&message=file_removed", http.StatusFound)
		return
	}
	var row expenseAttachment
	if err := a.db.WithContext(r.Context()).Where("id = ? AND user_id = ? AND removed_at IS NULL", id, userID).First(&row).Error; err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", row.ContentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": row.FileName}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	_, _ = io.Copy(w, bytes.NewReader(row.FileData))
}

func (a *app) handleExpenseAttachments(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := a.currentUserID(r)
	if !ok || a.db == nil {
		http.Error(w, "database session required", http.StatusServiceUnavailable)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxExpenseAttachmentRequestBytes)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		expenseID, parseErr := parsePositiveUint(r.URL.Query().Get("expense_id"))
		if parseErr != nil {
			http.Error(w, "file request is too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Redirect(w, r, expenseInvoiceActionURL(strconv.FormatUint(expenseID, 10), r.URL.Query().Get("period"), r.URL.Query().Get("status"), r.URL.Query().Get("search"), r.URL.Query().Get("sort"))+"&error=invalid_attachment", http.StatusFound)
		return
	}
	defer r.MultipartForm.RemoveAll()
	expenseID, err := parsePositiveUint(r.Form.Get("expense_id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	var expense manualExpense
	if err := a.db.WithContext(r.Context()).Where("id = ? AND user_id = ?", expenseID, userID).First(&expense).Error; err != nil {
		http.NotFound(w, r)
		return
	}
	returnURL := expenseInvoiceActionURL(strconv.FormatUint(expenseID, 10), r.Form.Get("period"), r.Form.Get("status"), r.Form.Get("search"), r.Form.Get("sort"))
	uploads, err := expenseAttachmentUploads(r)
	if err != nil || len(uploads) == 0 {
		http.Redirect(w, r, returnURL+"&error=invalid_attachment", http.StatusFound)
		return
	}
	if err := a.db.WithContext(r.Context()).Transaction(func(tx *gorm.DB) error { return saveExpenseAttachments(tx, userID, expenseID, uploads) }); err != nil {
		http.Error(w, "could not save files", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, returnURL+"&message=files_added", http.StatusFound)
}
