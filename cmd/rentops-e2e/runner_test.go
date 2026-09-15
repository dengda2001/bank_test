package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func TestE2EHTTPClientMaintainsCookieSessionAndRejectsUnauthorizedRequests(t *testing.T) {
	var authenticatedRequest bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login-local":
			if r.Method != http.MethodPost {
				t.Error("login endpoint received a non-POST request")
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			if err := r.ParseForm(); err != nil {
				t.Errorf("parse login form: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if r.Form.Get("username") != "e2e-user" || r.Form.Get("password") != "e2e-password" {
				http.Redirect(w, r, "/?error=invalid_login", http.StatusFound)
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "rentops_session", Value: "session-secret", Path: "/", HttpOnly: true})
			http.Redirect(w, r, "/rent-dashboard", http.StatusFound)
		case "/billing":
			if _, err := r.Cookie("rentops_session"); err != nil {
				http.Redirect(w, r, "/", http.StatusFound)
				return
			}
			authenticatedRequest = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("protected"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := newE2EHTTPClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	scenario := client.authenticationScenario(context.Background(), "e2e-user", "e2e-password")
	if scenario.Status != "passed" || len(scenario.Steps) != 4 {
		t.Fatalf("scenario=%+v", scenario)
	}
	if !authenticatedRequest {
		t.Fatal("protected request did not receive the session cookie from login")
	}
	for _, step := range scenario.Steps {
		if strings.Contains(step.Error, "e2e-password") {
			t.Fatalf("password leaked into step error: %+v", step)
		}
	}
}

func TestE2EHTTPClientRejectsOriginEscape(t *testing.T) {
	client, err := newE2EHTTPClient("http://127.0.0.1:8080")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"https://evil.example.test/", "//evil.example.test/", "billing"} {
		if _, err := client.endpoint(path); err == nil {
			t.Fatalf("path %q unexpectedly escaped endpoint validation", path)
		}
	}
}

func TestNewE2EFixtureManifestUsesUniqueRunIDPrefixedIdentifiers(t *testing.T) {
	runID := "rentops-e2e-20260916-120000-a1b2c3d4"
	manifest, err := newE2EFixtureManifest(runID, time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if err := manifest.validate(); err != nil {
		t.Fatal(err)
	}
	if manifest.Tenant.Name == "" || !strings.HasPrefix(manifest.Tenant.Name, runID) || !strings.HasPrefix(manifest.Tenant.RoomAddress, runID) {
		t.Fatalf("tenant fixture is not isolated: %+v", manifest.Tenant)
	}
	if manifest.Expected.EURIncomeCents != 170000 || manifest.Expected.GBPIncomeCents != 2500 || manifest.Expected.PendingIncomeCount != 2 {
		t.Fatalf("unexpected expected snapshot: %+v", manifest.Expected)
	}
}

func TestNewE2EFixtureManifestRejectsInvalidRunID(t *testing.T) {
	if _, err := newE2EFixtureManifest("shared-fixture", time.Now()); err == nil {
		t.Fatal("invalid run ID unexpectedly generated a fixture")
	}
}

func TestE2EReportCarriesFixtureWithoutCredentialFields(t *testing.T) {
	runID := "rentops-e2e-20260916-120000-a1b2c3d4"
	manifest, err := newE2EFixtureManifest(runID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	report := newE2EReport(e2ePreflight{Passed: true, Mode: "dry-run", RunID: runID}, time.Now())
	report.Fixture = &manifest
	path := filepath.Join(t.TempDir(), "report.json")
	if err := writeE2EReport(path, report); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, runID+" tenant") || strings.Contains(text, "password") || strings.Contains(text, "dsn") || strings.Contains(text, "token") {
		t.Fatalf("fixture report contains unexpected content: %s", body)
	}
}
