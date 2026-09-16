package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func (c *e2eHTTPClient) crossUserScenario(ctx context.Context, manifest e2eFixtureManifest, tenantID, transactionID uint64) e2eScenarioReport {
	scenario := e2eScenarioReport{Name: "cross-user-isolation", Status: "running", Steps: []e2eStepReport{}}
	fail := func(message string) e2eScenarioReport {
		scenario.Status = "failed"
		scenario.Error = message
		return scenario
	}
	if tenantID == 0 || transactionID == 0 {
		return fail("cross-user scenario requires target tenant and transaction IDs")
	}
	tenantPath := fmt.Sprintf("/tenants/%d", tenantID)
	tenantResponse, err := c.do(ctx, http.MethodGet, tenantPath, nil)
	tenantStep := e2eHTTPStep(http.MethodGet, tenantPath, map[string]any{
		"status_code": http.StatusNotFound,
		"data_hidden": true,
	}, tenantResponse, err)
	if err == nil {
		// A rejection page is allowed to carry an error body; what must never
		// appear is any identifier belonging to the other account's fixture.
		body := string(tenantResponse.Body)
		leaked := strings.Contains(body, manifest.RunID) ||
			strings.Contains(body, manifest.Tenant.Name) ||
			strings.Contains(body, manifest.Tenant.DisplayAlias)
		dataHidden := tenantResponse.StatusCode == http.StatusNotFound && !leaked
		actual := tenantStep.Actual.(map[string]any)
		actual["data_hidden"] = dataHidden
		actual["fixture_data_absent"] = !leaked
		tenantStep.Passed = dataHidden
		if !tenantStep.Passed {
			tenantStep.Error = "second account could read the first account tenant or received its page body"
		}
	}
	scenario.Steps = append(scenario.Steps, tenantStep)
	if !tenantStep.Passed {
		return fail("cross-user tenant isolation failed")
	}

	confirmPath := "/billing/confirm"
	confirmResponse, err := c.do(ctx, http.MethodPost, confirmPath, url.Values{
		"transaction_id": {strconv.FormatUint(transactionID, 10)},
		"tenant_id":      {strconv.FormatUint(tenantID, 10)},
		"period":         {"2026-09"},
	})
	confirmStep := e2eHTTPStep(http.MethodPost, confirmPath, map[string]any{
		"status_code": http.StatusFound,
		"location":    "/billing?error=confirmation_failed",
	}, confirmResponse, err)
	if err == nil {
		confirmStep.Passed = confirmResponse.StatusCode == http.StatusFound && confirmResponse.Location == "/billing?error=confirmation_failed"
		if !confirmStep.Passed {
			confirmStep.Error = "second account could confirm the first account transaction"
		}
	}
	scenario.Steps = append(scenario.Steps, confirmStep)
	if !confirmStep.Passed {
		return fail("cross-user transaction confirmation was not rejected")
	}

	revokePath := fmt.Sprintf("/billing/revoke?transaction_id=%d", transactionID)
	revokeResponse, err := c.do(ctx, http.MethodGet, revokePath, nil)
	revokeStep := e2eHTTPStep(http.MethodGet, revokePath, map[string]any{
		"status_code": http.StatusFound,
		"location":    "/billing?error=transaction_action_failed",
	}, revokeResponse, err)
	if err == nil {
		revokeStep.Passed = revokeResponse.StatusCode == http.StatusFound && revokeResponse.Location == "/billing?error=transaction_action_failed"
		if !revokeStep.Passed {
			revokeStep.Error = "second account could open the first account revoke preview"
		}
	}
	scenario.Steps = append(scenario.Steps, revokeStep)
	if !revokeStep.Passed {
		return fail("cross-user transaction preview was not rejected")
	}

	cashPath := "/cash-receipts/preview"
	cashResponse, err := c.do(ctx, http.MethodPost, cashPath, url.Values{
		"tenant_id":       {strconv.FormatUint(tenantID, 10)},
		"period":          {"2026-08"},
		"amount":          {"1.00"},
		"currency":        {"EUR"},
		"received_at":     {"2026-09-16"},
		"idempotency_key": {"cross-user-probe"},
	})
	cashStep := e2eHTTPStep(http.MethodPost, cashPath, map[string]any{
		"status_code": http.StatusFound,
		"error":       "cash_receipt_failed",
	}, cashResponse, err)
	if err == nil {
		cashStep.Passed = cashResponse.StatusCode == http.StatusFound && containsURLQuery(cashResponse.Location, "error", "cash_receipt_failed")
		if !cashStep.Passed {
			cashStep.Error = "second account could preview the first account cash receipt"
		}
	}
	scenario.Steps = append(scenario.Steps, cashStep)
	if !cashStep.Passed {
		return fail("cross-user cash receipt was not rejected")
	}

	scenario.Status = "passed"
	return scenario
}

func containsURLQuery(location, key, expected string) bool {
	parsed, err := url.Parse(location)
	return err == nil && parsed.Query().Get(key) == expected
}
