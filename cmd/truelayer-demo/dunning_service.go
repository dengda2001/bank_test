package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type dunningPreview struct {
	Candidate dunningCandidate
	Message   *dunningMessage
	Error     string
}

type dunningPreviewBatch struct {
	Rows               []dunningPreview
	Sender             dunningSenderConfig
	SenderConfigured   bool
	ConfigurationError string
}

type dunningSendResult struct {
	Candidate       dunningCandidate
	Attempt         *dunningSendAttempt
	Error           string
	Skipped         bool
	RetryRequestKey string
}

func (s *dunningService) findDunningAttemptByRequest(ctx context.Context, userID, obligationID uint64, requestKey string) (dunningSendAttempt, bool, error) {
	var attempt dunningSendAttempt
	err := s.db.WithContext(ctx).Where("user_id = ? AND rent_obligation_id = ? AND request_key = ?", userID, obligationID, requestKey).First(&attempt).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return dunningSendAttempt{}, false, nil
	}
	return attempt, err == nil, err
}

func (s *dunningService) reserveDunningAttempt(ctx context.Context, attempt dunningSendAttempt) (dunningSendAttempt, bool, error) {
	existing, found, err := s.findDunningAttemptByRequest(ctx, attempt.UserID, attempt.RentObligationID, attempt.RequestKey)
	if err != nil {
		return dunningSendAttempt{}, false, err
	}
	if found {
		return existing, true, nil
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_, charge, err := lockRoomForRentObligation(tx, attempt.UserID, attempt.RentObligationID)
		if err != nil {
			return err
		}
		var obligation rentObligation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND id = ?", attempt.UserID, attempt.RentObligationID).First(&obligation).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errDunningFactsChanged
			}
			return err
		}
		if obligation.RecordStatus != obligationRecordActive || obligation.RentChargeID != charge.ID || obligation.RoomRentPlanID != charge.RoomRentPlanID || monthStart(obligation.PeriodMonth) != monthStart(attempt.PeriodMonth) {
			return errDunningFactsChanged
		}
		var allocations []paymentAllocation
		if err := tx.Where("user_id = ? AND rent_obligation_id = ?", attempt.UserID, obligation.ID).Find(&allocations).Error; err != nil {
			return err
		}
		var receipts []cashReceipt
		if err := tx.Where("user_id = ? AND rent_obligation_id = ?", attempt.UserID, obligation.ID).Find(&receipts).Error; err != nil {
			return err
		}
		projected := projectRentObligation(obligation, allocations, receipts, time.Now().UTC())
		if projected.ExpectedAmountCents != attempt.ExpectedAmountCents || projected.PaidAmountCents != attempt.PaidAmountCents || maxInt64(projected.ExpectedAmountCents-projected.PaidAmountCents, 0) != attempt.BalanceAmountCents {
			return errDunningFactsChanged
		}
		return tx.Create(&attempt).Error
	})
	if err != nil {
		existing, found, lookupErr := s.findDunningAttemptByRequest(ctx, attempt.UserID, attempt.RentObligationID, attempt.RequestKey)
		if lookupErr != nil {
			return dunningSendAttempt{}, false, err
		}
		if found {
			return existing, true, nil
		}
		return dunningSendAttempt{}, false, err
	}
	return attempt, false, nil
}

var errDunningFactsChanged = errors.New("rent facts changed after dunning preview")

func dunningAttemptSnapshot(userID uint64, candidate dunningCandidate, message *dunningMessage, sender dunningSenderConfig, serviceFrom, requestKey, status, reason string, retryOfAttemptID uint64, now time.Time) dunningSendAttempt {
	subject := "Dunning not sent"
	body := reason
	if message != nil {
		subject = message.Subject
		body = message.Body
	}
	var retryOf *uint64
	if retryOfAttemptID != 0 {
		retryOf = &retryOfAttemptID
	}
	return dunningSendAttempt{
		UserID:              userID,
		TenantID:            candidate.TenantID,
		RentObligationID:    candidate.ObligationID,
		PeriodMonth:         mustParseDunningPeriod(candidate.Period),
		RecipientEmail:      candidate.Email,
		TemplateKind:        messageTemplateKind(message),
		Subject:             subject,
		Body:                firstNonEmpty(body, reason, "No message was delivered."),
		ExpectedAmountCents: candidate.ExpectedCents,
		PaidAmountCents:     candidate.PaidCents,
		BalanceAmountCents:  candidate.BalanceCents,
		Currency:            firstNonEmpty(candidate.Currency, ledgerCurrencyEUR),
		SenderDisplayName:   sender.DisplayName,
		ReplyToEmail:        sender.ReplyToEmail,
		ServiceFromEmail:    serviceFrom,
		DeliveryStatus:      status,
		ErrorMessage:        nullableString(reason),
		OperationID:         recordID("dunning", now),
		RequestKey:          requestKey,
		RetryOfAttemptID:    retryOf,
		RequestedAt:         now,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
}

func messageTemplateKind(message *dunningMessage) string {
	if message == nil {
		return ""
	}
	return message.TemplateKind
}

func mustParseDunningPeriod(value string) time.Time {
	period, _ := parsePeriodMonth(value)
	return period
}

func (s *dunningService) loadSenderConfig(ctx context.Context, userID uint64) (dunningSenderConfig, error) {
	if s == nil || s.db == nil || userID == 0 {
		return dunningSenderConfig{}, errors.New("database and userID are required")
	}
	var config dunningSenderConfig
	err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&config).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return dunningSenderConfig{}, nil
	}
	return config, err
}

func (s *dunningService) saveSenderConfig(ctx context.Context, userID uint64, input dunningSenderConfig) (dunningSenderConfig, error) {
	if userID == 0 {
		return dunningSenderConfig{}, errors.New("userID is required")
	}
	input.UserID = userID
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.ReplyToEmail = strings.TrimSpace(input.ReplyToEmail)
	if err := validateDunningSenderConfig(input); err != nil {
		return dunningSenderConfig{}, err
	}
	input.ID = 0
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"display_name", "reply_to_email", "updated_at"}),
	}).Create(&input).Error; err != nil {
		return dunningSenderConfig{}, err
	}
	return s.loadSenderConfig(ctx, userID)
}

func (s *dunningService) loadDunningCandidate(ctx context.Context, userID, obligationID uint64, periodMonth time.Time, now time.Time) (dunningCandidate, *dunningSendAttempt, bool, error) {
	if userID == 0 || obligationID == 0 {
		return dunningCandidate{}, nil, false, errors.New("userID and obligationID are required")
	}
	periodMonth = monthStart(periodMonth)
	var obligation rentObligation
	err := s.db.WithContext(ctx).Where("id = ? AND user_id = ? AND period_month = ?", obligationID, userID, periodMonth).First(&obligation).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return dunningCandidate{}, nil, false, nil
	}
	if err != nil {
		return dunningCandidate{}, nil, false, err
	}
	var tenantRow tenant
	if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", obligation.TenantID, userID).First(&tenantRow).Error; err != nil {
		return dunningCandidate{}, nil, false, err
	}
	var charge rentCharge
	if obligation.RentChargeID != 0 {
		if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", obligation.RentChargeID, userID).First(&charge).Error; err != nil {
			return dunningCandidate{}, nil, false, err
		}
	}
	var allocations []paymentAllocation
	if err := s.db.WithContext(ctx).Where("rent_obligation_id = ? AND user_id = ?", obligation.ID, userID).Find(&allocations).Error; err != nil {
		return dunningCandidate{}, nil, false, err
	}
	var cashReceipts []cashReceipt
	if err := s.db.WithContext(ctx).Where("rent_obligation_id = ? AND user_id = ?", obligation.ID, userID).Find(&cashReceipts).Error; err != nil {
		return dunningCandidate{}, nil, false, err
	}
	obligation = projectRentObligation(obligation, allocations, cashReceipts, now)
	var attempts []dunningSendAttempt
	if err := s.db.WithContext(ctx).Where("user_id = ? AND rent_obligation_id = ?", userID, obligation.ID).Order("created_at DESC, id DESC").Find(&attempts).Error; err != nil {
		return dunningCandidate{}, nil, false, err
	}
	var latest *dunningSendAttempt
	sentToday := false
	for index := range attempts {
		attempt := &attempts[index]
		if latest == nil {
			latest = attempt
		}
		if attempt.DeliveryStatus == dunningDeliverySent && attempt.SentAt != nil && dunningDublinDate(*attempt.SentAt).Equal(dunningDublinDate(now)) {
			sentToday = true
		}
	}
	candidate := buildDunningCandidate(obligation, tenantRow, now, latest, charge)
	candidate.SentToday = sentToday
	candidate.DefaultSelected = candidate.Selectable && !candidate.SentToday
	return candidate, latest, true, nil
}

func (s *dunningService) preview(ctx context.Context, userID uint64, periodMonth time.Time, obligationIDs []uint64, now time.Time) (dunningPreviewBatch, error) {
	if userID == 0 {
		return dunningPreviewBatch{}, errors.New("userID is required")
	}
	ids := uniqueDunningIDs(obligationIDs)
	if len(ids) == 0 || len(ids) > dashboardMaxPageSize {
		return dunningPreviewBatch{}, errors.New("a current-page obligation selection is required")
	}
	sender, err := s.loadSenderConfig(ctx, userID)
	if err != nil {
		return dunningPreviewBatch{}, err
	}
	batch := dunningPreviewBatch{Rows: make([]dunningPreview, 0, len(ids)), Sender: sender, SenderConfigured: sender.ID != 0}
	if !batch.SenderConfigured {
		batch.ConfigurationError = "请先保存房东发件配置"
	}
	for _, obligationID := range ids {
		candidate, _, found, err := s.loadDunningCandidate(ctx, userID, obligationID, periodMonth, now)
		if err != nil {
			return dunningPreviewBatch{}, err
		}
		if !found {
			continue
		}
		row := dunningPreview{Candidate: candidate}
		if !batch.SenderConfigured {
			row.Error = batch.ConfigurationError
		} else if candidate.BalanceCents <= 0 {
			row.Error = "账单已缴清，不能催缴"
		} else if !candidate.EmailValid {
			row.Error = firstNonEmpty(candidate.EmailError, "租客邮箱无效")
		} else {
			message, err := buildDunningMessage(candidate, sender, now)
			if err != nil {
				row.Error = err.Error()
			} else {
				row.Message = &message
			}
		}
		batch.Rows = append(batch.Rows, row)
	}
	return batch, nil
}

func uniqueDunningIDs(ids []uint64) []uint64 {
	seen := make(map[uint64]struct{}, len(ids))
	result := make([]uint64, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

func (s *dunningService) send(ctx context.Context, userID uint64, periodMonth time.Time, obligationIDs []uint64, requestKey string, forceResend bool, retryOfAttemptID uint64, serviceFrom string, delivery dunningMailDelivery, now time.Time) ([]dunningSendResult, error) {
	if userID == 0 {
		return nil, errors.New("userID is required")
	}
	requestKey = strings.TrimSpace(requestKey)
	if err := validateDunningRequestKey(requestKey); err != nil {
		return nil, err
	}
	ids := uniqueDunningIDs(obligationIDs)
	if len(ids) == 0 || len(ids) > dashboardMaxPageSize {
		return nil, errors.New("a current-page obligation selection is required")
	}
	if retryOfAttemptID != 0 && len(ids) != 1 {
		return nil, errors.New("a retry must select exactly one obligation")
	}
	if delivery == nil {
		return nil, errors.New("dunning delivery is not configured")
	}
	if validator, ok := delivery.(dunningDeliveryValidator); ok {
		if err := validator.Validate(); err != nil {
			return nil, fmt.Errorf("SMTP is not configured: %w", err)
		}
	}
	sender, err := s.loadSenderConfig(ctx, userID)
	if err != nil {
		return nil, err
	}
	if sender.ID == 0 {
		return nil, errors.New("sender configuration is required")
	}
	if err := validateDunningSenderConfig(sender); err != nil {
		return nil, err
	}
	periodMonth = monthStart(periodMonth)
	results := make([]dunningSendResult, 0, len(ids))
	for _, obligationID := range ids {
		result := dunningSendResult{}
		candidate, _, found, err := s.loadDunningCandidate(ctx, userID, obligationID, periodMonth, now)
		if err != nil {
			return nil, err
		}
		if !found {
			result.Error = "账单不存在或不属于当前用户"
			results = append(results, result)
			continue
		}
		result.Candidate = candidate
		if retryOfAttemptID != 0 {
			var retry dunningSendAttempt
			if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ? AND rent_obligation_id = ? AND delivery_status = ?", retryOfAttemptID, userID, obligationID, dunningDeliveryFailed).First(&retry).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					result.Error = "只能重试失败的催缴记录"
					results = append(results, result)
					continue
				}
				return nil, err
			}
		}
		if candidate.BalanceCents <= 0 {
			result.Error = "账单已缴清，不能催缴"
			attempt := dunningAttemptSnapshot(userID, candidate, nil, sender, serviceFrom, requestKey, dunningDeliverySkipped, result.Error, retryOfAttemptID, now)
			reserved, _, err := s.reserveDunningAttempt(ctx, attempt)
			if err != nil {
				if errors.Is(err, errDunningFactsChanged) {
					result.Error, result.Skipped = "账单责任已更新，请刷新后重试", true
					results = append(results, result)
					continue
				}
				return nil, err
			}
			result.Attempt = &reserved
			result.Skipped = true
			results = append(results, result)
			continue
		}
		if !candidate.EmailValid {
			result.Error = firstNonEmpty(candidate.EmailError, "租客邮箱无效")
			attempt := dunningAttemptSnapshot(userID, candidate, nil, sender, serviceFrom, requestKey, dunningDeliveryFailed, result.Error, retryOfAttemptID, now)
			reserved, _, err := s.reserveDunningAttempt(ctx, attempt)
			if err != nil {
				if errors.Is(err, errDunningFactsChanged) {
					result.Error, result.Skipped = "账单责任已更新，请刷新后重试", true
					results = append(results, result)
					continue
				}
				return nil, err
			}
			result.Attempt = &reserved
			results = append(results, result)
			continue
		}
		if candidate.SentToday && !forceResend {
			result.Error = "今天已经成功发送，若需重发请确认"
			attempt := dunningAttemptSnapshot(userID, candidate, nil, sender, serviceFrom, requestKey, dunningDeliverySkipped, result.Error, retryOfAttemptID, now)
			reserved, _, err := s.reserveDunningAttempt(ctx, attempt)
			if err != nil {
				if errors.Is(err, errDunningFactsChanged) {
					result.Error, result.Skipped = "账单责任已更新，请刷新后重试", true
					results = append(results, result)
					continue
				}
				return nil, err
			}
			result.Attempt = &reserved
			result.Skipped = true
			results = append(results, result)
			continue
		}
		message, err := buildDunningMessage(candidate, sender, now)
		if err != nil {
			result.Error = err.Error()
			attempt := dunningAttemptSnapshot(userID, candidate, nil, sender, serviceFrom, requestKey, dunningDeliveryFailed, result.Error, retryOfAttemptID, now)
			reserved, _, err := s.reserveDunningAttempt(ctx, attempt)
			if err != nil {
				if errors.Is(err, errDunningFactsChanged) {
					result.Error, result.Skipped = "账单责任已更新，请刷新后重试", true
					results = append(results, result)
					continue
				}
				return nil, err
			}
			result.Attempt = &reserved
			results = append(results, result)
			continue
		}
		attempt := dunningAttemptSnapshot(userID, candidate, &message, sender, serviceFrom, requestKey, dunningDeliveryAccepted, "", retryOfAttemptID, now)
		reserved, existing, err := s.reserveDunningAttempt(ctx, attempt)
		if err != nil {
			if errors.Is(err, errDunningFactsChanged) {
				result.Error, result.Skipped = "账单责任已更新，请刷新后重试", true
				results = append(results, result)
				continue
			}
			return nil, err
		}
		if existing {
			result.Attempt = &reserved
			result.Error = stringValue(reserved.ErrorMessage)
			result.Skipped = reserved.DeliveryStatus != dunningDeliverySent
			results = append(results, result)
			continue
		}
		err = delivery.Send(ctx, dunningEmail{FromEmail: serviceFrom, FromName: sender.DisplayName, To: message.RecipientEmail, ReplyTo: message.ReplyToEmail, Subject: message.Subject, Body: message.Body})
		if err != nil {
			errorText := dunningErrorText(err)
			if updateErr := s.updateDunningAttemptFailure(ctx, userID, reserved.ID, errorText); updateErr != nil {
				return nil, updateErr
			}
			reserved.DeliveryStatus = dunningDeliveryFailed
			reserved.ErrorMessage = nullableString(errorText)
			result.Error = errorText
		} else {
			if updateErr := s.updateDunningAttemptSent(ctx, userID, reserved.ID, now); updateErr != nil {
				return nil, updateErr
			}
			reserved.DeliveryStatus = dunningDeliverySent
			reserved.SentAt = &now
		}
		result.Attempt = &reserved
		results = append(results, result)
	}
	return results, nil
}

func validateDunningRequestKey(value string) error {
	if value == "" || len([]rune(value)) > 191 || strings.ContainsAny(value, "\r\n") {
		return errors.New("dunning request key is invalid")
	}
	return nil
}

func (s *dunningService) updateDunningAttemptFailure(ctx context.Context, userID, attemptID uint64, reason string) error {
	return s.db.WithContext(ctx).Model(&dunningSendAttempt{}).Where("id = ? AND user_id = ? AND delivery_status = ?", attemptID, userID, dunningDeliveryAccepted).Updates(map[string]any{"delivery_status": dunningDeliveryFailed, "error_message": nullableString(reason)}).Error
}

func (s *dunningService) updateDunningAttemptSent(ctx context.Context, userID, attemptID uint64, sentAt time.Time) error {
	return s.db.WithContext(ctx).Model(&dunningSendAttempt{}).Where("id = ? AND user_id = ? AND delivery_status = ?", attemptID, userID, dunningDeliveryAccepted).Updates(map[string]any{"delivery_status": dunningDeliverySent, "sent_at": sentAt}).Error
}

func dunningErrorText(err error) string {
	if err == nil {
		return ""
	}
	text := sanitizeMailHeaderText(err.Error())
	if len([]rune(text)) > 512 {
		text = string([]rune(text)[:512])
	}
	return text
}
