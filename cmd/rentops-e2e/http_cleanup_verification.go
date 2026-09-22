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
	if len(artifacts.TenantIDs) == 0 || len(artifacts.RoomIDs) == 0 {
		scenario.Status, scenario.Error = "failed", "post-cleanup HTTP verification requires concrete business IDs"
		return scenario
	}
	client, err := newE2EHTTPClient(options.BaseURL)
	if err != nil {
		scenario.Status, scenario.Error = "failed", "create post-cleanup HTTP verification client failed"
		return scenario
	}
	loginResponse, err := client.do(ctx, http.MethodPost, "/login-local", url.Values{"username": {options.SecondUsername}, "password": {options.SecondPassword}})
	loginStep := e2eHTTPStep(http.MethodPost, "/login-local", map[string]any{"status_code": http.StatusFound, "location": "/rent-dashboard"}, loginResponse, err)
	if err == nil {
		loginStep.Passed = loginResponse.StatusCode == http.StatusFound && loginResponse.Location == "/rent-dashboard"
		if !loginStep.Passed {
			loginStep.Error = "second account could not establish a post-cleanup session"
		}
	}
	scenario.Steps = append(scenario.Steps, loginStep)
	if !loginStep.Passed {
		scenario.Status, scenario.Error = "failed", "post-cleanup HTTP verification could not authenticate the second account"
		return scenario
	}
	for _, check := range []struct {
		name, path string
		status     int
		markers    []string
	}{
		{name: "primary tenant detail", path: fmt.Sprintf("/tenants/%d", artifacts.TenantIDs[0]), status: http.StatusNotFound, markers: []string{manifest.TenantA.Name, manifest.TenantA.PayerID}},
		{name: "primary room detail", path: fmt.Sprintf("/rooms/%d?period=%s", artifacts.RoomIDs[0], manifest.Period), status: http.StatusNotFound, markers: []string{manifest.RunID + " Rosewood Court"}},
		{name: "tenant list", path: "/tenants", status: http.StatusOK, markers: []string{manifest.TenantA.Name, manifest.TenantB.Name, manifest.TenantC.Name, manifest.TenantD.Name}},
		{name: "rent dashboard", path: "/rent-dashboard?view=tenants&period=" + url.QueryEscape(manifest.Period), status: http.StatusOK, markers: []string{manifest.TenantA.Name, manifest.TenantB.Name, manifest.TenantC.Name, manifest.TenantD.Name}},
		// 列表页的两张表不再渲染流水的 description（那句 "… rent payment" 只活在
		// 详情页上），所以这里换成付款人姓名和账户名——同样带 RunID，同样只在
		// 这一轮的流水还在时才出现在 /transactions 上。
		{name: "transaction list", path: "/transactions", status: http.StatusOK, markers: []string{manifest.TenantA.PayerName, manifest.RunID + " E2E account"}},
	} {
		response, requestErr := client.do(ctx, http.MethodGet, check.path, nil)
		step := e2eHTTPStep(http.MethodGet, check.path, map[string]any{"status_code": check.status, "run_data_absent": true}, response, requestErr)
		if requestErr == nil {
			body := string(response.Body)
			absent := true
			for _, marker := range check.markers {
				absent = absent && !strings.Contains(body, marker)
			}
			step.Passed = response.StatusCode == check.status && absent
			step.Actual.(map[string]any)["run_data_absent"] = absent
			step.Actual.(map[string]any)["response_body_bytes"] = len(response.Body)
			if !step.Passed {
				step.Error = check.name + " still exposes run-scoped data or returned an unexpected status"
			}
		}
		scenario.Steps = append(scenario.Steps, step)
		if !step.Passed {
			scenario.Status, scenario.Error = "failed", "post-cleanup HTTP residue verification failed"
			return scenario
		}
	}
	scenario.Status = "passed"
	return scenario
}
