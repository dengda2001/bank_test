package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidateE2EOptionsDryRunDoesNotAuthorizeWrites(t *testing.T) {
	preflight := validateE2EOptions(e2eOptions{
		BaseURL: "http://127.0.0.1:8080",
		RunID:   "rentops-e2e-20260916-120000-a1b2c3d4",
	})
	if !preflight.Passed || preflight.Mode != "dry-run" || preflight.WriteEnabled || preflight.CleanupEnabled {
		t.Fatalf("preflight=%+v", preflight)
	}
}

func TestValidateE2EOptionsExecuteRequiresAllowlistAndExplicitConfirmations(t *testing.T) {
	preflight := validateE2EOptions(e2eOptions{
		BaseURL: "http://127.0.0.1:8080",
		RunID:   "rentops-e2e-20260916-120000-a1b2c3d4",
		Execute: true,
	})
	if preflight.Passed || len(preflight.Failures) < 5 {
		t.Fatalf("preflight=%+v", preflight)
	}
	for _, expected := range []string{"allowlist", "MySQL DSN", "credentials", "write confirmation", "cleanup confirmation"} {
		found := false
		for _, failure := range preflight.Failures {
			if strings.Contains(failure, expected) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing failure %q in %+v", expected, preflight.Failures)
		}
	}
}

func TestValidateE2EOptionsRejectsRemoteTargetWithoutOptIn(t *testing.T) {
	preflight := validateE2EOptions(e2eOptions{
		BaseURL: "https://staging.example.test",
		RunID:   "rentops-e2e-20260916-120000-a1b2c3d4",
	})
	if preflight.Passed || !strings.Contains(strings.Join(preflight.Failures, "\n"), "remote") {
		t.Fatalf("preflight=%+v", preflight)
	}
}

func TestValidateE2EOptionsRejectsNonOriginBaseURL(t *testing.T) {
	for _, baseURL := range []string{
		"http://127.0.0.1:8080/?redirect=/danger",
		"http://127.0.0.1:8080/path",
		"http://user:password@127.0.0.1:8080",
	} {
		preflight := validateE2EOptions(e2eOptions{
			BaseURL: baseURL,
			RunID:   "rentops-e2e-20260916-120000-a1b2c3d4",
		})
		if preflight.Passed || !strings.Contains(strings.Join(preflight.Failures, "\n"), "absolute origin") {
			t.Fatalf("base URL %q unexpectedly passed: %+v", baseURL, preflight)
		}
	}
}

func TestWriteE2EReportRedactsSecretsAndUsesPrivateFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.json")
	report := newE2EReport(e2ePreflight{Passed: true, Mode: "dry-run", RunID: "rentops-e2e-20260916-120000-a1b2c3d4"}, time.Now())
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
		t.Fatalf("report leaked secret: %s", body)
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
}

func TestValidateE2ERunIDRequiresUniqueFormat(t *testing.T) {
	if err := validateE2ERunID("rentops-e2e-20260916-120000-a1b2c3d4"); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"", "test-run", "rentops-e2e-20260916-120000-zzzzzzzz", "rentops-e2e-20260916-120000-a1b2c3d4-extra"} {
		if err := validateE2ERunID(value); err == nil {
			t.Fatalf("run ID %q unexpectedly valid", value)
		}
	}
}
