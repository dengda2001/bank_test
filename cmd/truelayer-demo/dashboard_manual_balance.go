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

// settleRentObligation creates an auditable income transaction for exactly the
// outstanding rent and immediately assigns it to that obligation. The lock and
// all writes share one database transaction, so double-clicks cannot overpay.
func (s *transactionService) settleRentObligation(ctx context.Context, userID, obligationID uint64) (paymentTransaction, error) {
	if userID == 0 || obligationID == 0 {
		return paymentTransaction{}, errors.New("userID and rent obligation ID are required")
	}

	var created paymentTransaction
	err := s.db.WithContext(ctx).Transaction(func(txdb *gorm.DB) error {
		var obligation rentObligation
		if err := txdb.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", obligationID, userID).First(&obligation).Error; err != nil {
			return err
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

		now := time.Now().UTC()
		period := monthStart(obligation.PeriodMonth)
		created = paymentTransaction{
			UserID:               userID,
			Source:               manualBalanceTransactionSource,
			SourceBatchID:        nullableString(manualBalanceTransactionSource),
			StableTransactionKey: recordID("manual-balance", now),
			Direction:            "income",
			AmountCents:          remainingCents,
			Currency:             currency,
			TransactionTime:      &now,
			Description:          manualBalanceTransactionDescription,
			Reference:            "rent-balance-" + period.Format("2006-01"),
			PayerName:            nullableString(tenantRow.Name),
			PayerNameKind:        "manual",
			ParsedPeriodMonth:    &period,
			ParsedPeriodSource:   manualBalanceTransactionSource,
			ParsedPeriodNote:     "dashboard manual balance",
			MatchReason:          "manual balance adjustment",
			MatchStatus:          "unmatched",
		}
		if err := txdb.Create(&created).Error; err != nil {
			return err
		}
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
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, dashboardManualBalanceRedirect(r.Form, "", "invalid_manual_balance"), http.StatusFound)
		return
	}
	obligationID, err := parsePositiveUint(r.Form.Get("obligation_id"))
	if err != nil {
		http.Redirect(w, r, dashboardManualBalanceRedirect(r.Form, "", "invalid_manual_balance"), http.StatusFound)
		return
	}
	_, err = newTransactionService(a.db).settleRentObligation(r.Context(), userID, obligationID)
	switch {
	case err == nil:
		http.Redirect(w, r, dashboardManualBalanceRedirect(r.Form, "manual_balance_saved", ""), http.StatusFound)
	case errors.Is(err, errManualBalanceNotNeeded):
		http.Redirect(w, r, dashboardManualBalanceRedirect(r.Form, "manual_balance_not_needed", ""), http.StatusFound)
	default:
		http.Redirect(w, r, dashboardManualBalanceRedirect(r.Form, "", "manual_balance_failed"), http.StatusFound)
	}
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
