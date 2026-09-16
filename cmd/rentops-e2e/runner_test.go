package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
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
	for _, expected := range []string{"allowlist", "MySQL DSN", "credentials", "second E2E account", "SMTP sink", "write confirmation", "cleanup confirmation"} {
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
	if !strings.Contains(strings.Join(preflight.Failures, "\n"), "database name allowlist") {
		t.Fatalf("missing database allowlist failure in %+v", preflight)
	}
}

func TestValidateE2EOptionsExcludesMailDeliveryOnlyOnExplicitOptIn(t *testing.T) {
	base := e2eOptions{
		BaseURL:           "http://127.0.0.1:8080",
		RunID:             "rentops-e2e-20260916-120000-a1b2c3d4",
		Execute:           true,
		TargetName:        "rentops-e2e-local",
		TargetAllowlist:   "rentops-e2e-local",
		DatabaseAllowlist: "rentops_e2e_run",
		MySQLDSN:          "rentops:secret@tcp(127.0.0.1:3306)/rentops_e2e_run",
		FixtureDir:        "/tmp/rentops-e2e-fixture",
		Username:          "rentops-e2e-20260916-120000-a1b2c3d4-primary",
		Password:          "primary-password",
		SecondUsername:    "rentops-e2e-second",
		SecondPassword:    "second-password",
		ConfirmWrites:     e2eWriteConfirmation,
		ConfirmCleanup:    e2eCleanupConfirmation,
	}
	withSink := base
	withSink.SMTPSink = "controlled-sink"
	if preflight := validateE2EOptions(withSink); !preflight.Passed || preflight.DunningDeliverySkipped {
		t.Fatalf("sink-configured preflight=%+v", preflight)
	}
	if preflight := validateE2EOptions(base); preflight.Passed {
		t.Fatalf("preflight without a sink or an opt-in passed: %+v", preflight)
	}
	skipped := base
	skipped.SkipDunning = true
	preflight := validateE2EOptions(skipped)
	if !preflight.Passed || !preflight.DunningDeliverySkipped {
		t.Fatalf("skip opt-in preflight=%+v", preflight)
	}
	for _, check := range preflight.Checks {
		if strings.Contains(check, "explicitly skipped") {
			return
		}
	}
	t.Fatalf("skip opt-in preflight did not record the excluded scope: %+v", preflight)
}

func TestRecordE2EScenarioKeepsSkippedScenariosUnverified(t *testing.T) {
	report := newE2EReport(e2ePreflight{Passed: true, Mode: "execute", RunID: "rentops-e2e-20260916-120000-a1b2c3d4"}, time.Now())
	report.Status = "running"
	if err := recordE2EScenario(&report, e2eSkippedDunningDeliveryScenario(e2eOptions{SkipDunning: true}), nil); err != nil {
		t.Fatalf("skipped scenario ended the run: %v", err)
	}
	if report.Status == "failed" || report.Error != "" {
		t.Fatalf("report=%+v", report)
	}
	if len(report.Scenarios) != 1 || report.Scenarios[0].Status != "skipped" {
		t.Fatalf("scenarios=%+v", report.Scenarios)
	}
	if len(report.Unverified) != 1 || report.Unverified[0].Name != "dunning-delivery" || report.Unverified[0].Reason == "" {
		t.Fatalf("unverified=%+v", report.Unverified)
	}
}

func TestWriteE2EReportPersistsUnverifiedScenarios(t *testing.T) {
	report := newE2EReport(e2ePreflight{Passed: true, Mode: "execute", RunID: "rentops-e2e-20260916-120000-a1b2c3d4"}, time.Now())
	if err := recordE2EScenario(&report, e2eSkippedDunningDeliveryScenario(e2eOptions{SkipDunning: true}), nil); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "e2e-report.json")
	if err := writeE2EReport(path, report); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Status     string `json:"status"`
		Unverified []struct {
			Name   string `json:"name"`
			Reason string `json:"reason"`
		} `json:"unverified"`
		Scenarios []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"scenarios"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if len(decoded.Unverified) != 1 || decoded.Unverified[0].Name != "dunning-delivery" || decoded.Unverified[0].Reason == "" {
		t.Fatalf("persisted unverified=%+v", decoded.Unverified)
	}
	if len(decoded.Scenarios) != 1 || decoded.Scenarios[0].Status != "skipped" {
		t.Fatalf("persisted scenarios=%+v", decoded.Scenarios)
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
	loginActual, ok := scenario.Steps[0].Actual.(map[string]any)
	if !ok {
		t.Fatalf("login actual summary type=%T", scenario.Steps[0].Actual)
	}
	loginRequest, ok := loginActual["request"].(map[string]any)
	if !ok {
		t.Fatalf("login request summary type=%T", loginActual["request"])
	}
	loginForm, ok := loginRequest["form"].(map[string]any)
	if !ok || loginForm["password"] != "[REDACTED]" || loginForm["username"] != "e2e-user" {
		t.Fatalf("unexpected redacted request summary=%+v", loginRequest)
	}
}

func TestVerifyE2EHTTPNoResidueChecksDeletedDataThroughSecondAccount(t *testing.T) {
	manifest, err := newE2EFixtureManifest("rentops-e2e-20260916-120000-a1b2c3d4", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login-local":
			if err := r.ParseForm(); err != nil || r.Form.Get("username") != "second" || r.Form.Get("password") != "second-password" {
				http.Redirect(w, r, "/?error=invalid_login", http.StatusFound)
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "rentops_session", Value: "second-session", Path: "/"})
			http.Redirect(w, r, "/rent-dashboard", http.StatusFound)
		case "/tenants/42", "/tenants/43":
			http.NotFound(w, r)
		case "/tenants", "/billing":
			if _, err := r.Cookie("rentops_session"); err != nil {
				http.Redirect(w, r, "/", http.StatusFound)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("clean second-account response"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	scenario := verifyE2EHTTPNoResidue(context.Background(), e2eOptions{
		BaseURL:        server.URL,
		SecondUsername: "second",
		SecondPassword: "second-password",
	}, manifest, e2EBusinessArtifacts{MainTenantID: 42, DunningTenantID: 43, TransactionID: 101})
	if scenario.Status != "passed" || len(scenario.Steps) != 5 {
		t.Fatalf("scenario=%+v", scenario)
	}
}

func TestVerifyE2EHTTPNoResidueIgnoresOperatorAccountName(t *testing.T) {
	runID := "rentops-e2e-20260916-120000-a1b2c3d4"
	manifest, err := newE2EFixtureManifest(runID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	// The tenant list renders the configured operator name, which carries the
	// run ID prefix. That chrome is not residue; fixture business data is.
	newServer := func(tenantListBody string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/login-local" {
				http.SetCookie(w, &http.Cookie{Name: "rentops_session", Value: "second", Path: "/"})
				http.Redirect(w, r, "/rent-dashboard", http.StatusFound)
				return
			}
			if _, cookieErr := r.Cookie("rentops_session"); cookieErr != nil {
				http.Redirect(w, r, "/", http.StatusFound)
				return
			}
			switch r.URL.Path {
			case "/tenants/42", "/tenants/43":
				http.NotFound(w, r)
			case "/tenants", "/billing":
				w.WriteHeader(http.StatusOK)
				fmt.Fprint(w, tenantListBody)
			default:
				http.NotFound(w, r)
			}
		}))
	}
	options := e2eOptions{
		SecondUsername: runID + "-second",
		SecondPassword: "second-password",
		Username:       runID + "-primary",
	}
	artifacts := e2EBusinessArtifacts{MainTenantID: 42, DunningTenantID: 43, TransactionID: 101}

	chromeOnly := newServer("当前用户：" + runID + "-primary")
	defer chromeOnly.Close()
	options.BaseURL = chromeOnly.URL
	if scenario := verifyE2EHTTPNoResidue(context.Background(), options, manifest, artifacts); scenario.Status != "passed" {
		t.Fatalf("operator chrome was treated as residue: %+v", scenario)
	}

	withResidue := newServer("当前用户：" + runID + "-primary" + manifest.Tenant.Name)
	defer withResidue.Close()
	options.BaseURL = withResidue.URL
	scenario := verifyE2EHTTPNoResidue(context.Background(), options, manifest, artifacts)
	if scenario.Status != "failed" || scenario.Error != "post-cleanup HTTP residue verification failed" {
		t.Fatalf("fixture residue was not detected: %+v", scenario)
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

func TestE2ETenantScenarioCreatesAndReadsTenantAndPayer(t *testing.T) {
	manifest, err := newE2EFixtureManifest("rentops-e2e-20260916-120000-a1b2c3d4", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var createdTenant bool
	var createdPayer bool
	var updatedTenant bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/login-local" {
			if _, err := r.Cookie("rentops_session"); err != nil {
				http.Redirect(w, r, "/", http.StatusFound)
				return
			}
		}
		switch r.URL.Path {
		case "/login-local":
			if err := r.ParseForm(); err != nil || r.Form.Get("password") != "e2e-password" {
				http.Redirect(w, r, "/?error=invalid_login", http.StatusFound)
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "rentops_session", Value: "session-secret", Path: "/"})
			http.Redirect(w, r, "/rent-dashboard", http.StatusFound)
		case "/billing":
			w.WriteHeader(http.StatusOK)
		case "/tenants":
			if r.Method == http.MethodPost {
				if err := r.ParseForm(); err != nil || r.Form.Get("name") != manifest.Tenant.Name || r.Form.Get("monthly_rent") != "950.00" {
					t.Errorf("unexpected tenant form: %v", r.Form)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				if r.Form.Get("tenant_id") == "42" {
					if r.Form.Get("display_alias") != manifest.Tenant.DisplayAlias+" updated" || r.Form.Get("room_address") != manifest.Tenant.RoomAddress+" updated" {
						t.Errorf("unexpected tenant update form: %v", r.Form)
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					updatedTenant = true
					http.Redirect(w, r, "/tenants?message=tenant_updated", http.StatusFound)
					return
				}
				createdTenant = true
				http.Redirect(w, r, "/tenants?message=tenant_added", http.StatusFound)
				return
			}
			fmt.Fprintf(w, `<table><tr><td>%s</td><td><a href="/tenants/42">详情</a></td></tr></table>`, manifest.Tenant.Name)
		case "/tenants/42/payers":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			if err := r.ParseForm(); err != nil || r.Form.Get("payer_name") != manifest.Payer.Name || r.Form.Get("payer_id") != manifest.Payer.PayerID {
				t.Errorf("unexpected payer form: %v", r.Form)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			createdPayer = true
			http.Redirect(w, r, "/tenants/42?message=payer_added", http.StatusFound)
		case "/tenants/42":
			if !createdTenant {
				t.Fatal("tenant detail read happened before tenant creation")
			}
			if updatedTenant {
				fmt.Fprintf(w, `<main>%s %s %s %s %s</main>`, manifest.Tenant.Name, manifest.Tenant.DisplayAlias+" updated", manifest.Tenant.RoomAddress+" updated", manifest.Payer.Name, manifest.Payer.PayerID)
				return
			}
			fmt.Fprintf(w, `<main>%s %s %s</main>`, manifest.Tenant.Name, manifest.Payer.Name, manifest.Payer.PayerID)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := newE2EHTTPClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	authScenario := client.authenticationScenario(context.Background(), "e2e-user", "e2e-password")
	if authScenario.Status != "passed" {
		t.Fatalf("authentication scenario=%+v", authScenario)
	}
	tenantScenario, tenantID := client.tenantScenario(context.Background(), manifest)
	if tenantScenario.Status != "passed" || tenantID != 42 || !createdPayer || !updatedTenant {
		t.Fatalf("tenant scenario=%+v tenantID=%d payer=%v updated=%v", tenantScenario, tenantID, createdPayer, updatedTenant)
	}
}

func TestE2EDunningTenantScenarioCreatesAnUnpaidTenant(t *testing.T) {
	manifest, err := newE2EFixtureManifest("rentops-e2e-20260916-120000-a1b2c3d4", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var created bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/login-local" {
			if _, cookieErr := r.Cookie("rentops_session"); cookieErr != nil {
				http.Redirect(w, r, "/", http.StatusFound)
				return
			}
		}
		switch r.URL.Path {
		case "/login-local":
			if err := r.ParseForm(); err != nil || r.Form.Get("password") != "e2e-password" {
				http.Redirect(w, r, "/?error=invalid_login", http.StatusFound)
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "rentops_session", Value: "session-secret", Path: "/"})
			http.Redirect(w, r, "/rent-dashboard", http.StatusFound)
		case "/billing":
			w.WriteHeader(http.StatusOK)
		case "/tenants":
			if r.Method == http.MethodPost {
				_ = r.ParseForm()
				if r.Form.Get("name") != manifest.DunningTenant.Name || r.Form.Get("monthly_rent") != "500.00" {
					t.Errorf("unexpected dunning tenant form: %v", r.Form)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				created = true
				http.Redirect(w, r, "/tenants?message=tenant_added", http.StatusFound)
				return
			}
			fmt.Fprintf(w, `<table><tr><td>%s</td><td><a href="/tenants/43">详情</a></td></tr></table>`, manifest.DunningTenant.Name)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := newE2EHTTPClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	authScenario := client.authenticationScenario(context.Background(), "e2e-user", "e2e-password")
	if authScenario.Status != "passed" {
		t.Fatalf("authentication scenario=%+v", authScenario)
	}
	scenario, tenantID := client.dunningTenantScenario(context.Background(), manifest)
	if scenario.Status != "passed" || tenantID != 43 || !created {
		t.Fatalf("scenario=%+v tenantID=%d created=%v", scenario, tenantID, created)
	}
}

func TestE2EBankImportScenarioVerifiesFactsAndIdempotency(t *testing.T) {
	manifest, err := newE2EFixtureManifest("rentops-e2e-20260916-120000-a1b2c3d4", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	importCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/login-local" {
			if _, err := r.Cookie("rentops_session"); err != nil {
				http.Redirect(w, r, "/", http.StatusFound)
				return
			}
		}
		switch r.URL.Path {
		case "/login-local":
			if err := r.ParseForm(); err != nil || r.Form.Get("password") != "e2e-password" {
				http.Redirect(w, r, "/?error=invalid_login", http.StatusFound)
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "rentops_session", Value: "session-secret", Path: "/"})
			http.Redirect(w, r, "/rent-dashboard", http.StatusFound)
		case "/billing":
			w.WriteHeader(http.StatusOK)
			if importCount > 0 {
				for _, transaction := range manifest.Bank.Transactions {
					fmt.Fprintf(w, `<tr class="income"><td>%s</td><td>%s</td><td>%s</td></tr>`, transaction.StoredProviderTransactionID(), transaction.Description, formatE2EMoney(transaction.Amount))
				}
				fmt.Fprint(w, `<span>EUR 950.00</span><span>GBP 25.00</span>`)
			}
		case "/import-legacy":
			importCount++
			http.Redirect(w, r, "/billing?message=legacy_imported", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := newE2EHTTPClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	authScenario := client.authenticationScenario(context.Background(), "e2e-user", "e2e-password")
	if authScenario.Status != "passed" {
		t.Fatalf("authentication scenario=%+v", authScenario)
	}
	scenario := client.bankImportScenario(context.Background(), manifest)
	if scenario.Status != "passed" || importCount != 2 {
		t.Fatalf("scenario=%+v imports=%d", scenario, importCount)
	}
}

func TestExtractE2ETransactionIDAndStatusFromBillingRow(t *testing.T) {
	body := []byte(`<table><tr class="income"><td>stable-tx-1</td><td><span class="status status-link matched">已关联</span><form><input type="hidden" name="transaction_id" value="42"></form></td></tr></table>`)
	id, err := extractE2ETransactionID(body, "stable-tx-1")
	if err != nil || id != 42 {
		t.Fatalf("id=%d err=%v", id, err)
	}
	row, err := e2eTransactionRow(body, "stable-tx-1")
	if err != nil || e2eTransactionStatus(row) != "matched" {
		t.Fatalf("row=%q err=%v status=%q", row, err, e2eTransactionStatus(row))
	}
	if _, err := extractE2ETransactionID(body, "missing"); err == nil {
		t.Fatal("missing transaction unexpectedly extracted")
	}
}

func TestE2ELedgerScenarioRunsHTTPActionsAndChecksConservation(t *testing.T) {
	manifest, err := newE2EFixtureManifest("rentops-e2e-20260916-120000-a1b2c3d4", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	transactionIDs := map[string]uint64{
		manifest.Bank.Transactions[0].StoredProviderTransactionID(): 101,
		manifest.Bank.Transactions[1].StoredProviderTransactionID(): 102,
		manifest.Bank.Transactions[2].StoredProviderTransactionID(): 103,
		manifest.Bank.Transactions[3].StoredProviderTransactionID(): 104,
		manifest.Bank.Transactions[4].StoredProviderTransactionID(): 105,
	}
	statuses := map[uint64]string{101: "unmatched", 102: "unmatched", 103: "unmatched", 104: "unmatched", 105: "unmatched"}
	allocated := map[uint64]string{101: "EUR 0.00", 102: "EUR 0.00", 103: "GBP 0.00", 104: "EUR 0.00", 105: "EUR 0.00"}
	amounts := map[uint64]string{101: "EUR 950.00", 102: "EUR 400.00", 103: "GBP 25.00", 104: "EUR 300.00", 105: "EUR 50.00"}
	providerForID := make(map[uint64]string, len(transactionIDs))
	for providerID, transactionID := range transactionIDs {
		providerForID[transactionID] = providerID
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/login-local" {
			if _, cookieErr := r.Cookie("rentops_session"); cookieErr != nil {
				http.Redirect(w, r, "/", http.StatusFound)
				return
			}
		}
		switch r.URL.Path {
		case "/login-local":
			if err := r.ParseForm(); err != nil || r.Form.Get("password") != "e2e-password" {
				http.Redirect(w, r, "/?error=invalid_login", http.StatusFound)
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "rentops_session", Value: "session-secret", Path: "/"})
			http.Redirect(w, r, "/rent-dashboard", http.StatusFound)
		case "/billing":
			w.WriteHeader(http.StatusOK)
			for _, transaction := range manifest.Bank.Transactions {
				id := transactionIDs[transaction.StoredProviderTransactionID()]
				remaining := amounts[id]
				if allocated[id] == "EUR 950.00" {
					remaining = "EUR 0.00"
				} else if allocated[id] == "EUR 400.00" || allocated[id] == "EUR 300.00" || allocated[id] == "EUR 50.00" {
					remaining = "EUR 0.00"
				}
				fmt.Fprintf(w, `<tr class="%s"><td>%s</td><td>%s</td><td><span class="status status-link %s">状态</span>已分配 %s 余款 %s</td><td><form><input type="hidden" name="transaction_id" value="%d"></form></td></tr>`, statuses[id], transaction.StoredProviderTransactionID(), transaction.Description, statuses[id], allocated[id], remaining, id)
			}
		case "/billing/confirm":
			_ = r.ParseForm()
			id, _ := strconv.ParseUint(r.Form.Get("transaction_id"), 10, 64)
			if id != 101 || r.Form.Get("tenant_id") != "42" || r.Form.Get("period") != "2026-09" {
				http.Redirect(w, r, "/billing?error=confirmation_failed", http.StatusFound)
				return
			}
			statuses[id] = "matched"
			allocated[id] = "EUR 950.00"
			http.Redirect(w, r, "/billing?message=rent_confirmed", http.StatusFound)
		case "/billing/allocate":
			_ = r.ParseForm()
			id, _ := strconv.ParseUint(r.Form.Get("transaction_id"), 10, 64)
			if id == 103 {
				http.Redirect(w, r, "/billing?error=allocation_failed", http.StatusFound)
				return
			}
			switch id {
			case 102:
				allocated[id] = "EUR 400.00"
			case 104:
				allocated[id] = "EUR 300.00"
			case 105:
				allocated[id] = "EUR 50.00"
			default:
				http.Redirect(w, r, "/billing?error=allocation_failed", http.StatusFound)
				return
			}
			statuses[id] = "matched"
			http.Redirect(w, r, "/billing?message=allocation_saved", http.StatusFound)
		case "/billing/ignore":
			_ = r.ParseForm()
			id, _ := strconv.ParseUint(r.Form.Get("transaction_id"), 10, 64)
			if id != 103 {
				http.Redirect(w, r, "/billing?error=transaction_action_failed", http.StatusFound)
				return
			}
			statuses[id] = "ignored"
			http.Redirect(w, r, "/billing?message=transaction_action_saved", http.StatusFound)
		case "/billing/revoke":
			if r.Method == http.MethodGet {
				_ = r.ParseForm()
				id, _ := strconv.ParseUint(r.URL.Query().Get("transaction_id"), 10, 64)
				if id != 101 {
					http.NotFound(w, r)
					return
				}
				fmt.Fprintf(w, `<main>%s %d</main>`, manifest.Bank.Transactions[0].Description, id)
				return
			}
			_ = r.ParseForm()
			id, _ := strconv.ParseUint(r.Form.Get("transaction_id"), 10, 64)
			if id != 101 {
				http.Redirect(w, r, "/billing?error=transaction_action_failed", http.StatusFound)
				return
			}
			statuses[id] = "unmatched"
			allocated[id] = "EUR 0.00"
			http.Redirect(w, r, "/billing?message=transaction_action_saved", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := newE2EHTTPClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	authScenario := client.authenticationScenario(context.Background(), "e2e-user", "e2e-password")
	if authScenario.Status != "passed" {
		t.Fatalf("authentication scenario=%+v", authScenario)
	}
	scenario := client.ledgerScenario(context.Background(), manifest, 42)
	if scenario.Status != "passed" || len(providerForID) != 5 {
		t.Fatalf("ledger scenario=%+v providerIDs=%v", scenario, providerForID)
	}
}

func TestExtractE2ECashReceiptID(t *testing.T) {
	id, err := extractE2ECashReceiptID([]byte(`<a href="/cash-receipts/void?receipt_id=501">作废</a>`))
	if err != nil || id != 501 {
		t.Fatalf("id=%d err=%v", id, err)
	}
	if _, err := extractE2ECashReceiptID([]byte(`<main>no receipt</main>`)); err == nil {
		t.Fatal("missing receipt link unexpectedly parsed")
	}
}

func TestExtractE2EDunningObligationID(t *testing.T) {
	id, err := extractE2EDunningObligationID([]byte(`<input type="checkbox" name="obligation_id" value="900">`))
	if err != nil || id != 900 {
		t.Fatalf("id=%d err=%v", id, err)
	}
	if _, err := extractE2EDunningObligationID([]byte(`<main>no candidate</main>`)); err == nil {
		t.Fatal("missing obligation unexpectedly parsed")
	}
}

func TestE2EDashboardAndDunningScenariosVerifyReadsAndDelivery(t *testing.T) {
	manifest, err := newE2EFixtureManifest("rentops-e2e-20260916-120000-a1b2c3d4", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	sendCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/login-local" {
			if _, cookieErr := r.Cookie("rentops_session"); cookieErr != nil {
				http.Redirect(w, r, "/", http.StatusFound)
				return
			}
		}
		switch r.URL.Path {
		case "/login-local":
			if err := r.ParseForm(); err != nil || r.Form.Get("password") != "e2e-password" {
				http.Redirect(w, r, "/?error=invalid_login", http.StatusFound)
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "rentops_session", Value: "session-secret", Path: "/"})
			http.Redirect(w, r, "/rent-dashboard", http.StatusFound)
		case "/billing":
			w.WriteHeader(http.StatusOK)
		case "/rent-dashboard":
			w.WriteHeader(http.StatusOK)
			if r.URL.Query().Get("status") == "not-a-status" {
				fmt.Fprint(w, `<main>筛选条件无效 invalid_dashboard_filter</main>`)
				return
			}
			fmt.Fprintf(w, `<main><div class="label">本月应收</div><strong>EUR 950.00</strong><div class="label">已收租金</div><strong>EUR 950.00</strong><div class="label">剩余未收</div><strong>EUR 0.00</strong>%s %s 已缴清 当前显示 1 户 <input type="checkbox" name="obligation_id" value="900"></main>`, manifest.Tenant.Name, manifest.DunningTenant.Name)
		case "/tenants/42":
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `<main>%s 2026-08 2026-09</main>`, manifest.Tenant.Name)
		case "/dunning/config":
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `<main>发件配置已保存 %s landlord</main>`, manifest.RunID)
		case "/dunning/preview":
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `<main>%s 邮件催缴预览</main>`, manifest.DunningTenant.Name)
		case "/dunning/send":
			sendCount++
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `<main>%s 发送结果 已发送</main>`, manifest.DunningTenant.Name)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := newE2EHTTPClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	authScenario := client.authenticationScenario(context.Background(), "e2e-user", "e2e-password")
	if authScenario.Status != "passed" {
		t.Fatalf("authentication scenario=%+v", authScenario)
	}
	dashboardScenario := client.dashboardScenario(context.Background(), manifest, 42)
	if dashboardScenario.Status != "passed" {
		t.Fatalf("dashboard scenario=%+v", dashboardScenario)
	}
	candidateScenario, obligationID := client.dunningCandidateScenario(context.Background(), manifest)
	if candidateScenario.Status != "passed" || obligationID != 900 {
		t.Fatalf("candidate scenario=%+v obligationID=%d", candidateScenario, obligationID)
	}
	dunningConfigScenario := client.dunningConfigurationScenario(context.Background(), manifest, 900)
	if dunningConfigScenario.Status != "passed" {
		t.Fatalf("dunning configuration scenario=%+v", dunningConfigScenario)
	}
	dunningScenario := client.dunningDeliveryScenario(context.Background(), manifest, 900)
	if dunningScenario.Status != "passed" || sendCount != 2 {
		t.Fatalf("dunning scenario=%+v sendCount=%d", dunningScenario, sendCount)
	}
}

func TestE2ECrossUserScenarioRejectsTenantLedgerAndCashAccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		isSecondUser := false
		if cookie, cookieErr := r.Cookie("rentops_session"); cookieErr == nil && cookie.Value == "second" {
			isSecondUser = true
		}
		if r.URL.Path == "/login-local" {
			_ = r.ParseForm()
			if r.Form.Get("username") == "second" && r.Form.Get("password") == "second-password" {
				http.SetCookie(w, &http.Cookie{Name: "rentops_session", Value: "second", Path: "/"})
				http.Redirect(w, r, "/rent-dashboard", http.StatusFound)
				return
			}
			if r.Form.Get("password") == "first-password" {
				http.SetCookie(w, &http.Cookie{Name: "rentops_session", Value: "first", Path: "/"})
				http.Redirect(w, r, "/rent-dashboard", http.StatusFound)
				return
			}
			http.Redirect(w, r, "/?error=invalid_login", http.StatusFound)
			return
		}
		if r.URL.Path == "/billing" {
			if !isSecondUser {
				http.Redirect(w, r, "/", http.StatusFound)
				return
			}
			w.WriteHeader(http.StatusOK)
			return
		}
		if !isSecondUser {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		switch r.URL.Path {
		case "/tenants/42":
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, "404 page not found\n")
		case "/billing/confirm":
			http.Redirect(w, r, "/billing?error=confirmation_failed", http.StatusFound)
		case "/billing/revoke":
			http.Redirect(w, r, "/billing?error=transaction_action_failed", http.StatusFound)
		case "/cash-receipts/preview":
			http.Redirect(w, r, "/cash-receipts/new?error=cash_receipt_failed", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := newE2EHTTPClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	authScenario := client.authenticationScenario(context.Background(), "second", "second-password")
	if authScenario.Status != "passed" {
		t.Fatalf("second account authentication scenario=%+v", authScenario)
	}
	manifest, err := newE2EFixtureManifest("rentops-e2e-20260916-120000-a1b2c3d4", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	scenario := client.crossUserScenario(context.Background(), manifest, 42, 101)
	if scenario.Status != "passed" {
		t.Fatalf("cross-user scenario=%+v", scenario)
	}
}

func TestE2ECrossUserScenarioFailsWhenRejectionLeaksForeignData(t *testing.T) {
	manifest, err := newE2EFixtureManifest("rentops-e2e-20260916-120000-a1b2c3d4", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/login-local" {
			if _, cookieErr := r.Cookie("rentops_session"); cookieErr != nil {
				http.Redirect(w, r, "/", http.StatusFound)
				return
			}
		}
		switch r.URL.Path {
		case "/login-local":
			if err := r.ParseForm(); err != nil || r.Form.Get("password") != "second-password" {
				http.Redirect(w, r, "/?error=invalid_login", http.StatusFound)
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "rentops_session", Value: "second", Path: "/"})
			http.Redirect(w, r, "/rent-dashboard", http.StatusFound)
		case "/billing":
			w.WriteHeader(http.StatusOK)
		case "/tenants/42":
			// A 404 that still echoes the other account's tenant name is a leak.
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintf(w, "404 page not found: %s\n", manifest.Tenant.Name)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := newE2EHTTPClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if auth := client.authenticationScenario(context.Background(), "second", "second-password"); auth.Status != "passed" {
		t.Fatalf("authentication scenario=%+v", auth)
	}
	scenario := client.crossUserScenario(context.Background(), manifest, 42, 101)
	if scenario.Status != "failed" || scenario.Error != "cross-user tenant isolation failed" {
		t.Fatalf("leaking rejection was not rejected: %+v", scenario)
	}
	actual, ok := scenario.Steps[0].Actual.(map[string]any)
	if !ok {
		t.Fatalf("unexpected step actual: %#v", scenario.Steps[0].Actual)
	}
	if actual["fixture_data_absent"] != false || actual["data_hidden"] != false {
		t.Fatalf("leak was not reported: %#v", actual)
	}
}

func TestRunE2EBusinessScenariosStopsAfterFirstFailureAndPersistsReport(t *testing.T) {
	manifest, err := newE2EFixtureManifest("rentops-e2e-20260916-120000-a1b2c3d4", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var tenantPosts, dunningPosts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/login-local" {
			if _, cookieErr := r.Cookie("rentops_session"); cookieErr != nil {
				http.Redirect(w, r, "/", http.StatusFound)
				return
			}
		}
		switch r.URL.Path {
		case "/login-local":
			_ = r.ParseForm()
			if r.Form.Get("password") != "e2e-password" {
				http.Redirect(w, r, "/?error=invalid_login", http.StatusFound)
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "rentops_session", Value: "session", Path: "/"})
			http.Redirect(w, r, "/rent-dashboard", http.StatusFound)
		case "/billing":
			w.WriteHeader(http.StatusOK)
		case "/tenants":
			if r.Method == http.MethodPost {
				tenantPosts++
			}
			http.Error(w, "tenant failure", http.StatusInternalServerError)
		case "/dunning/config", "/dunning/preview", "/dunning/send":
			dunningPosts++
			http.Error(w, "must not run after failure", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	report := newE2EReport(e2ePreflight{Passed: true, Mode: "execute", RunID: manifest.RunID}, time.Now())
	options := e2eOptions{BaseURL: server.URL, Username: "e2e-user", Password: "e2e-password"}
	writes := make([]e2eReport, 0, 3)
	err = runE2EBusinessScenarios(context.Background(), options, manifest, &report, func(snapshot e2eReport) error {
		writes = append(writes, snapshot)
		return nil
	}, nil)
	if err == nil || report.Status != "failed" {
		t.Fatalf("err=%v report=%+v", err, report)
	}
	if tenantPosts != 1 || dunningPosts != 0 {
		t.Fatalf("tenantPosts=%d dunningPosts=%d", tenantPosts, dunningPosts)
	}
	if len(report.Scenarios) != 2 || report.Scenarios[0].Name != "authentication" || report.Scenarios[1].Name != "tenant-and-payer" || report.Scenarios[1].Status != "failed" {
		t.Fatalf("scenarios=%+v", report.Scenarios)
	}
	if len(writes) != 3 || writes[len(writes)-1].Status != "failed" {
		t.Fatalf("progress writes=%+v", writes)
	}
}

func TestE2EDiscoverCrossUserTransactionRequiresPositiveFixtureID(t *testing.T) {
	manifest, err := newE2EFixtureManifest("rentops-e2e-20260916-120000-a1b2c3d4", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/billing" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintf(w, `<tr class="income"><form><input name="transaction_id" value="101"><span>%s</span></form></tr>`, manifest.Bank.Transactions[0].StoredProviderTransactionID())
	}))
	defer server.Close()
	client, err := newE2EHTTPClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	scenario, transactionID := client.discoverE2ECrossUserTransaction(context.Background(), manifest)
	if scenario.Status != "passed" || transactionID != 101 {
		t.Fatalf("scenario=%+v transactionID=%d", scenario, transactionID)
	}
}

func TestE2ECashReceiptScenarioRunsPreviewIdempotencyVoidAndCorrection(t *testing.T) {
	manifest, err := newE2EFixtureManifest("rentops-e2e-20260916-120000-a1b2c3d4", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	cashPaid := int64(0)
	receiptID := uint64(0)
	voided := false
	firstReceiptVoided := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/login-local" {
			if _, cookieErr := r.Cookie("rentops_session"); cookieErr != nil {
				http.Redirect(w, r, "/", http.StatusFound)
				return
			}
		}
		switch r.URL.Path {
		case "/login-local":
			if err := r.ParseForm(); err != nil || r.Form.Get("password") != "e2e-password" {
				http.Redirect(w, r, "/?error=invalid_login", http.StatusFound)
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "rentops_session", Value: "session-secret", Path: "/"})
			http.Redirect(w, r, "/rent-dashboard", http.StatusFound)
		case "/billing":
			w.WriteHeader(http.StatusOK)
		case "/cash-receipts/new":
			fmt.Fprint(w, `<form action="/cash-receipts/preview"><input value="EUR"><span>tenant 42</span></form>`)
		case "/cash-receipts/preview":
			_ = r.ParseForm()
			if r.Form.Get("currency") != "EUR" {
				http.Redirect(w, r, "/cash-receipts/new?error=cash_receipt_failed&period=2026-08&tenant_id=42", http.StatusFound)
				return
			}
			if r.Form.Get("amount") == "1.00" && cashPaid == 350 {
				http.Redirect(w, r, "/cash-receipts/new?error=cash_overbalance&period=2026-08&tenant_id=42", http.StatusFound)
				return
			}
			if r.Form.Get("amount") == "300.00" {
				fmt.Fprint(w, `<main>确认现金入账 EUR 600.00 EUR 50.00 <form action="/cash-receipts"></form></main>`)
				return
			}
			fmt.Fprint(w, `<main>确认现金入账 EUR 600.00 EUR 350.00 EUR 0.00 <form action="/cash-receipts"></form></main>`)
		case "/cash-receipts":
			_ = r.ParseForm()
			if r.Form.Get("amount") == "300.00" {
				cashPaid = 300
				receiptID = 501
				voided = false
			} else if r.Form.Get("amount") == "350.00" {
				cashPaid = 350
				receiptID = 502
				voided = false
			} else {
				t.Errorf("unexpected cash amount: %v", r.Form)
			}
			http.Redirect(w, r, "/tenants/42?message=cash_receipt_saved&from_month=2026-08&to_month=2026-08", http.StatusFound)
		case "/tenants/42":
			w.WriteHeader(http.StatusOK)
			switch {
			case cashPaid == 300 && !voided:
				fmt.Fprint(w, `<main>EUR 300.00 EUR 50.00 <a href="/cash-receipts/void?receipt_id=501">作废</a></main>`)
			case cashPaid == 350 && !voided:
				fmt.Fprint(w, `<main>EUR 350.00 EUR 0.00 <a href="/cash-receipts/void?receipt_id=502">作废</a></main>`)
			default:
				fmt.Fprint(w, `<main>EUR 600.00 EUR 350.00</main>`)
			}
		case "/cash-receipts/void":
			if r.Method == http.MethodGet {
				fmt.Fprintf(w, `<main>EUR 300.00 %d</main>`, receiptID)
				return
			}
			_ = r.ParseForm()
			if r.Form.Get("receipt_id") != "501" {
				http.Redirect(w, r, "/cash-receipts/void?error=cash_void_failed", http.StatusFound)
				return
			}
			cashPaid = 0
			voided = true
			firstReceiptVoided = true
			http.Redirect(w, r, "/tenants/42?message=cash_receipt_voided&from_month=2026-08&to_month=2026-08", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := newE2EHTTPClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	authScenario := client.authenticationScenario(context.Background(), "e2e-user", "e2e-password")
	if authScenario.Status != "passed" {
		t.Fatalf("authentication scenario=%+v", authScenario)
	}
	scenario := client.cashReceiptScenario(context.Background(), manifest, 42)
	if scenario.Status != "passed" || receiptID != 502 || !firstReceiptVoided || cashPaid != 350 {
		t.Fatalf("cash scenario=%+v receiptID=%d firstVoided=%v cashPaid=%d", scenario, receiptID, firstReceiptVoided, cashPaid)
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
	if manifest.Expected.PayerCount != 2 || manifest.Expected.EURIncomeCents != 170000 || manifest.Expected.GBPIncomeCents != 2500 || manifest.Expected.PendingIncomeCount != 3 {
		t.Fatalf("unexpected expected snapshot: %+v", manifest.Expected)
	}
}

func TestValidateE2ECleanupOptionsRequiresRunScopedTargetAndConfirmation(t *testing.T) {
	base := e2eOptions{
		Execute:           true,
		RunID:             "rentops-e2e-20260916-120000-a1b2c3d4",
		TargetName:        "local-e2e",
		TargetAllowlist:   "local-e2e",
		DatabaseAllowlist: "rentops_e2e",
		Username:          "rentops-e2e-20260916-120000-a1b2c3d4-owner",
		MySQLDSN:          "root@tcp(127.0.0.1:3306)/rentops_e2e",
		ConfirmCleanup:    e2eCleanupConfirmation,
	}
	if err := validateE2ECleanupOptions(base); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*e2eOptions){
		"target":       func(options *e2eOptions) { options.TargetName = "shared" },
		"database":     func(options *e2eOptions) { options.DatabaseAllowlist = "" },
		"confirmation": func(options *e2eOptions) { options.ConfirmCleanup = "yes" },
		"username":     func(options *e2eOptions) { options.Username = "owner" },
	} {
		t.Run(name, func(t *testing.T) {
			options := base
			mutate(&options)
			if err := validateE2ECleanupOptions(options); err == nil {
				t.Fatal("unsafe cleanup options unexpectedly passed")
			}
		})
	}
}

func TestNewE2EFixtureManifestRejectsInvalidRunID(t *testing.T) {
	if _, err := newE2EFixtureManifest("shared-fixture", time.Now()); err == nil {
		t.Fatal("invalid run ID unexpectedly generated a fixture")
	}
}

func TestE2EFixtureManifestRejectsUnscopedOrMismatchedFacts(t *testing.T) {
	runID := "rentops-e2e-20260916-120000-a1b2c3d4"
	manifest, err := newE2EFixtureManifest(runID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	manifest.Bank.AccountID = "shared-account"
	if err := manifest.validate(); err == nil || !strings.Contains(err.Error(), "run ID prefix") {
		t.Fatalf("unscoped account was not rejected: %v", err)
	}
	manifest, err = newE2EFixtureManifest(runID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	manifest.Bank.Transactions[0].Amount.Cents++
	if err := manifest.validate(); err == nil || !strings.Contains(err.Error(), "totals") {
		t.Fatalf("mismatched amount was not rejected: %v", err)
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

func TestMaterializeE2ELegacyFixturesCreatesExclusivePrivateInputs(t *testing.T) {
	manifest, err := newE2EFixtureManifest("rentops-e2e-20260916-120000-a1b2c3d4", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	paths, err := materializeE2ELegacyFixtures(manifest, filepath.Join(t.TempDir(), "fixtures"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{paths.BankLogFile, paths.TenantFile, paths.ExpenseFile} {
		info, statErr := os.Stat(path)
		if statErr != nil {
			t.Fatal(statErr)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("fixture %s permissions=%o", path, info.Mode().Perm())
		}
	}
	body, err := os.ReadFile(paths.BankLogFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), manifest.RunID+"-tx-eur-full") || strings.Contains(string(body), "password") {
		t.Fatalf("unexpected bank fixture content: %s", body)
	}
	if _, err := materializeE2ELegacyFixtures(manifest, filepath.Dir(paths.BankLogFile)); err == nil || !strings.Contains(err.Error(), "must be empty") {
		t.Fatal("non-empty fixture directory was not rejected")
	}
}

func TestRemoveE2ELegacyFixturesOnlyRemovesTheMaterializedSet(t *testing.T) {
	manifest, err := newE2EFixtureManifest("rentops-e2e-20260916-120000-a1b2c3d4", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(t.TempDir(), "fixtures")
	paths, err := materializeE2ELegacyFixtures(manifest, directory)
	if err != nil {
		t.Fatal(err)
	}
	if err := removeE2ELegacyFixtures(paths); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		t.Fatalf("fixture directory still exists or returned unexpected error: %v", err)
	}
}

func TestExecuteE2ERunProbesTargetBeforeMaterializingFixtures(t *testing.T) {
	manifest, err := newE2EFixtureManifest("rentops-e2e-20260916-120000-a1b2c3d4", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(t.TempDir(), "fixtures")
	report := newE2EReport(e2ePreflight{Passed: true, Mode: "execute", RunID: manifest.RunID}, time.Now())
	options := e2eOptions{
		BaseURL:           "http://127.0.0.1:8080",
		RunID:             manifest.RunID,
		FixtureDir:        directory,
		DatabaseAllowlist: "rentops_e2e",
		MySQLDSN:          "root@tcp(127.0.0.1:3306)/another_database",
		Execute:           true,
	}
	if err := executeE2ERun(context.Background(), options, manifest, &report, nil); err == nil {
		t.Fatal("unsafe target unexpectedly passed the probe")
	}
	if report.Status != "failed" || len(report.Scenarios) != 1 || report.Scenarios[0].Name != "target-probe" {
		t.Fatalf("report=%+v", report)
	}
	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		t.Fatalf("fixture directory was created before target probe completed: %v", err)
	}
}
