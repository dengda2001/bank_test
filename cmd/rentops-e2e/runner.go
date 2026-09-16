package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	e2eWriteConfirmation   = "I_UNDERSTAND_NON_PRODUCTION"
	e2eCleanupConfirmation = "I_UNDERSTAND_DELETE_RUN_ID_ONLY"
)

var e2eRunIDPattern = regexp.MustCompile(`^rentops-e2e-[0-9]{8}-[0-9]{6}-[a-f0-9]{8}$`)

type e2eOptions struct {
	BaseURL           string
	RunID             string
	ReportPath        string
	TargetName        string
	TargetAllowlist   string
	DatabaseAllowlist string
	MySQLDSN          string
	FixtureDir        string
	Username          string
	Password          string
	SecondUsername    string
	SecondPassword    string
	SMTPSink          string
	SkipDunning       bool
	Execute           bool
	ConfirmWrites     string
	ConfirmCleanup    string
	AllowRemote       bool
}

type e2ePreflight struct {
	Passed         bool   `json:"passed"`
	Mode           string `json:"mode"`
	TargetName     string `json:"target_name,omitempty"`
	BaseURL        string `json:"base_url"`
	RunID          string `json:"run_id"`
	WriteEnabled   bool   `json:"write_enabled"`
	CleanupEnabled bool   `json:"cleanup_enabled"`
	// DunningDeliverySkipped records that mail delivery was excluded by explicit
	// configuration, so the report never presents it as covered.
	DunningDeliverySkipped bool     `json:"dunning_delivery_skipped,omitempty"`
	Checks                 []string `json:"checks"`
	Failures               []string `json:"failures,omitempty"`
}

type e2eStepReport struct {
	Method     string `json:"method"`
	Path       string `json:"path"`
	StatusCode int    `json:"status_code,omitempty"`
	Expected   any    `json:"expected,omitempty"`
	Actual     any    `json:"actual,omitempty"`
	Passed     bool   `json:"passed"`
	Error      string `json:"error,omitempty"`
}

type e2eScenarioReport struct {
	Name   string          `json:"name"`
	Status string          `json:"status"`
	Steps  []e2eStepReport `json:"steps,omitempty"`
	Error  string          `json:"error,omitempty"`
	// SkipReason explains why a scenario was excluded. A skipped scenario is
	// never counted as a passed one.
	SkipReason string `json:"skip_reason,omitempty"`
}

// e2eUnverifiedScenario makes the gap between "the run passed" and "everything
// in the acceptance criteria was exercised" explicit and machine-readable.
type e2eUnverifiedScenario struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

type e2eReport struct {
	RunID      string              `json:"run_id"`
	Mode       string              `json:"mode"`
	TargetName string              `json:"target_name,omitempty"`
	Fixture    *e2eFixtureManifest `json:"fixture,omitempty"`
	StartedAt  time.Time           `json:"started_at"`
	FinishedAt time.Time           `json:"finished_at"`
	Status     string              `json:"status"`
	Preflight  e2ePreflight        `json:"preflight"`
	Scenarios  []e2eScenarioReport `json:"scenarios,omitempty"`
	// Unverified lists scenarios that did not run. Status "passed" only means
	// every executed assertion passed.
	Unverified []e2eUnverifiedScenario `json:"unverified,omitempty"`
	Cleanup    *e2eCleanupReport       `json:"cleanup,omitempty"`
	Error      string                  `json:"error,omitempty"`
}

type e2eCleanupReport struct {
	Status         string `json:"status"`
	Verified       bool   `json:"verified"`
	HTTPVerified   bool   `json:"http_verified"`
	FixtureRemoved bool   `json:"fixture_removed"`
	Error          string `json:"error,omitempty"`
}

func newRunID(now time.Time) (string, error) {
	suffix := make([]byte, 4)
	if _, err := rand.Read(suffix); err != nil {
		return "", fmt.Errorf("generate run ID: %w", err)
	}
	return fmt.Sprintf("rentops-e2e-%s-%s", now.UTC().Format("20060102-150405"), hex.EncodeToString(suffix)), nil
}

func validateE2ERunID(value string) error {
	if !e2eRunIDPattern.MatchString(strings.TrimSpace(value)) {
		return errors.New("run ID must match rentops-e2e-YYYYMMDD-HHMMSS-xxxxxxxx")
	}
	return nil
}

func e2eOptionsFromEnv() (e2eOptions, error) {
	return e2eOptions{
		BaseURL:           strings.TrimRight(strings.TrimSpace(os.Getenv("RENTOPS_E2E_BASE_URL")), "/"),
		RunID:             strings.TrimSpace(os.Getenv("RENTOPS_E2E_RUN_ID")),
		ReportPath:        strings.TrimSpace(os.Getenv("RENTOPS_E2E_REPORT_PATH")),
		TargetName:        strings.TrimSpace(os.Getenv("RENTOPS_E2E_TARGET_NAME")),
		TargetAllowlist:   strings.TrimSpace(os.Getenv("RENTOPS_E2E_TARGET_ALLOWLIST")),
		DatabaseAllowlist: strings.TrimSpace(os.Getenv("RENTOPS_E2E_DATABASE_ALLOWLIST")),
		MySQLDSN:          strings.TrimSpace(firstE2EEnv("RENTOPS_E2E_MYSQL_DSN", "RENTOPS_MYSQL_TEST_DSN", "MYSQL_DSN")),
		FixtureDir:        strings.TrimSpace(os.Getenv("RENTOPS_E2E_FIXTURE_DIR")),
		Username:          os.Getenv("RENTOPS_E2E_USERNAME"),
		Password:          os.Getenv("RENTOPS_E2E_PASSWORD"),
		SecondUsername:    os.Getenv("RENTOPS_E2E_SECOND_USERNAME"),
		SecondPassword:    os.Getenv("RENTOPS_E2E_SECOND_PASSWORD"),
		SMTPSink:          strings.TrimSpace(os.Getenv("RENTOPS_E2E_SMTP_SINK")),
		SkipDunning:       os.Getenv("RENTOPS_E2E_SKIP_DUNNING_DELIVERY") == "1",
		ConfirmWrites:     strings.TrimSpace(os.Getenv("RENTOPS_E2E_CONFIRM_WRITES")),
		ConfirmCleanup:    strings.TrimSpace(os.Getenv("RENTOPS_E2E_CONFIRM_CLEANUP")),
		AllowRemote:       os.Getenv("RENTOPS_E2E_ALLOW_REMOTE") == "1",
	}, nil
}

func firstE2EEnv(keys ...string) string {
	for _, key := range keys {
		if value := os.Getenv(key); value != "" {
			return value
		}
	}
	return ""
}

func validateE2EOptions(options e2eOptions) e2ePreflight {
	mode := "dry-run"
	if options.Execute {
		mode = "execute"
	}
	preflight := e2ePreflight{
		Passed:       true,
		Mode:         mode,
		TargetName:   options.TargetName,
		BaseURL:      options.BaseURL,
		RunID:        options.RunID,
		WriteEnabled: options.Execute,
		Checks:       []string{},
		Failures:     []string{},
	}
	fail := func(message string) {
		preflight.Passed = false
		preflight.Failures = append(preflight.Failures, message)
	}
	if err := validateE2ERunID(options.RunID); err != nil {
		fail(err.Error())
	} else {
		preflight.Checks = append(preflight.Checks, "run ID has an explicit isolated format")
	}
	parsedURL, err := url.Parse(options.BaseURL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" || parsedURL.Path != "" || parsedURL.RawQuery != "" || parsedURL.Fragment != "" || parsedURL.User != nil {
		fail("base URL must be an absolute origin without a path")
	} else if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		fail("base URL scheme must be http or https")
	} else {
		preflight.Checks = append(preflight.Checks, "base URL is an absolute origin")
		if !isLoopbackHost(parsedURL.Hostname()) && !options.AllowRemote {
			fail("remote base URL requires RENTOPS_E2E_ALLOW_REMOTE=1")
		} else {
			preflight.Checks = append(preflight.Checks, "base URL target is explicitly local or remote-enabled")
		}
	}
	if !options.Execute {
		preflight.Checks = append(preflight.Checks, "dry-run mode does not authorize network writes")
		return preflight
	}
	if options.TargetName == "" || options.TargetAllowlist == "" || options.TargetName != options.TargetAllowlist {
		fail("target name must exactly match the explicit non-production allowlist")
	} else {
		preflight.Checks = append(preflight.Checks, "target name exactly matches non-production allowlist")
	}
	if options.MySQLDSN == "" {
		fail("an isolated MySQL DSN is required for execute mode")
	} else {
		preflight.Checks = append(preflight.Checks, "an isolated MySQL DSN is present")
	}
	if options.FixtureDir == "" {
		fail("a dedicated empty legacy fixture directory is required")
	} else {
		preflight.Checks = append(preflight.Checks, "a dedicated legacy fixture directory is configured")
	}
	if options.Username == "" || options.Password == "" {
		fail("dedicated E2E login credentials are required")
	} else {
		preflight.Checks = append(preflight.Checks, "dedicated E2E login credentials are present")
	}
	if !strings.HasPrefix(options.Username, options.RunID) {
		fail("primary E2E username must start with the run ID")
	} else {
		preflight.Checks = append(preflight.Checks, "primary E2E username is run-ID scoped")
	}
	if options.SecondUsername == "" || options.SecondPassword == "" {
		fail("a dedicated second E2E account is required for cross-user checks")
	} else if options.SecondUsername == options.Username {
		fail("primary and second E2E accounts must be different")
	} else {
		preflight.Checks = append(preflight.Checks, "dedicated second E2E account credentials are present")
	}
	if options.DatabaseAllowlist == "" {
		fail("an explicit non-production database name allowlist is required")
	} else {
		preflight.Checks = append(preflight.Checks, "an explicit non-production database name allowlist is present")
	}
	if options.SkipDunning {
		preflight.DunningDeliverySkipped = true
		preflight.Checks = append(preflight.Checks, "dunning delivery is explicitly skipped; its scenarios stay unverified")
	} else if options.SMTPSink == "" {
		fail("an explicit SMTP sink identifier is required for dunning send checks")
	} else {
		preflight.Checks = append(preflight.Checks, "an explicit SMTP sink identifier is present")
	}
	if options.ConfirmWrites != e2eWriteConfirmation {
		fail("explicit non-production write confirmation is required")
	} else {
		preflight.Checks = append(preflight.Checks, "write confirmation is explicit")
	}
	if options.ConfirmCleanup != e2eCleanupConfirmation {
		fail("explicit run-ID-only cleanup confirmation is required")
	} else {
		preflight.CleanupEnabled = true
		preflight.Checks = append(preflight.Checks, "cleanup confirmation is explicit and run-ID scoped")
	}
	return preflight
}

func isLoopbackHost(host string) bool {
	switch strings.ToLower(strings.TrimSpace(host)) {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}

func newE2EReport(preflight e2ePreflight, now time.Time) e2eReport {
	status := "ready"
	if preflight.Mode == "dry-run" {
		status = "dry-run"
	}
	if !preflight.Passed {
		status = "blocked"
	}
	return e2eReport{
		RunID:      preflight.RunID,
		Mode:       preflight.Mode,
		TargetName: preflight.TargetName,
		StartedAt:  now.UTC(),
		FinishedAt: now.UTC(),
		Status:     status,
		Preflight:  preflight,
		Scenarios:  []e2eScenarioReport{},
	}
}

func writeE2EReport(path string, report e2eReport) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("report path is required")
	}
	cleanReport := redactE2EValue(report)
	body, err := json.MarshalIndent(cleanReport, "", "  ")
	if err != nil {
		return fmt.Errorf("encode report: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create report directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".rentops-e2e-report-*")
	if err != nil {
		return fmt.Errorf("create report temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("protect report temp file: %w", err)
	}
	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write report: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close report: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("publish report: %w", err)
	}
	return nil
}

func redactE2EValue(value any) any {
	switch typed := value.(type) {
	case e2eReport:
		return e2eReport{
			RunID: typed.RunID, Mode: typed.Mode, TargetName: typed.TargetName,
			Fixture:   typed.Fixture,
			StartedAt: typed.StartedAt, FinishedAt: typed.FinishedAt, Status: typed.Status,
			Preflight:  redactE2EValue(typed.Preflight).(e2ePreflight),
			Scenarios:  redactE2EValue(typed.Scenarios).([]e2eScenarioReport),
			Unverified: typed.Unverified,
			Cleanup:    typed.Cleanup,
			Error:      typed.Error,
		}
	case e2ePreflight:
		return typed
	case []e2eScenarioReport:
		result := make([]e2eScenarioReport, len(typed))
		for index, scenario := range typed {
			result[index] = scenario
			result[index].Steps = make([]e2eStepReport, len(scenario.Steps))
			for stepIndex, step := range scenario.Steps {
				result[index].Steps[stepIndex] = step
				result[index].Steps[stepIndex].Expected = redactE2EValue(step.Expected)
				result[index].Steps[stepIndex].Actual = redactE2EValue(step.Actual)
			}
		}
		return result
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			if isSensitiveE2EKey(key) {
				result[key] = "[REDACTED]"
				continue
			}
			result[key] = redactE2EValue(item)
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for index, item := range typed {
			result[index] = redactE2EValue(item)
		}
		return result
	default:
		return value
	}
}

func isSensitiveE2EKey(key string) bool {
	key = strings.ToLower(strings.ReplaceAll(key, "-", "_"))
	for _, marker := range []string{"password", "secret", "token", "dsn", "authorization", "cookie"} {
		if strings.Contains(key, marker) {
			return true
		}
	}
	return false
}
