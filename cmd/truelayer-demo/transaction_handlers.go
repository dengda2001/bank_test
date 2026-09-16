package main

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
)

func (a *app) handleTransactionAllocation(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := a.currentUserID(r)
	if !ok || a.db == nil {
		http.Error(w, "database session required", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/billing?error=invalid_allocation", http.StatusFound)
		return
	}
	transactionID, err := parsePositiveUint(r.Form.Get("transaction_id"))
	if err != nil {
		http.Redirect(w, r, "/billing?error=invalid_allocation", http.StatusFound)
		return
	}
	kinds := allocationFormValues(r.Form, "allocation_kind")
	amounts := allocationFormValues(r.Form, "amount")
	tenantIDs := allocationFormValues(r.Form, "tenant_id")
	periods := allocationFormValues(r.Form, "period")
	notes := allocationFormValues(r.Form, "note")
	if len(kinds) == 0 || len(kinds) != len(amounts) || (len(tenantIDs) > 0 && len(tenantIDs) != len(kinds)) || (len(periods) > 0 && len(periods) != len(kinds)) || (len(notes) > 0 && len(notes) != len(kinds)) || len(kinds) > 20 {
		http.Redirect(w, r, "/billing?error=invalid_allocation", http.StatusFound)
		return
	}
	drafts := make([]transactionAllocationDraft, 0, len(kinds))
	for index, kind := range kinds {
		amount, amountErr := parsePositiveAmount(amounts[index])
		if amountErr != nil || moneyToCents(amount) <= 0 {
			http.Redirect(w, r, "/billing?error=invalid_allocation", http.StatusFound)
			return
		}
		kind = strings.TrimSpace(kind)
		if kind != allocationKindRent && kind != allocationKindDeposit && kind != allocationKindOther {
			http.Redirect(w, r, "/billing?error=invalid_allocation", http.StatusFound)
			return
		}
		tenantValue := ""
		if len(tenantIDs) > 0 {
			tenantValue = tenantIDs[index]
		}
		tenantID, parseErr := parseOptionalUint(tenantValue)
		if parseErr != nil {
			http.Redirect(w, r, "/billing?error=invalid_allocation", http.StatusFound)
			return
		}
		periodValue := ""
		if len(periods) > 0 {
			periodValue = strings.TrimSpace(periods[index])
		}
		var obligationID uint64
		if kind == allocationKindRent {
			if tenantID == 0 || periodValue == "" {
				http.Redirect(w, r, "/billing?error=invalid_allocation", http.StatusFound)
				return
			}
			period, parseErr := parsePeriodMonth(periodValue)
			if parseErr != nil {
				http.Redirect(w, r, "/billing?error=invalid_allocation", http.StatusFound)
				return
			}
			var obligation rentObligation
			if err := a.db.WithContext(r.Context()).Where("id > 0 AND user_id = ? AND tenant_id = ? AND period_month = ?", userID, tenantID, period).First(&obligation).Error; err != nil {
				http.Redirect(w, r, "/billing?error=allocation_failed", http.StatusFound)
				return
			}
			obligationID = obligation.ID
		}
		note := ""
		if len(notes) > 0 {
			note = notes[index]
		}
		drafts = append(drafts, transactionAllocationDraft{TenantID: tenantID, RentObligationID: obligationID, AmountCents: moneyToCents(amount), Kind: kind, Note: note})
	}
	service := newTransactionService(a.db)
	_, err = service.allocateTransaction(r.Context(), userID, transactionID, drafts, strings.TrimSpace(r.Form.Get("idempotency_key")), "manual")
	if err != nil {
		http.Redirect(w, r, "/billing?error=allocation_failed", http.StatusFound)
		return
	}
	http.Redirect(w, r, "/billing?message=allocation_saved", http.StatusFound)
}

func (a *app) handleTransactionAction(w http.ResponseWriter, r *http.Request, actionKind string) {
	if !a.requireAuth(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := a.currentUserID(r)
	if !ok || a.db == nil {
		http.Error(w, "database session required", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/billing?error=invalid_transaction_action", http.StatusFound)
		return
	}
	transactionID, err := parsePositiveUint(r.Form.Get("transaction_id"))
	if err != nil {
		http.Redirect(w, r, "/billing?error=invalid_transaction_action", http.StatusFound)
		return
	}
	reason := r.Form.Get("reason")
	idempotencyKey := r.Form.Get("idempotency_key")
	service := newTransactionService(a.db)
	switch actionKind {
	case transactionActionIgnore:
		err = service.ignoreTransaction(r.Context(), userID, transactionID, reason, idempotencyKey)
	case transactionActionRestore:
		err = service.restoreTransaction(r.Context(), userID, transactionID, reason, idempotencyKey)
	case transactionActionRevokeAllocations:
		_, err = service.revokeTransactionAllocations(r.Context(), userID, transactionID, reason, idempotencyKey)
	default:
		err = errors.New("invalid transaction action")
	}
	if err != nil {
		http.Redirect(w, r, "/billing?error=transaction_action_failed", http.StatusFound)
		return
	}
	http.Redirect(w, r, "/billing?message=transaction_action_saved", http.StatusFound)
}

func (a *app) handleTransactionIgnore(w http.ResponseWriter, r *http.Request) {
	a.handleTransactionAction(w, r, transactionActionIgnore)
}

func (a *app) handleTransactionRestore(w http.ResponseWriter, r *http.Request) {
	a.handleTransactionAction(w, r, transactionActionRestore)
}

func (a *app) handleTransactionRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		a.handleTransactionRevokePreview(w, r)
		return
	}
	a.handleTransactionAction(w, r, transactionActionRevokeAllocations)
}

func (a *app) handleTransactionRematch(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := a.currentUserID(r)
	if !ok || a.db == nil {
		http.Error(w, "database session required", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/billing?error=invalid_rematch", http.StatusFound)
		return
	}
	transactionID, err := parsePositiveUint(r.Form.Get("transaction_id"))
	if err != nil {
		http.Redirect(w, r, "/billing?error=invalid_rematch", http.StatusFound)
		return
	}
	targetTenantID, err := parsePositiveUint(r.Form.Get("tenant_id"))
	if err != nil {
		http.Redirect(w, r, "/billing?error=invalid_rematch", http.StatusFound)
		return
	}
	periodValue := strings.TrimSpace(r.Form.Get("period"))
	if periodValue == "" {
		http.Redirect(w, r, "/billing?error=invalid_rematch", http.StatusFound)
		return
	}
	targetPeriod, err := parsePeriodMonth(periodValue)
	if err != nil {
		http.Redirect(w, r, "/billing?error=invalid_rematch", http.StatusFound)
		return
	}
	if err := newTransactionService(a.db).rematchRentAllocation(r.Context(), userID, transactionID, targetTenantID, targetPeriod); err != nil {
		http.Redirect(w, r, "/billing?error=rematch_failed", http.StatusFound)
		return
	}
	http.Redirect(w, r, "/billing?message=match_updated", http.StatusFound)
}

func (a *app) handleTransactionRevokePreview(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	userID, ok := a.currentUserID(r)
	if !ok || a.db == nil {
		http.Error(w, "database session required", http.StatusBadRequest)
		return
	}
	transactionID, err := parsePositiveUint(r.URL.Query().Get("transaction_id"))
	if err != nil {
		http.Redirect(w, r, "/billing?error=invalid_transaction_action", http.StatusFound)
		return
	}
	preview, err := newTransactionService(a.db).previewTransactionRevoke(r.Context(), userID, transactionID)
	if err != nil {
		http.Redirect(w, r, "/billing?error=transaction_action_failed", http.StatusFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := revokePreviewTemplate.Execute(w, transactionRevokePreviewDataFromModel(preview)); err != nil {
		http.Error(w, "unable to render transaction preview", http.StatusInternalServerError)
	}
}

func (a *app) handlePayerPreview(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := a.currentUserID(r)
	if !ok || a.db == nil {
		http.Error(w, "database session required", http.StatusBadRequest)
		return
	}
	rows, err := newTransactionService(a.db).previewHistoricalPayerMatches(r.Context(), userID)
	if err != nil {
		http.Error(w, "unable to build payer preview", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := payerPreviewTemplate.Execute(w, payerPreviewPageData{Rows: rows}); err != nil {
		http.Error(w, "unable to render payer preview", http.StatusInternalServerError)
	}
}

func (a *app) handlePayerConfirm(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := a.currentUserID(r)
	if !ok || a.db == nil {
		http.Error(w, "database session required", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/billing?error=invalid_payer_confirmation", http.StatusFound)
		return
	}
	transactionID, err := parsePositiveUint(r.Form.Get("transaction_id"))
	if err != nil {
		http.Redirect(w, r, "/billing?error=invalid_payer_confirmation", http.StatusFound)
		return
	}
	tenantID, err := parsePositiveUint(r.Form.Get("tenant_id"))
	if err != nil {
		http.Redirect(w, r, "/billing?error=invalid_payer_confirmation", http.StatusFound)
		return
	}
	periodValue := strings.TrimSpace(r.Form.Get("period"))
	if periodValue == "" {
		http.Redirect(w, r, "/billing?error=invalid_payer_confirmation", http.StatusFound)
		return
	}
	period, err := parsePeriodMonth(periodValue)
	if err != nil {
		http.Redirect(w, r, "/billing?error=invalid_payer_confirmation", http.StatusFound)
		return
	}
	rememberPayer := r.Form.Get("remember_payer") != "0"
	if err := newTransactionService(a.db).confirmHistoricalPayerMatch(r.Context(), userID, transactionID, tenantID, period, rememberPayer); err != nil {
		http.Redirect(w, r, "/billing?error=payer_confirmation_failed", http.StatusFound)
		return
	}
	http.Redirect(w, r, "/billing?message=payer_confirmed", http.StatusFound)
}

func parsePositiveUint(value string) (uint64, error) {
	parsed, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
	if err != nil || parsed == 0 {
		return 0, errors.New("positive integer is required")
	}
	return parsed, nil
}

func parseOptionalUint(value string) (uint64, error) {
	if strings.TrimSpace(value) == "" {
		return 0, nil
	}
	return parsePositiveUint(value)
}

func allocationFormValues(values map[string][]string, name string) []string {
	if items := values[name+"[]"]; len(items) > 0 {
		return items
	}
	if item, ok := values[name]; ok {
		return item
	}
	return nil
}
