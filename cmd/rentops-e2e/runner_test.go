package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testRunID = "rentops-e2e-20260916-120000-a1b2c3d4"

func TestValidateE2EOptionsDryRunDoesNotAuthorizeWrites(t *testing.T) {
	preflight := validateE2EOptions(e2eOptions{BaseURL: "http://127.0.0.1:8080", RunID: testRunID})
	if !preflight.Passed || preflight.Mode != "dry-run" || preflight.WriteEnabled || preflight.CleanupEnabled {
		t.Fatalf("preflight=%+v", preflight)
	}
}

func TestValidateE2EOptionsExecuteRequiresExplicitRunAndDatabaseAllowlist(t *testing.T) {
	preflight := validateE2EOptions(e2eOptions{BaseURL: "http://127.0.0.1:8080", RunID: testRunID, Execute: true})
	if preflight.Passed || len(preflight.Failures) < 6 {
		t.Fatalf("incomplete execute configuration unexpectedly passed: %+v", preflight)
	}
	for _, expected := range []string{"allowlist", "MySQL DSN", "primary account", "second account", "write confirmation", "cleanup confirmation"} {
		if !strings.Contains(strings.Join(preflight.Failures, "\n"), expected) {
			t.Errorf("missing preflight failure %q: %+v", expected, preflight.Failures)
		}
	}
}

func TestValidateE2EOptionsAcceptsFullyScopedDisposableRun(t *testing.T) {
	options := e2eOptions{
		BaseURL: "http://127.0.0.1:8080", RunID: testRunID, Execute: true,
		TargetName: "rentops-e2e-local", TargetAllowlist: "rentops-e2e-local",
		DatabaseAllowlist: "rentops_e2e_20260916_120000_a1b2c3d4",
		MySQLDSN:          "rentops:secret@tcp(127.0.0.1:3306)/rentops_e2e_20260916_120000_a1b2c3d4",
		Username:          testRunID + "-primary", Password: "primary-password",
		SecondUsername: testRunID + "-second", SecondPassword: "second-password",
		ConfirmWrites: e2eWriteConfirmation, ConfirmCleanup: e2eCleanupConfirmation,
	}
	if preflight := validateE2EOptions(options); !preflight.Passed || !preflight.WriteEnabled || !preflight.CleanupEnabled {
		t.Fatalf("preflight=%+v", preflight)
	}
}

func TestValidateE2EOptionsRejectsRemoteAndNonOriginTargets(t *testing.T) {
	remote := validateE2EOptions(e2eOptions{BaseURL: "https://staging.example.test", RunID: testRunID})
	if remote.Passed || !strings.Contains(strings.Join(remote.Failures, "\n"), "remote") {
		t.Fatalf("remote preflight=%+v", remote)
	}
	for _, baseURL := range []string{"http://127.0.0.1:8080/path", "http://127.0.0.1:8080/?query=1", "http://user:password@127.0.0.1:8080"} {
		preflight := validateE2EOptions(e2eOptions{BaseURL: baseURL, RunID: testRunID})
		if preflight.Passed || !strings.Contains(strings.Join(preflight.Failures, "\n"), "absolute origin") {
			t.Errorf("base URL %q unexpectedly passed: %+v", baseURL, preflight)
		}
	}
}

func TestE2EManifestUsesDublinCurrentAndFutureMonths(t *testing.T) {
	generatedAt := time.Date(2026, time.September, 30, 23, 30, 0, 0, time.UTC)
	manifest, err := newE2EFixtureManifest(testRunID, generatedAt)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Period != "2026-10" || manifest.FuturePeriod != "2026-11" {
		t.Fatalf("Dublin periods current=%q future=%q", manifest.Period, manifest.FuturePeriod)
	}
	if manifest.OverdueDueDay < 1 || manifest.OverdueDueDay > 31 {
		t.Fatalf("overdue due day=%d", manifest.OverdueDueDay)
	}
	if err := manifest.validate(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateE2ERunIDRequiresUniqueFormat(t *testing.T) {
	if err := validateE2ERunID(testRunID); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"", "test-run", "rentops-e2e-20260916-120000-zzzzzzzz", testRunID + "-extra"} {
		if err := validateE2ERunID(value); err == nil {
			t.Errorf("run ID %q unexpectedly valid", value)
		}
	}
}

func TestWriteE2EReportRedactsSecretsAndUsesPrivateFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.json")
	report := newE2EReport(e2ePreflight{Passed: true, Mode: "dry-run", RunID: testRunID}, time.Now())
	report.Scenarios = []e2eScenarioReport{{Name: "redaction", Status: "passed", Steps: []e2eStepReport{{
		Method: "POST", Path: "/login-local", Passed: true,
		Expected: map[string]any{"password": "do-not-write", "status": 302},
		Actual:   map[string]any{"cookie": "session-secret", "status": 302},
	}}}}
	if err := writeE2EReport(path, report); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("report permissions=%o", info.Mode().Perm())
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "do-not-write") || strings.Contains(string(body), "session-secret") || !strings.Contains(string(body), "REDACTED") {
		t.Fatalf("report leaked a secret: %s", body)
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
}

func TestE2EFormSummaryRedactsSensitiveValues(t *testing.T) {
	values := e2eFormSummary(map[string][]string{"username": {"tenant"}, "password": {"secret"}, "idempotency_key": {"run-key"}})
	if values["password"] != "[REDACTED]" || values["username"] != "tenant" || values["idempotency_key"] != "run-key" {
		t.Fatalf("form summary=%v", values)
	}
}
