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

func TestVerifyStateRejectsMismatch(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/callback?state=from-query", nil)
	r.AddCookie(&http.Cookie{Name: stateCookieName, Value: "from-cookie"})

	if err := verifyState(r); err == nil {
		t.Fatal("expected state mismatch error")
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
