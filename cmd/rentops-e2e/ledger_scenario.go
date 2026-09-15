package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var e2eTransactionIDPattern = regexp.MustCompile(`name="transaction_id" value="([1-9][0-9]*)"`)
var e2eTransactionStatusPattern = regexp.MustCompile(`class="status status-link ([^"]+)"`)

func (c *e2eHTTPClient) ledgerScenario(ctx context.Context, manifest e2eFixtureManifest, tenantID uint64) e2eScenarioReport {
	scenario := e2eScenarioReport{Name: "ledger-actions", Status: "running", Steps: []e2eStepReport{}}
	fail := func(message string) e2eScenarioReport {
		scenario.Status = "failed"
		scenario.Error = message
		return scenario
	}
	if tenantID == 0 {
		return fail("ledger scenario requires a tenant ID")
	}
	initialResponse, err := c.do(ctx, http.MethodGet, "/billing", nil)
	initialStep := e2eHTTPStep(http.MethodGet, "/billing", map[string]any{
		"status_code":       http.StatusOK,
		"transaction_count": manifest.Expected.BankTransactionCount,
		"transaction_ids":   "all fixture IDs are discoverable",
	}, initialResponse, err)
	transactionIDs := map[string]uint64{}
	if err == nil {
		for _, transaction := range manifest.Bank.Transactions {
			id, extractErr := extractE2ETransactionID(initialResponse.Body, transaction.ProviderTransactionID)
			if extractErr != nil {
				err = fmt.Errorf("%s: %w", transaction.ProviderTransactionID, extractErr)
				break
			}
			transactionIDs[transaction.ProviderTransactionID] = id
		}
		initialStep.Actual.(map[string]any)["transaction_count"] = len(transactionIDs)
		initialStep.Actual.(map[string]any)["transaction_ids"] = len(transactionIDs) == manifest.Expected.BankTransactionCount
		initialStep.Passed = initialResponse.StatusCode == http.StatusOK && err == nil && len(transactionIDs) == manifest.Expected.BankTransactionCount
		if !initialStep.Passed {
			initialStep.Error = "billing page did not expose every imported transaction ID"
		}
	}
	scenario.Steps = append(scenario.Steps, initialStep)
	if !initialStep.Passed {
		return fail("ledger scenario could not discover imported transactions")
	}

	fullID := transactionIDs[manifest.Bank.Transactions[0].ProviderTransactionID]
	fullConfirmPath := "/billing/confirm"
	fullConfirmResponse, err := c.do(ctx, http.MethodPost, fullConfirmPath, url.Values{
		"transaction_id": {strconv.FormatUint(fullID, 10)},
		"tenant_id":      {strconv.FormatUint(tenantID, 10)},
		"period":         {"2026-09"},
		"remember_payer": {"1"},
	})
	fullConfirmStep := e2eHTTPStep(http.MethodPost, fullConfirmPath, map[string]any{
		"status_code": http.StatusFound,
		"location":    "/billing?message=rent_confirmed",
	}, fullConfirmResponse, err)
	if err == nil {
		fullConfirmStep.Passed = fullConfirmResponse.StatusCode == http.StatusFound && fullConfirmResponse.Location == "/billing?message=rent_confirmed"
		if !fullConfirmStep.Passed {
			fullConfirmStep.Error = "full rent confirmation did not succeed"
		}
	}
	scenario.Steps = append(scenario.Steps, fullConfirmStep)
	if !fullConfirmStep.Passed {
		return fail("full rent confirmation failed")
	}

	partialID := transactionIDs[manifest.Bank.Transactions[1].ProviderTransactionID]
	partialPath := "/billing/allocate"
	partialResponse, err := c.do(ctx, http.MethodPost, partialPath, url.Values{
		"transaction_id":    {strconv.FormatUint(partialID, 10)},
		"allocation_kind[]": {"rent", "deposit"},
		"tenant_id[]":       {strconv.FormatUint(tenantID, 10), strconv.FormatUint(tenantID, 10)},
		"period[]":          {"2026-08", ""},
		"amount[]":          {"300.00", "100.00"},
		"note[]":            {"", manifest.RunID + " deposit allocation"},
		"idempotency_key":   {manifest.RunID + "-allocation-partial"},
	})
	partialStep := e2eHTTPStep(http.MethodPost, partialPath, map[string]any{
		"status_code": http.StatusFound,
		"location":    "/billing?message=allocation_saved",
	}, partialResponse, err)
	if err == nil {
		partialStep.Passed = partialResponse.StatusCode == http.StatusFound && partialResponse.Location == "/billing?message=allocation_saved"
		if !partialStep.Passed {
			partialStep.Error = "rent/deposit split did not succeed"
		}
	}
	scenario.Steps = append(scenario.Steps, partialStep)
	if !partialStep.Passed {
		return fail("rent/deposit split failed")
	}

	crossMonthID := transactionIDs[manifest.Bank.Transactions[3].ProviderTransactionID]
	crossMonthResponse, err := c.do(ctx, http.MethodPost, partialPath, url.Values{
		"transaction_id":  {strconv.FormatUint(crossMonthID, 10)},
		"allocation_kind": {"rent"},
		"tenant_id":       {strconv.FormatUint(tenantID, 10)},
		"period":          {"2026-08"},
		"amount":          {"300.00"},
		"idempotency_key": {manifest.RunID + "-allocation-cross-month"},
	})
	crossMonthStep := e2eHTTPStep(http.MethodPost, partialPath, map[string]any{
		"status_code": http.StatusFound,
		"location":    "/billing?message=allocation_saved",
	}, crossMonthResponse, err)
	if err == nil {
		crossMonthStep.Passed = crossMonthResponse.StatusCode == http.StatusFound && crossMonthResponse.Location == "/billing?message=allocation_saved"
		if !crossMonthStep.Passed {
			crossMonthStep.Error = "cross-month rent allocation did not succeed"
		}
	}
	scenario.Steps = append(scenario.Steps, crossMonthStep)
	if !crossMonthStep.Passed {
		return fail("cross-month rent allocation failed")
	}

	otherID := transactionIDs[manifest.Bank.Transactions[4].ProviderTransactionID]
	otherResponse, err := c.do(ctx, http.MethodPost, partialPath, url.Values{
		"transaction_id":  {strconv.FormatUint(otherID, 10)},
		"allocation_kind": {"other_income"},
		"amount":          {"50.00"},
		"note":            {manifest.RunID + " other income"},
		"idempotency_key": {manifest.RunID + "-allocation-other"},
	})
	otherStep := e2eHTTPStep(http.MethodPost, partialPath, map[string]any{
		"status_code": http.StatusFound,
		"location":    "/billing?message=allocation_saved",
	}, otherResponse, err)
	if err == nil {
		otherStep.Passed = otherResponse.StatusCode == http.StatusFound && otherResponse.Location == "/billing?message=allocation_saved"
		if !otherStep.Passed {
			otherStep.Error = "other income allocation did not succeed"
		}
	}
	scenario.Steps = append(scenario.Steps, otherStep)
	if !otherStep.Passed {
		return fail("other income allocation failed")
	}

	foreignID := transactionIDs[manifest.Bank.Transactions[2].ProviderTransactionID]
	foreignResponse, err := c.do(ctx, http.MethodPost, partialPath, url.Values{
		"transaction_id":  {strconv.FormatUint(foreignID, 10)},
		"allocation_kind": {"rent"},
		"tenant_id":       {strconv.FormatUint(tenantID, 10)},
		"period":          {"2026-09"},
		"amount":          {"25.00"},
		"idempotency_key": {manifest.RunID + "-allocation-foreign"},
	})
	foreignStep := e2eHTTPStep(http.MethodPost, partialPath, map[string]any{
		"status_code": http.StatusFound,
		"location":    "/billing?error=allocation_failed",
	}, foreignResponse, err)
	if err == nil {
		foreignStep.Passed = foreignResponse.StatusCode == http.StatusFound && foreignResponse.Location == "/billing?error=allocation_failed"
		if !foreignStep.Passed {
			foreignStep.Error = "non-EUR rent allocation was not rejected"
		}
	}
	scenario.Steps = append(scenario.Steps, foreignStep)
	if !foreignStep.Passed {
		return fail("non-EUR allocation rejection failed")
	}

	ignorePath := "/billing/ignore"
	ignoreForm := url.Values{
		"transaction_id":  {strconv.FormatUint(foreignID, 10)},
		"reason":          {manifest.RunID + " foreign currency does not match EUR ledger"},
		"idempotency_key": {manifest.RunID + "-ignore-foreign"},
	}
	ignoreResponse, err := c.do(ctx, http.MethodPost, ignorePath, ignoreForm)
	ignoreStep := e2eHTTPStep(http.MethodPost, ignorePath, map[string]any{
		"status_code": http.StatusFound,
		"location":    "/billing?message=transaction_action_saved",
	}, ignoreResponse, err)
	if err == nil {
		ignoreStep.Passed = ignoreResponse.StatusCode == http.StatusFound && ignoreResponse.Location == "/billing?message=transaction_action_saved"
		if !ignoreStep.Passed {
			ignoreStep.Error = "foreign transaction could not be ignored"
		}
	}
	scenario.Steps = append(scenario.Steps, ignoreStep)
	if !ignoreStep.Passed {
		return fail("foreign transaction ignore failed")
	}
	repeatIgnoreResponse, err := c.do(ctx, http.MethodPost, ignorePath, ignoreForm)
	repeatIgnoreStep := e2eHTTPStep(http.MethodPost, ignorePath, map[string]any{
		"status_code": http.StatusFound,
		"location":    "/billing?message=transaction_action_saved",
	}, repeatIgnoreResponse, err)
	if err == nil {
		repeatIgnoreStep.Passed = repeatIgnoreResponse.StatusCode == http.StatusFound && repeatIgnoreResponse.Location == "/billing?message=transaction_action_saved"
		if !repeatIgnoreStep.Passed {
			repeatIgnoreStep.Error = "repeated foreign ignore was not idempotent"
		}
	}
	scenario.Steps = append(scenario.Steps, repeatIgnoreStep)
	if !repeatIgnoreStep.Passed {
		return fail("foreign transaction ignore idempotency failed")
	}

	fullRevokePreviewPath := fmt.Sprintf("/billing/revoke?transaction_id=%d", fullID)
	revokePreviewResponse, err := c.do(ctx, http.MethodGet, fullRevokePreviewPath, nil)
	revokePreviewStep := e2eHTTPStep(http.MethodGet, fullRevokePreviewPath, map[string]any{
		"status_code":         http.StatusOK,
		"transaction_visible": true,
	}, revokePreviewResponse, err)
	if err == nil {
		visible := strings.Contains(string(revokePreviewResponse.Body), manifest.Bank.Transactions[0].Description) && strings.Contains(string(revokePreviewResponse.Body), strconv.FormatUint(fullID, 10))
		revokePreviewStep.Actual.(map[string]any)["transaction_visible"] = visible
		revokePreviewStep.Passed = revokePreviewResponse.StatusCode == http.StatusOK && visible
		if !revokePreviewStep.Passed {
			revokePreviewStep.Error = "revoke preview did not expose the matched transaction"
		}
	}
	scenario.Steps = append(scenario.Steps, revokePreviewStep)
	if !revokePreviewStep.Passed {
		return fail("revoke preview failed")
	}

	revokePath := "/billing/revoke"
	revokeForm := url.Values{
		"transaction_id":  {strconv.FormatUint(fullID, 10)},
		"reason":          {manifest.RunID + " revoke and re-match"},
		"idempotency_key": {manifest.RunID + "-revoke-full"},
	}
	revokeResponse, err := c.do(ctx, http.MethodPost, revokePath, revokeForm)
	revokeStep := e2eHTTPStep(http.MethodPost, revokePath, map[string]any{
		"status_code": http.StatusFound,
		"location":    "/billing?message=transaction_action_saved",
	}, revokeResponse, err)
	if err == nil {
		revokeStep.Passed = revokeResponse.StatusCode == http.StatusFound && revokeResponse.Location == "/billing?message=transaction_action_saved"
		if !revokeStep.Passed {
			revokeStep.Error = "matched transaction revoke did not succeed"
		}
	}
	scenario.Steps = append(scenario.Steps, revokeStep)
	if !revokeStep.Passed {
		return fail("matched transaction revoke failed")
	}
	repeatRevokeResponse, err := c.do(ctx, http.MethodPost, revokePath, revokeForm)
	repeatRevokeStep := e2eHTTPStep(http.MethodPost, revokePath, map[string]any{
		"status_code": http.StatusFound,
		"location":    "/billing?message=transaction_action_saved",
	}, repeatRevokeResponse, err)
	if err == nil {
		repeatRevokeStep.Passed = repeatRevokeResponse.StatusCode == http.StatusFound && repeatRevokeResponse.Location == "/billing?message=transaction_action_saved"
		if !repeatRevokeStep.Passed {
			repeatRevokeStep.Error = "repeated transaction revoke was not idempotent"
		}
	}
	scenario.Steps = append(scenario.Steps, repeatRevokeStep)
	if !repeatRevokeStep.Passed {
		return fail("transaction revoke idempotency failed")
	}

	rematchResponse, err := c.do(ctx, http.MethodPost, fullConfirmPath, url.Values{
		"transaction_id": {strconv.FormatUint(fullID, 10)},
		"tenant_id":      {strconv.FormatUint(tenantID, 10)},
		"period":         {"2026-09"},
		"remember_payer": {"1"},
	})
	rematchStep := e2eHTTPStep(http.MethodPost, fullConfirmPath, map[string]any{
		"status_code": http.StatusFound,
		"location":    "/billing?message=rent_confirmed",
	}, rematchResponse, err)
	if err == nil {
		rematchStep.Passed = rematchResponse.StatusCode == http.StatusFound && rematchResponse.Location == "/billing?message=rent_confirmed"
		if !rematchStep.Passed {
			rematchStep.Error = "revoked transaction could not be confirmed again"
		}
	}
	scenario.Steps = append(scenario.Steps, rematchStep)
	if !rematchStep.Passed {
		return fail("transaction re-match failed")
	}

	finalResponse, err := c.do(ctx, http.MethodGet, "/billing", nil)
	finalStep := e2eHTTPStep(http.MethodGet, "/billing", map[string]any{
		"status_code":         http.StatusOK,
		"full_status":         "matched",
		"foreign_status":      "ignored",
		"amount_conservation": true,
	}, finalResponse, err)
	if err == nil {
		fullRow, fullErr := e2eTransactionRow(finalResponse.Body, manifest.Bank.Transactions[0].ProviderTransactionID)
		foreignRow, foreignErr := e2eTransactionRow(finalResponse.Body, manifest.Bank.Transactions[2].ProviderTransactionID)
		fullStatus := e2eTransactionStatus(fullRow)
		foreignStatus := e2eTransactionStatus(foreignRow)
		conserved := strings.Contains(fullRow, "已分配 EUR 950.00") && strings.Contains(fullRow, "余款 EUR 0.00")
		actual := finalStep.Actual.(map[string]any)
		actual["full_status"] = fullStatus
		actual["foreign_status"] = foreignStatus
		actual["amount_conservation"] = conserved
		finalStep.Passed = finalResponse.StatusCode == http.StatusOK && fullErr == nil && foreignErr == nil && fullStatus == "matched" && foreignStatus == "ignored" && conserved
		if !finalStep.Passed {
			finalStep.Error = "final billing read did not confirm status and amount conservation"
		}
	}
	scenario.Steps = append(scenario.Steps, finalStep)
	if !finalStep.Passed {
		return fail("final ledger state verification failed")
	}

	scenario.Status = "passed"
	return scenario
}

func extractE2ETransactionID(body []byte, providerID string) (uint64, error) {
	row, err := e2eTransactionRow(body, providerID)
	if err != nil {
		return 0, err
	}
	match := e2eTransactionIDPattern.FindStringSubmatch(row)
	if len(match) != 2 {
		return 0, errors.New("transaction ID is not present in billing row")
	}
	id, err := strconv.ParseUint(match[1], 10, 64)
	if err != nil || id == 0 {
		return 0, errors.New("transaction row has an invalid ID")
	}
	return id, nil
}

func e2eTransactionRow(body []byte, providerID string) (string, error) {
	text := string(body)
	providerIndex := strings.Index(text, providerID)
	if providerIndex < 0 {
		return "", errors.New("provider transaction ID is not present in billing response")
	}
	rowStart := strings.LastIndex(text[:providerIndex], `<tr class="`)
	if rowStart < 0 {
		return "", errors.New("billing transaction row is not present in response")
	}
	rowEndOffset := strings.Index(text[providerIndex:], "</tr>")
	if rowEndOffset < 0 {
		return "", errors.New("billing transaction row is incomplete")
	}
	return text[rowStart : providerIndex+rowEndOffset], nil
}

func e2eTransactionStatus(row string) string {
	match := e2eTransactionStatusPattern.FindStringSubmatch(row)
	if len(match) != 2 {
		return ""
	}
	return match[1]
}
