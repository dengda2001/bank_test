package main

import "time"

type transactionPeriodEvidence struct {
	Month    *time.Time
	Explicit bool
	Label    string
}

// bankTransactionPeriod keeps an arrival-month suggestion separate from a
// month explicitly written in the bank description. Reference is intentionally
// absent: provider reference numbers can look like dates.
func bankTransactionPeriod(description string, transactionTime *time.Time) transactionPeriodEvidence {
	if month, ok := parseReferencedPeriod(description, firstNonZeroTime(transactionTime)); ok {
		return transactionPeriodEvidence{Month: &month, Explicit: true, Label: "Description"}
	}
	if transactionTime != nil && !transactionTime.IsZero() {
		month := monthStart(*transactionTime)
		return transactionPeriodEvidence{Month: &month, Label: "转账月份 · 待确认"}
	}
	return transactionPeriodEvidence{}
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
