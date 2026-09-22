package main

import (
	"context"
	"errors"
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
	if err := recordE2EScenario(report, probeE2ETarget(ctx, options), writeReport); err != nil {
		setE2ECleanupNotRun(report, "cleanup was not run because target preflight failed")
		_ = persistE2EProgress(report, writeReport)
		return err
	}
	artifacts := e2EBusinessArtifacts{}
	if err := runE2EBusinessScenarios(ctx, options, manifest, report, writeReport, &artifacts); err != nil {
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
	postCleanup := verifyE2EHTTPNoResidue(ctx, options, manifest, artifacts)
	if err := recordE2EScenario(report, postCleanup, writeReport); err != nil {
		report.Cleanup.HTTPVerified = false
		report.Error = firstE2EError(postCleanup.Error, "post-cleanup HTTP verification failed")
		_ = persistE2EProgress(report, writeReport)
		return err
	}
	report.Cleanup.HTTPVerified = true
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
