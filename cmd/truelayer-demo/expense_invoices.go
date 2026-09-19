package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
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
	"gorm.io/gorm/clause"
)

const maxExpenseInvoiceBytes = 8 << 20

var errInvalidExpenseInvoice = errors.New("invalid expense invoice")

type manualExpenseInvoice struct {
	ID            uint64 `gorm:"primaryKey"`
	UserID        uint64
	ExpenseID     uint64
	InvoiceNumber string
	Vendor        string
	InvoiceDate   time.Time
	AmountCents   int64
	Currency      string
	FileName      string
	ContentType   string
	SHA256        string
	FileData      []byte `gorm:"column:file_data"`
	Note          *string
	IsCurrent     bool
	ReplacedAt    *time.Time
	CreatedAt     time.Time
}

func (manualExpenseInvoice) TableName() string { return "manual_expense_invoices" }

type expenseInvoiceView struct {
	ID               uint64
	InvoiceNumber    string
	Vendor           string
	DateDisplay      string
	AmountDisplay    string
	FileName         string
	DownloadURL      string
	Note             string
	CreatedAtDisplay string
}

type expenseInvoiceFormView struct {
	ExpenseID     string
	Description   string
	ExpenseAmount string
	InvoiceNumber string
	Vendor        string
	InvoiceDate   string
	InvoiceAmount string
	Note          string
	Current       *expenseInvoiceView
	History       []expenseInvoiceView
	ReturnURL     string
	PostURL       string
	Period        string
	StatusFilter  string
	Search        string
}

func invoiceView(row manualExpenseInvoice) expenseInvoiceView {
	return expenseInvoiceView{
		ID: row.ID, InvoiceNumber: row.InvoiceNumber, Vendor: row.Vendor,
		DateDisplay:   row.InvoiceDate.Format(dateLayout),
		AmountDisplay: formatMoney(centsToMoney(row.AmountCents), row.Currency, 2),
		FileName:      row.FileName, DownloadURL: fmt.Sprintf("/expenses/invoices/%d", row.ID),
		Note: stringValue(row.Note), CreatedAtDisplay: row.CreatedAt.Format(dateLayout),
	}
}

func expenseInvoiceActionURL(expenseID, period, status, search string) string {
	query := url.Values{}
	query.Set("invoice", expenseID)
	query.Set("period", validatedPeriodValue(period))
	if status == "all" || status == "invoice_linked" || status == "invoice_missing" {
		query.Set("status", status)
	}
	if search = strings.TrimSpace(search); search != "" {
		query.Set("search", search)
	}
	return "/expenses?" + query.Encode()
}

func (a *app) loadCurrentExpenseInvoices(ctx context.Context, userID uint64, expenseIDs []uint64) (map[uint64]manualExpenseInvoice, error) {
	result := make(map[uint64]manualExpenseInvoice, len(expenseIDs))
	if len(expenseIDs) == 0 || a.db == nil || userID == 0 {
		return result, nil
	}
	var rows []manualExpenseInvoice
	if err := a.db.WithContext(ctx).Where("user_id = ? AND expense_id IN ? AND is_current = ?", userID, expenseIDs, true).
		Order("expense_id ASC, id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		if _, exists := result[row.ExpenseID]; !exists {
			result[row.ExpenseID] = row
		}
	}
	return result, nil
}

func (a *app) expenseInvoiceForm(ctx context.Context, userID uint64, expense expenseRecord, period, status, search string) (*expenseInvoiceFormView, error) {
	expenseID, err := strconv.ParseUint(expense.ID, 10, 64)
	if err != nil || expenseID == 0 || a.db == nil {
		return nil, errInvalidExpenseInvoice
	}
	var rows []manualExpenseInvoice
	if err := a.db.WithContext(ctx).Where("user_id = ? AND expense_id = ?", userID, expenseID).Order("id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	view := &expenseInvoiceFormView{
		ExpenseID: expense.ID, Description: expense.Description,
		ExpenseAmount: strconv.FormatFloat(expense.Amount, 'f', 2, 64),
		InvoiceDate:   time.Now().UTC().Format(dateLayout), InvoiceAmount: strconv.FormatFloat(expense.Amount, 'f', 2, 64),
		ReturnURL: expensePageURL(url.Values{"period": {period}, "status": {status}, "search": {search}}, "", "", false),
		PostURL:   expenseInvoicePostURL(expenseID, period, status, search),
		Period:    period, StatusFilter: status, Search: search,
	}
	for _, row := range rows {
		if row.IsCurrent && view.Current == nil {
			current := invoiceView(row)
			view.Current = &current
			view.InvoiceNumber, view.Vendor = row.InvoiceNumber, row.Vendor
			view.InvoiceDate = row.InvoiceDate.Format(dateLayout)
			view.InvoiceAmount = strconv.FormatFloat(centsToMoney(row.AmountCents), 'f', 2, 64)
			view.Note = stringValue(row.Note)
			continue
		}
		view.History = append(view.History, invoiceView(row))
	}
	return view, nil
}

func expenseInvoicePostURL(expenseID uint64, period, status, search string) string {
	query := url.Values{"invoice": {strconv.FormatUint(expenseID, 10)}, "period": {validatedPeriodValue(period)}}
	if status == "all" || status == "invoice_linked" || status == "invoice_missing" {
		query.Set("status", status)
	}
	if search = strings.TrimSpace(search); search != "" {
		query.Set("search", search)
	}
	return "/expenses/invoices?" + query.Encode()
}

func parseExpenseInvoiceMetadata(values url.Values, fileData []byte, fileName string) (manualExpenseInvoice, error) {
	expenseID, err := parsePositiveUint(values.Get("expense_id"))
	if err != nil || expenseID == 0 {
		return manualExpenseInvoice{}, errInvalidExpenseInvoice
	}
	number := strings.TrimSpace(values.Get("invoice_number"))
	vendor := strings.TrimSpace(values.Get("vendor"))
	if number == "" || len([]rune(number)) > 191 || vendor == "" || len([]rune(vendor)) > 191 || len([]rune(values.Get("note"))) > 2000 {
		return manualExpenseInvoice{}, errInvalidExpenseInvoice
	}
	date, err := parseDate(values.Get("invoice_date"))
	if err != nil {
		return manualExpenseInvoice{}, errInvalidExpenseInvoice
	}
	amount, err := parsePositiveAmount(values.Get("invoice_amount"))
	if err != nil {
		return manualExpenseInvoice{}, errInvalidExpenseInvoice
	}
	if len(fileData) == 0 || len(fileData) > maxExpenseInvoiceBytes || fileName == "" {
		return manualExpenseInvoice{}, errInvalidExpenseInvoice
	}
	contentType := http.DetectContentType(fileData[:min(len(fileData), 512)])
	if !allowedExpenseInvoiceContentType(contentType) {
		return manualExpenseInvoice{}, errInvalidExpenseInvoice
	}
	fileName = safeExpenseInvoiceFileName(fileName)
	if fileName == "" {
		return manualExpenseInvoice{}, errInvalidExpenseInvoice
	}
	digest := sha256.Sum256(fileData)
	return manualExpenseInvoice{
		ExpenseID: expenseID, InvoiceNumber: number, Vendor: vendor, InvoiceDate: date,
		AmountCents: moneyToCents(amount), Currency: ledgerCurrencyEUR,
		FileName: fileName, ContentType: contentType, SHA256: hex.EncodeToString(digest[:]),
		FileData: fileData, Note: nullableString(strings.TrimSpace(values.Get("note"))), IsCurrent: true,
	}, nil
}

func allowedExpenseInvoiceContentType(contentType string) bool {
	switch contentType {
	case "application/pdf", "image/jpeg", "image/png", "image/webp":
		return true
	default:
		return false
	}
}

func safeExpenseInvoiceFileName(name string) string {
	name = strings.ReplaceAll(strings.TrimSpace(name), "\\", "/")
	name = filepath.Base(name)
	name = strings.TrimSpace(strings.Trim(name, "."))
	if name == "" || name == "/" || len([]rune(name)) > 255 {
		return ""
	}
	return name
}

func (a *app) bindExpenseInvoice(ctx context.Context, userID uint64, invoice manualExpenseInvoice) error {
	if userID == 0 || invoice.ExpenseID == 0 || a.db == nil {
		return errInvalidExpenseInvoice
	}
	return a.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var expense manualExpense
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", invoice.ExpenseID, userID).First(&expense).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, sql.ErrNoRows) {
				return errInvalidExpenseInvoice
			}
			return err
		}
		now := time.Now().UTC()
		if err := tx.Model(&manualExpenseInvoice{}).Where("user_id = ? AND expense_id = ? AND is_current = ?", userID, invoice.ExpenseID, true).
			Updates(map[string]any{"is_current": false, "replaced_at": now}).Error; err != nil {
			return err
		}
		invoice.UserID = userID
		invoice.Currency = firstNonEmpty(expense.Currency, ledgerCurrencyEUR)
		return tx.Create(&invoice).Error
	})
}

func (a *app) handleExpenseInvoiceFile(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := a.currentUserID(r)
	if !ok || a.db == nil {
		http.Error(w, "database session required", http.StatusServiceUnavailable)
		return
	}
	rawID := strings.TrimPrefix(r.URL.Path, "/expenses/invoices/")
	id, err := strconv.ParseUint(rawID, 10, 64)
	if err != nil || id == 0 {
		http.NotFound(w, r)
		return
	}
	var invoice manualExpenseInvoice
	if err := a.db.WithContext(r.Context()).Where("id = ? AND user_id = ?", id, userID).First(&invoice).Error; err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", invoice.ContentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": invoice.FileName}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	_, _ = io.Copy(w, bytes.NewReader(invoice.FileData))
}

func (a *app) postExpenseInvoice(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.currentUserID(r)
	if !ok || a.db == nil {
		http.Error(w, "database session required", http.StatusServiceUnavailable)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxExpenseInvoiceBytes+(1<<20))
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		values := r.URL.Query()
		values.Set("expense_id", values.Get("invoice"))
		http.Redirect(w, r, expensePageURL(values, "", "invalid_invoice", false), http.StatusFound)
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()
	file, header, err := r.FormFile("invoice_file")
	if err != nil {
		http.Redirect(w, r, expensePageURL(r.Form, "", "invalid_invoice", false), http.StatusFound)
		return
	}
	defer file.Close()
	fileData, err := io.ReadAll(io.LimitReader(file, maxExpenseInvoiceBytes+1))
	if err != nil || len(fileData) > maxExpenseInvoiceBytes {
		http.Redirect(w, r, expensePageURL(r.Form, "", "invalid_invoice", false), http.StatusFound)
		return
	}
	invoice, err := parseExpenseInvoiceMetadata(r.Form, fileData, header.Filename)
	if err != nil {
		http.Redirect(w, r, expensePageURL(r.Form, "", "invalid_invoice", false), http.StatusFound)
		return
	}
	if err := a.bindExpenseInvoice(r.Context(), userID, invoice); err != nil {
		if errors.Is(err, errInvalidExpenseInvoice) {
			http.Redirect(w, r, expensePageURL(r.Form, "", "invalid_invoice", false), http.StatusFound)
			return
		}
		http.Error(w, "could not save expense invoice", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, expensePageURL(r.Form, "invoice_bound", "", false), http.StatusFound)
}

func (a *app) handleExpenseInvoice(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	a.postExpenseInvoice(w, r)
}
