package main

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

type matchDecision struct {
	Status                string
	TenantID              uint64
	RentObligationID      uint64
	AllocationAmountCents int64
	PeriodMonth           time.Time
	ConfirmationSource    string
	Reason                string
	BackfillPayerID       bool
}

// AIB provider references such as IE26090266821225 identify one bank
// transaction. They are not stable payer identities and must never create an
// identity match, even if an upstream payload places one in a payer-id field.
var aibTransactionReferencePattern = regexp.MustCompile(`(?i)^IE[0-9]{14}$`)

func isBankTransactionReference(value string) bool {
	return aibTransactionReferencePattern.MatchString(strings.TrimSpace(value))
}

func stablePayerID(value string) string {
	value = strings.TrimSpace(value)
	if isBankTransactionReference(value) {
		return ""
	}
	return value
}

func decideRentMatch(tx paymentTransactionInput, tenants []tenant, obligations []rentObligation) matchDecision {
	if tx.Direction != "income" {
		return matchDecision{Status: "unmatched", Reason: "not income"}
	}
	nameMatches := tenantsByName(tenants, tx.PayerName)
	if len(nameMatches) == 1 {
		decision := decideForTenant(tx, nameMatches[0], obligations, "manual_name")
		if decision.Status == "matched" || decision.Status == "partial" {
			decision.Status = "candidate"
			decision.Reason = "single name candidate"
		}
		return decision
	}
	if len(nameMatches) > 1 {
		return matchDecision{Status: "needs_review", Reason: "multiple tenant name candidates"}
	}
	return matchDecision{Status: "unmatched", Reason: "no tenant candidate"}
}

// decideStrictRentMatch is the only decision path used by background
// reconciliation. Bank descriptions and a narrow arrival-date window provide
// month evidence; identity must be unique and confirmed independently.
func decideStrictRentMatch(tx paymentTransactionInput, payers []tenantPayer, tenants []tenant, obligations []rentObligation) matchDecision {
	if tx.Direction != "income" {
		return matchDecision{Status: "unmatched", Reason: "not income"}
	}
	period, dateInferred := autoRentPeriod(tx)
	tx.ParsedPeriodMonth = period
	tenantByID := make(map[uint64]tenant, len(tenants))
	for _, row := range tenants {
		tenantByID[row.ID] = row
	}
	var payerMatches []tenant
	matchSource := "auto_name"
	if payerID := normalizeMatchText(stablePayerID(tx.PayerID)); payerID != "" {
		payerMatches = strictTenantsByPayerID(payers, tenantByID, payerID)
		if len(payerMatches) > 1 {
			return matchDecision{Status: "needs_review", Reason: "multiple tenants for payer id"}
		}
		if len(payerMatches) == 1 {
			matchSource = "auto_id"
		}
	}
	if normalizeMatchText(tx.PayerName) != "" {
		nameMatches := strictTenantsByPayerName(payers, tenantByID, tx.PayerName)
		if len(nameMatches) > 1 {
			return matchDecision{Status: "needs_review", Reason: "multiple tenants for remembered payer name"}
		}
		if len(nameMatches) == 1 {
			if len(payerMatches) == 1 && payerMatches[0].ID != nameMatches[0].ID {
				return matchDecision{Status: "needs_review", Reason: "payer identities disagree"}
			}
			if len(payerMatches) == 0 {
				payerMatches = nameMatches
				matchSource = "auto_name"
			}
		}
	}
	if tx.PayerNameKind == "confirmed" {
		exactMatches := tenantsByName(tenants, tx.PayerName)
		if len(exactMatches) > 1 {
			return matchDecision{Status: "needs_review", Reason: "multiple tenants with exact payer name"}
		}
		if len(exactMatches) == 1 {
			if len(payerMatches) == 1 && payerMatches[0].ID != exactMatches[0].ID {
				return matchDecision{Status: "needs_review", Reason: "payer name conflicts with remembered tenant"}
			}
			if len(payerMatches) == 0 {
				payerMatches = exactMatches
				matchSource = "auto_exact_name"
			}
		}
	}
	if len(payerMatches) == 0 {
		return matchDecision{Status: "unmatched", Reason: "no confirmed tenant identity"}
	}
	tenantRow := payerMatches[0]
	if tx.AmountCents <= 0 {
		return matchDecision{Status: "needs_review", TenantID: tenantRow.ID, ConfirmationSource: matchSource, Reason: "amount is invalid"}
	}
	if tx.ParsedPeriodMonth == nil || tx.ParsedPeriodMonth.IsZero() {
		return matchDecision{Status: "candidate", TenantID: tenantRow.ID, ConfirmationSource: matchSource, Reason: "explicit rent period is missing"}
	}
	if dateInferred {
		if strings.TrimSpace(tx.Currency) == "" {
			return matchDecision{Status: "needs_review", TenantID: tenantRow.ID, ConfirmationSource: matchSource, Reason: "currency is missing"}
		}
		count := 0
		for _, candidate := range obligations {
			if candidate.TenantID == tenantRow.ID && candidate.RecordStatus != obligationRecordVoided && monthStart(candidate.PeriodMonth).Equal(monthStart(*tx.ParsedPeriodMonth)) {
				count++
			}
		}
		if count > 1 {
			return matchDecision{Status: "needs_review", TenantID: tenantRow.ID, ConfirmationSource: matchSource, Reason: "multiple rent responsibilities for suggested month"}
		}
		obligation, ok := selectObligationForPeriod(tenantRow.ID, *tx.ParsedPeriodMonth, obligations)
		if ok && strings.TrimSpace(obligation.Currency) == "" {
			return matchDecision{Status: "needs_review", TenantID: tenantRow.ID, ConfirmationSource: matchSource, Reason: "rent currency is missing"}
		}
		decision := decideForTenantWithObligation(tx, tenantRow, obligation, ok, matchSource)
		if decision.Status == "partial" {
			decision.Status = "candidate"
			decision.AllocationAmountCents = 0
			decision.Reason = "date-based rent match requires exact unpaid amount"
		}
		return decision
	}
	if obligation, ok := obligationForTenantPeriod(tenantRow.ID, *tx.ParsedPeriodMonth, obligations); ok && obligation.PaidAmountCents >= obligation.ExpectedAmountCents {
		return decideForTenantWithObligation(tx, tenantRow, obligation, true, matchSource)
	}
	return decideForTenantInPeriod(tx, tenantRow, obligations, *tx.ParsedPeriodMonth, matchSource)
}

// autoRentPeriod uses an arrival date only at the ends of a month. A clearly
// non-rent description never qualifies for date-derived allocation.
func autoRentPeriod(tx paymentTransactionInput) (*time.Time, bool) {
	if tx.Source != "truelayer" {
		return tx.ParsedPeriodMonth, false
	}
	evidence := bankTransactionPeriod(tx.Description, tx.TransactionTime)
	if evidence.Explicit {
		return evidence.Month, false
	}
	if evidence.Month == nil || tx.TransactionTime == nil || bankNonRentWord.MatchString(tx.Description) {
		return nil, false
	}
	day := tx.TransactionTime.In(bankLocalTime).Day()
	currentThrough, nextFrom := rentAutoWindowDays()
	if day <= currentThrough || day >= nextFrom {
		return evidence.Month, true
	}
	return nil, false
}

func strictTenantsByPayerID(payers []tenantPayer, tenants map[uint64]tenant, payerID string) []tenant {
	payerID = normalizeMatchText(stablePayerID(payerID))
	if payerID == "" {
		return nil
	}
	seen := make(map[uint64]struct{})
	result := make([]tenant, 0)
	for _, payer := range payers {
		if payer.RemovedAt != nil || normalizeMatchText(stablePayerID(stringValue(payer.PayerID))) != payerID {
			continue
		}
		row, ok := tenants[payer.TenantID]
		if !ok {
			continue
		}
		if _, ok := seen[row.ID]; ok {
			continue
		}
		seen[row.ID] = struct{}{}
		result = append(result, row)
	}
	return result
}

func strictTenantsByPayerName(payers []tenantPayer, tenants map[uint64]tenant, payerName string) []tenant {
	name := normalizeMatchText(payerName)
	seen := make(map[uint64]struct{})
	result := make([]tenant, 0)
	for _, payer := range payers {
		if payer.RemovedAt != nil || payer.PayerNameNormalized != name {
			continue
		}
		row, ok := tenants[payer.TenantID]
		if !ok {
			continue
		}
		if _, ok := seen[row.ID]; ok {
			continue
		}
		seen[row.ID] = struct{}{}
		result = append(result, row)
	}
	return result
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
		TenantID:              row.ID,
		RentObligationID:      obligation.ID,
		AllocationAmountCents: tx.AmountCents,
		PeriodMonth:           obligation.PeriodMonth,
		ConfirmationSource:    source,
	}
	if strings.ToUpper(strings.TrimSpace(tx.Currency)) != strings.ToUpper(strings.TrimSpace(obligation.Currency)) {
		decision.Status = "needs_review"
		decision.Reason = "currency mismatch"
		return decision
	}
	if remaining <= 0 {
		decision.Status = "candidate"
		decision.AllocationAmountCents = 0
		decision.Reason = "referenced rent period is already covered; confirmation required"
		return decision
	}
	if tx.AmountCents > remaining {
		decision.Status = "partial"
		decision.AllocationAmountCents = remaining
		decision.Reason = "payment exceeds this tenant responsibility; remaining source needs review"
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
	if tx.Source == "truelayer" {
		period := bankTransactionPeriod(tx.Description, tx.TransactionTime).explicitMonth()
		if period == nil {
			return rentObligation{}, false
		}
		return selectObligationForPeriod(tenantID, *period, obligations)
	}
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
	obligation, ok := obligationForTenantPeriod(tenantID, targetPeriod, obligations)
	if !ok || obligation.PaidAmountCents >= obligation.ExpectedAmountCents {
		return rentObligation{}, false
	}
	return obligation, true
}

func obligationForTenantPeriod(tenantID uint64, targetPeriod time.Time, obligations []rentObligation) (rentObligation, bool) {
	targetPeriod = monthStart(targetPeriod)
	for _, obligation := range obligations {
		if obligation.TenantID == tenantID &&
			monthStart(obligation.PeriodMonth).Equal(targetPeriod) &&
			obligation.RecordStatus != obligationRecordVoided {
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
	return nil // Payer identities live in tenant_payers and use decideStrictRentMatch.
}

func tenantsByName(tenants []tenant, payerName string) []tenant {
	payerName = normalizeMatchText(payerName)
	if payerName == "" {
		return nil
	}
	var matches []tenant
	for _, row := range tenants {
		if normalizeMatchText(row.Name) == payerName {
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
	return nil // Remembered payer names are resolved from tenant_payers.
}

func normalizeMatchText(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(value))), " ")
}

var (
	yearMonthPattern             = regexp.MustCompile(`\b(20\d{2})[-/](0?[1-9]|1[0-2])\b`)
	monthYearPattern             = regexp.MustCompile(`\b(0?[1-9]|1[0-2])/(20\d{2})\b`)
	shortMonthYearPattern        = regexp.MustCompile(`\b(0?[1-9]|1[0-2])/(\d{2})\b`)
	chineseYearMonthRegex        = regexp.MustCompile(`(20\d{2})年(1[0-2]|0?[1-9])月`)
	chineseMonthRegex            = regexp.MustCompile(`(^|[^\d])(1[0-2]|0?[1-9])月`)
	compactEnglishMonthYearRegex = regexp.MustCompile(`(?i)\b(jan(?:uary)?|feb(?:ruary)?|mar(?:ch)?|apr(?:il)?|may|jun(?:e)?|jul(?:y)?|aug(?:ust)?|sep(?:t)?(?:ember)?|oct(?:ober)?|nov(?:ember)?|dec(?:ember)?)(20\d{2}|\d{2})\b`)
	englishMonthYearRegex        = regexp.MustCompile(`(?i)\b(jan(?:uary)?|feb(?:ruary)?|mar(?:ch)?|apr(?:il)?|may|jun(?:e)?|jul(?:y)?|aug(?:ust)?|sep(?:t)?(?:ember)?|oct(?:ober)?|nov(?:ember)?|dec(?:ember)?)\s+(20\d{2})\b`)
	numericRentRegex             = regexp.MustCompile(`(?i)\brent\s+(0?[1-9]|1[0-2])\b`)
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
	if m := compactEnglishMonthYearRegex.FindStringSubmatch(text); len(m) == 3 {
		month, ok := monthNames[strings.ToLower(m[1])]
		if ok {
			year, err := strconv.Atoi(m[2])
			if err == nil {
				if year < 100 {
					year += 2000
				}
				return time.Date(year, month, 1, 0, 0, 0, 0, time.UTC), true
			}
		}
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
