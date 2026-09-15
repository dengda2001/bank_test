package main

import "strings"

type rentDashboardBankMetrics struct {
	PendingCents     int64
	PendingCount     int
	OtherIncomeCents int64
	OtherIncomeCount int
}

func summarizeRentDashboardBankMetrics(transactions []paymentTransaction, allocations []paymentAllocation) rentDashboardBankMetrics {
	allocationsByTransaction := make(map[uint64][]paymentAllocation)
	for _, allocation := range allocations {
		allocationsByTransaction[allocation.PaymentTransactionID] = append(allocationsByTransaction[allocation.PaymentTransactionID], allocation)
	}
	metrics := rentDashboardBankMetrics{}
	for _, transaction := range transactions {
		pendingStatus := isPendingMatchStatus(transaction.MatchStatus) || transaction.MatchStatus == ""
		allocationSummary := summarizeTransactionAllocations(transaction, allocationsByTransaction[transaction.ID])
		if pendingStatus && allocationSummary.RemainingCents > 0 {
			metrics.PendingCount++
			if strings.EqualFold(strings.TrimSpace(transaction.Currency), ledgerCurrencyEUR) {
				metrics.PendingCents += allocationSummary.RemainingCents
			}
		}
		if !strings.EqualFold(strings.TrimSpace(transaction.Currency), ledgerCurrencyEUR) {
			continue
		}
		if allocationSummary.KindCents[allocationKindOther] > 0 {
			metrics.OtherIncomeCents += allocationSummary.KindCents[allocationKindOther]
			metrics.OtherIncomeCount++
		}
	}
	return metrics
}
