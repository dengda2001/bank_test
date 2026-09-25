package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

type transactionExpenseDrawerData struct {
	TransactionID      string
	Description        string
	Amount             string
	Date               string
	Account            string
	Category           string
	Properties         []expensePropertyOption
	Rooms              []expenseRoomOption
	SelectedPropertyID uint64
	SelectedRoomID     uint64
	CurrentInvoice     *expenseInvoiceView
	Files              []expenseAttachmentView
	Linked             bool
	CloseURL           string
	PostURL            string
	Error              string
}

func transactionExpenseActionURL(query url.Values, transactionID uint64) string {
	values := cloneQueryValues(query)
	for _, key := range []string{"detail", "match", "match_tenant", "match_history_page", "cash", "expense", "expense_link", "error", "message"} {
		values.Del(key)
	}
	values.Set("expense_link", strconv.FormatUint(transactionID, 10))
	return "/transactions?" + values.Encode()
}

func (a *app) loadTransactionExpenseDrawer(ctx context.Context, userID, transactionID uint64, query url.Values) (*transactionExpenseDrawerData, error) {
	if a.db == nil || userID == 0 || transactionID == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	var source paymentTransaction
	if err := a.db.WithContext(ctx).Where("id = ? AND user_id = ?", transactionID, userID).First(&source).Error; err != nil {
		return nil, err
	}
	if source.Direction != "expense" {
		return nil, gorm.ErrRecordNotFound
	}
	var expense manualExpense
	err := a.db.WithContext(ctx).Where("user_id = ? AND payment_transaction_id = ? AND record_status = ?", userID, transactionID, obligationRecordActive).First(&expense).Error
	linked := err == nil
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if !linked && source.Source != "truelayer" {
		return nil, gorm.ErrRecordNotFound
	}
	var propertyID, roomID uint64
	if linked {
		if expense.PropertyID != nil {
			propertyID = *expense.PropertyID
		}
		if expense.RoomID != nil {
			roomID = *expense.RoomID
		}
	}
	closeURL := transactionListURL(query)
	options, err := a.loadExpenseDrawerData(ctx, userID, "", propertyID, roomID, closeURL, "")
	if err != nil {
		return nil, err
	}
	drawer := &transactionExpenseDrawerData{
		TransactionID: strconv.FormatUint(transactionID, 10),
		Description:   firstNonEmpty(source.Description, source.Reference, "银行支出"),
		Amount:        formatMoney(centsToMoney(source.AmountCents), source.Currency, 2),
		Date:          formatTransactionTimestamp(source.TransactionTime),
		Account:       firstNonEmpty(stringValue(source.AccountName), "—"),
		Category:      firstNonEmpty(expense.Category, "其他"),
		Properties:    options.Properties, Rooms: options.Rooms,
		SelectedPropertyID: options.SelectedPropertyID, SelectedRoomID: options.SelectedRoomID,
		Linked: linked, CloseURL: closeURL,
		PostURL: "/transactions/expense-link?transaction_id=" + strconv.FormatUint(transactionID, 10) + "&return_to=" + url.QueryEscape(closeURL),
		Error:   query.Get("error"),
	}
	if linked {
		files, err := listExpenseAttachments(ctx, a.db, userID, []uint64{expense.ID})
		if err != nil {
			return nil, err
		}
		for _, file := range files {
			drawer.Files = append(drawer.Files, attachmentView(file))
		}
		invoices, err := a.loadCurrentExpenseInvoices(ctx, userID, []uint64{expense.ID})
		if err != nil {
			return nil, err
		}
		if invoice, ok := invoices[expense.ID]; ok {
			view := invoiceView(invoice)
			drawer.CurrentInvoice = &view
		}
	}
	return drawer, nil
}

func optionalTransactionExpenseInvoice(r *http.Request) (*manualExpenseInvoice, error) {
	if r.MultipartForm == nil {
		return nil, errInvalidExpenseInvoice
	}
	files := r.MultipartForm.File["invoice_file"]
	if len(files) == 0 || files[0].Filename == "" {
		for _, name := range []string{"invoice_number", "vendor", "invoice_date", "invoice_amount"} {
			if strings.TrimSpace(r.Form.Get(name)) != "" {
				return nil, errInvalidExpenseInvoice
			}
		}
		return nil, nil
	}
	if len(files) != 1 {
		return nil, errInvalidExpenseInvoice
	}
	file, header, err := r.FormFile("invoice_file")
	if err != nil {
		return nil, errInvalidExpenseInvoice
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxExpenseInvoiceBytes+1))
	if err != nil || len(data) > maxExpenseInvoiceBytes {
		return nil, errInvalidExpenseInvoice
	}
	invoice, err := parseExpenseInvoiceUpload(r.Form, data, header.Filename)
	if err != nil {
		return nil, err
	}
	return &invoice, nil
}

func (a *app) handleTransactionExpenseLink(w http.ResponseWriter, r *http.Request) {
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
		transactionID, _ := parsePositiveUint(r.URL.Query().Get("transaction_id"))
		redirectTransactionExpenseResult(w, r, transactionID, "error", "invalid_attachment")
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()
	transactionID, err := parsePositiveUint(r.Form.Get("transaction_id"))
	if err != nil {
		http.Error(w, "invalid transaction", http.StatusBadRequest)
		return
	}
	propertyID, err := parsePositiveUint(r.Form.Get("property_id"))
	if err != nil {
		redirectTransactionExpenseResult(w, r, transactionID, "error", "invalid_expense_link")
		return
	}
	roomID, err := parseOptionalUint(r.Form.Get("room_id"))
	if err != nil {
		redirectTransactionExpenseResult(w, r, transactionID, "error", "invalid_expense_link")
		return
	}
	var selectedRoom *uint64
	if roomID != 0 {
		selectedRoom = &roomID
	}
	uploads, err := expenseAttachmentUploads(r)
	if err != nil {
		redirectTransactionExpenseResult(w, r, transactionID, "error", "invalid_attachment")
		return
	}
	_, err = newExpenseService(a.db).saveTransactionExpense(r.Context(), userID, transactionExpenseInput{
		TransactionID: transactionID, PropertyID: propertyID, RoomID: selectedRoom,
		Category: r.Form.Get("category"), Attachments: uploads,
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		http.NotFound(w, r)
		return
	}
	if errors.Is(err, errInvalidTransactionExpense) || errors.Is(err, errInvalidExpenseInvoice) {
		redirectTransactionExpenseResult(w, r, transactionID, "error", "invalid_expense_link")
		return
	}
	if err != nil {
		http.Error(w, "could not save expense attribution", http.StatusInternalServerError)
		return
	}
	redirectTransactionExpenseResult(w, r, transactionID, "message", "expense_linked")
}

func redirectTransactionExpenseResult(w http.ResponseWriter, r *http.Request, transactionID uint64, key, value string) {
	target, _ := url.ParseRequestURI(transactionReturnTarget(r))
	query := target.Query()
	query.Set(key, value)
	if key == "error" {
		query.Set("expense_link", strconv.FormatUint(transactionID, 10))
	}
	http.Redirect(w, r, target.Path+"?"+query.Encode(), http.StatusFound)
}
