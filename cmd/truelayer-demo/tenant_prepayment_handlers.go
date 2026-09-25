package main

import (
	"net/http"
	"net/url"
	"strconv"
)

func (a *app) handleTenantPrepaymentApply(w http.ResponseWriter, r *http.Request) {
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
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	tenantID, err := parsePositiveUint(r.Form.Get("tenant_id"))
	if err != nil {
		http.Error(w, "invalid tenant", http.StatusBadRequest)
		return
	}
	returnPage := func(code, value string) {
		query := url.Values{code: {value}}
		http.Redirect(w, r, "/tenants/"+strconv.FormatUint(tenantID, 10)+"?"+query.Encode(), http.StatusFound)
	}
	creditID, creditErr := parsePositiveUint(r.Form.Get("prepayment_id"))
	amountCents, amountErr := parseOptionalRentPlanAmountCents(r.Form.Get("amount"))
	period, periodErr := parsePeriodMonth(r.Form.Get("period"))
	if creditErr != nil || amountErr != nil || amountCents <= 0 || periodErr != nil {
		returnPage("error", "prepayment_invalid")
		return
	}
	if err := newMonthlyRentFactsService(a.db).ensureMonthlyRentFacts(r.Context(), userID, period, rentFactsIntentExplicitPayment); err != nil {
		returnPage("error", "prepayment_bill_missing")
		return
	}
	var obligation rentObligation
	if err := a.db.WithContext(r.Context()).Where("user_id = ? AND tenant_id = ? AND period_month = ? AND record_status <> ?", userID, tenantID, period, obligationRecordVoided).First(&obligation).Error; err != nil {
		returnPage("error", "prepayment_bill_missing")
		return
	}
	_, err = newTransactionService(a.db).applyTenantPrepayment(r.Context(), userID, creditID, obligation.ID, amountCents, r.Form.Get("request_key"))
	if err != nil {
		returnPage("error", "prepayment_apply_failed")
		return
	}
	returnPage("message", "prepayment_applied")
}
