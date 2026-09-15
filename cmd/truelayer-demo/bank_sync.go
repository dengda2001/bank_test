package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	bankSyncModeInitialYear = "initial_year"
	bankSyncModeRefresh90d  = "refresh_90d"

	bankSyncStatusRunning   = "running"
	bankSyncStatusSucceeded = "succeeded"
	bankSyncStatusPartial   = "partial"
	bankSyncStatusFailed    = "failed"

	bankSyncAccountSucceeded = "succeeded"
	bankSyncAccountFailed    = "failed"
)

type bankSyncRun struct {
	ID            uint64 `gorm:"primaryKey"`
	UserID        uint64
	Provider      string
	Environment   string
	Mode          string
	RequestedFrom time.Time
	RequestedTo   time.Time
	Status        string
	StartedAt     time.Time
	FinishedAt    *time.Time
	ErrorMessage  *string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type bankSyncRunAccount struct {
	ID               uint64 `gorm:"primaryKey"`
	UserID           uint64
	BankSyncRunID    uint64
	AccountID        string
	AccountName      string
	RequestedFrom    time.Time
	RequestedTo      time.Time
	CoveredFrom      *time.Time
	CoveredTo        *time.Time
	Status           string
	TransactionCount int
	ErrorMessage     *string
	StartedAt        time.Time
	FinishedAt       *time.Time
}

type bankSyncAccountResult struct {
	Status       string
	ErrorMessage string
}

type bankSyncStore struct {
	db  *gorm.DB
	cfg config
}

func newBankSyncStore(db *gorm.DB, cfg config) *bankSyncStore {
	return &bankSyncStore{db: db, cfg: cfg}
}

func (a *app) startUserBankSync(ctx context.Context, userID uint64, mode, configuredFrom string, now time.Time) (*bankSyncStore, bankSyncRun, error) {
	if a.db == nil || userID == 0 {
		return nil, bankSyncRun{}, nil
	}
	store := newBankSyncStore(a.db, a.cfg)
	run, err := store.startRun(ctx, userID, mode, configuredFrom, now)
	if err != nil {
		return nil, bankSyncRun{}, err
	}
	return store, run, nil
}

func syncRequestWindow(mode, configuredFrom string, now time.Time) (string, time.Time, error) {
	if now.IsZero() {
		return "", time.Time{}, errors.New("sync time is required")
	}
	now = now.UTC()
	switch mode {
	case bankSyncModeInitialYear:
		from := now.AddDate(-1, 0, 0)
		if parsed, err := time.Parse(dateLayout, configuredFrom); err == nil && !parsed.After(now) {
			from = parsed
		}
		return from.Format(dateLayout), now, nil
	case bankSyncModeRefresh90d:
		fromText := refreshTransactionFrom(configuredFrom, now)
		from, err := time.Parse(dateLayout, fromText)
		if err != nil {
			return "", time.Time{}, err
		}
		return from.Format(dateLayout), now, nil
	default:
		return "", time.Time{}, errors.New("sync mode is invalid")
	}
}

func (s *bankSyncStore) startRun(ctx context.Context, userID uint64, mode, from string, now time.Time) (bankSyncRun, error) {
	if userID == 0 {
		return bankSyncRun{}, errors.New("userID is required")
	}
	requestedFrom, requestedTo, err := syncRequestWindow(mode, from, now)
	if err != nil {
		return bankSyncRun{}, err
	}
	fromDate, err := time.Parse(dateLayout, requestedFrom)
	if err != nil {
		return bankSyncRun{}, err
	}
	run := bankSyncRun{
		UserID:        userID,
		Provider:      "truelayer",
		Environment:   s.cfg.Environment,
		Mode:          mode,
		RequestedFrom: fromDate,
		RequestedTo:   requestedTo,
		Status:        bankSyncStatusRunning,
		StartedAt:     now.UTC(),
	}
	if err := s.db.WithContext(ctx).Create(&run).Error; err != nil {
		return bankSyncRun{}, err
	}
	return run, nil
}

func (s *bankSyncStore) finishRun(ctx context.Context, userID, runID uint64, result demoResult, fetchErr error) error {
	if userID == 0 || runID == 0 {
		return errors.New("userID and runID are required")
	}
	return s.db.WithContext(ctx).Transaction(func(txdb *gorm.DB) error {
		var run bankSyncRun
		if err := txdb.Where("id = ? AND user_id = ?", runID, userID).First(&run).Error; err != nil {
			return err
		}
		finishedAt := time.Now().UTC()
		accountResults := make([]bankSyncAccountResult, 0, len(result.Accounts))
		for _, account := range result.Accounts {
			status := bankSyncAccountSucceeded
			errorMessage := ""
			if len(account.Errors) > 0 {
				status = bankSyncAccountFailed
				errorMessage = safeSyncError(strings.Join(account.Errors, "; "))
			}
			accountResults = append(accountResults, bankSyncAccountResult{Status: status, ErrorMessage: errorMessage})
			row := bankSyncRunAccount{
				UserID:           userID,
				BankSyncRunID:    runID,
				AccountID:        account.Account.AccountID,
				AccountName:      firstNonEmpty(account.Account.DisplayName, account.Account.AccountID),
				RequestedFrom:    run.RequestedFrom,
				RequestedTo:      run.RequestedTo,
				Status:           status,
				TransactionCount: countAccountTransactions(account.Transactions),
				StartedAt:        run.StartedAt,
				FinishedAt:       &finishedAt,
			}
			if status == bankSyncAccountSucceeded {
				coveredFrom := run.RequestedFrom
				coveredTo := run.RequestedTo
				row.CoveredFrom = &coveredFrom
				row.CoveredTo = &coveredTo
			} else {
				row.ErrorMessage = &errorMessage
			}
			if err := txdb.Create(&row).Error; err != nil {
				return err
			}
		}
		status := summarizeBankSyncAccounts(accountResults)
		var runError *string
		if fetchErr != nil {
			status = bankSyncStatusFailed
			message := safeSyncError(fetchErr.Error())
			runError = &message
		} else {
			for i := range accountResults {
				if accountResults[i].ErrorMessage != "" {
					message := accountResults[i].ErrorMessage
					runError = &message
					break
				}
			}
		}
		return txdb.Model(&bankSyncRun{}).Where("id = ? AND user_id = ?", runID, userID).Updates(map[string]any{
			"status":        status,
			"finished_at":   finishedAt,
			"error_message": runError,
		}).Error
	})
}

func countAccountTransactions(raw json.RawMessage) int {
	if len(raw) == 0 {
		return 0
	}
	var rows rawTransactionList
	if err := json.Unmarshal(raw, &rows); err != nil {
		return 0
	}
	return len(rows.Results)
}

func safeSyncError(message string) string {
	message = strings.Join(strings.Fields(message), " ")
	if len(message) > 512 {
		return message[:512]
	}
	return message
}

func summarizeBankSyncAccounts(rows []bankSyncAccountResult) string {
	if len(rows) == 0 {
		return bankSyncStatusFailed
	}
	succeeded := 0
	for _, row := range rows {
		if row.Status == bankSyncAccountSucceeded {
			succeeded++
		}
	}
	switch {
	case succeeded == len(rows):
		return bankSyncStatusSucceeded
	case succeeded == 0:
		return bankSyncStatusFailed
	default:
		return bankSyncStatusPartial
	}
}

func syncResultHasSuccessfulAccount(result demoResult) bool {
	for _, account := range result.Accounts {
		if len(account.Errors) == 0 {
			return true
		}
	}
	return false
}
