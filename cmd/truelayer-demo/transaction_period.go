package main

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"
)

const (
	defaultRentNextMonthFromDay      = 15
	defaultRentAutoCurrentThroughDay = 5
	defaultRentAutoNextMonthFromDay  = 25
)

var (
	bankRentWord        = regexp.MustCompile(`(?i)\brent\b|租金|房租|月租`)
	bankNonRentWord     = regexp.MustCompile(`(?i)\b(?:deposits?|desposits?|refunds?|repayments?|expenses?|loans?|borrow(?:ed|ing)?)\b|押金|退款|报销|借款`)
	bankTxnDate         = regexp.MustCompile(`(?i)\btxndate\s*:\s*\d{1,2}\s*[a-z]{3,9}\s*20\d{2}\b`)
	bankDayMonthYear    = regexp.MustCompile(`(?i)\b\d{1,2}\s*[a-z]{3,9}\s*20\d{2}\b`)
	bankFullNumericDate = regexp.MustCompile(`\b(?:20\d{2}[-/.]\d{1,2}[-/.]\d{1,2}|\d{1,2}[-/.]\d{1,2}[-/.](?:20\d{2}|\d{2}))\b`)
	bankLocalTime       = func() *time.Location {
		location, err := time.LoadLocation("Europe/Dublin")
		if err != nil {
			return time.UTC
		}
		return location
	}()
)

type transactionPeriodEvidence struct {
	Month    *time.Time
	Explicit bool
	Label    string
}

// bankTransactionPeriod keeps an arrival-month suggestion separate from a
// month explicitly written in the bank description. Reference is intentionally
// absent: provider reference numbers can look like dates.
func bankTransactionPeriod(description string, transactionTime *time.Time) transactionPeriodEvidence {
	bankDate := firstNonZeroTime(transactionTime).In(bankLocalTime)
	periodText := bankTxnDate.ReplaceAllString(description, " ")
	periodText = bankDayMonthYear.ReplaceAllString(periodText, " ")
	periodText = bankFullNumericDate.ReplaceAllString(periodText, " ")
	if bankRentWord.MatchString(periodText) && !bankNonRentWord.MatchString(periodText) {
		if month, ok := parseReferencedPeriod(periodText, bankDate); ok {
			return transactionPeriodEvidence{Month: &month, Explicit: true, Label: "Description"}
		}
	}
	if transactionTime != nil && !transactionTime.IsZero() {
		month := time.Date(bankDate.Year(), bankDate.Month(), 1, 0, 0, 0, 0, time.UTC)
		if bankDate.Day() >= rentNextMonthFromDay() {
			month = month.AddDate(0, 1, 0)
		}
		return transactionPeriodEvidence{Month: &month, Label: "入账日期推测 · 待确认"}
	}
	return transactionPeriodEvidence{}
}

func rentNextMonthFromDay() int {
	value, err := strconv.Atoi(strings.TrimSpace(os.Getenv("RENT_NEXT_MONTH_FROM_DAY")))
	if err != nil || value < 1 || value > 31 {
		return defaultRentNextMonthFromDay
	}
	return value
}

func rentAutoWindowDays() (currentThrough, nextFrom int) {
	currentThrough = defaultRentAutoCurrentThroughDay
	nextFrom = defaultRentAutoNextMonthFromDay
	cutoff := rentNextMonthFromDay()
	if value, err := strconv.Atoi(strings.TrimSpace(os.Getenv("RENT_AUTO_CURRENT_THROUGH_DAY"))); err == nil && value >= 1 && value < cutoff {
		currentThrough = value
	}
	if value, err := strconv.Atoi(strings.TrimSpace(os.Getenv("RENT_AUTO_NEXT_MONTH_FROM_DAY"))); err == nil && value >= cutoff && value <= 31 {
		nextFrom = value
	}
	if currentThrough >= cutoff || nextFrom < cutoff {
		return 0, 32
	}
	return currentThrough, nextFrom
}

func transactionPeriodForModel(row paymentTransaction) transactionPeriodEvidence {
	if row.Source == "truelayer" {
		period := bankTransactionPeriod(row.Description, row.TransactionTime)
		if row.Direction != "income" && !period.Explicit {
			return transactionPeriodEvidence{}
		}
		return period
	}
	if row.ParsedPeriodMonth != nil && !row.ParsedPeriodMonth.IsZero() {
		month := monthStart(*row.ParsedPeriodMonth)
		return transactionPeriodEvidence{Month: &month, Explicit: true, Label: "已记录月份"}
	}
	return transactionPeriodEvidence{}
}

func (e transactionPeriodEvidence) display() string {
	if e.Month == nil {
		return ""
	}
	return monthStart(*e.Month).Format("2006-01")
}

func (e transactionPeriodEvidence) explicitMonth() *time.Time {
	if e.Explicit {
		return e.Month
	}
	return nil
}
