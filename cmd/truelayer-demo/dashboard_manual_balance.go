package main

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	manualBalanceTransactionSource      = "manual_balance"
	manualBalanceTransactionDescription = "手动平账"
	manualBalanceConfirmationSource     = "manual_balance"
)

var errManualBalanceNotNeeded = errors.New("rent obligation has no outstanding balance")
var errManualBalanceReasonRequired = errors.New("manual balance reason is required")

// settleRentObligation creates an auditable income transaction for exactly the
// outstanding rent and immediately assigns it to that obligation. The lock and
// all writes share one database transaction, so double-clicks cannot overpay.
func (s *transactionService) settleRentObligation(ctx context.Context, userID, obligationID uint64, reasons ...string) (paymentTransaction, error) {
	if userID == 0 || obligationID == 0 {
		return paymentTransaction{}, errors.New("userID and rent obligation ID are required")
	}
	reason := ""
	if len(reasons) > 0 {
		reason = strings.TrimSpace(reasons[0])
	}
	if reason == "" {
		return paymentTransaction{}, errManualBalanceReasonRequired
	}
	if len([]rune(reason)) > 512 {
		return paymentTransaction{}, errors.New("manual balance reason is too long")
	}

	var created paymentTransaction
	err := s.db.WithContext(ctx).Transaction(func(txdb *gorm.DB) error {
		now := time.Now().UTC()
		periodPlaceholder := monthStart(now)
		created = paymentTransaction{
			UserID:                 userID,
			Source:                 manualBalanceTransactionSource,
			SourceBatchID:          nullableString(manualBalanceTransactionSource),
			StableTransactionKey:   recordID("manual-balance", now),
			Direction:              "income",
			AmountCents:            1,
			Currency:               ledgerCurrencyEUR,
			TransactionTime:        &now,
			Description:            manualBalanceTransactionDescription,
			Reference:              "rent-balance-" + periodPlaceholder.Format("2006-01"),
			PayerNameKind:          "manual",
			ParsedPeriodMonth:      &periodPlaceholder,
			ParsedPeriodSource:     manualBalanceTransactionSource,
			ParsedPeriodNote:       "dashboard manual balance",
			MatchReason:            "manual balance adjustment",
			ManualAdjustmentReason: reason,
			MatchStatus:            "unmatched",
		}
		if err := txdb.Create(&created).Error; err != nil {
			return err
		}
		// This transaction is new and local to this write. Lock it before the
		// room/charge/obligation chain to keep the bank allocation lock order.
		var source paymentTransaction
		if err := txdb.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", created.ID, userID).First(&source).Error; err != nil {
			return err
		}
		lockedObligations, err := lockRentObligationRoomsInTx(txdb, userID, []uint64{obligationID})
		if err != nil {
			return err
		}
		obligation, ok := lockedObligations[obligationID]
		if !ok {
			return ErrRentFactsConflict
		}
		if obligation.RecordStatus == obligationRecordVoided {
			return errors.New("cannot settle a voided rent obligation")
		}
		currency, err := normalizeLedgerCurrency(obligation.Currency)
		if err != nil {
			return err
		}
		var tenantRow tenant
		if err := txdb.Where("id = ? AND user_id = ?", obligation.TenantID, userID).First(&tenantRow).Error; err != nil {
			return err
		}
		var allocations []paymentAllocation
		if err := txdb.Where("rent_obligation_id = ? AND user_id = ?", obligation.ID, userID).Find(&allocations).Error; err != nil {
			return err
		}
		var cashReceipts []cashReceipt
		if err := txdb.Where("rent_obligation_id = ? AND user_id = ?", obligation.ID, userID).Find(&cashReceipts).Error; err != nil {
			return err
		}
		projected := projectRentObligation(obligation, allocations, cashReceipts, time.Now().UTC())
		if projected.PaidAmountCents > projected.ExpectedAmountCents {
			return errors.New("rent obligation paid projection exceeds expected amount")
		}
		remainingCents := projected.ExpectedAmountCents - projected.PaidAmountCents
		if remainingCents <= 0 {
			return errManualBalanceNotNeeded
		}

		period := monthStart(obligation.PeriodMonth)
		if err := txdb.Model(&paymentTransaction{}).Where("id = ? AND user_id = ?", created.ID, userID).Updates(map[string]any{
			"amount_cents":        remainingCents,
			"currency":            currency,
			"payer_name":          nullableString(tenantRow.Name),
			"reference":           "rent-balance-" + period.Format("2006-01"),
			"parsed_period_month": period,
		}).Error; err != nil {
			return err
		}
		created.AmountCents = remainingCents
		created.Currency = currency
		created.PayerName = nullableString(tenantRow.Name)
		created.Reference = "rent-balance-" + period.Format("2006-01")
		created.ParsedPeriodMonth = &period
		summary, err := s.allocateTransactionInTx(txdb, userID, created.ID, []transactionAllocationDraft{{
			TenantID:         obligation.TenantID,
			RentObligationID: obligation.ID,
			AmountCents:      remainingCents,
			Kind:             allocationKindRent,
		}}, recordID("manual-balance-allocation", now), manualBalanceConfirmationSource)
		if err != nil {
			return err
		}
		created.MatchStatus = summary.Status
		created.MatchedTenantID = &obligation.TenantID
		return nil
	})
	return created, err
}

func (a *app) handleDashboardManualBalance(w http.ResponseWriter, r *http.Request) {
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
	redirect := func(message, actionError string) {
		http.Redirect(w, r, manualBalanceRedirectURL(r.Form, r.URL.Path, message, actionError), http.StatusFound)
	}
	if err := r.ParseForm(); err != nil {
		redirect("", "invalid_manual_balance")
		return
	}
	obligationID, err := parsePositiveUint(r.Form.Get("obligation_id"))
	if err != nil {
		redirect("", "invalid_manual_balance")
		return
	}
	reason := strings.TrimSpace(firstNonEmpty(r.Form.Get("reason"), r.Form.Get("manual_balance_reason")))
	if reason == "" {
		redirect("", "manual_balance_reason_required")
		return
	}
	// "处理方式" defaults to 匹配现有收款, which is exactly what
	// settleRentObligation already did. The other three options exist in the
	// form for prototype parity but have no accounting rule behind them yet, so
	// they must stop here -- before any write -- and say so.
	disposition := strings.TrimSpace(r.Form.Get("disposition"))
	if !settleDispositionImplemented(disposition) {
		redirect("", "manual_balance_disposition_unimplemented")
		return
	}
	_, err = newTransactionService(a.db).settleRentObligation(r.Context(), userID, obligationID, reason)
	switch {
	case err == nil:
		redirect("manual_balance_saved", "")
	case errors.Is(err, errManualBalanceNotNeeded):
		redirect("manual_balance_not_needed", "")
	case errors.Is(err, ErrRentFactsConflict):
		redirect("", "rent_facts_conflict")
	default:
		redirect("", "manual_balance_failed")
	}
}

func manualBalanceRedirectURL(values url.Values, requestPath, message, actionError string) string {
	returnValues := values
	legacyBillsRequest := strings.HasPrefix(requestPath, "/bills")
	if raw := strings.TrimSpace(values.Get("return_to")); raw != "" {
		if target, err := url.ParseRequestURI(raw); err == nil && !target.IsAbs() && target.Host == "" && target.User == nil && target.Fragment == "" && target.Path == "/rent-dashboard" {
			returnValues = target.Query()
			legacyBillsRequest = false
		}
	}
	if legacyBillsRequest && returnValues.Get("status") == "unpaid" {
		copied := make(url.Values, len(returnValues))
		for key, items := range returnValues {
			copied[key] = append([]string(nil), items...)
		}
		returnValues = copied
		returnValues.Set("status", "outstanding")
	}
	filters, err := rentWorkspaceFiltersFromQuery(returnValues)
	if err != nil {
		period, periodErr := parsePeriodMonth(strings.TrimSpace(returnValues.Get("period")))
		if periodErr != nil {
			period = monthStart(time.Now().UTC())
		}
		filters = defaultRentWorkspaceFilters(period)
		filters.Search = strings.TrimSpace(returnValues.Get("search"))
		if len([]rune(filters.Search)) > 191 {
			filters.Search = ""
		}
	}
	if legacyBillsRequest {
		filters.View = rentWorkspaceViewTenants
	}
	filters.PropertyID = 0
	filters.RoomID = 0
	target, err := url.Parse(rentWorkspaceURL(filters, filters.Page))
	if err != nil {
		return "/rent-dashboard?view=tenants"
	}
	query := target.Query()
	if message != "" {
		query.Set("message", message)
	}
	if actionError != "" {
		query.Set("error", actionError)
	}
	target.RawQuery = query.Encode()
	return target.String()
}

func dashboardManualBalanceRedirect(values url.Values, message, actionError string) string {
	filters, err := rentDashboardFiltersFromQuery(values)
	if err != nil {
		filters = defaultRentDashboardFilters()
	}
	period := strings.TrimSpace(values.Get("period"))
	if _, err := parsePeriodMonth(period); err != nil {
		period = ""
	}
	redirectURL := rentDashboardURL(period, filters.Search, filters.Status, filters.Sort, filters.Page, filters.PageSize)
	parsed, err := url.Parse(redirectURL)
	if err != nil {
		return "/rent-dashboard"
	}
	query := parsed.Query()
	if message != "" {
		query.Set("message", message)
	}
	if actionError != "" {
		query.Set("error", actionError)
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
