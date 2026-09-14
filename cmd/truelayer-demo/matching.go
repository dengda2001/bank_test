package main

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

type matchDecision struct {
	Status             string
	TenantID           uint64
	RentObligationID   uint64
	PeriodMonth        time.Time
	ConfirmationSource string
	Reason             string
	BackfillPayerID    bool
}

func decideRentMatch(tx paymentTransactionInput, tenants []tenant, obligations []rentObligation) matchDecision {
	if tx.Direction != "income" {
		return matchDecision{Status: "unmatched", Reason: "not income"}
	}
	if tx.PayerID != "" {
		matches := tenantsByPayerID(tenants, tx.PayerID)
		if len(matches) == 1 {
			return decideForTenant(tx, matches[0], obligations, "auto_id")
		}
		if len(matches) > 1 {
			return matchDecision{Status: "needs_review", Reason: "multiple tenants for payer id"}
		}
	}
	trustedNameMatches := tenantsByTrustedPayerName(tenants, tx.PayerName)
	if len(trustedNameMatches) == 1 {
		decision := decideForTenant(tx, trustedNameMatches[0], obligations, "auto_name")
		decision.Reason = "remembered payer name"
		return decision
	}
	if len(trustedNameMatches) > 1 {
		return matchDecision{Status: "needs_review", Reason: "multiple remembered payer names"}
	}
	nameMatches := tenantsByName(tenants, tx.PayerName)
	if len(nameMatches) == 1 {
		decision := decideForTenant(tx, nameMatches[0], obligations, "manual_name")
		if decision.Status == "matched" || decision.Status == "partial" {
			decision.Status = "candidate"
			decision.BackfillPayerID = tx.PayerID != "" && stringValue(nameMatches[0].PayerID) == ""
			decision.Reason = "single name candidate"
		}
		return decision
	}
	if len(nameMatches) > 1 {
		return matchDecision{Status: "needs_review", Reason: "multiple tenant name candidates"}
	}
	return matchDecision{Status: "unmatched", Reason: "no tenant candidate"}
}

func decideForTenant(tx paymentTransactionInput, row tenant, obligations []rentObligation, source string) matchDecision {
	obligation, ok := selectObligationForTransaction(tx, row.ID, obligations)
	return decideForTenantWithObligation(tx, row, obligation, ok, source)
}

func decideForTenantInPeriod(tx paymentTransactionInput, row tenant, obligations []rentObligation, period time.Time, source string) matchDecision {
	obligation, ok := selectObligationForPeriod(row.ID, period, obligations)
	return decideForTenantWithObligation(tx, row, obligation, ok, source)
}

func decideForTenantWithObligation(tx paymentTransactionInput, row tenant, obligation rentObligation, ok bool, source string) matchDecision {
	if !ok {
		return matchDecision{Status: "needs_review", TenantID: row.ID, ConfirmationSource: source, Reason: "no open obligation"}
	}
	remaining := obligation.ExpectedAmountCents - obligation.PaidAmountCents
	decision := matchDecision{
		TenantID:           row.ID,
		RentObligationID:   obligation.ID,
		PeriodMonth:        obligation.PeriodMonth,
		ConfirmationSource: source,
	}
	if strings.ToUpper(strings.TrimSpace(tx.Currency)) != strings.ToUpper(strings.TrimSpace(obligation.Currency)) {
		decision.Status = "needs_review"
		decision.Reason = "currency mismatch"
		return decision
	}
	if tx.AmountCents > remaining {
		decision.Status = "needs_review"
		decision.Reason = "overpayment"
		return decision
	}
	if tx.AmountCents < remaining {
		decision.Status = "partial"
		decision.Reason = "partial payment"
		return decision
	}
	decision.Status = "matched"
	decision.Reason = "exact payment"
	return decision
}

func selectObligationForTransaction(tx paymentTransactionInput, tenantID uint64, obligations []rentObligation) (rentObligation, bool) {
	text := strings.Join([]string{tx.Description, tx.Reference}, " ")
	transactionTime := time.Now().UTC()
	if tx.TransactionTime != nil {
		transactionTime = *tx.TransactionTime
	}
	targetPeriod := monthStart(transactionTime)
	if period, ok := parseReferencedPeriod(text, transactionTime); ok {
		targetPeriod = period
	}
	return selectObligationForPeriod(tenantID, targetPeriod, obligations)
}

func selectObligationForPeriod(tenantID uint64, targetPeriod time.Time, obligations []rentObligation) (rentObligation, bool) {
	targetPeriod = monthStart(targetPeriod)
	for _, obligation := range obligations {
		if obligation.TenantID == tenantID &&
			monthStart(obligation.PeriodMonth).Equal(targetPeriod) &&
			obligation.PaidAmountCents < obligation.ExpectedAmountCents {
			return obligation, true
		}
	}

	return rentObligation{}, false
}

func monthOffset(from, to time.Time) int {
	from = monthStart(from)
	to = monthStart(to)
	return (to.Year()-from.Year())*12 + int(to.Month()-from.Month())
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func tenantsByPayerID(tenants []tenant, payerID string) []tenant {
	payerID = normalizeMatchText(payerID)
	var matches []tenant
	for _, row := range tenants {
		if normalizeMatchText(stringValue(row.PayerID)) == payerID {
			matches = append(matches, row)
		}
	}
	return matches
}

func tenantsByName(tenants []tenant, payerName string) []tenant {
	payerName = normalizeMatchText(payerName)
	if payerName == "" {
		return nil
	}
	var matches []tenant
	for _, row := range tenants {
		if normalizeMatchText(row.Name) == payerName || normalizeMatchText(stringValue(row.PayerNameHint)) == payerName {
			matches = append(matches, row)
		}
	}
	return matches
}

func tenantsByTrustedPayerName(tenants []tenant, payerName string) []tenant {
	payerName = normalizeMatchText(payerName)
	if payerName == "" {
		return nil
	}
	var matches []tenant
	for _, row := range tenants {
		if hint := normalizeMatchText(stringValue(row.PayerNameHint)); hint != "" && hint == payerName {
			matches = append(matches, row)
		}
	}
	return matches
}

func normalizeMatchText(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(value))), " ")
}

var (
	yearMonthPattern      = regexp.MustCompile(`\b(20\d{2})[-/](0?[1-9]|1[0-2])\b`)
	monthYearPattern      = regexp.MustCompile(`\b(0?[1-9]|1[0-2])/(20\d{2})\b`)
	shortMonthYearPattern = regexp.MustCompile(`\b(0?[1-9]|1[0-2])/(\d{2})\b`)
	chineseYearMonthRegex = regexp.MustCompile(`(20\d{2})年(1[0-2]|0?[1-9])月`)
	chineseMonthRegex     = regexp.MustCompile(`(^|[^\d])(1[0-2]|0?[1-9])月`)
	englishMonthYearRegex = regexp.MustCompile(`(?i)\b(jan(?:uary)?|feb(?:ruary)?|mar(?:ch)?|apr(?:il)?|may|jun(?:e)?|jul(?:y)?|aug(?:ust)?|sep(?:t)?(?:ember)?|oct(?:ober)?|nov(?:ember)?|dec(?:ember)?)\s+(20\d{2})\b`)
	numericRentRegex      = regexp.MustCompile(`(?i)\brent\s+(0?[1-9]|1[0-2])\b`)
)

var monthNames = map[string]time.Month{
	"jan":       time.January,
	"january":   time.January,
	"feb":       time.February,
	"february":  time.February,
	"mar":       time.March,
	"march":     time.March,
	"apr":       time.April,
	"april":     time.April,
	"may":       time.May,
	"jun":       time.June,
	"june":      time.June,
	"jul":       time.July,
	"july":      time.July,
	"aug":       time.August,
	"august":    time.August,
	"sep":       time.September,
	"sept":      time.September,
	"september": time.September,
	"oct":       time.October,
	"october":   time.October,
	"nov":       time.November,
	"november":  time.November,
	"dec":       time.December,
	"december":  time.December,
}

func parseReferencedPeriod(text string, transactionTime time.Time) (time.Time, bool) {
	text = strings.ToLower(text)
	if m := yearMonthPattern.FindStringSubmatch(text); len(m) == 3 {
		return periodFromParts(m[1], m[2])
	}
	if m := monthYearPattern.FindStringSubmatch(text); len(m) == 3 {
		return periodFromParts(m[2], m[1])
	}
	if m := shortMonthYearPattern.FindStringSubmatch(text); len(m) == 3 {
		return periodFromParts("20"+m[2], m[1])
	}
	if m := chineseYearMonthRegex.FindStringSubmatch(text); len(m) == 3 {
		return periodFromParts(m[1], m[2])
	}
	if m := englishMonthYearRegex.FindStringSubmatch(text); len(m) == 3 {
		month, ok := monthNames[strings.ToLower(m[1])]
		if ok {
			year, err := strconv.Atoi(m[2])
			if err == nil {
				return time.Date(year, month, 1, 0, 0, 0, 0, time.UTC), true
			}
		}
	}
	words := regexp.MustCompile(`[a-z]+`).FindAllString(text, -1)
	for i, word := range words {
		month, ok := monthNames[word]
		if !ok {
			continue
		}
		if i+1 < len(words) {
			if year, err := strconv.Atoi(words[i+1]); err == nil && year >= 2000 {
				return time.Date(year, month, 1, 0, 0, 0, 0, time.UTC), true
			}
		}
		return inferYearForMonth(month, transactionTime), true
	}
	if m := chineseMonthRegex.FindStringSubmatch(text); len(m) == 3 {
		month, _ := strconv.Atoi(m[2])
		return inferYearForMonth(time.Month(month), transactionTime), true
	}
	if m := numericRentRegex.FindStringSubmatch(text); len(m) == 2 {
		month, _ := strconv.Atoi(m[1])
		return inferYearForMonth(time.Month(month), transactionTime), true
	}
	return time.Time{}, false
}

func periodFromParts(yearText, monthText string) (time.Time, bool) {
	year, err := strconv.Atoi(yearText)
	if err != nil {
		return time.Time{}, false
	}
	month, err := strconv.Atoi(monthText)
	if err != nil || month < 1 || month > 12 {
		return time.Time{}, false
	}
	return time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC), true
}

func inferYearForMonth(month time.Month, transactionTime time.Time) time.Time {
	txMonth := monthStart(transactionTime)
	year := txMonth.Year()
	candidate := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	if candidate.After(txMonth.AddDate(0, 1, 0)) {
		candidate = candidate.AddDate(-1, 0, 0)
	}
	if candidate.Before(txMonth.AddDate(0, -6, 0)) {
		candidate = candidate.AddDate(1, 0, 0)
	}
	return candidate
}
