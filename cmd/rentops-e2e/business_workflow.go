package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type e2EReportWriter func(e2eReport) error

type e2EBusinessArtifacts struct {
	MainTenantID    uint64
	DunningTenantID uint64
	TransactionID   uint64
}

func runE2EBusinessScenarios(ctx context.Context, options e2eOptions, manifest e2eFixtureManifest, report *e2eReport, writeReport e2EReportWriter, artifacts *e2EBusinessArtifacts) error {
	if report == nil {
		return errors.New("E2E report is required")
	}
	if err := manifest.validate(); err != nil {
		return fmt.Errorf("validate E2E fixture manifest: %w", err)
	}
	client, err := newE2EHTTPClient(options.BaseURL)
	if err != nil {
		return fmt.Errorf("create primary E2E HTTP client: %w", err)
	}
	report.Status = "running"
	if err := persistE2EProgress(report, writeReport); err != nil {
		return err
	}

	authentication := client.authenticationScenario(ctx, options.Username, options.Password)
	if err := recordE2EScenario(report, authentication, writeReport); err != nil {
		return err
	}
	tenantScenario, tenantID := client.tenantScenario(ctx, manifest)
	if artifacts != nil {
		artifacts.MainTenantID = tenantID
	}
	if err := recordE2EScenario(report, tenantScenario, writeReport); err != nil {
		return err
	}
	dunningTenantScenario, dunningTenantID := client.dunningTenantScenario(ctx, manifest)
	if artifacts != nil {
		artifacts.DunningTenantID = dunningTenantID
	}
	if err := recordE2EScenario(report, dunningTenantScenario, writeReport); err != nil {
		return err
	}
	bankScenario := client.bankImportScenario(ctx, manifest)
	if err := recordE2EScenario(report, bankScenario, writeReport); err != nil {
		return err
	}
	ledgerScenario := client.ledgerScenario(ctx, manifest, tenantID)
	if err := recordE2EScenario(report, ledgerScenario, writeReport); err != nil {
		return err
	}
	cashScenario := client.cashReceiptScenario(ctx, manifest, tenantID)
	if err := recordE2EScenario(report, cashScenario, writeReport); err != nil {
		return err
	}
	dashboardScenario := client.dashboardScenario(ctx, manifest, tenantID)
	if err := recordE2EScenario(report, dashboardScenario, writeReport); err != nil {
		return err
	}
	dunningCandidateScenario, obligationID := client.dunningCandidateScenario(ctx, manifest)
	if err := recordE2EScenario(report, dunningCandidateScenario, writeReport); err != nil {
		return err
	}
	dunningConfigScenario := client.dunningConfigurationScenario(ctx, manifest, obligationID)
	if err := recordE2EScenario(report, dunningConfigScenario, writeReport); err != nil {
		return err
	}
	dunningScenario := e2eSkippedDunningDeliveryScenario(options)
	if !options.SkipDunning {
		dunningScenario = client.dunningDeliveryScenario(ctx, manifest, obligationID)
	}
	if err := recordE2EScenario(report, dunningScenario, writeReport); err != nil {
		return err
	}

	transactionDiscovery, transactionID := client.discoverE2ECrossUserTransaction(ctx, manifest)
	if artifacts != nil {
		artifacts.TransactionID = transactionID
	}
	if err := recordE2EScenario(report, transactionDiscovery, writeReport); err != nil {
		return err
	}
	secondClient, err := newE2EHTTPClient(options.BaseURL)
	if err != nil {
		return fmt.Errorf("create second E2E HTTP client: %w", err)
	}
	secondAuthentication := secondClient.authenticationScenario(ctx, options.SecondUsername, options.SecondPassword)
	if err := recordE2EScenario(report, secondAuthentication, writeReport); err != nil {
		return err
	}
	crossUserScenario := secondClient.crossUserScenario(ctx, manifest, tenantID, transactionID)
	if err := recordE2EScenario(report, crossUserScenario, writeReport); err != nil {
		return err
	}

	report.Status = "business_passed"
	report.FinishedAt = time.Now().UTC()
	return persistE2EProgress(report, writeReport)
}

// e2eSkippedDunningDeliveryScenario records an explicit gap. The scenario is
// reported as skipped and listed under the report's unverified entries so that a
// passing run is never read as "mail delivery was verified".
func e2eSkippedDunningDeliveryScenario(options e2eOptions) e2eScenarioReport {
	return e2eScenarioReport{
		Name:       "dunning-delivery",
		Status:     "skipped",
		SkipReason: "mail delivery was excluded by RENTOPS_E2E_SKIP_DUNNING_DELIVERY=1",
	}
}

func recordE2EScenario(report *e2eReport, scenario e2eScenarioReport, writeReport e2EReportWriter) error {
	if report == nil {
		return errors.New("E2E report is required")
	}
	report.Scenarios = append(report.Scenarios, scenario)
	report.FinishedAt = time.Now().UTC()
	if scenario.Status == "skipped" {
		report.Unverified = append(report.Unverified, e2eUnverifiedScenario{
			Name:   scenario.Name,
			Reason: firstE2EError(scenario.SkipReason, "scenario was explicitly skipped"),
		})
		return persistE2EProgress(report, writeReport)
	}
	if scenario.Status != "passed" {
		report.Status = "failed"
		report.Error = firstE2EError(scenario.Error, "E2E scenario failed: "+scenario.Name)
		if err := persistE2EProgress(report, writeReport); err != nil {
			return err
		}
		return errors.New(report.Error)
	}
	return persistE2EProgress(report, writeReport)
}

func persistE2EProgress(report *e2eReport, writeReport e2EReportWriter) error {
	if writeReport == nil {
		return nil
	}
	if err := writeReport(*report); err != nil {
		return fmt.Errorf("write E2E progress report: %w", err)
	}
	return nil
}

func firstE2EError(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func (c *e2eHTTPClient) discoverE2ECrossUserTransaction(ctx context.Context, manifest e2eFixtureManifest) (e2eScenarioReport, uint64) {
	scenario := e2eScenarioReport{Name: "cross-user-target-discovery", Status: "running", Steps: []e2eStepReport{}}
	fail := func(message string) (e2eScenarioReport, uint64) {
		scenario.Status = "failed"
		scenario.Error = message
		return scenario, 0
	}
	providerID := manifest.Bank.Transactions[0].StoredProviderTransactionID()
	response, err := c.do(ctx, http.MethodGet, "/billing", nil)
	step := e2eHTTPStep(http.MethodGet, "/billing", map[string]any{
		"status_code":             http.StatusOK,
		"provider_transaction_id": providerID,
		"transaction_id":          "positive ID",
	}, response, err)
	var transactionID uint64
	if err == nil {
		transactionID, err = extractE2ETransactionID(response.Body, providerID)
		step.Actual.(map[string]any)["transaction_id"] = transactionID
		step.Passed = response.StatusCode == http.StatusOK && err == nil && transactionID > 0
		if !step.Passed {
			step.Error = "primary account transaction ID was not discoverable for cross-user checks"
		}
	}
	scenario.Steps = append(scenario.Steps, step)
	if !step.Passed {
		return fail("cross-user target transaction discovery failed")
	}
	scenario.Status = "passed"
	return scenario, transactionID
}
