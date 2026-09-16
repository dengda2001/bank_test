package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var e2EDashboardMetricPattern = regexp.MustCompile(`<div class="label">([^<]+)</div><strong>([^<]+)</strong>`)

func (c *e2eHTTPClient) dashboardScenario(ctx context.Context, manifest e2eFixtureManifest, tenantID uint64) e2eScenarioReport {
	scenario := e2eScenarioReport{Name: "dashboard-and-history", Status: "running", Steps: []e2eStepReport{}}
	fail := func(message string) e2eScenarioReport {
		scenario.Status = "failed"
		scenario.Error = message
		return scenario
	}
	period := "2026-08"
	dashboardPath := "/rent-dashboard?period=" + url.QueryEscape(period)
	dashboardResponse, err := c.do(ctx, http.MethodGet, dashboardPath, nil)
	dashboardStep := e2eHTTPStep(http.MethodGet, dashboardPath, map[string]any{
		"status_code":         http.StatusOK,
		"tenant_visible":      true,
		"monthly_rent":        "EUR 950.00",
		"paid_rent":           "EUR 950.00",
		"remaining_rent":      "EUR 0.00",
		"amount_conservation": true,
	}, dashboardResponse, err)
	if err == nil {
		body := string(dashboardResponse.Body)
		tenantVisible := strings.Contains(body, manifest.Tenant.Name)
		monthlyRent := e2EDashboardMetric(body, "本月应收") == "EUR 950.00"
		paidRent := e2EDashboardMetric(body, "已收租金") == "EUR 950.00"
		remainingRent := e2EDashboardMetric(body, "剩余未收") == "EUR 0.00"
		actual := dashboardStep.Actual.(map[string]any)
		actual["tenant_visible"] = tenantVisible
		actual["monthly_rent"] = monthlyRent
		actual["paid_rent"] = paidRent
		actual["remaining_rent"] = remainingRent
		actual["amount_conservation"] = monthlyRent && paidRent && remainingRent
		dashboardStep.Passed = dashboardResponse.StatusCode == http.StatusOK && tenantVisible && actual["amount_conservation"] == true
		if !dashboardStep.Passed {
			dashboardStep.Error = "dashboard did not expose the isolated tenant and exact rent totals"
		}
	}
	scenario.Steps = append(scenario.Steps, dashboardStep)
	if !dashboardStep.Passed {
		return fail("dashboard base read failed")
	}

	filterValues := url.Values{
		"period":    {period},
		"search":    {manifest.Tenant.DisplayAlias},
		"status":    {"paid"},
		"sort":      {"tenant_asc"},
		"page":      {"1"},
		"page_size": {"12"},
	}
	filteredPath := "/rent-dashboard?" + filterValues.Encode()
	filteredResponse, err := c.do(ctx, http.MethodGet, filteredPath, nil)
	filteredStep := e2eHTTPStep(http.MethodGet, filteredPath, map[string]any{
		"status_code":     http.StatusOK,
		"filtered_tenant": true,
		"status_filter":   "paid",
		"pagination_page": 1,
	}, filteredResponse, err)
	if err == nil {
		body := string(filteredResponse.Body)
		filteredTenant := strings.Contains(body, manifest.Tenant.Name) || strings.Contains(body, manifest.Tenant.DisplayAlias)
		statusFilter := strings.Contains(body, "已缴清") || strings.Contains(body, `value="paid" selected`)
		paginationPage := strings.Contains(body, "当前显示") || strings.Contains(body, "第 1")
		actual := filteredStep.Actual.(map[string]any)
		actual["filtered_tenant"] = filteredTenant
		actual["status_filter"] = statusFilter
		actual["pagination_page"] = paginationPage
		filteredStep.Passed = filteredResponse.StatusCode == http.StatusOK && filteredTenant && statusFilter && paginationPage
		if !filteredStep.Passed {
			filteredStep.Error = "dashboard filters did not preserve the isolated tenant and requested state"
		}
	}
	scenario.Steps = append(scenario.Steps, filteredStep)
	if !filteredStep.Passed {
		return fail("dashboard filter read failed")
	}

	invalidPath := "/rent-dashboard?period=" + url.QueryEscape(period) + "&status=not-a-status"
	invalidResponse, err := c.do(ctx, http.MethodGet, invalidPath, nil)
	invalidStep := e2eHTTPStep(http.MethodGet, invalidPath, map[string]any{
		"status_code":    http.StatusOK,
		"invalid_filter": true,
	}, invalidResponse, err)
	if err == nil {
		invalidFilter := strings.Contains(string(invalidResponse.Body), "invalid_dashboard_filter") || strings.Contains(string(invalidResponse.Body), "筛选条件无效")
		invalidStep.Actual.(map[string]any)["invalid_filter"] = invalidFilter
		invalidStep.Passed = invalidResponse.StatusCode == http.StatusOK && invalidFilter
		if !invalidStep.Passed {
			invalidStep.Error = "invalid dashboard filter was not surfaced as a controlled error"
		}
	}
	scenario.Steps = append(scenario.Steps, invalidStep)
	if !invalidStep.Passed {
		return fail("dashboard invalid filter assertion failed")
	}

	if tenantID == 0 {
		return fail("dashboard scenario requires a tenant ID for history cross-check")
	}
	historyPath := fmt.Sprintf("/tenants/%d?from_month=2026-08&to_month=2026-09&page_size=25", tenantID)
	historyResponse, err := c.do(ctx, http.MethodGet, historyPath, nil)
	historyStep := e2eHTTPStep(http.MethodGet, historyPath, map[string]any{
		"status_code":     http.StatusOK,
		"tenant_visible":  true,
		"history_visible": true,
	}, historyResponse, err)
	if err == nil {
		body := string(historyResponse.Body)
		tenantVisible := strings.Contains(body, manifest.Tenant.Name) || strings.Contains(body, manifest.Tenant.DisplayAlias)
		historyVisible := strings.Contains(body, "2026-08") && strings.Contains(body, "2026-09")
		actual := historyStep.Actual.(map[string]any)
		actual["tenant_visible"] = tenantVisible
		actual["history_visible"] = historyVisible
		historyStep.Passed = historyResponse.StatusCode == http.StatusOK && tenantVisible && historyVisible
		if !historyStep.Passed {
			historyStep.Error = "tenant history did not cross-check both requested rent months"
		}
	}
	scenario.Steps = append(scenario.Steps, historyStep)
	if !historyStep.Passed {
		return fail("tenant history cross-check failed")
	}

	scenario.Status = "passed"
	return scenario
}

func e2EDashboardMetric(body, label string) string {
	for _, match := range e2EDashboardMetricPattern.FindAllStringSubmatch(body, -1) {
		if len(match) == 3 && strings.TrimSpace(match[1]) == label {
			return strings.TrimSpace(match[2])
		}
	}
	return ""
}

func extractE2EDunningObligationID(body []byte) (uint64, error) {
	return extractPositiveE2EID(body, `name="obligation_id" value="`)
}

func (c *e2eHTTPClient) dunningCandidateScenario(ctx context.Context, manifest e2eFixtureManifest) (e2eScenarioReport, uint64) {
	scenario := e2eScenarioReport{Name: "dunning-candidate-discovery", Status: "running", Steps: []e2eStepReport{}}
	fail := func(message string) (e2eScenarioReport, uint64) {
		scenario.Status = "failed"
		scenario.Error = message
		return scenario, 0
	}
	values := url.Values{
		"period":    {"2026-09"},
		"search":    {manifest.DunningTenant.DisplayAlias},
		"status":    {"unpaid"},
		"sort":      {"due_asc"},
		"page":      {"1"},
		"page_size": {"12"},
	}
	path := "/rent-dashboard?" + values.Encode()
	response, err := c.do(ctx, http.MethodGet, path, nil)
	step := e2eHTTPStep(http.MethodGet, path, map[string]any{
		"status_code":       http.StatusOK,
		"candidate_visible": true,
	}, response, err)
	var obligationID uint64
	if err == nil {
		obligationID, err = extractE2EDunningObligationID(response.Body)
		candidateVisible := response.StatusCode == http.StatusOK && strings.Contains(string(response.Body), manifest.DunningTenant.Name)
		step.Actual.(map[string]any)["candidate_visible"] = candidateVisible
		step.Actual.(map[string]any)["obligation_id"] = obligationID
		step.Passed = candidateVisible && err == nil
		if !step.Passed {
			step.Error = "unpaid dunning tenant candidate or obligation ID was not visible"
		}
	}
	scenario.Steps = append(scenario.Steps, step)
	if !step.Passed {
		return fail("dunning candidate discovery failed")
	}
	scenario.Status = "passed"
	return scenario, obligationID
}

func extractPositiveE2EID(body []byte, prefix string) (uint64, error) {
	text := string(body)
	index := strings.Index(text, prefix)
	if index < 0 {
		return 0, fmt.Errorf("ID field %q is not present in response", prefix)
	}
	valueStart := index + len(prefix)
	valueEnd := valueStart
	for valueEnd < len(text) && text[valueEnd] >= '0' && text[valueEnd] <= '9' {
		valueEnd++
	}
	if valueEnd == valueStart {
		return 0, fmt.Errorf("ID field %q is empty", prefix)
	}
	id, err := strconv.ParseUint(text[valueStart:valueEnd], 10, 64)
	if err != nil || id == 0 {
		return 0, fmt.Errorf("ID field %q is invalid", prefix)
	}
	return id, nil
}
