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

var e2eTenantIDPattern = regexp.MustCompile(`href="/tenants/([1-9][0-9]*)"`)

func (c *e2eHTTPClient) tenantScenario(ctx context.Context, manifest e2eFixtureManifest) (e2eScenarioReport, uint64) {
	scenario := e2eScenarioReport{Name: "tenant-and-payer", Status: "running", Steps: []e2eStepReport{}}
	fail := func(message string) (e2eScenarioReport, uint64) {
		scenario.Status = "failed"
		scenario.Error = message
		return scenario, 0
	}
	values := e2eTenantFormValues(manifest.Tenant)
	values.Set("property_hint", manifest.RunID+" property")
	createResponse, err := c.do(ctx, http.MethodPost, "/tenants", values)
	createStep := e2eHTTPStep(http.MethodPost, "/tenants", map[string]any{
		"status_code": http.StatusFound,
		"location":    "/tenants?message=tenant_added",
	}, createResponse, err)
	if err == nil {
		createStep.Passed = createResponse.StatusCode == http.StatusFound && createResponse.Location == "/tenants?message=tenant_added"
		if !createStep.Passed {
			createStep.Error = "tenant creation did not return the expected success redirect"
		}
	}
	scenario.Steps = append(scenario.Steps, createStep)
	if !createStep.Passed {
		return fail("tenant creation failed")
	}

	listResponse, err := c.do(ctx, http.MethodGet, "/tenants", nil)
	listStep := e2eHTTPStep(http.MethodGet, "/tenants", map[string]any{
		"status_code":    http.StatusOK,
		"tenant_visible": true,
	}, listResponse, err)
	var tenantID uint64
	if err == nil {
		tenantID, err = extractE2ETenantID(listResponse.Body, manifest.Tenant.Name)
		listStep.Actual.(map[string]any)["tenant_visible"] = err == nil
		listStep.Actual.(map[string]any)["tenant_id"] = tenantID
		listStep.Passed = listResponse.StatusCode == http.StatusOK && err == nil
		if !listStep.Passed {
			listStep.Error = "created tenant was not visible with a concrete ID in the tenant list"
		}
	}
	scenario.Steps = append(scenario.Steps, listStep)
	if !listStep.Passed {
		return fail("tenant creation could not be verified by a follow-up read")
	}

	payerPath := fmt.Sprintf("/tenants/%d/payers", tenantID)
	payerResponse, err := c.do(ctx, http.MethodPost, payerPath, url.Values{
		"payer_name": {manifest.Payer.Name},
		"payer_id":   {manifest.Payer.PayerID},
	})
	payerStep := e2eHTTPStep(http.MethodPost, payerPath, map[string]any{
		"status_code": http.StatusFound,
		"location":    fmt.Sprintf("/tenants/%d?message=payer_added", tenantID),
	}, payerResponse, err)
	if err == nil {
		payerStep.Passed = payerResponse.StatusCode == http.StatusFound && payerResponse.Location == fmt.Sprintf("/tenants/%d?message=payer_added", tenantID)
		if !payerStep.Passed {
			payerStep.Error = "payer creation did not return the expected success redirect"
		}
	}
	scenario.Steps = append(scenario.Steps, payerStep)
	if !payerStep.Passed {
		return fail("payer relation creation failed")
	}

	detailPath := fmt.Sprintf("/tenants/%d", tenantID)
	detailResponse, err := c.do(ctx, http.MethodGet, detailPath, nil)
	detailStep := e2eHTTPStep(http.MethodGet, detailPath, map[string]any{
		"status_code":   http.StatusOK,
		"payer_visible": true,
	}, detailResponse, err)
	if err == nil {
		payerVisible := strings.Contains(string(detailResponse.Body), manifest.Payer.Name) && strings.Contains(string(detailResponse.Body), manifest.Payer.PayerID)
		detailStep.Actual.(map[string]any)["payer_visible"] = payerVisible
		detailStep.Passed = detailResponse.StatusCode == http.StatusOK && payerVisible
		if !detailStep.Passed {
			detailStep.Error = "created payer relation was not visible in tenant detail"
		}
	}
	scenario.Steps = append(scenario.Steps, detailStep)
	if !detailStep.Passed {
		return fail("payer relation could not be verified by a follow-up read")
	}

	updatedAlias := manifest.Tenant.DisplayAlias + " updated"
	updatedAddress := manifest.Tenant.RoomAddress + " updated"
	values.Set("tenant_id", strconv.FormatUint(tenantID, 10))
	values.Set("display_alias", updatedAlias)
	values.Set("room_address", updatedAddress)
	updateResponse, err := c.do(ctx, http.MethodPost, "/tenants", values)
	updateStep := e2eHTTPStep(http.MethodPost, "/tenants", map[string]any{
		"status_code": http.StatusFound,
		"location":    "/tenants?message=tenant_updated",
	}, updateResponse, err)
	if err == nil {
		updateStep.Passed = updateResponse.StatusCode == http.StatusFound && updateResponse.Location == "/tenants?message=tenant_updated"
		if !updateStep.Passed {
			updateStep.Error = "tenant update did not return the expected success redirect"
		}
	}
	scenario.Steps = append(scenario.Steps, updateStep)
	if !updateStep.Passed {
		return fail("tenant profile update failed")
	}

	updatedDetailResponse, err := c.do(ctx, http.MethodGet, detailPath, nil)
	updatedDetailStep := e2eHTTPStep(http.MethodGet, detailPath, map[string]any{
		"status_code":     http.StatusOK,
		"updated_profile": true,
	}, updatedDetailResponse, err)
	if err == nil {
		updatedProfile := strings.Contains(string(updatedDetailResponse.Body), updatedAlias) && strings.Contains(string(updatedDetailResponse.Body), updatedAddress)
		updatedDetailStep.Actual.(map[string]any)["updated_profile"] = updatedProfile
		updatedDetailStep.Passed = updatedDetailResponse.StatusCode == http.StatusOK && updatedProfile
		if !updatedDetailStep.Passed {
			updatedDetailStep.Error = "updated tenant profile was not visible in tenant detail"
		}
	}
	scenario.Steps = append(scenario.Steps, updatedDetailStep)
	if !updatedDetailStep.Passed {
		return fail("tenant profile update could not be verified by a follow-up read")
	}

	scenario.Status = "passed"
	return scenario, tenantID
}

func (c *e2eHTTPClient) dunningTenantScenario(ctx context.Context, manifest e2eFixtureManifest) (e2eScenarioReport, uint64) {
	scenario := e2eScenarioReport{Name: "dunning-tenant", Status: "running", Steps: []e2eStepReport{}}
	fail := func(message string) (e2eScenarioReport, uint64) {
		scenario.Status = "failed"
		scenario.Error = message
		return scenario, 0
	}
	createResponse, err := c.do(ctx, http.MethodPost, "/tenants", e2eTenantFormValues(manifest.DunningTenant))
	createStep := e2eHTTPStep(http.MethodPost, "/tenants", map[string]any{
		"status_code": http.StatusFound,
		"location":    "/tenants?message=tenant_added",
	}, createResponse, err)
	if err == nil {
		createStep.Passed = createResponse.StatusCode == http.StatusFound && createResponse.Location == "/tenants?message=tenant_added"
		if !createStep.Passed {
			createStep.Error = "dunning tenant creation did not return the expected success redirect"
		}
	}
	scenario.Steps = append(scenario.Steps, createStep)
	if !createStep.Passed {
		return fail("dunning tenant creation failed")
	}
	listResponse, err := c.do(ctx, http.MethodGet, "/tenants", nil)
	listStep := e2eHTTPStep(http.MethodGet, "/tenants", map[string]any{
		"status_code":    http.StatusOK,
		"tenant_visible": true,
	}, listResponse, err)
	var tenantID uint64
	if err == nil {
		tenantID, err = extractE2ETenantID(listResponse.Body, manifest.DunningTenant.Name)
		listStep.Actual.(map[string]any)["tenant_visible"] = err == nil
		listStep.Actual.(map[string]any)["tenant_id"] = tenantID
		listStep.Passed = listResponse.StatusCode == http.StatusOK && err == nil
		if !listStep.Passed {
			listStep.Error = "dunning tenant was not visible with a concrete ID in the tenant list"
		}
	}
	scenario.Steps = append(scenario.Steps, listStep)
	if !listStep.Passed {
		return fail("dunning tenant could not be verified by a follow-up read")
	}
	scenario.Status = "passed"
	return scenario, tenantID
}

func e2eTenantFormValues(tenant e2eTenantFixture) url.Values {
	return url.Values{
		"name":               {tenant.Name},
		"display_alias":      {tenant.DisplayAlias},
		"email":              {tenant.Email},
		"payer_id":           {tenant.PayerID},
		"payer_name_hint":    {tenant.PayerNameHint},
		"monthly_rent":       {formatE2EMoney(tenant.MonthlyRent)},
		"currency":           {tenant.MonthlyRent.Currency},
		"interval_unit":      {tenant.IntervalUnit},
		"interval_count":     {strconv.Itoa(tenant.IntervalCount)},
		"billing_start_date": {tenant.BillingStartDate},
		"due_day":            {strconv.Itoa(tenant.DueDay)},
		"rent_start_date":    {tenant.RentStartDate},
		"status":             {tenant.Status},
		"room_label":         {tenant.RoomLabel},
		"room_address":       {tenant.RoomAddress},
		"property_hint":      {tenant.RoomLabel + " property"},
	}
}

func extractE2ETenantID(body []byte, tenantName string) (uint64, error) {
	text := string(body)
	nameIndex := strings.Index(text, tenantName)
	if nameIndex < 0 {
		return 0, errors.New("tenant name is not present in response")
	}
	rowStart := strings.LastIndex(text[:nameIndex], "<tr")
	if rowStart < 0 {
		return 0, errors.New("tenant table row is not present in response")
	}
	rowEnd := strings.Index(text[nameIndex:], "</tr>")
	if rowEnd < 0 {
		return 0, errors.New("tenant table row is incomplete")
	}
	row := text[rowStart : nameIndex+rowEnd]
	match := e2eTenantIDPattern.FindStringSubmatch(row)
	if len(match) != 2 {
		return 0, errors.New("tenant detail link is not present in response")
	}
	tenantID, err := strconv.ParseUint(match[1], 10, 64)
	if err != nil || tenantID == 0 {
		return 0, errors.New("tenant detail link has an invalid ID")
	}
	return tenantID, nil
}

func formatE2EMoney(money e2eMoney) string {
	if money.Cents < 0 {
		return "-" + formatE2EMoney(e2eMoney{Cents: -money.Cents, Currency: money.Currency})
	}
	return strconv.FormatInt(money.Cents/100, 10) + "." + fmt.Sprintf("%02d", money.Cents%100)
}
