package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func verifyE2EHTTPNoResidue(ctx context.Context, options e2eOptions, manifest e2eFixtureManifest, artifacts e2EBusinessArtifacts) e2eScenarioReport {
	scenario := e2eScenarioReport{Name: "post-cleanup-http-verification", Status: "running", Steps: []e2eStepReport{}}
	fail := func(message string) e2eScenarioReport {
		scenario.Status = "failed"
		scenario.Error = message
		return scenario
	}
	if artifacts.MainTenantID == 0 || artifacts.DunningTenantID == 0 || artifacts.TransactionID == 0 {
		return fail("post-cleanup HTTP verification requires concrete business IDs")
	}
	client, err := newE2EHTTPClient(options.BaseURL)
	if err != nil {
		return fail("create post-cleanup HTTP verification client failed")
	}

	loginResponse, err := client.do(ctx, http.MethodPost, "/login-local", url.Values{
		"username": {options.SecondUsername},
		"password": {options.SecondPassword},
	})
	loginStep := e2eHTTPStep(http.MethodPost, "/login-local", map[string]any{
		"status_code":        http.StatusFound,
		"location":           "/rent-dashboard",
		"session_cookie_set": true,
	}, loginResponse, err)
	if err == nil {
		loginStep.Passed = loginResponse.StatusCode == http.StatusFound && loginResponse.Location == "/rent-dashboard" && containsString(loginResponse.CookieNames, "rentops_session")
		if !loginStep.Passed {
			loginStep.Error = "second account could not establish a post-cleanup session"
		}
	}
	scenario.Steps = append(scenario.Steps, loginStep)
	if !loginStep.Passed {
		return fail("post-cleanup HTTP verification could not authenticate the second account")
	}

	markers := e2EFixtureHTTPMarkers(manifest)
	// The application chrome renders the operator account name, which is itself
	// derived from the run ID. Removing the account names keeps the scan focused
	// on business data that must be gone after cleanup.
	scrubAccountNames := strings.NewReplacer(options.Username, "", options.SecondUsername, "")
	for _, check := range []struct {
		name         string
		path         string
		expectedCode int
	}{
		{name: "main tenant detail", path: fmt.Sprintf("/tenants/%d", artifacts.MainTenantID), expectedCode: http.StatusNotFound},
		{name: "dunning tenant detail", path: fmt.Sprintf("/tenants/%d", artifacts.DunningTenantID), expectedCode: http.StatusNotFound},
		{name: "tenant list", path: "/tenants", expectedCode: http.StatusOK},
		{name: "billing list", path: "/billing", expectedCode: http.StatusOK},
	} {
		response, requestErr := client.do(ctx, http.MethodGet, check.path, nil)
		step := e2eHTTPStep(http.MethodGet, check.path, map[string]any{
			"status_code":     check.expectedCode,
			"run_data_absent": true,
		}, response, requestErr)
		markersPresent := false
		if requestErr == nil {
			markersPresent = e2EBodyContainsAny([]byte(scrubAccountNames.Replace(string(response.Body))), markers)
			actual := step.Actual.(map[string]any)
			actual["run_data_absent"] = !markersPresent
			actual["response_body_bytes"] = len(response.Body)
			step.Passed = response.StatusCode == check.expectedCode && !markersPresent
			if !step.Passed {
				step.Error = fmt.Sprintf("%s still exposes run-scoped data or returned status %d", check.name, response.StatusCode)
			}
		}
		scenario.Steps = append(scenario.Steps, step)
		if !step.Passed {
			return fail("post-cleanup HTTP residue verification failed")
		}
	}

	scenario.Status = "passed"
	return scenario
}

func e2EFixtureHTTPMarkers(manifest e2eFixtureManifest) []string {
	markers := []string{
		manifest.RunID,
		manifest.Tenant.Name,
		manifest.Tenant.DisplayAlias,
		manifest.Tenant.Email,
		manifest.Tenant.RoomLabel,
		manifest.Tenant.RoomAddress,
		manifest.Payer.Name,
		manifest.Payer.PayerID,
		manifest.DunningTenant.Name,
		manifest.DunningTenant.DisplayAlias,
		manifest.DunningTenant.Email,
		manifest.DunningTenant.RoomLabel,
		manifest.DunningTenant.RoomAddress,
		manifest.DunningTenant.PayerNameHint,
		manifest.DunningTenant.PayerID,
		manifest.Bank.AccountID,
		manifest.Bank.AccountName,
		manifest.Bank.BatchID,
	}
	for _, transaction := range manifest.Bank.Transactions {
		markers = append(markers,
			transaction.TransactionID,
			transaction.NormalisedProviderTransactionID,
			transaction.StoredProviderTransactionID(),
			transaction.Description,
			transaction.Reference,
			transaction.PayerID,
			transaction.PayerName,
		)
	}
	seen := make(map[string]struct{}, len(markers))
	result := make([]string, 0, len(markers))
	for _, marker := range markers {
		marker = strings.TrimSpace(marker)
		if marker == "" {
			continue
		}
		if _, ok := seen[marker]; ok {
			continue
		}
		seen[marker] = struct{}{}
		result = append(result, marker)
	}
	return result
}

func e2EBodyContainsAny(body []byte, markers []string) bool {
	text := string(body)
	for _, marker := range markers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}
