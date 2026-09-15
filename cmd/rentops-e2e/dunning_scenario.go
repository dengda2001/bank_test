package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func (c *e2eHTTPClient) dunningScenario(ctx context.Context, manifest e2eFixtureManifest, obligationID uint64) e2eScenarioReport {
	scenario := e2eScenarioReport{Name: "dunning", Status: "running", Steps: []e2eStepReport{}}
	fail := func(message string) e2eScenarioReport {
		scenario.Status = "failed"
		scenario.Error = message
		return scenario
	}
	if obligationID == 0 {
		return fail("dunning scenario requires an obligation ID")
	}
	period := "2026-09"
	baseForm := url.Values{
		"period":    {period},
		"status":    {"unpaid"},
		"sort":      {"due_asc"},
		"page":      {"1"},
		"page_size": {"12"},
	}
	configForm := cloneE2EValues(baseForm)
	configForm.Set("display_name", manifest.RunID+" landlord")
	configForm.Set("reply_to_email", manifest.RunID+"@invalid.test")
	configResponse, err := c.do(ctx, http.MethodPost, "/dunning/config", configForm)
	configStep := e2eHTTPStep(http.MethodPost, "/dunning/config", map[string]any{
		"status_code":         http.StatusOK,
		"configuration_saved": true,
	}, configResponse, err)
	if err == nil {
		saved := strings.Contains(string(configResponse.Body), "发件配置已保存") || strings.Contains(string(configResponse.Body), manifest.RunID+" landlord")
		configStep.Actual.(map[string]any)["configuration_saved"] = saved
		configStep.Passed = configResponse.StatusCode == http.StatusOK && saved
		if !configStep.Passed {
			configStep.Error = "dunning sender configuration was not acknowledged"
		}
	}
	scenario.Steps = append(scenario.Steps, configStep)
	if !configStep.Passed {
		return fail("dunning configuration failed")
	}

	previewForm := cloneE2EValues(baseForm)
	previewForm.Set("obligation_id", strconv.FormatUint(obligationID, 10))
	previewForm.Set("request_key", manifest.RunID+"-dunning-preview")
	previewResponse, err := c.do(ctx, http.MethodPost, "/dunning/preview", previewForm)
	previewStep := e2eHTTPStep(http.MethodPost, "/dunning/preview", map[string]any{
		"status_code":     http.StatusOK,
		"preview_visible": true,
	}, previewResponse, err)
	if err == nil {
		previewVisible := strings.Contains(string(previewResponse.Body), manifest.DunningTenant.Name) || strings.Contains(string(previewResponse.Body), "催缴") || strings.Contains(string(previewResponse.Body), "预览")
		previewStep.Actual.(map[string]any)["preview_visible"] = previewVisible
		previewStep.Passed = previewResponse.StatusCode == http.StatusOK && previewVisible
		if !previewStep.Passed {
			previewStep.Error = "dunning preview did not expose a preview result"
		}
	}
	scenario.Steps = append(scenario.Steps, previewStep)
	if !previewStep.Passed {
		return fail("dunning preview failed")
	}

	sendForm := cloneE2EValues(previewForm)
	sendForm.Set("request_key", manifest.RunID+"-dunning-send")
	sendResponse, err := c.do(ctx, http.MethodPost, "/dunning/send", sendForm)
	sendStep := e2eHTTPStep(http.MethodPost, "/dunning/send", map[string]any{
		"status_code":      http.StatusOK,
		"delivery_outcome": "sent or queued",
	}, sendResponse, err)
	if err == nil {
		outcome := e2eDunningOutcome(sendResponse.Body)
		candidateVisible := strings.Contains(string(sendResponse.Body), manifest.DunningTenant.Name)
		sendStep.Actual.(map[string]any)["delivery_outcome"] = outcome
		sendStep.Actual.(map[string]any)["candidate_visible"] = candidateVisible
		sendStep.Passed = sendResponse.StatusCode == http.StatusOK && candidateVisible && (outcome == "sent" || outcome == "queued")
		if !sendStep.Passed {
			sendStep.Error = "dunning send did not produce a sent or queued outcome"
		}
	}
	scenario.Steps = append(scenario.Steps, sendStep)
	if !sendStep.Passed {
		return fail("dunning send failed")
	}

	repeatResponse, err := c.do(ctx, http.MethodPost, "/dunning/send", sendForm)
	repeatStep := e2eHTTPStep(http.MethodPost, "/dunning/send", map[string]any{
		"status_code":      http.StatusOK,
		"delivery_outcome": "same request remains safe",
	}, repeatResponse, err)
	if err == nil {
		outcome := e2eDunningOutcome(repeatResponse.Body)
		candidateVisible := strings.Contains(string(repeatResponse.Body), manifest.DunningTenant.Name)
		repeatStep.Actual.(map[string]any)["delivery_outcome"] = outcome
		repeatStep.Actual.(map[string]any)["candidate_visible"] = candidateVisible
		repeatStep.Passed = repeatResponse.StatusCode == http.StatusOK && candidateVisible && (outcome == "sent" || outcome == "queued")
		if !repeatStep.Passed {
			repeatStep.Error = "repeated dunning request did not preserve a safe delivery outcome"
		}
	}
	scenario.Steps = append(scenario.Steps, repeatStep)
	if !repeatStep.Passed {
		return fail("dunning request idempotency failed")
	}

	scenario.Status = "passed"
	return scenario
}

func cloneE2EValues(values url.Values) url.Values {
	clone := make(url.Values, len(values))
	for key, items := range values {
		clone[key] = append([]string(nil), items...)
	}
	return clone
}

func e2eDunningOutcome(body []byte) string {
	text := string(body)
	if strings.Contains(text, "已发送") {
		return "sent"
	}
	if strings.Contains(text, "排队中") {
		return "queued"
	}
	if strings.Contains(text, "发送失败") {
		return "failed"
	}
	if strings.Contains(text, "已跳过") {
		return "skipped"
	}
	return fmt.Sprintf("unknown (%d bytes)", len(body))
}
