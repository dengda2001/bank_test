package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func executeE2ERun(ctx context.Context, options e2eOptions, manifest e2eFixtureManifest, report *e2eReport, writeReport e2EReportWriter) error {
	if report == nil {
		return errors.New("E2E report is required")
	}
	report.Status = "running"
	report.Error = ""
	if err := persistE2EProgress(report, writeReport); err != nil {
		return err
	}

	probe := probeE2ETarget(ctx, options)
	if err := recordE2EScenario(report, probe, writeReport); err != nil {
		return err
	}

	fixtureScenario := e2eScenarioReport{
		Name:   "legacy-fixture-materialization",
		Status: "running",
		Steps:  []e2eStepReport{{Method: "FILES", Path: "legacy fixture directory", Expected: map[string]any{"empty_directory": true, "private_files": true}, Actual: map[string]any{}}},
	}
	paths, err := materializeE2ELegacyFixtures(manifest, options.FixtureDir)
	if err != nil {
		fixtureScenario.Status = "failed"
		fixtureScenario.Error = "legacy fixture materialization failed"
		fixtureScenario.Steps[0].Error = fixtureScenario.Error
		if recordErr := recordE2EScenario(report, fixtureScenario, writeReport); recordErr != nil {
			return recordErr
		}
		return errors.New(fixtureScenario.Error)
	}
	fixtureScenario.Status = "passed"
	fixtureScenario.Steps[0].Passed = true
	fixtureScenario.Steps[0].Actual = map[string]any{
		"empty_directory": true,
		"private_files":   true,
		"files":           []string{filepath.Base(paths.BankLogFile), filepath.Base(paths.TenantFile), filepath.Base(paths.ExpenseFile)},
	}
	if err := recordE2EScenario(report, fixtureScenario, writeReport); err != nil {
		return err
	}

	if err := runE2EBusinessScenarios(ctx, options, manifest, report, writeReport); err != nil {
		setE2ECleanupNotRun(report, "cleanup was not run because a business scenario failed")
		_ = persistE2EProgress(report, writeReport)
		return err
	}

	cleanup := cleanupE2ERun(ctx, options, manifest)
	report.Cleanup = &cleanup
	report.FinishedAt = time.Now().UTC()
	if cleanup.Status != "passed" || !cleanup.Verified {
		report.Status = "failed"
		report.Error = firstE2EError(cleanup.Error, "E2E cleanup failed")
		_ = persistE2EProgress(report, writeReport)
		return errors.New(report.Error)
	}
	if err := removeE2ELegacyFixtures(paths); err != nil {
		report.Status = "failed"
		report.Error = "successful database cleanup left local fixture files"
		report.Cleanup.Status = "failed"
		report.Cleanup.Error = report.Error
		_ = persistE2EProgress(report, writeReport)
		return errors.New(report.Error)
	}
	report.Cleanup.FixtureRemoved = true
	report.Status = "passed"
	report.Error = ""
	report.FinishedAt = time.Now().UTC()
	return persistE2EProgress(report, writeReport)
}

func setE2ECleanupNotRun(report *e2eReport, reason string) {
	if report == nil {
		return
	}
	report.Cleanup = &e2eCleanupReport{Status: "not_run", Error: reason}
	if report.Status == "running" || report.Status == "business_passed" {
		report.Status = "failed"
	}
	report.Error = firstE2EError(report.Error, reason)
	report.FinishedAt = time.Now().UTC()
}

func removeE2ELegacyFixtures(paths e2eLegacyPaths) error {
	files := []string{paths.BankLogFile, paths.TenantFile, paths.ExpenseFile}
	if err := validateE2ELegacyPaths(paths); err != nil {
		return err
	}
	for _, path := range files {
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	directory := filepath.Dir(paths.BankLogFile)
	if err := os.Remove(directory); err != nil {
		return fmt.Errorf("remove empty fixture directory: %w", err)
	}
	return nil
}

func validateE2ELegacyPaths(paths e2eLegacyPaths) error {
	files := map[string]string{
		paths.BankLogFile: "bank-results.jsonl",
		paths.TenantFile:  "tenants.json",
		paths.ExpenseFile: "expenses.json",
	}
	if len(files) != 3 {
		return errors.New("fixture paths must be distinct")
	}
	directory := filepath.Dir(paths.BankLogFile)
	for path, expectedBase := range files {
		if filepath.Dir(path) != directory || filepath.Base(path) != expectedBase {
			return errors.New("fixture path is outside the materialized fixture set")
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return errors.New("fixture path is not a regular file")
		}
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	if len(entries) != len(files) {
		return errors.New("fixture directory contains unexpected files")
	}
	for _, entry := range entries {
		if _, ok := files[filepath.Join(directory, entry.Name())]; !ok {
			return errors.New("fixture directory contains an unexpected file")
		}
	}
	return nil
}
