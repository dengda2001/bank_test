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
	amount, err := parsePositiveAmount(r.Form.Get("amount"))
	if err != nil || moneyToCents(amount) <= 0 {
		http.Redirect(w, r, "/billing?error=invalid_allocation", http.StatusFound)
		return
	}
	kind := strings.TrimSpace(r.Form.Get("allocation_kind"))
	if kind != allocationKindRent && kind != allocationKindDeposit && kind != allocationKindOther {
		http.Redirect(w, r, "/billing?error=invalid_allocation", http.StatusFound)
		return
	}
	tenantID, err := parseOptionalUint(r.Form.Get("tenant_id"))
	if err != nil {
		http.Redirect(w, r, "/billing?error=invalid_allocation", http.StatusFound)
		return
	}
	var obligationID uint64
	if kind == allocationKindRent {
		if tenantID == 0 {
			http.Redirect(w, r, "/billing?error=invalid_allocation", http.StatusFound)
			return
		}
		periodValue := strings.TrimSpace(r.Form.Get("period"))
		if periodValue == "" {
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
	service := newTransactionService(a.db)
	_, err = service.allocateTransaction(r.Context(), userID, transactionID, []transactionAllocationDraft{{
		TenantID:         tenantID,
		RentObligationID: obligationID,
		AmountCents:      moneyToCents(amount),
		Kind:             kind,
		Note:             r.Form.Get("note"),
	}}, strings.TrimSpace(r.Form.Get("idempotency_key")), "manual")
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
	a.handleTransactionAction(w, r, transactionActionRevokeAllocations)
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
