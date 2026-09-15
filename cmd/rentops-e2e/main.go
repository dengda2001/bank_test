package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	options, err := e2eOptionsFromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	flag.StringVar(&options.BaseURL, "base-url", options.BaseURL, "running RentOps origin")
	flag.StringVar(&options.RunID, "run-id", options.RunID, "isolated run ID")
	flag.StringVar(&options.ReportPath, "report", options.ReportPath, "JSON report path")
	flag.StringVar(&options.TargetName, "target", options.TargetName, "exact non-production target name")
	flag.BoolVar(&options.Execute, "execute", false, "run write-enabled E2E scenarios")
	flag.Parse()
	options.BaseURL = strings.TrimRight(strings.TrimSpace(options.BaseURL), "/")
	if options.RunID == "" {
		options.RunID, err = newRunID(time.Now().UTC())
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	}
	if options.BaseURL == "" {
		options.BaseURL = "http://127.0.0.1:8080"
	}
	if options.ReportPath == "" {
		options.ReportPath = filepath.Join(os.TempDir(), options.RunID+".json")
	}
	preflight := validateE2EOptions(options)
	report := newE2EReport(preflight, time.Now().UTC())
	if manifest, manifestErr := newE2EFixtureManifest(options.RunID, report.StartedAt); manifestErr == nil {
		report.Fixture = &manifest
	}
	if !preflight.Passed {
		report.Error = "E2E preflight failed; no network or data mutation was attempted"
		if err := writeE2EReport(options.ReportPath, report); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
		fmt.Fprintln(os.Stderr, report.Error)
		for _, failure := range preflight.Failures {
			fmt.Fprintln(os.Stderr, "-", failure)
		}
		os.Exit(1)
	}
	if options.Execute {
		report.Status = "not_implemented"
		report.Error = "write-enabled E2E scenarios are not implemented yet"
		_ = writeE2EReport(options.ReportPath, report)
		fmt.Fprintln(os.Stderr, report.Error)
		os.Exit(1)
	}
	if err := writeE2EReport(options.ReportPath, report); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(options.ReportPath)
}
