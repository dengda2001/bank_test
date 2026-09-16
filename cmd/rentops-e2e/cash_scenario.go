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

var e2eCashReceiptIDPattern = regexp.MustCompile(`href="/cash-receipts/void\?receipt_id=([1-9][0-9]*)"`)

func (c *e2eHTTPClient) cashReceiptScenario(ctx context.Context, manifest e2eFixtureManifest, tenantID uint64) e2eScenarioReport {
	scenario := e2eScenarioReport{Name: "cash-receipt", Status: "running", Steps: []e2eStepReport{}}
	fail := func(message string) e2eScenarioReport {
		scenario.Status = "failed"
		scenario.Error = message
		return scenario
	}
	if tenantID == 0 {
		return fail("cash receipt scenario requires a tenant ID")
	}
	newPath := fmt.Sprintf("/cash-receipts/new?period=2026-08&tenant_id=%d", tenantID)
	newResponse, err := c.do(ctx, http.MethodGet, newPath, nil)
	newStep := e2eHTTPStep(http.MethodGet, newPath, map[string]any{
		"status_code": http.StatusOK,
		"currency":    "EUR",
	}, newResponse, err)
	if err == nil {
		euroForm := strings.Contains(string(newResponse.Body), "EUR") && strings.Contains(string(newResponse.Body), "cash-receipts/preview")
		newStep.Actual.(map[string]any)["currency"] = euroForm
		newStep.Passed = newResponse.StatusCode == http.StatusOK && euroForm
		if !newStep.Passed {
			newStep.Error = "cash receipt form did not expose the EUR-only workflow"
		}
	}
	scenario.Steps = append(scenario.Steps, newStep)
	if !newStep.Passed {
		return fail("cash receipt form could not be opened")
	}

	firstForm := e2eCashReceiptForm(tenantID, "300.00", manifest.RunID+"-cash-first", "EUR", manifest.RunID+" first cash receipt")
	previewPath := "/cash-receipts/preview"
	previewResponse, err := c.do(ctx, http.MethodPost, previewPath, firstForm)
	previewStep := e2eHTTPStep(http.MethodPost, previewPath, map[string]any{
		"status_code":     http.StatusOK,
		"current_paid":    "EUR 600.00",
		"after_remaining": "EUR 50.00",
		"preview_only":    true,
	}, previewResponse, err)
	if err == nil {
		body := string(previewResponse.Body)
		currentPaid := strings.Contains(body, "EUR 600.00")
		afterRemaining := strings.Contains(body, "EUR 50.00")
		previewOnly := strings.Contains(body, "确认现金入账") && strings.Contains(body, `action="/cash-receipts"`)
		actual := previewStep.Actual.(map[string]any)
		actual["current_paid"] = currentPaid
		actual["after_remaining"] = afterRemaining
		actual["preview_only"] = previewOnly
		previewStep.Passed = previewResponse.StatusCode == http.StatusOK && currentPaid && afterRemaining && previewOnly
		if !previewStep.Passed {
			previewStep.Error = "cash receipt preview did not expose exact current and projected balances"
		}
	}
	scenario.Steps = append(scenario.Steps, previewStep)
	if !previewStep.Passed {
		return fail("cash receipt preview failed")
	}

	createPath := "/cash-receipts"
	createResponse, err := c.do(ctx, http.MethodPost, createPath, firstForm)
	createStep := e2eHTTPStep(http.MethodPost, createPath, map[string]any{
		"status_code": http.StatusFound,
		"location":    fmt.Sprintf("/tenants/%d?message=cash_receipt_saved&from_month=2026-08&to_month=2026-08", tenantID),
	}, createResponse, err)
	if err == nil {
		expectedLocation := fmt.Sprintf("/tenants/%d?message=cash_receipt_saved&from_month=2026-08&to_month=2026-08", tenantID)
		createStep.Passed = createResponse.StatusCode == http.StatusFound && createResponse.Location == expectedLocation
		if !createStep.Passed {
			createStep.Error = "cash receipt creation did not return the expected tenant history redirect"
		}
	}
	scenario.Steps = append(scenario.Steps, createStep)
	if !createStep.Passed {
		return fail("cash receipt creation failed")
	}
	repeatCreateResponse, err := c.do(ctx, http.MethodPost, createPath, firstForm)
	repeatCreateStep := e2eHTTPStep(http.MethodPost, createPath, map[string]any{
		"status_code": http.StatusFound,
		"location":    fmt.Sprintf("/tenants/%d?message=cash_receipt_saved&from_month=2026-08&to_month=2026-08", tenantID),
	}, repeatCreateResponse, err)
	if err == nil {
		expectedLocation := fmt.Sprintf("/tenants/%d?message=cash_receipt_saved&from_month=2026-08&to_month=2026-08", tenantID)
		repeatCreateStep.Passed = repeatCreateResponse.StatusCode == http.StatusFound && repeatCreateResponse.Location == expectedLocation
		if !repeatCreateStep.Passed {
			repeatCreateStep.Error = "repeated cash receipt creation did not return the original success result"
		}
	}
	scenario.Steps = append(scenario.Steps, repeatCreateStep)
	if !repeatCreateStep.Passed {
		return fail("cash receipt idempotency failed on repeated creation")
	}

	detailPath := fmt.Sprintf("/tenants/%d", tenantID)
	detailResponse, err := c.do(ctx, http.MethodGet, detailPath, nil)
	detailStep := e2eHTTPStep(http.MethodGet, detailPath, map[string]any{
		"status_code":    http.StatusOK,
		"receipt_amount": "EUR 300.00",
		"remaining":      "EUR 50.00",
	}, detailResponse, err)
	receiptID, receiptErr := uint64(0), error(nil)
	if err == nil {
		receiptID, receiptErr = extractE2ECashReceiptID(detailResponse.Body)
		body := string(detailResponse.Body)
		visible := receiptErr == nil && strings.Contains(body, "EUR 300.00") && strings.Contains(body, "EUR 50.00")
		actual := detailStep.Actual.(map[string]any)
		actual["receipt_amount"] = strings.Contains(body, "EUR 300.00")
		actual["remaining"] = strings.Contains(body, "EUR 50.00")
		actual["receipt_id"] = receiptID
		detailStep.Passed = detailResponse.StatusCode == http.StatusOK && visible
		if !detailStep.Passed {
			detailStep.Error = "tenant history did not expose the cash receipt and remaining balance"
		}
	}
	scenario.Steps = append(scenario.Steps, detailStep)
	if !detailStep.Passed || receiptErr != nil {
		return fail("cash receipt could not be verified in tenant history")
	}

	voidPreviewPath := fmt.Sprintf("/cash-receipts/void?receipt_id=%d", receiptID)
	voidPreviewResponse, err := c.do(ctx, http.MethodGet, voidPreviewPath, nil)
	voidPreviewStep := e2eHTTPStep(http.MethodGet, voidPreviewPath, map[string]any{
		"status_code":     http.StatusOK,
		"receipt_visible": true,
	}, voidPreviewResponse, err)
	if err == nil {
		visible := strings.Contains(string(voidPreviewResponse.Body), "EUR 300.00") && strings.Contains(string(voidPreviewResponse.Body), strconv.FormatUint(receiptID, 10))
		voidPreviewStep.Actual.(map[string]any)["receipt_visible"] = visible
		voidPreviewStep.Passed = voidPreviewResponse.StatusCode == http.StatusOK && visible
		if !voidPreviewStep.Passed {
			voidPreviewStep.Error = "cash void preview did not expose the expected receipt"
		}
	}
	scenario.Steps = append(scenario.Steps, voidPreviewStep)
	if !voidPreviewStep.Passed {
		return fail("cash void preview failed")
	}

	voidPath := "/cash-receipts/void"
	voidForm := url.Values{
		"receipt_id": {strconv.FormatUint(receiptID, 10)},
		"reason":     {manifest.RunID + " correction of first cash amount"},
	}
	voidResponse, err := c.do(ctx, http.MethodPost, voidPath, voidForm)
	voidStep := e2eHTTPStep(http.MethodPost, voidPath, map[string]any{
		"status_code": http.StatusFound,
		"location":    fmt.Sprintf("/tenants/%d?message=cash_receipt_voided&from_month=2026-08&to_month=2026-08", tenantID),
	}, voidResponse, err)
	if err == nil {
		expectedLocation := fmt.Sprintf("/tenants/%d?message=cash_receipt_voided&from_month=2026-08&to_month=2026-08", tenantID)
		voidStep.Passed = voidResponse.StatusCode == http.StatusFound && voidResponse.Location == expectedLocation
		if !voidStep.Passed {
			voidStep.Error = "cash receipt void did not return the expected tenant history redirect"
		}
	}
	scenario.Steps = append(scenario.Steps, voidStep)
	if !voidStep.Passed {
		return fail("cash receipt void failed")
	}
	repeatVoidResponse, err := c.do(ctx, http.MethodPost, voidPath, voidForm)
	repeatVoidStep := e2eHTTPStep(http.MethodPost, voidPath, map[string]any{
		"status_code": http.StatusFound,
		"location":    fmt.Sprintf("/tenants/%d?message=cash_receipt_voided&from_month=2026-08&to_month=2026-08", tenantID),
	}, repeatVoidResponse, err)
	if err == nil {
		expectedLocation := fmt.Sprintf("/tenants/%d?message=cash_receipt_voided&from_month=2026-08&to_month=2026-08", tenantID)
		repeatVoidStep.Passed = repeatVoidResponse.StatusCode == http.StatusFound && repeatVoidResponse.Location == expectedLocation
		if !repeatVoidStep.Passed {
			repeatVoidStep.Error = "repeated cash receipt void was not safely repeatable"
		}
	}
	scenario.Steps = append(scenario.Steps, repeatVoidStep)
	if !repeatVoidStep.Passed {
		return fail("cash receipt void repeat failed")
	}
	voidedDetailResponse, err := c.do(ctx, http.MethodGet, detailPath, nil)
	voidedDetailStep := e2eHTTPStep(http.MethodGet, detailPath, map[string]any{
		"status_code":   http.StatusOK,
		"void_restored": true,
		"remaining":     "EUR 350.00",
	}, voidedDetailResponse, err)
	if err == nil {
		body := string(voidedDetailResponse.Body)
		voidRestored := strings.Contains(body, "EUR 350.00") && !strings.Contains(body, "receipt_id=501")
		voidedDetailStep.Actual.(map[string]any)["void_restored"] = voidRestored
		voidedDetailStep.Actual.(map[string]any)["remaining"] = strings.Contains(body, "EUR 350.00")
		voidedDetailStep.Passed = voidedDetailResponse.StatusCode == http.StatusOK && voidRestored
		if !voidedDetailStep.Passed {
			voidedDetailStep.Error = "voided cash receipt did not restore the expected tenant balance"
		}
	}
	scenario.Steps = append(scenario.Steps, voidedDetailStep)
	if !voidedDetailStep.Passed {
		return fail("voided cash receipt could not be verified")
	}

	correctionForm := e2eCashReceiptForm(tenantID, "350.00", manifest.RunID+"-cash-correction", "EUR", manifest.RunID+" corrected cash receipt")
	correctionPreviewResponse, err := c.do(ctx, http.MethodPost, previewPath, correctionForm)
	correctionPreviewStep := e2eHTTPStep(http.MethodPost, previewPath, map[string]any{
		"status_code":     http.StatusOK,
		"cash_amount":     "350.00 EUR",
		"after_remaining": "EUR 0.00",
	}, correctionPreviewResponse, err)
	if err == nil {
		body := string(correctionPreviewResponse.Body)
		// The preview renders the entered cash amount as "<amount> <currency>"
		// while the balance cards use "<currency> <amount>".
		cashAmount := strings.Contains(body, "350.00 EUR")
		afterRemaining := strings.Contains(body, "EUR 0.00")
		actual := correctionPreviewStep.Actual.(map[string]any)
		actual["cash_amount"] = cashAmount
		actual["after_remaining"] = afterRemaining
		correctionPreviewStep.Passed = correctionPreviewResponse.StatusCode == http.StatusOK && cashAmount && afterRemaining
		if !correctionPreviewStep.Passed {
			correctionPreviewStep.Error = "cash correction preview did not restore the exact remaining balance"
		}
	}
	scenario.Steps = append(scenario.Steps, correctionPreviewStep)
	if !correctionPreviewStep.Passed {
		return fail("cash correction preview failed")
	}

	correctionResponse, err := c.do(ctx, http.MethodPost, createPath, correctionForm)
	correctionStep := e2eHTTPStep(http.MethodPost, createPath, map[string]any{
		"status_code": http.StatusFound,
		"location":    fmt.Sprintf("/tenants/%d?message=cash_receipt_saved&from_month=2026-08&to_month=2026-08", tenantID),
	}, correctionResponse, err)
	if err == nil {
		expectedLocation := fmt.Sprintf("/tenants/%d?message=cash_receipt_saved&from_month=2026-08&to_month=2026-08", tenantID)
		correctionStep.Passed = correctionResponse.StatusCode == http.StatusFound && correctionResponse.Location == expectedLocation
		if !correctionStep.Passed {
			correctionStep.Error = "corrected cash receipt creation failed"
		}
	}
	scenario.Steps = append(scenario.Steps, correctionStep)
	if !correctionStep.Passed {
		return fail("cash correction creation failed")
	}
	repeatCorrectionResponse, err := c.do(ctx, http.MethodPost, createPath, correctionForm)
	repeatCorrectionStep := e2eHTTPStep(http.MethodPost, createPath, map[string]any{
		"status_code": http.StatusFound,
		"location":    fmt.Sprintf("/tenants/%d?message=cash_receipt_saved&from_month=2026-08&to_month=2026-08", tenantID),
	}, repeatCorrectionResponse, err)
	if err == nil {
		expectedLocation := fmt.Sprintf("/tenants/%d?message=cash_receipt_saved&from_month=2026-08&to_month=2026-08", tenantID)
		repeatCorrectionStep.Passed = repeatCorrectionResponse.StatusCode == http.StatusFound && repeatCorrectionResponse.Location == expectedLocation
		if !repeatCorrectionStep.Passed {
			repeatCorrectionStep.Error = "repeated corrected cash receipt creation was not idempotent"
		}
	}
	scenario.Steps = append(scenario.Steps, repeatCorrectionStep)
	if !repeatCorrectionStep.Passed {
		return fail("cash correction idempotency failed")
	}

	finalDetailResponse, err := c.do(ctx, http.MethodGet, detailPath, nil)
	finalDetailStep := e2eHTTPStep(http.MethodGet, detailPath, map[string]any{
		"status_code":      http.StatusOK,
		"corrected_amount": "EUR 350.00",
		"remaining":        "EUR 0.00",
	}, finalDetailResponse, err)
	if err == nil {
		body := string(finalDetailResponse.Body)
		corrected := strings.Contains(body, "EUR 350.00")
		remaining := strings.Contains(body, "EUR 0.00")
		finalDetailStep.Actual.(map[string]any)["corrected_amount"] = corrected
		finalDetailStep.Actual.(map[string]any)["remaining"] = remaining
		finalDetailStep.Passed = finalDetailResponse.StatusCode == http.StatusOK && corrected && remaining
		if !finalDetailStep.Passed {
			finalDetailStep.Error = "tenant history did not expose the corrected cash balance"
		}
	}
	scenario.Steps = append(scenario.Steps, finalDetailStep)
	if !finalDetailStep.Passed {
		return fail("corrected cash receipt could not be verified")
	}

	foreignForm := e2eCashReceiptForm(tenantID, "1.00", manifest.RunID+"-cash-foreign", "GBP", manifest.RunID+" foreign currency")
	foreignResponse, err := c.do(ctx, http.MethodPost, previewPath, foreignForm)
	foreignStep := e2eHTTPStep(http.MethodPost, previewPath, map[string]any{
		"status_code": http.StatusFound,
		"error":       "cash_receipt_failed",
	}, foreignResponse, err)
	if err == nil {
		foreignStep.Passed = foreignResponse.StatusCode == http.StatusFound && strings.Contains(foreignResponse.Location, "error=cash_receipt_failed")
		if !foreignStep.Passed {
			foreignStep.Error = "non-EUR cash receipt was not rejected"
		}
	}
	scenario.Steps = append(scenario.Steps, foreignStep)
	if !foreignStep.Passed {
		return fail("non-EUR cash rejection failed")
	}

	overbalanceForm := e2eCashReceiptForm(tenantID, "1.00", manifest.RunID+"-cash-overbalance", "EUR", manifest.RunID+" overbalance")
	overbalanceResponse, err := c.do(ctx, http.MethodPost, previewPath, overbalanceForm)
	overbalanceStep := e2eHTTPStep(http.MethodPost, previewPath, map[string]any{
		"status_code": http.StatusFound,
		"error":       "cash_overbalance",
	}, overbalanceResponse, err)
	if err == nil {
		overbalanceStep.Passed = overbalanceResponse.StatusCode == http.StatusFound && strings.Contains(overbalanceResponse.Location, "error=cash_overbalance")
		if !overbalanceStep.Passed {
			overbalanceStep.Error = "cash overbalance was not rejected"
		}
	}
	scenario.Steps = append(scenario.Steps, overbalanceStep)
	if !overbalanceStep.Passed {
		return fail("cash overbalance rejection failed")
	}

	scenario.Status = "passed"
	return scenario
}

func e2eCashReceiptForm(tenantID uint64, amount, idempotencyKey, currency, note string) url.Values {
	return url.Values{
		"tenant_id":       {strconv.FormatUint(tenantID, 10)},
		"period":          {"2026-08"},
		"amount":          {amount},
		"currency":        {currency},
		"received_at":     {"2026-09-16"},
		"note":            {note},
		"idempotency_key": {idempotencyKey},
	}
}

func extractE2ECashReceiptID(body []byte) (uint64, error) {
	match := e2eCashReceiptIDPattern.FindStringSubmatch(string(body))
	if len(match) != 2 {
		return 0, errors.New("cash receipt void link is not present in tenant detail")
	}
	id, err := strconv.ParseUint(match[1], 10, 64)
	if err != nil || id == 0 {
		return 0, errors.New("cash receipt void link has an invalid ID")
	}
	return id, nil
}
