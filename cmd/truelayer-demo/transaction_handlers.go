package main

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func transactionReturnTarget(r *http.Request) string {
	fallback := "/billing"
	if strings.HasPrefix(r.URL.Path, "/transactions/") {
		fallback = "/transactions"
	}
	raw := strings.TrimSpace(r.FormValue("return_to"))
	target, err := url.ParseRequestURI(raw)
	if err != nil || target.IsAbs() || target.Host != "" || (target.Path != "/billing" && target.Path != "/transactions") {
		return fallback
	}
	allowed := map[string]bool{"match_status": true, "scope": true, "period": true, "payer": true, "tenant_id": true, "direction": true, "rent_period": true, "allocation": true, "sort": true, "page": true, "page_size": true, "pending": true, "arrival_from": true, "arrival_to": true}
	query := url.Values{}
	for key, values := range target.Query() {
		if !allowed[key] || len(values) != 1 || len(values[0]) > 191 {
			continue
		}
		query.Set(key, values[0])
	}
	if len(query) == 0 {
		return target.Path
	}
	return target.Path + "?" + query.Encode()
}

func redirectTransactionResult(w http.ResponseWriter, r *http.Request, key, value string) {
	target := transactionReturnTarget(r)
	parsed, _ := url.ParseRequestURI(target)
	query := parsed.Query()
	query.Set(key, value)
	http.Redirect(w, r, parsed.Path+"?"+query.Encode(), http.StatusFound)
}

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
		redirectTransactionResult(w, r, "error", "invalid_allocation")
		return
	}
	transactionID, err := parsePositiveUint(r.Form.Get("transaction_id"))
	if err != nil {
		redirectTransactionResult(w, r, "error", "invalid_allocation")
		return
	}
	kinds := allocationFormValues(r.Form, "allocation_kind")
	amounts := allocationFormValues(r.Form, "amount")
	tenantIDs := allocationFormValues(r.Form, "tenant_id")
	periods := allocationFormValues(r.Form, "period")
	notes := allocationFormValues(r.Form, "note")
	if len(kinds) == 0 || len(kinds) != len(amounts) || (len(tenantIDs) > 0 && len(tenantIDs) != len(kinds)) || (len(periods) > 0 && len(periods) != len(kinds)) || (len(notes) > 0 && len(notes) != len(kinds)) || len(kinds) > 20 {
		redirectTransactionResult(w, r, "error", "invalid_allocation")
		return
	}
	drafts := make([]transactionAllocationDraft, 0, len(kinds))
	for index, kind := range kinds {
		amount, amountErr := parsePositiveAmount(amounts[index])
		if amountErr != nil || moneyToCents(amount) <= 0 {
			redirectTransactionResult(w, r, "error", "invalid_allocation")
			return
		}
		kind = strings.TrimSpace(kind)
		if kind != allocationKindRent && kind != allocationKindDeposit && kind != allocationKindOther {
			redirectTransactionResult(w, r, "error", "invalid_allocation")
			return
		}
		tenantValue := ""
		if len(tenantIDs) > 0 {
			tenantValue = tenantIDs[index]
		}
		tenantID, parseErr := parseOptionalUint(tenantValue)
		if parseErr != nil {
			redirectTransactionResult(w, r, "error", "invalid_allocation")
			return
		}
		periodValue := ""
		if len(periods) > 0 {
			periodValue = strings.TrimSpace(periods[index])
		}
		var obligationID uint64
		if kind == allocationKindRent {
			if tenantID == 0 || periodValue == "" {
				redirectTransactionResult(w, r, "error", "invalid_allocation")
				return
			}
			period, parseErr := parsePeriodMonth(periodValue)
			if parseErr != nil {
				redirectTransactionResult(w, r, "error", "invalid_allocation")
				return
			}
			var obligation rentObligation
			if err := a.db.WithContext(r.Context()).Where("id > 0 AND user_id = ? AND tenant_id = ? AND period_month = ?", userID, tenantID, period).First(&obligation).Error; err != nil {
				redirectTransactionResult(w, r, "error", "allocation_failed")
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
		redirectTransactionResult(w, r, "error", "allocation_failed")
		return
	}
	redirectTransactionResult(w, r, "message", "allocation_saved")
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
		redirectTransactionResult(w, r, "error", "invalid_transaction_action")
		return
	}
	transactionID, err := parsePositiveUint(r.Form.Get("transaction_id"))
	if err != nil {
		redirectTransactionResult(w, r, "error", "invalid_transaction_action")
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
		redirectTransactionResult(w, r, "error", "transaction_action_failed")
		return
	}
	redirectTransactionResult(w, r, "message", "transaction_action_saved")
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
		redirectTransactionResult(w, r, "error", "invalid_rematch")
		return
	}
	transactionID, err := parsePositiveUint(r.Form.Get("transaction_id"))
	if err != nil {
		redirectTransactionResult(w, r, "error", "invalid_rematch")
		return
	}
	targetTenantID, err := parsePositiveUint(r.Form.Get("tenant_id"))
	if err != nil {
		redirectTransactionResult(w, r, "error", "invalid_rematch")
		return
	}
	periodValue := strings.TrimSpace(r.Form.Get("period"))
	if periodValue == "" {
		redirectTransactionResult(w, r, "error", "invalid_rematch")
		return
	}
	targetPeriod, err := parsePeriodMonth(periodValue)
	if err != nil {
		redirectTransactionResult(w, r, "error", "invalid_rematch")
		return
	}
	if err := newTransactionService(a.db).rematchRentAllocation(r.Context(), userID, transactionID, targetTenantID, targetPeriod); err != nil {
		redirectTransactionResult(w, r, "error", "rematch_failed")
		return
	}
	redirectTransactionResult(w, r, "message", "match_updated")
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
		redirectTransactionResult(w, r, "error", "invalid_transaction_action")
		return
	}
	preview, err := newTransactionService(a.db).previewTransactionRevoke(r.Context(), userID, transactionID)
	if err != nil {
		redirectTransactionResult(w, r, "error", "transaction_action_failed")
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
