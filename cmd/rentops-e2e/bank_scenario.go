package main

import (
	"context"
	"net/http"
	"strings"
)

func (c *e2eHTTPClient) bankImportScenario(ctx context.Context, manifest e2eFixtureManifest) e2eScenarioReport {
	scenario := e2eScenarioReport{Name: "bank-import", Status: "running", Steps: []e2eStepReport{}}
	fail := func(message string) e2eScenarioReport {
		scenario.Status = "failed"
		scenario.Error = message
		return scenario
	}
	importResponse, err := c.do(ctx, http.MethodPost, "/import-legacy", nil)
	importStep := e2eHTTPStep(http.MethodPost, "/import-legacy", map[string]any{
		"status_code": http.StatusFound,
		"location":    "/billing?message=legacy_imported",
	}, importResponse, err)
	if err == nil {
		importStep.Passed = importResponse.StatusCode == http.StatusFound && importResponse.Location == "/billing?message=legacy_imported"
		if !importStep.Passed {
			importStep.Error = "legacy import did not return the expected success redirect"
		}
	}
	scenario.Steps = append(scenario.Steps, importStep)
	if !importStep.Passed {
		return fail("legacy bank import failed")
	}

	billingResponse, err := c.do(ctx, http.MethodGet, "/billing", nil)
	billingStep := e2eHTTPStep(http.MethodGet, "/billing", map[string]any{
		"status_code":          http.StatusOK,
		"transaction_count":    manifest.Expected.BankTransactionCount,
		"transactions_visible": true,
	}, billingResponse, err)
	if err == nil {
		visible := billingResponse.StatusCode == http.StatusOK
		for _, transaction := range manifest.Bank.Transactions {
			visible = visible && strings.Contains(string(billingResponse.Body), transaction.ProviderTransactionID) && strings.Contains(string(billingResponse.Body), transaction.Description)
		}
		visible = visible && strings.Contains(string(billingResponse.Body), "EUR 950.00") && strings.Contains(string(billingResponse.Body), "GBP 25.00")
		actual := billingStep.Actual.(map[string]any)
		actual["transaction_count"] = countE2ETransactionRows(billingResponse.Body, manifest)
		actual["transactions_visible"] = visible
		billingStep.Passed = visible && actual["transaction_count"] == manifest.Expected.BankTransactionCount
		if !billingStep.Passed {
			billingStep.Error = "billing page did not expose the exact imported transaction set"
		}
	}
	scenario.Steps = append(scenario.Steps, billingStep)
	if !billingStep.Passed {
		return fail("imported bank transactions could not be verified")
	}

	repeatResponse, err := c.do(ctx, http.MethodPost, "/import-legacy", nil)
	repeatStep := e2eHTTPStep(http.MethodPost, "/import-legacy", map[string]any{
		"status_code": http.StatusFound,
		"location":    "/billing?message=legacy_imported",
	}, repeatResponse, err)
	if err == nil {
		repeatStep.Passed = repeatResponse.StatusCode == http.StatusFound && repeatResponse.Location == "/billing?message=legacy_imported"
		if !repeatStep.Passed {
			repeatStep.Error = "repeated legacy import did not return the expected success redirect"
		}
	}
	scenario.Steps = append(scenario.Steps, repeatStep)
	if !repeatStep.Passed {
		return fail("repeated legacy bank import failed")
	}

	repeatedBillingResponse, err := c.do(ctx, http.MethodGet, "/billing", nil)
	repeatedBillingStep := e2eHTTPStep(http.MethodGet, "/billing", map[string]any{
		"status_code":       http.StatusOK,
		"transaction_count": manifest.Expected.BankTransactionCount,
		"duplicate_rows":    false,
	}, repeatedBillingResponse, err)
	if err == nil {
		rowCount := countE2ETransactionRows(repeatedBillingResponse.Body, manifest)
		duplicateRows := false
		for _, transaction := range manifest.Bank.Transactions {
			if strings.Count(string(repeatedBillingResponse.Body), transaction.ProviderTransactionID) != 1 {
				duplicateRows = true
				break
			}
		}
		repeatedBillingStep.Actual.(map[string]any)["transaction_count"] = rowCount
		repeatedBillingStep.Actual.(map[string]any)["duplicate_rows"] = duplicateRows
		repeatedBillingStep.Passed = repeatedBillingResponse.StatusCode == http.StatusOK && rowCount == manifest.Expected.BankTransactionCount && !duplicateRows
		if !repeatedBillingStep.Passed {
			repeatedBillingStep.Error = "repeated legacy import changed the transaction set"
		}
	}
	scenario.Steps = append(scenario.Steps, repeatedBillingStep)
	if !repeatedBillingStep.Passed {
		return fail("legacy import idempotency failed")
	}

	scenario.Status = "passed"
	return scenario
}

func countE2ETransactionRows(body []byte, manifest e2eFixtureManifest) int {
	text := string(body)
	return strings.Count(text, `<tr class="income">`) + strings.Count(text, `<tr class="expense">`)
}
