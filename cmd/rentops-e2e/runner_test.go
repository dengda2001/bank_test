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
					fmt.Fprintf(w, `<tr class="income"><td>%s</td><td>%s</td><td>%s</td></tr>`, transaction.ProviderTransactionID, transaction.Description, formatE2EMoney(transaction.Amount))
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
		manifest.Bank.Transactions[0].ProviderTransactionID: 101,
		manifest.Bank.Transactions[1].ProviderTransactionID: 102,
		manifest.Bank.Transactions[2].ProviderTransactionID: 103,
		manifest.Bank.Transactions[3].ProviderTransactionID: 104,
		manifest.Bank.Transactions[4].ProviderTransactionID: 105,
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
				id := transactionIDs[transaction.ProviderTransactionID]
				remaining := amounts[id]
				if allocated[id] == "EUR 950.00" {
					remaining = "EUR 0.00"
				} else if allocated[id] == "EUR 400.00" || allocated[id] == "EUR 300.00" || allocated[id] == "EUR 50.00" {
					remaining = "EUR 0.00"
				}
				fmt.Fprintf(w, `<tr class="%s"><td>%s</td><td>%s</td><td><span class="status status-link %s">状态</span>已分配 %s 余款 %s</td><td><form><input type="hidden" name="transaction_id" value="%d"></form></td></tr>`, statuses[id], transaction.ProviderTransactionID, transaction.Description, statuses[id], allocated[id], remaining, id)
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
	if manifest.Expected.EURIncomeCents != 170000 || manifest.Expected.GBPIncomeCents != 2500 || manifest.Expected.PendingIncomeCount != 2 {
		t.Fatalf("unexpected expected snapshot: %+v", manifest.Expected)
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
