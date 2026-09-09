package main

import (
	"encoding/base64"
	"encoding/json"
	"io"
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
		AdminUsername: "ddrzh",
		AdminPassword: "ddrzh512",
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
	form := strings.NewReader("username=ddrzh&password=ddrzh512")
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

func TestHashPasswordDoesNotStorePlaintext(t *testing.T) {
	hash, err := hashPassword("ddrzh512")
	if err != nil {
		t.Fatal(err)
	}
	if hash == "ddrzh512" {
		t.Fatal("password hash should not equal plaintext")
	}
	if !verifyPassword(hash, "ddrzh512") {
		t.Fatal("password hash should verify the original password")
	}
	if verifyPassword(hash, "wrong-password") {
		t.Fatal("password hash verified the wrong password")
	}
}

func TestUserSessionCookieAuthenticatesWithUserID(t *testing.T) {
	a := testApp()
	req := httptest.NewRequest(http.MethodGet, "/billing", nil)
	req.AddCookie(userSessionCookie(a.cfg, 42, "ddrzh", time.Now().Add(sessionTTL)))

	if !a.isAuthenticated(req) {
		t.Fatal("v2 user session cookie should authenticate")
	}
	if userID, ok := a.currentUserID(req); !ok || userID != 42 {
		t.Fatalf("currentUserID=(%d,%v) want (42,true)", userID, ok)
	}
}

func TestEncodeRefreshTokenEncryptsAndRoundTrips(t *testing.T) {
	cfg := config{
		Environment:            "live",
		BankTokenEncryptionKey: base64.StdEncoding.EncodeToString([]byte("12345678901234567890123456789012")),
	}

	ciphertext, nonce, mode, err := encodeRefreshToken(cfg, "refresh-token-secret")
	if err != nil {
		t.Fatal(err)
	}
	if mode != "encrypted" {
		t.Fatalf("mode=%q want encrypted", mode)
	}
	if strings.Contains(ciphertext, "refresh-token-secret") {
		t.Fatalf("ciphertext contains plaintext token: %s", ciphertext)
	}
	plain, err := decodeRefreshToken(cfg, ciphertext, nonce, mode)
	if err != nil {
		t.Fatal(err)
	}
	if plain != "refresh-token-secret" {
		t.Fatalf("decoded token=%q want original token", plain)
	}
}

func TestEncodeRefreshTokenPlaintextEscapeIsLocalOnly(t *testing.T) {
	cfg := config{Environment: "sandbox", AllowPlaintextTokens: true}

	ciphertext, nonce, mode, err := encodeRefreshToken(cfg, "refresh-token-dev")
	if err != nil {
		t.Fatal(err)
	}
	if ciphertext != "refresh-token-dev" || nonce != "" || mode != "plaintext_dev" {
		t.Fatalf("unexpected plaintext dev encoding: ciphertext=%q nonce=%q mode=%q", ciphertext, nonce, mode)
	}
	if _, err := decodeRefreshToken(config{Environment: "live", AllowPlaintextTokens: true}, ciphertext, nonce, mode); err == nil {
		t.Fatal("live should not decode plaintext dev tokens")
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

func TestTenantsRequiresSession(t *testing.T) {
	a := testApp()
	rec := httptest.NewRecorder()

	a.handleTenants(rec, httptest.NewRequest(http.MethodGet, "/tenants", nil))

	if rec.Code != http.StatusFound {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusFound)
	}
	if loc := rec.Header().Get("Location"); loc != "/" {
		t.Fatalf("Location=%q want /", loc)
	}
}

func TestCreateTenantPersistsManualRecord(t *testing.T) {
	tenantPath := filepath.Join(t.TempDir(), "tenants.json")
	a := testApp()
	a.cfg.TenantFile = tenantPath

	form := url.Values{}
	form.Set("name", "Aoife Murphy")
	form.Set("monthly_rent", "950")
	form.Set("currency", "EUR")
	form.Set("room_address", "Room A12, 14 Harcourt Street, Dublin")
	req := httptest.NewRequest(http.MethodPost, "/tenants", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(sessionCookie(a.cfg, time.Now().Add(sessionTTL)))
	rec := httptest.NewRecorder()

	a.handleTenants(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusFound)
	}
	if loc := rec.Header().Get("Location"); loc != "/tenants?message=tenant_added" {
		t.Fatalf("Location=%q want /tenants?message=tenant_added", loc)
	}
	rows, err := a.loadTenants()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("tenant count=%d want 1", len(rows))
	}
	if rows[0].Name != "Aoife Murphy" || rows[0].MonthlyRent != 950 || rows[0].RoomAddress != "Room A12, 14 Harcourt Street, Dublin" {
		t.Fatalf("unexpected tenant row: %+v", rows[0])
	}
}

func TestValidateTenantInputAcceptsMonthlyExtensibleFields(t *testing.T) {
	input := tenantInput{
		Name:             "Aoife Murphy",
		PayerID:          "payer-123",
		PayerNameHint:    "AOIFE MURPHY",
		MonthlyRent:      950,
		Currency:         "EUR",
		IntervalUnit:     "month",
		IntervalCount:    1,
		BillingStartDate: "2026-09-01",
		DueDay:           5,
		RentStartDate:    "2026-09-01",
		Status:           "active",
		RoomLabel:        "A12",
		RoomAddress:      "14 Harcourt Street, Dublin",
		PropertyHint:     "Dublin House",
	}

	if err := validateTenantInput(input); err != nil {
		t.Fatalf("valid tenant input rejected: %v", err)
	}
}

func TestValidateTenantInputRejectsUnsupportedIntervalForPhaseOne(t *testing.T) {
	input := tenantInput{
		Name:             "Aoife Murphy",
		MonthlyRent:      950,
		Currency:         "EUR",
		IntervalUnit:     "week",
		IntervalCount:    1,
		BillingStartDate: "2026-09-01",
		DueDay:           5,
		RentStartDate:    "2026-09-01",
		Status:           "active",
		RoomAddress:      "14 Harcourt Street, Dublin",
	}

	if err := validateTenantInput(input); err == nil {
		t.Fatal("expected weekly interval to be rejected in phase one")
	}
}

func TestCreateExpensePersistsManualRecord(t *testing.T) {
	expensePath := filepath.Join(t.TempDir(), "expenses.json")
	a := testApp()
	a.cfg.ExpenseFile = expensePath

	form := url.Values{}
	form.Set("description", "Boiler repair")
	form.Set("category", "Maintenance")
	form.Set("amount", "125.50")
	form.Set("expense_date", "2026-09-09")
	form.Set("payment_method", "Card")
	form.Set("currency", "EUR")
	req := httptest.NewRequest(http.MethodPost, "/expenses", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(sessionCookie(a.cfg, time.Now().Add(sessionTTL)))
	rec := httptest.NewRecorder()

	a.handleExpenses(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusFound)
	}
	if loc := rec.Header().Get("Location"); loc != "/expenses?message=expense_added" {
		t.Fatalf("Location=%q want /expenses?message=expense_added", loc)
	}
	rows, err := a.loadExpenses()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expense count=%d want 1", len(rows))
	}
	if rows[0].Description != "Boiler repair" || rows[0].Category != "Maintenance" || rows[0].Amount != 125.50 || rows[0].ExpenseDate != "2026-09-09" {
		t.Fatalf("unexpected expense row: %+v", rows[0])
	}
}

func TestExpenseInputFromFormKeepsRoomAndTenantHints(t *testing.T) {
	form := url.Values{}
	form.Set("description", "Boiler repair")
	form.Set("amount", "125.50")
	form.Set("category", "Maintenance")
	form.Set("expense_date", "bad-date")
	form.Set("room_hint", "A12")
	form.Set("tenant_hint", "Aoife Murphy")
	now := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)

	input, err := expenseInputFromForm(form, now)
	if err != nil {
		t.Fatal(err)
	}
	if input.ExpenseDate != "2026-09-10" {
		t.Fatalf("expense date=%q want fallback date", input.ExpenseDate)
	}
	if input.RoomHint != "A12" || input.TenantHint != "Aoife Murphy" {
		t.Fatalf("hints not preserved: %+v", input)
	}
	if err := validateExpenseInput(input); err != nil {
		t.Fatalf("valid expense input rejected: %v", err)
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

func TestRefreshTransactionFromCapsOlderConfiguredDate(t *testing.T) {
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)

	got := refreshTransactionFrom("2026-01-01", now)
	if got != "2026-06-10" {
		t.Fatalf("refresh from=%q want 90-day cutoff 2026-06-10", got)
	}
}

func TestRefreshTransactionFromKeepsRecentConfiguredDate(t *testing.T) {
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)

	got := refreshTransactionFrom("2026-08-01", now)
	if got != "2026-08-01" {
		t.Fatalf("refresh from=%q want configured recent date 2026-08-01", got)
	}
}

func TestHandleRefreshRedirectsOnTransactionFetchFailure(t *testing.T) {
	tokenPath := filepath.Join(t.TempDir(), "token.json")
	logPath := filepath.Join(t.TempDir(), "bank-data.jsonl")
	a := testApp()
	a.cfg.From = "2026-01-01"
	a.cfg.TokenFile = tokenPath
	a.cfg.LogFile = logPath

	var transactionFrom string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/connect/token":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"access_token":"access-token-123","refresh_token":"refresh-token-123","token_type":"Bearer"}`)
		case "/data/v1/accounts":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"results":[{"account_id":"acct-1","display_name":"Rent account","currency":"EUR"}]}`)
		case "/data/v1/accounts/acct-1/balance":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"results":[]}`)
		case "/data/v1/accounts/acct-1/transactions":
			transactionFrom = r.URL.Query().Get("from")
			http.Error(w, `{"error":"access_denied"}`, http.StatusForbidden)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	a.cfg.AuthBaseURL = server.URL
	a.cfg.APIBaseURL = server.URL
	a.httpClient = server.Client()

	if err := a.saveStoredToken(tokenResponse{RefreshToken: "refresh-token-123"}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/refresh", nil)
	req.AddCookie(sessionCookie(a.cfg, time.Now().Add(sessionTTL)))
	rec := httptest.NewRecorder()

	a.handleRefresh(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusFound)
	}
	if loc := rec.Header().Get("Location"); loc != "/billing?error=data_fetch_failed" {
		t.Fatalf("Location=%q want /billing?error=data_fetch_failed", loc)
	}
	if transactionFrom == "2026-01-01" {
		t.Fatalf("refresh used uncapped historical from date %q", transactionFrom)
	}
	if _, err := os.Stat(logPath); err == nil {
		t.Fatal("refresh failure should not append a bank data log entry")
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestBillingTemplateShowsDataFetchError(t *testing.T) {
	var body strings.Builder

	err := billingTemplate.Execute(&body, billingPageData{
		Username:    "ddrzh",
		Environment: "sandbox",
		LastSync:    "No sync yet",
		Error:       "data_fetch_failed",
		TokenFile:   "truelayer-token.json",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body.String(), "Bank data refresh failed") {
		t.Fatalf("billing page did not render data fetch error notice: %s", body.String())
	}
}

func TestBillingTemplateRendersTransactionFilters(t *testing.T) {
	var body strings.Builder
	err := billingTemplate.Execute(&body, billingPageData{
		Username:          "ddrzh",
		Environment:       "sandbox",
		DirectionFilter:   "expense",
		MatchStatusFilter: "unmatched",
		PeriodFilter:      "2026-09",
		TransactionRows: []transactionPageRow{{
			Direction:        "expense",
			DirectionLabel:   "Expense",
			PayerName:        "Boiler repair",
			AmountDisplay:    "EUR 125.50",
			DateDisplay:      "09 Sep 2026",
			Description:      "Boiler repair",
			Reference:        "Maintenance",
			AccountName:      "Rent account",
			MatchStatus:      "unmatched",
			MatchStatusLabel: "Unmatched",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"name=\"period\"", "name=\"direction\"", "name=\"match_status\"", "Boiler repair", "Expense"} {
		if !strings.Contains(body.String(), expected) {
			t.Fatalf("billing page missing %q: %s", expected, body.String())
		}
	}
}

func TestRentDashboardTemplateRendersMonthlyStatus(t *testing.T) {
	var body strings.Builder
	err := rentDashboardTemplate.Execute(&body, rentDashboardPageData{
		Username:      "ddrzh",
		Environment:   "sandbox",
		Period:        "2026-09",
		ExpectedTotal: "EUR 950.00",
		Rows: []rentDashboardRow{{
			TenantName:     "Aoife Murphy",
			RoomAddress:    "Room A12",
			DueDate:        "2026-09-05",
			ExpectedAmount: "EUR 950.00",
			Status:         "paid",
			StatusLabel:    "Paid",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"2026-09", "Tenant rent status", "Aoife Murphy", "Rent Dashboard", "Transactions"} {
		if !strings.Contains(body.String(), expected) {
			t.Fatalf("dashboard missing %q: %s", expected, body.String())
		}
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

func TestResolveDatabaseDSNPrefersMYSQLDSN(t *testing.T) {
	cfg := config{
		MySQLDSN:    "root:pass@tcp(127.0.0.1:3306)/rentops?parseTime=true",
		DatabaseURL: "mysql://other:secret@db.example.com:3306/remote",
	}

	got, err := resolveDatabaseDSN(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got != cfg.MySQLDSN {
		t.Fatalf("dsn=%q want MYSQL_DSN", got)
	}
}

func TestResolveDatabaseDSNConvertsDatabaseURL(t *testing.T) {
	cfg := config{
		DatabaseURL: "mysql://rent:secret@127.0.0.1:3306/rentops?parseTime=true&loc=UTC",
	}

	got, err := resolveDatabaseDSN(cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := "rent:secret@tcp(127.0.0.1:3306)/rentops?loc=UTC&parseTime=true"
	if got != want {
		t.Fatalf("dsn=%q want %q", got, want)
	}
}

func TestResolveDatabaseDSNRejectsMissingDatabaseConfig(t *testing.T) {
	if _, err := resolveDatabaseDSN(config{}); err == nil {
		t.Fatal("expected missing database config to be rejected")
	}
}

func TestValidateTokenStorageRequiresEncryptionKeyInLive(t *testing.T) {
	cfg := config{Environment: "live"}

	if err := validateTokenStorageConfig(cfg); err == nil {
		t.Fatal("expected live token storage without encryption key to be rejected")
	}
}

func TestValidateTokenStorageAllowsSandboxPlaintextEscape(t *testing.T) {
	cfg := config{Environment: "sandbox", AllowPlaintextTokens: true}

	if err := validateTokenStorageConfig(cfg); err != nil {
		t.Fatalf("sandbox plaintext escape should be allowed: %v", err)
	}
}

func TestValidateTokenStorageAcceptsBase64EncryptionKey(t *testing.T) {
	cfg := config{
		Environment:            "live",
		BankTokenEncryptionKey: base64.StdEncoding.EncodeToString([]byte("12345678901234567890123456789012")),
	}

	if err := validateTokenStorageConfig(cfg); err != nil {
		t.Fatalf("valid encryption key rejected: %v", err)
	}
}

func TestDiscoverMigrationsSortsSQLFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "002_second.sql"), []byte("SELECT 2;"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "001_first.sql"), []byte("SELECT 1;"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignored"), 0o600); err != nil {
		t.Fatal(err)
	}

	files, err := discoverMigrations(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("migration count=%d want 2", len(files))
	}
	if files[0].version != "001_first" || files[1].version != "002_second" {
		t.Fatalf("unexpected order: %+v", files)
	}
}

func TestSplitSQLStatementsDropsEmptyStatements(t *testing.T) {
	got := splitSQLStatements("CREATE TABLE a (id int); ;\nCREATE TABLE b (id int);\n")
	want := []string{"CREATE TABLE a (id int)", "CREATE TABLE b (id int)"}
	if len(got) != len(want) {
		t.Fatalf("statement count=%d want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("statement[%d]=%q want %q", i, got[i], want[i])
		}
	}
}

func TestReadDemoResultsSupportsJSONLAndPrettyJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bank-data.jsonl")
	body := "{\"fetched_at\":\"2026-09-01T00:00:00Z\"}\n{\n  \"fetched_at\": \"2026-09-02T00:00:00Z\"\n}\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	results, err := readDemoResults(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[0].FetchedAt != "2026-09-01T00:00:00Z" || results[1].FetchedAt != "2026-09-02T00:00:00Z" {
		t.Fatalf("unexpected demo results: %+v", results)
	}
}

func TestValidateTransactionFilters(t *testing.T) {
	if err := validateTransactionFilters(transactionFilters{Direction: "income", MatchStatus: "candidate", PeriodMonth: "2026-09"}); err != nil {
		t.Fatalf("valid filters rejected: %v", err)
	}
	if err := validateTransactionFilters(transactionFilters{Direction: "transfer"}); err == nil {
		t.Fatal("unsupported direction should be rejected")
	}
	if err := validateTransactionFilters(transactionFilters{PeriodMonth: "2026-13"}); err == nil {
		t.Fatal("invalid period should be rejected")
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
					"transaction_id":"preferred-name-1",
					"timestamp":"2026-06-04T08:00:00Z",
					"description":"5/26 *MOBI REMAIN RENT",
					"amount":50,
					"currency":"EUR",
					"meta":{"counter_party_preferred_name":"Mike","provider_reference":"IE26050622210934"}
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
	if len(rows) != 3 {
		t.Fatalf("income rows=%d want 3", len(rows))
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
	if rows[2].PayerName != "Mike" || rows[2].PayerNameKind != "confirmed" {
		t.Fatalf("expected confirmed payer name from counter_party_preferred_name: %+v", rows[2])
	}
	if rows[2].Reference != "IE26050622210934" {
		t.Fatalf("expected provider reference fallback, got %+v", rows[2])
	}
}

func TestNormalizePaymentTransactionsIncludesIncomeAndExpenses(t *testing.T) {
	result := demoResult{
		FetchedAt: "2026-09-10T10:00:00Z",
		Accounts: []demoAccount{{
			Account: account{AccountID: "acct-1", DisplayName: "Rent account", Currency: "EUR"},
			Transactions: json.RawMessage(`{"results":[
				{
					"transaction_id":"income-1",
					"normalised_provider_transaction_id":"stable-income-1",
					"timestamp":"2026-09-01T08:00:00Z",
					"description":"AOIFE MURPHY SEPT RENT",
					"amount":950,
					"currency":"EUR",
					"transaction_type":"CREDIT",
					"payer_id":"payer-123",
					"payer_name":"Aoife Murphy"
				},
				{
					"transaction_id":"expense-1",
					"provider_transaction_id":"stable-expense-1",
					"timestamp":"2026-09-02T08:00:00Z",
					"description":"BOILER REPAIR",
					"amount":-125.50,
					"currency":"EUR",
					"transaction_type":"DEBIT"
				}
			]}`),
		}},
	}

	rows := normalizePaymentTransactions(result)
	if len(rows) != 2 {
		t.Fatalf("transaction count=%d want 2", len(rows))
	}
	if rows[0].Direction != "income" || rows[0].AmountCents != 95000 {
		t.Fatalf("unexpected income row: %+v", rows[0])
	}
	if rows[0].StableTransactionKey != "provider:truelayer:stable-income-1" {
		t.Fatalf("income stable key=%q", rows[0].StableTransactionKey)
	}
	if rows[0].PayerID != "payer-123" || rows[0].PayerName != "Aoife Murphy" {
		t.Fatalf("payer fields not normalized: %+v", rows[0])
	}
	if rows[1].Direction != "expense" || rows[1].AmountCents != 12550 {
		t.Fatalf("unexpected expense row: %+v", rows[1])
	}
	if rows[1].StableTransactionKey != "provider:truelayer:stable-expense-1" {
		t.Fatalf("expense stable key=%q", rows[1].StableTransactionKey)
	}
}

func TestStableTransactionKeyFallsBackToDeterministicHash(t *testing.T) {
	ts := time.Date(2026, 9, 10, 8, 0, 0, 0, time.UTC)
	input := paymentTransactionInput{
		Source:          "truelayer",
		AccountID:       "acct-1",
		Direction:       "income",
		AmountCents:     95000,
		Currency:        "EUR",
		TransactionTime: &ts,
		Description:     "AOIFE MURPHY SEPT RENT",
	}

	first := stableTransactionKey(input)
	second := stableTransactionKey(input)
	if first == "" || !strings.HasPrefix(first, "hash:") {
		t.Fatalf("fallback stable key=%q want hash prefix", first)
	}
	if first != second {
		t.Fatalf("stable key is not deterministic: %q vs %q", first, second)
	}
}

func TestTenantActiveInMonthUsesRentDatesWithoutProration(t *testing.T) {
	rentStart := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)
	rentEnd := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	row := tenant{
		Status:        "active",
		RentStartDate: rentStart,
		RentEndDate:   &rentEnd,
	}

	if !tenantActiveInMonth(row, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatal("tenant should be active for September because the dates overlap")
	}
	if !tenantActiveInMonth(row, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatal("tenant should be active for October because the dates overlap")
	}
	if tenantActiveInMonth(row, time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatal("tenant should not be active after rent end month")
	}
}

func TestTenantActiveInMonthRespectsBillingStartDate(t *testing.T) {
	row := tenant{
		Status:           "active",
		RentStartDate:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		BillingStartDate: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
	}
	if tenantActiveInMonth(row, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatal("tenant should not have an obligation before billing start")
	}
	if !tenantActiveInMonth(row, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatal("tenant should have an obligation from billing start")
	}
}

func TestDueDateForMonthClampsInvalidMonthDay(t *testing.T) {
	period := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	got := dueDateForMonth(period, 31)
	want := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("due date=%s want %s", got, want)
	}
}

func TestObligationStatus(t *testing.T) {
	now := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	due := time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name        string
		expected    int64
		paid        int64
		needsReview bool
		want        string
	}{
		{name: "paid", expected: 95000, paid: 95000, want: "paid"},
		{name: "partial", expected: 95000, paid: 60000, want: "partial"},
		{name: "overdue", expected: 95000, paid: 0, want: "overdue"},
		{name: "review", expected: 95000, paid: 0, needsReview: true, want: "needs_review"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := obligationStatus(tc.expected, tc.paid, due, now, tc.needsReview)
			if got != tc.want {
				t.Fatalf("status=%q want %q", got, tc.want)
			}
		})
	}
}

func TestParseReferencedPeriodSupportedFormats(t *testing.T) {
	txTime := time.Date(2026, 10, 2, 8, 0, 0, 0, time.UTC)
	cases := map[string]string{
		"rent 2026-09":   "2026-09-01",
		"rent 2026/09":   "2026-09-01",
		"rent 09/2026":   "2026-09-01",
		"rent 9/26":      "2026-09-01",
		"Sep 2026 rent":  "2026-09-01",
		"September rent": "2026-09-01",
		"2026年9月 房租":     "2026-09-01",
		"9月 rent":        "2026-09-01",
		"rent Sep":       "2026-09-01",
		"rent 09":        "2026-09-01",
	}
	for text, wantText := range cases {
		t.Run(text, func(t *testing.T) {
			got, ok := parseReferencedPeriod(text, txTime)
			if !ok {
				t.Fatal("expected period reference")
			}
			want, _ := time.Parse(dateLayout, wantText)
			if !got.Equal(want) {
				t.Fatalf("period=%s want %s", got.Format(dateLayout), wantText)
			}
		})
	}
}

func TestDecideRentMatchUsesPayerIDForExactPayment(t *testing.T) {
	payerID := "payer-123"
	tenantRow := tenant{ID: 7, PayerID: &payerID}
	period := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	obligation := rentObligation{ID: 11, TenantID: 7, PeriodMonth: period, ExpectedAmountCents: 95000}
	tx := paymentTransactionInput{
		Direction:       "income",
		AmountCents:     95000,
		PayerID:         "payer-123",
		Description:     "rent 2026-09",
		TransactionTime: ptrTime(time.Date(2026, 10, 2, 8, 0, 0, 0, time.UTC)),
	}

	decision := decideRentMatch(tx, []tenant{tenantRow}, []rentObligation{obligation})
	if decision.Status != "matched" || decision.RentObligationID != 11 || decision.ConfirmationSource != "auto_id" {
		t.Fatalf("unexpected decision: %+v", decision)
	}
}

func TestDecideRentMatchTreatsNameMatchAsCandidateWithBackfill(t *testing.T) {
	tenantRow := tenant{ID: 7, Name: "Aoife Murphy"}
	obligation := rentObligation{ID: 11, TenantID: 7, PeriodMonth: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), ExpectedAmountCents: 95000}
	tx := paymentTransactionInput{
		Direction:       "income",
		AmountCents:     95000,
		PayerID:         "payer-123",
		PayerName:       "Aoife Murphy",
		TransactionTime: ptrTime(time.Date(2026, 9, 4, 8, 0, 0, 0, time.UTC)),
	}

	decision := decideRentMatch(tx, []tenant{tenantRow}, []rentObligation{obligation})
	if decision.Status != "candidate" || !decision.BackfillPayerID {
		t.Fatalf("unexpected name candidate decision: %+v", decision)
	}
}

func TestDecideRentMatchOverpaymentNeedsReview(t *testing.T) {
	payerID := "payer-123"
	tenantRow := tenant{ID: 7, PayerID: &payerID}
	obligation := rentObligation{ID: 11, TenantID: 7, PeriodMonth: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), ExpectedAmountCents: 95000}
	tx := paymentTransactionInput{
		Direction:       "income",
		AmountCents:     120000,
		PayerID:         "payer-123",
		TransactionTime: ptrTime(time.Date(2026, 9, 4, 8, 0, 0, 0, time.UTC)),
	}

	decision := decideRentMatch(tx, []tenant{tenantRow}, []rentObligation{obligation})
	if decision.Status != "needs_review" || decision.Reason != "overpayment" {
		t.Fatalf("unexpected overpayment decision: %+v", decision)
	}
}

func ptrTime(t time.Time) *time.Time {
	return &t
}

func TestCountUnknownPayersCountsUnknownNamesOnly(t *testing.T) {
	rows := []incomeTransaction{
		{PayerNameKind: "confirmed", PayerID: "unknown"},
		{PayerNameKind: "unknown", PayerID: "payer-1"},
		{PayerNameKind: "inferred", PayerID: "unknown"},
	}

	got := countUnknownPayers(rows)
	if got != 1 {
		t.Fatalf("unknown payer count=%d want only rows with unknown names", got)
	}
}

func assertQuery(t *testing.T, u *url.URL, key, want string) {
	t.Helper()
	if got := u.Query().Get(key); got != want {
		t.Fatalf("%s=%q want %q", key, got, want)
	}
}
