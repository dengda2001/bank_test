package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-sql-driver/mysql"
)

func probeE2ETarget(ctx context.Context, options e2eOptions) e2eScenarioReport {
	scenario := e2eScenarioReport{Name: "target-probe", Status: "running", Steps: []e2eStepReport{}}
	fail := func(message string) e2eScenarioReport {
		scenario.Status = "failed"
		scenario.Error = message
		return scenario
	}
	parsedDSN, err := mysql.ParseDSN(options.MySQLDSN)
	if err != nil || parsedDSN.DBName == "" {
		return fail("MySQL DSN must identify a concrete database name")
	}
	databaseStep := e2eStepReport{
		Method:   "PROBE",
		Path:     "database identity",
		Expected: map[string]any{"database_name": options.DatabaseAllowlist},
		Actual:   map[string]any{"database_name": parsedDSN.DBName},
		Passed:   parsedDSN.DBName == options.DatabaseAllowlist,
	}
	if !databaseStep.Passed {
		databaseStep.Error = "DSN database name does not match the explicit allowlist"
	}
	scenario.Steps = append(scenario.Steps, databaseStep)
	if !databaseStep.Passed {
		return fail("database name allowlist probe failed")
	}

	db, err := sql.Open("mysql", options.MySQLDSN)
	if err != nil {
		return fail("open MySQL probe connection: " + err.Error())
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return fail("ping MySQL probe connection: " + err.Error())
	}
	var actualDatabase string
	if err := db.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&actualDatabase); err != nil {
		return fail("query MySQL database identity: " + err.Error())
	}
	actualDatabase = strings.TrimSpace(actualDatabase)
	identityStep := e2eStepReport{
		Method:   "PROBE",
		Path:     "SELECT DATABASE()",
		Expected: map[string]any{"database_name": options.DatabaseAllowlist},
		Actual:   map[string]any{"database_name": actualDatabase},
		Passed:   actualDatabase == options.DatabaseAllowlist,
	}
	if !identityStep.Passed {
		identityStep.Error = "connected MySQL database does not match the explicit allowlist"
	}
	scenario.Steps = append(scenario.Steps, identityStep)
	if !identityStep.Passed {
		return fail("connected database identity probe failed")
	}

	client, err := newE2EHTTPClient(options.BaseURL)
	if err != nil {
		return fail("create HTTP target probe client: " + err.Error())
	}
	response, err := client.do(ctx, http.MethodGet, "/", nil)
	httpStep := e2eHTTPStep(http.MethodGet, "/", map[string]any{
		"status_code": "200 or 302",
	}, response, err)
	if err == nil {
		httpStep.Passed = response.StatusCode == http.StatusOK || response.StatusCode == http.StatusFound
		httpStep.Actual.(map[string]any)["reachable"] = httpStep.Passed
		if !httpStep.Passed {
			httpStep.Error = fmt.Sprintf("target root returned unexpected status %d", response.StatusCode)
		}
	}
	scenario.Steps = append(scenario.Steps, httpStep)
	if !httpStep.Passed {
		return fail("HTTP target reachability probe failed")
	}

	scenario.Status = "passed"
	return scenario
}

func validateE2ETargetProbeOptions(options e2eOptions) error {
	if strings.TrimSpace(options.MySQLDSN) == "" {
		return errors.New("MySQL DSN is required")
	}
	if strings.TrimSpace(options.DatabaseAllowlist) == "" {
		return errors.New("database allowlist is required")
	}
	return nil
}
