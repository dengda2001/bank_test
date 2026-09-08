package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testApp() app {
	return app{cfg: config{
		AuthBaseURL:   "https://auth.truelayer-sandbox.com",
		ClientID:      "client-123",
		ClientSecret:  "secret-123",
		RedirectURI:   "http://localhost:8080/callback",
		Scopes:        []string{"info", "accounts", "balance", "transactions"},
		AdminUsername: "admin",
		AdminPassword: "password",
		SessionSecret: "test-session-secret",
	}}
}

func TestAuthURLIncludesOAuthParameters(t *testing.T) {
	a := app{cfg: config{
		AuthBaseURL: "https://auth.truelayer-sandbox.com",
		ClientID:    "client-123",
		RedirectURI: "http://localhost:8080/callback",
		Scopes:      []string{"info", "accounts", "balance", "transactions"},
		Providers:   "ie-ob-all",
	}}

	raw := a.authURL("state-123")
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}

	assertQuery(t, u, "response_type", "code")
	assertQuery(t, u, "client_id", "client-123")
	assertQuery(t, u, "redirect_uri", "http://localhost:8080/callback")
	assertQuery(t, u, "scope", "info accounts balance transactions")
	assertQuery(t, u, "state", "state-123")
	assertQuery(t, u, "providers", "ie-ob-all")
}

func TestAuthURLTemplateOnlyOverwritesState(t *testing.T) {
	a := app{cfg: config{
		AuthURL: "https://auth.truelayer.com/?client_id=from-console&redirect_uri=https%3A%2F%2Fexample.test%2Fcb&state=old",
	}}

	raw := a.authURL("new-state")
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}

	assertQuery(t, u, "client_id", "from-console")
	assertQuery(t, u, "redirect_uri", "https://example.test/cb")
	assertQuery(t, u, "state", "new-state")
}

// Regression test for the iOS live flow: the OAuth callback can be handed off
// from the browser that initiated /login (e.g. WeChat's in-app browser) to a
// different one (Safari), which has no copy of the login cookie. State must be
// validated against the server-side issued-state store, not the cookie.
func TestLoginThenCallbackVerifiesAcrossBrowser(t *testing.T) {
	a := testApp()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	req.AddCookie(sessionCookie(a.cfg, time.Now().Add(sessionTTL)))
	a.handleLogin(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("login status=%d want %d", rec.Code, http.StatusFound)
	}
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	state := loc.Query().Get("state")
	if state == "" {
		t.Fatal("login redirect did not include a state parameter")
	}

	// OAuth state is still verified server-side, independent of the app login
	// cookie that handleCallback now checks separately.
	cb := httptest.NewRequest(http.MethodGet, "/callback?state="+url.QueryEscape(state), nil)
	if err := verifyState(cb); err != nil {
		t.Fatalf("issued state was rejected: %v", err)
	}
}

func TestLocalLoginSetsSessionCookie(t *testing.T) {
	a := testApp()
	form := strings.NewReader("username=admin&password=password")
	req := httptest.NewRequest(http.MethodPost, "/login-local", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	a.handleLocalLogin(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusFound)
	}
	if loc := rec.Header().Get("Location"); loc != "/billing" {
		t.Fatalf("Location=%q want /billing", loc)
	}
	if len(rec.Result().Cookies()) == 0 {
		t.Fatal("expected a session cookie")
	}
}

func TestBillingRequiresSession(t *testing.T) {
	a := testApp()
	rec := httptest.NewRecorder()

	a.handleBilling(rec, httptest.NewRequest(http.MethodGet, "/billing", nil))

	if rec.Code != http.StatusFound {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusFound)
	}
	if loc := rec.Header().Get("Location"); loc != "/" {
		t.Fatalf("Location=%q want /", loc)
	}
}

func TestBankLoginRequiresSession(t *testing.T) {
	a := testApp()
	rec := httptest.NewRecorder()

	a.handleLogin(rec, httptest.NewRequest(http.MethodGet, "/login", nil))

	if rec.Code != http.StatusFound {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusFound)
	}
	if loc := rec.Header().Get("Location"); loc != "/" {
		t.Fatalf("Location=%q want /", loc)
	}
}

func TestCallbackRequiresSessionBeforeConsumingState(t *testing.T) {
	a := testApp()
	state := "issued-state-for-callback"
	issuedStates.record(state)
	req := httptest.NewRequest(http.MethodGet, "/callback?code=abc&state="+url.QueryEscape(state), nil)
	rec := httptest.NewRecorder()

	a.handleCallback(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusFound)
	}
	if loc := rec.Header().Get("Location"); loc != "/" {
		t.Fatalf("Location=%q want /", loc)
	}
	if !issuedStates.consume(state) {
		t.Fatal("callback without app session consumed OAuth state")
	}
}

func TestRefreshRequiresSession(t *testing.T) {
	a := testApp()
	rec := httptest.NewRecorder()

	a.handleRefresh(rec, httptest.NewRequest(http.MethodGet, "/refresh", nil))

	if rec.Code != http.StatusFound {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusFound)
	}
	if loc := rec.Header().Get("Location"); loc != "/" {
		t.Fatalf("Location=%q want /", loc)
	}
}

func TestHasStoredToken(t *testing.T) {
	tokenPath := filepath.Join(t.TempDir(), "token.json")
	a := testApp()
	a.cfg.TokenFile = tokenPath

	if a.hasStoredToken() {
		t.Fatal("token should not exist yet")
	}
	if err := a.saveStoredToken(tokenResponse{RefreshToken: "refresh-token-123"}); err != nil {
		t.Fatal(err)
	}
	if !a.hasStoredToken() {
		t.Fatal("expected stored token to be detected")
	}
}

func TestVerifyStateRejectsUnknownState(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/callback?state="+url.QueryEscape("never-issued"), nil)
	if err := verifyState(r); err == nil {
		t.Fatal("expected a state that was never issued to be rejected")
	}
}

func TestTxQueryToIsCurrentTimeUTC(t *testing.T) {
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)

	q := txQuery("2026-01-01", now)
	if got := q.Get("to"); got != "2026-09-08T12:00:00Z" {
		t.Fatalf("to=%q want current UTC time 2026-09-08T12:00:00Z", got)
	}
	if got := q.Get("from"); got != "2026-01-01" {
		t.Fatalf("from=%q want 2026-01-01", got)
	}
}

func TestTxQueryDropsFutureFrom(t *testing.T) {
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)

	q := txQuery("2027-01-01", now) // from is in the future
	if got := q.Get("from"); got != "" {
		t.Fatalf("future from=%q want empty (dropped)", got)
	}
	if got := q.Get("to"); got != "2026-09-08T12:00:00Z" {
		t.Fatalf("to=%q want current UTC time even when from is dropped", got)
	}
}

func TestAppendStoredTokenPersistsRefreshTokenOnly(t *testing.T) {
	tokenPath := filepath.Join(t.TempDir(), "token.json")
	a := app{cfg: config{TokenFile: tokenPath}}

	if err := a.saveStoredToken(tokenResponse{
		AccessToken:  "access-token-should-not-be-stored",
		RefreshToken: "refresh-token-123",
		TokenType:    "Bearer",
	}); err != nil {
		t.Fatal(err)
	}

	body, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "access-token-should-not-be-stored") {
		t.Fatalf("stored token file contains access token: %s", body)
	}

	info, err := os.Stat(tokenPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("token file mode=%v want 0600", got)
	}

	stored, err := a.loadStoredToken()
	if err != nil {
		t.Fatal(err)
	}
	if stored.RefreshToken != "refresh-token-123" {
		t.Fatalf("refresh_token=%q want refresh-token-123", stored.RefreshToken)
	}
}

func TestVerifyStateRejectsExpiredState(t *testing.T) {
	state := "expired-state-abc"
	issuedStates.record(state)
	issuedStates.mu.Lock()
	issuedStates.items[state] = time.Now().Add(-time.Minute)
	issuedStates.mu.Unlock()

	r := httptest.NewRequest(http.MethodGet, "/callback?state="+state, nil)
	if err := verifyState(r); err == nil {
		t.Fatal("expected an expired state to be rejected")
	}
}

func TestSplitWordsAcceptsSpacesAndCommas(t *testing.T) {
	got := splitWords("info,accounts balance")
	want := []string{"info", "accounts", "balance"}
	if len(got) != len(want) {
		t.Fatalf("len=%d want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("got[%d]=%q want %q", i, got[i], want[i])
		}
	}
}

func TestAppendDemoResultLogWritesBalancesAndTransactions(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "bank-data.jsonl")
	a := app{cfg: config{LogFile: logPath}}
	result := demoResult{
		Environment: "sandbox",
		FetchedAt:   "2026-06-01T16:00:00Z",
		Accounts: []demoAccount{
			{
				Account: account{
					AccountID:   "account-123",
					DisplayName: "Current account",
					Currency:    "EUR",
				},
				Balance:      json.RawMessage(`{"results":[{"currency":"EUR","available":123.45}]}`),
				Transactions: json.RawMessage(`{"results":[{"transaction_id":"tx-1","amount":-10.50}]}`),
			},
		},
	}

	if err := a.appendDemoResultLog(result); err != nil {
		t.Fatal(err)
	}

	body, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	if len(lines) != 1 {
		t.Fatalf("log line count=%d want 1", len(lines))
	}

	var logged demoResult
	if err := json.Unmarshal([]byte(lines[0]), &logged); err != nil {
		t.Fatal(err)
	}
	if len(logged.Accounts) != 1 {
		t.Fatalf("account count=%d want 1", len(logged.Accounts))
	}
	if !strings.Contains(string(logged.Accounts[0].Balance), "123.45") {
		t.Fatalf("balance was not logged: %s", logged.Accounts[0].Balance)
	}
	if !strings.Contains(string(logged.Accounts[0].Transactions), "tx-1") {
		t.Fatalf("transactions were not logged: %s", logged.Accounts[0].Transactions)
	}
}

func TestIncomeTransactionsNormalizeCreditAndPositiveOnly(t *testing.T) {
	result := demoResult{
		FetchedAt: "2026-06-01T16:00:00Z",
		Accounts: []demoAccount{{
			Account: account{AccountID: "acct-1", DisplayName: "Rent account", Currency: "EUR"},
			Transactions: json.RawMessage(`{"results":[
				{
					"transaction_id":"credit-1",
					"normalised_provider_transaction_id":"txn-stable-1",
					"timestamp":"2026-06-01T08:00:00Z",
					"description":"RENT-A12-T003 AOIFE MURPHY",
					"amount":950,
					"currency":"EUR",
					"transaction_type":"CREDIT",
					"meta":{"remitter_name":"Aoife Murphy","remitter_id":"payer-003","payment_reference":"RENT-A12-T003"}
				},
				{
					"transaction_id":"positive-1",
					"timestamp":"2026-06-02T08:00:00Z",
					"description":"D BYRNE RENT",
					"amount":600,
					"currency":"EUR"
				},
				{
					"transaction_id":"debit-1",
					"timestamp":"2026-06-03T08:00:00Z",
					"description":"SUPPLIES",
					"amount":-50,
					"currency":"EUR",
					"transaction_type":"DEBIT"
				}
			]}`),
		}},
	}

	rows := normalizeIncomeTransactions(result)
	if len(rows) != 2 {
		t.Fatalf("income rows=%d want 2", len(rows))
	}
	if rows[0].TransactionID != "credit-1" || rows[0].SourceID != "txn-stable-1" {
		t.Fatalf("unexpected first row ids: %+v", rows[0])
	}
	if rows[0].PayerName != "Aoife Murphy" || rows[0].PayerNameKind != "confirmed" {
		t.Fatalf("unexpected payer: %+v", rows[0])
	}
	if rows[0].PayerID != "payer-003" || rows[0].Reference != "RENT-A12-T003" {
		t.Fatalf("unexpected payer id/reference: %+v", rows[0])
	}
	if rows[1].PayerName != "D BYRNE RENT" || rows[1].PayerNameKind != "inferred" {
		t.Fatalf("expected inferred payer name from description: %+v", rows[1])
	}
	if rows[1].PayerID != "unknown" {
		t.Fatalf("expected unknown payer id, got %+v", rows[1])
	}
}

func assertQuery(t *testing.T, u *url.URL, key, want string) {
	t.Helper()
	if got := u.Query().Get(key); got != want {
		t.Fatalf("%s=%q want %q", key, got, want)
	}
}
