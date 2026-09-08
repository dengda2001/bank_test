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
	a := &app{cfg: config{AuthBaseURL: "https://auth.truelayer-sandbox.com"}}

	rec := httptest.NewRecorder()
	a.handleLogin(rec, httptest.NewRequest(http.MethodGet, "/login", nil))
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

	// Callback arrives with the issued state but NO cookie — a different
	// browser context than the one that started the flow.
	cb := httptest.NewRequest(http.MethodGet, "/callback?state="+url.QueryEscape(state), nil)
	if err := verifyState(cb); err != nil {
		t.Fatalf("callback with issued state but no cookie was rejected: %v", err)
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

func assertQuery(t *testing.T, u *url.URL, key, want string) {
	t.Helper()
	if got := u.Query().Get(key); got != want {
		t.Fatalf("%s=%q want %q", key, got, want)
	}
}
