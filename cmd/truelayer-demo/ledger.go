package main

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	ledgerCurrencyEUR = "EUR"

	allocationKindRent        = "rent"
	allocationKindDeposit     = "deposit"
	allocationKindOther       = "other_income"
	allocationKindPrepayment  = "prepayment"
	allocationKindOtherIncome = allocationKindOther
	allocationStatusConfirmed = "confirmed"
	allocationStatusVoided    = "voided"

	obligationRecordActive = "active"
	obligationRecordVoided = "voided"
)

// ledgerAllocationCheck is the cross-layer input used before inserting an
// allocation. Amounts are integer cents; callers must load the current values
// inside their write transaction before constructing this check.
type ledgerAllocationCheck struct {
	UserID                  uint64
	SourceUserID            uint64
	TenantID                uint64
	ObligationTenantID      uint64
	SourceAmountCents       int64
	ExistingAllocatedCents  int64
	AmountCents             int64
	ObligationExpectedCents int64
	ObligationPaidCents     int64
	SourceCurrency          string
	ObligationCurrency      string
	Kind                    string
}

func normalizeLedgerCurrency(value string) (string, error) {
	if strings.EqualFold(strings.TrimSpace(value), ledgerCurrencyEUR) {
		return ledgerCurrencyEUR, nil
	}
	return "", errors.New("only EUR currency is supported in phase 1")
}

func validateLedgerAllocation(input ledgerAllocationCheck) error {
	if input.UserID == 0 || input.SourceUserID == 0 || input.UserID != input.SourceUserID {
		return errors.New("allocation user ownership mismatch")
	}
	if input.AmountCents <= 0 {
		return errors.New("allocation amount must be positive")
	}
	if input.SourceAmountCents <= 0 || input.ExistingAllocatedCents < 0 {
		return errors.New("source allocation budget is invalid")
	}
	if input.ExistingAllocatedCents > input.SourceAmountCents-input.AmountCents {
		return errors.New("allocation exceeds source amount")
	}
	sourceCurrency, err := normalizeLedgerCurrency(input.SourceCurrency)
	if err != nil {
		return err
	}
	switch input.Kind {
	case allocationKindRent:
		obligationCurrency, err := normalizeLedgerCurrency(input.ObligationCurrency)
		if err != nil {
			return err
		}
		if sourceCurrency != obligationCurrency {
			return fmt.Errorf("allocation currency mismatch: %s versus %s", sourceCurrency, obligationCurrency)
		}
		if input.TenantID == 0 || input.ObligationTenantID == 0 || input.TenantID != input.ObligationTenantID {
			return errors.New("allocation tenant ownership mismatch")
		}
		if input.ObligationExpectedCents <= 0 || input.ObligationPaidCents < 0 {
			return errors.New("obligation budget is invalid")
		}
		if input.AmountCents > input.ObligationExpectedCents-input.ObligationPaidCents {
			return errors.New("allocation exceeds obligation balance")
		}
	case allocationKindPrepayment:
		if input.TenantID == 0 {
			return errors.New("prepayment tenant is required")
		}
	case allocationKindDeposit, allocationKindOther:
		// Non-rent allocations do not consume a rent obligation balance. They
		// still consume the source transaction budget checked above.
	default:
		return errors.New("allocation kind is invalid")
	}
	return nil
}

func ledgerAllocationIsEffective(row paymentAllocation) bool {
	if row.Status != allocationStatusConfirmed {
		return false
	}
	kind := ledgerAllocationKind(row)
	return kind == allocationKindRent || kind == allocationKindDeposit || kind == allocationKindOther || kind == allocationKindPrepayment
}

func ledgerAllocationKind(row paymentAllocation) string {
	if row.AllocationKind == "" {
		// Rows written before the ledger migration are confirmed rent rows.
		return allocationKindRent
	}
	return row.AllocationKind
}

func ledgerPaidAmount(rows []paymentAllocation) int64 {
	var total int64
	for _, row := range rows {
		if ledgerAllocationIsEffective(row) && ledgerAllocationKind(row) == allocationKindRent {
			total += row.AmountCents
		}
	}
	return total
}

func ledgerObligationStatus(expected, paid int64, dueDate, now time.Time, recordStatus string) string {
	if recordStatus == obligationRecordVoided {
		return obligationRecordVoided
	}
	if paid >= expected {
		return "paid"
	}
	if paid > 0 {
		return "partial"
	}
	location, err := time.LoadLocation("Europe/Dublin")
	if err != nil {
		location = time.UTC
	}
	dueLocal := dueDate.In(location)
	nowLocal := now.In(location)
	if nowLocal.Year() > dueLocal.Year() || (nowLocal.Year() == dueLocal.Year() && nowLocal.YearDay() > dueLocal.YearDay()) {
		return "overdue"
	}
	return "open"
}

// projectLedgerObligation derives the cached rent total and payment status
// from effective allocations. The caller supplies only allocations for this
// obligation and persists the returned projection in its transaction.
func projectLedgerObligation(obligation rentObligation, allocations []paymentAllocation, now time.Time) rentObligation {
	paid := ledgerPaidAmount(allocations)
	obligation.PaidAmountCents = paid
	obligation.Status = ledgerObligationStatus(obligation.ExpectedAmountCents, paid, obligation.DueDate, now, obligation.RecordStatus)
	return obligation
}
