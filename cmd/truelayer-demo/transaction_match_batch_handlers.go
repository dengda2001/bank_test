package main

import (
	"net/http"
	"strings"
)

func parseTransactionForm(r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return err
	}
	if strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "multipart/form-data") {
		return r.ParseMultipartForm(1 << 20)
	}
	return nil
}

func redirectTransactionBatchError(w http.ResponseWriter, r *http.Request, transactionID uint64, code string) {
	if transactionID == 0 {
		redirectTransactionResult(w, r, "error", code)
		return
	}
	var selectedTenantID uint64
	if value := strings.TrimSpace(r.Form.Get("match_tenant")); value != "" {
		selectedTenantID, _ = parsePositiveUint(value)
	}
	target, err := transactionReviewFailureURL(transactionReturnTarget(r), transactionID, selectedTenantID, code)
	if err != nil {
		redirectTransactionResult(w, r, "error", code)
		return
	}
	http.Redirect(w, r, target, http.StatusFound)
}

func (a *app) handleRentMatchBatchConfirmation(w http.ResponseWriter, r *http.Request) {
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
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := parseTransactionForm(r); err != nil {
		redirectTransactionResult(w, r, "error", "invalid_batch_match")
		return
	}
	transactionID, err := parsePositiveUint(r.Form.Get("transaction_id"))
	if err != nil {
		redirectTransactionResult(w, r, "error", "invalid_batch_match")
		return
	}
	tenantValues := allocationFormValues(r.Form, "tenant_id")
	periodValues := allocationFormValues(r.Form, "period")
	amountValues := allocationFormValues(r.Form, "amount")
	if len(tenantValues) == 0 || len(tenantValues) > 20 || len(tenantValues) != len(periodValues) || len(tenantValues) != len(amountValues) {
		redirectTransactionBatchError(w, r, transactionID, "invalid_batch_match")
		return
	}
	items := make([]rentMatchBatchItem, 0, len(tenantValues))
	for index, value := range tenantValues {
		tenantID, tenantErr := parsePositiveUint(value)
		period := strings.TrimSpace(periodValues[index])
		amountCents, amountErr := parseOptionalRentPlanAmountCents(amountValues[index])
		if tenantErr != nil || amountErr != nil || amountCents <= 0 {
			redirectTransactionBatchError(w, r, transactionID, "invalid_batch_match")
			return
		}
		items = append(items, rentMatchBatchItem{TenantID: tenantID, Period: period, AmountCents: amountCents})
	}
	rememberTenantID, err := parseOptionalUint(r.Form.Get("remember_tenant_id"))
	if err != nil {
		redirectTransactionBatchError(w, r, transactionID, "invalid_batch_match")
		return
	}
	_, err = newTransactionService(a.db).confirmRentMatchBatch(r.Context(), userID, transactionID, items, rememberTenantID, strings.TrimSpace(r.Form.Get("request_key")))
	if err != nil {
		redirectTransactionBatchError(w, r, transactionID, transactionFailureCode(err, "batch_match_failed"))
		return
	}
	redirectTransactionResult(w, r, "message", "rent_confirmed")
}
