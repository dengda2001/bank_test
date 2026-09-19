package main

import (
	"context"
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

// The page chrome must name the signed-in account, not whichever account
// APP_ADMIN_USERNAME happens to configure. A second account signing in used to
// see "当前用户：ddrzh" on every page while its own data stayed correctly isolated.
func TestDisplayUsernameFollowsTheSessionAccount(t *testing.T) {
	a := testApp()

	request := func(cookie *http.Cookie) string {
		req := httptest.NewRequest(http.MethodGet, "/tenants", nil)
		if cookie != nil {
			req.AddCookie(cookie)
		}
		return a.displayUsername(req)
	}

	if got := request(userSessionCookie(a.cfg, 2, "rentops-demo", time.Now().Add(sessionTTL))); got != "rentops-demo" {
		t.Fatalf("displayUsername for a second account=%q want %q", got, "rentops-demo")
	}
	if got := request(userSessionCookie(a.cfg, 1, "ddrzh", time.Now().Add(sessionTTL))); got != "ddrzh" {
		t.Fatalf("displayUsername for the administrator=%q want %q", got, "ddrzh")
	}
	if got := request(nil); got != a.cfg.AdminUsername {
		t.Fatalf("displayUsername without a session=%q want %q", got, a.cfg.AdminUsername)
	}
	expired := userSessionCookie(a.cfg, 2, "rentops-demo", time.Now().Add(-time.Minute))
	if got := request(expired); got != a.cfg.AdminUsername {
		t.Fatalf("displayUsername with an expired session=%q want %q", got, a.cfg.AdminUsername)
	}
	tampered := userSessionCookie(a.cfg, 2, "rentops-demo", time.Now().Add(sessionTTL))
	tampered.Value = strings.Replace(tampered.Value, "rentops-demo", "ddrzh", 1)
	if got := request(tampered); got != a.cfg.AdminUsername {
		t.Fatalf("displayUsername with a tampered session=%q want %q", got, a.cfg.AdminUsername)
	}
}

func TestWorkspaceTemplatesIncludeSharedCalendarPicker(t *testing.T) {
	tests := []struct {
		name   string
		render func(*strings.Builder) error
	}{
		{name: "billing", render: func(body *strings.Builder) error {
			return billingTemplate.Execute(body, billingPageData{})
		}},
		{name: "dashboard", render: func(body *strings.Builder) error {
			return rentDashboardTemplate.Execute(body, rentDashboardPageData{})
		}},
		{name: "tenants", render: func(body *strings.Builder) error {
			return tenantTemplate.Execute(body, tenantPageData{})
		}},
		{name: "expenses", render: func(body *strings.Builder) error {
			return expenseTemplate.Execute(body, expensePageData{})
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var body strings.Builder
			if err := test.render(&body); err != nil {
				t.Fatalf("render template: %v", err)
			}
			page := body.String()
			for _, expected := range []string{
				".calendar-popover",
				"calendar-input",
				"input[type=\"date\"], input[type=\"month\"]",
				// Three-level zoom: the header title is clickable (日→月→年) and the
				// outermost level is a decade grid.
				".calendar-title {\n",
				".calendar-year-grid {",
				"zoomTo('month', '选择月份')",
				"zoomTo('year', '选择年份')",
				"const renderYear = () => {",
				// Outside-click detection must use the dispatch-time event path:
				// render() detaches the clicked node, so a contains() check on the
				// live DOM makes the picker close on its own inner clicks.
				"event.composedPath().includes(wrapper)",
			} {
				if !strings.Contains(page, expected) {
					t.Fatalf("expected shared calendar picker marker %q", expected)
				}
			}
		})
	}
}

func TestWorkspaceUIPrimitivesHaveConsistentInteractionStates(t *testing.T) {
	for _, expected := range []string{
		".btn:focus-visible",
		".btn:disabled",
		".btn[aria-disabled=\"true\"]",
		"input:focus-visible, select:focus-visible, textarea:focus-visible",
		".btn {\n      min-height: 40px;",
	} {
		if !strings.Contains(workspacePageCSS, expected) {
			t.Fatalf("workspace CSS missing interaction primitive %q", expected)
		}
	}
}

func TestWorkspaceCSSUsesFlatDesignTokens(t *testing.T) {
	// The flat look is a desktop constraint: everything at or above the 640 tier
	// -- the base rules, the desktop shell and the @media (max-width: 1100px)
	// tier -- must stay free of depth effects. The 640 block below is the frozen
	// mobile contract, and there the drawer scrim and the frozen column's inset
	// shadow are functional (they say "there is more to the right"), so the
	// guard stops at the breakpoint.
	baseCSS, _, _ := strings.Cut(workspacePageCSS, "@media (max-width: 640px)")
	for _, expected := range []string{
		"--surface: #ffffff;",
		"--foreground: #111827;",
		"--accent: #2563eb;",
		"background: var(--background-base);",
		".panel { border: 1px solid var(--border); border-radius: 12px; background: var(--surface); }",
	} {
		if !strings.Contains(baseCSS, expected) {
			t.Fatalf("flat workspace CSS missing %q", expected)
		}
	}
	for _, forbidden := range []string{"backdrop-filter:", "box-shadow:", "radial-gradient", "linear-gradient", "rgba("} {
		if strings.Contains(baseCSS, forbidden) {
			t.Fatalf("flat workspace CSS still contains depth effect %q", forbidden)
		}
	}
}

func TestWorkspaceCSSGuardsResponsiveContent(t *testing.T) {
	for _, expected := range []string{
		"overflow-x: hidden;",
		".table-wrap { max-width: 100%; overflow-x: auto; }",
		"@media (max-width: 640px)",
		"input[type=\"checkbox\"], input[type=\"radio\"] { width: auto;",
	} {
		if !strings.Contains(workspacePageCSS, expected) {
			t.Fatalf("responsive workspace CSS missing %q", expected)
		}
	}
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
	if loc := rec.Header().Get("Location"); loc != "/rent-dashboard" {
		t.Fatalf("Location=%q want /rent-dashboard", loc)
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

	// `to` is the current instant; a date-only `from` of today-90 (2026-06-10)
	// would still be >90 days before now, which Irish providers reject with a
	// 403 SCA Active check failed. The cutoff must be today-89.
	got := refreshTransactionFrom("2026-01-01", now)
	if got != "2026-06-11" {
		t.Fatalf("refresh from=%q want 90-day cutoff 2026-06-11", got)
	}
}

func TestRefreshTransactionFromKeepsRecentConfiguredDate(t *testing.T) {
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)

	got := refreshTransactionFrom("2026-08-01", now)
	if got != "2026-08-01" {
		t.Fatalf("refresh from=%q want configured recent date 2026-08-01", got)
	}
}

// TestRefreshTransactionFromStaysInsideLookbackLateInUtcDay locks the bug where
// a refresh early in a +08 morning (late in the UTC day) used the UTC date to
// subtract the lookback and produced a from one day too old, crossing the
// bank's 90-day boundary.
func TestRefreshTransactionFromStaysInsideLookbackLateInUtcDay(t *testing.T) {
	// 2026-09-08 17:30 UTC == 2026-09-09 01:30 in +08.
	now := time.Date(2026, 9, 8, 17, 30, 0, 0, time.UTC)

	got := refreshTransactionFrom("2026-01-01", now)
	if got != "2026-06-11" {
		t.Fatalf("refresh from=%q want cutoff 2026-06-11 (naive today-90 would give 2026-06-10)", got)
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

func TestFetchDemoResultKeepsSuccessfulAccountsWhenAnotherTransactionFetchFails(t *testing.T) {
	a := testApp()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/data/v1/accounts":
			io.WriteString(w, `{"results":[{"account_id":"acct-ok","display_name":"Rent account","currency":"EUR"},{"account_id":"acct-failed","display_name":"Reserve account","currency":"EUR"}]}`)
		case "/data/v1/accounts/acct-ok/balance", "/data/v1/accounts/acct-failed/balance":
			io.WriteString(w, `{"results":[]}`)
		case "/data/v1/accounts/acct-ok/transactions":
			io.WriteString(w, `{"results":[{"transaction_id":"tx-ok","timestamp":"2026-09-01T00:00:00Z","amount":950,"currency":"EUR","transaction_type":"CREDIT"}]}`)
		case "/data/v1/accounts/acct-failed/transactions":
			http.Error(w, `{"error":"access_denied"}`, http.StatusForbidden)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	a.cfg.APIBaseURL = server.URL
	a.httpClient = server.Client()

	result, err := a.fetchDemoResultWithOptions(context.Background(), "access-token", "2026-09-01", true)
	if err != nil {
		t.Fatalf("partial fetch returned error: %v", err)
	}
	if len(result.Accounts) != 2 {
		t.Fatalf("account count=%d want 2", len(result.Accounts))
	}
	if len(result.Accounts[0].Transactions) == 0 || len(result.Accounts[1].Errors) == 0 {
		t.Fatalf("partial account results not preserved: %+v", result.Accounts)
	}
}

func TestBillingTemplateShowsDataFetchError(t *testing.T) {
	var body strings.Builder

	err := billingTemplate.Execute(&body, billingPageData{
		workspaceShell: workspaceShell{Username: "ddrzh", Environment: "sandbox"},
		LastSync:       "尚未同步",
		Error:          "data_fetch_failed",
		TokenFile:      "truelayer-token.json",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body.String(), "银行数据刷新失败") {
		t.Fatalf("billing page did not render data fetch error notice: %s", body.String())
	}
}

func TestBillingTemplateRendersTransactionFilters(t *testing.T) {
	var body strings.Builder
	err := billingTemplate.Execute(&body, billingPageData{
		workspaceShell:    workspaceShell{Username: "ddrzh", Environment: "sandbox"},
		DirectionFilter:   "expense",
		MatchStatusFilter: "unmatched",
		PeriodFilter:      "2026-09",
		TransactionRows: []transactionPageRow{{
			Direction:        "expense",
			DirectionLabel:   "支出",
			PayerName:        "Boiler repair",
			AmountDisplay:    "EUR 125.50",
			DateDisplay:      "09 Sep 2026",
			Description:      "Boiler repair",
			AccountName:      "Rent account",
			MatchStatus:      "unmatched",
			MatchStatusLabel: "未关联",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"name=\"period\"", "name=\"direction\"", "name=\"match_status\"", `onchange="this.form.submit()"`, "Boiler repair", "支出"} {
		if !strings.Contains(body.String(), expected) {
			t.Fatalf("billing page missing %q: %s", expected, body.String())
		}
	}
}

func TestBillingTemplateShowsMonthChoiceForRememberedTenant(t *testing.T) {
	var body strings.Builder
	err := billingTemplate.Execute(&body, billingPageData{
		TransactionRows: []transactionPageRow{{
			Direction:        "income",
			MatchStatus:      "needs_review",
			MatchStatusLabel: "需处理",
			TenantID:         7,
			NeedsMonthChoice: true,
			MonthOptions: []billingMonthOption{{
				Period:    "2026-08",
				Label:     "2026年8月",
				Expected:  "EUR 950.00",
				Paid:      "EUR 0.00",
				Remaining: "EUR 950.00",
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"已识别租客，请确认租金月份", "选择月份...", "2026年8月", "应收 EUR 950.00", "确认匹配"} {
		if !strings.Contains(body.String(), expected) {
			t.Fatalf("billing page missing month-choice text %q: %s", expected, body.String())
		}
	}
}

func TestBillingTemplateOffersHistoricalPayerPreview(t *testing.T) {
	var body strings.Builder
	if err := billingTemplate.Execute(&body, billingPageData{}); err != nil {
		t.Fatal(err)
	}
	page := body.String()
	for _, expected := range []string{"/billing/payer/preview", "添加拆分项", "allocation_kind[]"} {
		if !strings.Contains(page, expected) {
			t.Fatalf("billing page missing historical preview or split control %q", expected)
		}
	}
}

func TestBillingTemplateHidesAllocationForCompletedOrIgnoredIncome(t *testing.T) {
	for _, status := range []string{"matched", "ignored"} {
		var body strings.Builder
		if err := billingTemplate.Execute(&body, billingPageData{TransactionRows: []transactionPageRow{{Direction: "income", MatchStatus: status}}}); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(body.String(), `<form class="allocation-form"`) {
			t.Fatalf("status=%s should not offer a new allocation form", status)
		}
	}
}

func TestAllocationFormValuesSupportsRepeatedAndScalarFields(t *testing.T) {
	repeated := url.Values{"amount[]": {"100.00", "50.00"}}
	if got := allocationFormValues(repeated, "amount"); len(got) != 2 || got[1] != "50.00" {
		t.Fatalf("repeated allocation values=%v", got)
	}
	scalar := url.Values{"amount": {"100.00"}}
	if got := allocationFormValues(scalar, "amount"); len(got) != 1 || got[0] != "100.00" {
		t.Fatalf("scalar allocation values=%v", got)
	}
}

func TestBillingMutationRoutesRequireAuthentication(t *testing.T) {
	routes := []struct {
		path    string
		handler func(*app) http.HandlerFunc
	}{
		{path: "/billing/allocate", handler: func(a *app) http.HandlerFunc { return a.handleTransactionAllocation }},
		{path: "/billing/rematch", handler: func(a *app) http.HandlerFunc { return a.handleTransactionRematch }},
		{path: "/billing/ignore", handler: func(a *app) http.HandlerFunc { return a.handleTransactionIgnore }},
		{path: "/billing/restore", handler: func(a *app) http.HandlerFunc { return a.handleTransactionRestore }},
		{path: "/billing/revoke", handler: func(a *app) http.HandlerFunc { return a.handleTransactionRevoke }},
		{path: "/billing/payer/preview", handler: func(a *app) http.HandlerFunc { return a.handlePayerPreview }},
		{path: "/billing/payer/confirm", handler: func(a *app) http.HandlerFunc { return a.handlePayerConfirm }},
	}
	for _, route := range routes {
		a := testApp()
		rec := httptest.NewRecorder()
		route.handler(&a).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, route.path, nil))
		if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/" {
			t.Fatalf("path=%s status=%d location=%q want unauthenticated redirect", route.path, rec.Code, rec.Header().Get("Location"))
		}
	}
}

func TestRevokePreviewTemplateShowsSourceAndEffectiveAllocations(t *testing.T) {
	var body strings.Builder
	err := revokePreviewTemplate.Execute(&body, transactionRevokePreviewData{
		TransactionID:                 "7",
		Description:                   "September rent",
		AmountDisplay:                 "EUR 2,000.00",
		AllocatedAmountDisplay:        "EUR 1,000.00",
		CurrentRemainingAmountDisplay: "EUR 1,000.00",
		RemainingAmountDisplay:        "EUR 2,000.00",
		Allocations: []transactionRevokePreviewAllocation{{
			Kind:          "房租",
			AmountDisplay: "EUR 1,000.00",
			TenantName:    "Aoife Murphy",
			PeriodDisplay: "2026-09",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"September rent", "EUR 2,000.00", "EUR 1,000.00", "Aoife Murphy", "/billing/revoke"} {
		if !strings.Contains(body.String(), expected) {
			t.Fatalf("revoke preview missing %q: %s", expected, body.String())
		}
	}
}

func TestRevokePreviewDataShowsFullBalanceAfterRevoke(t *testing.T) {
	tenantID := uint64(7)
	preview := transactionRevokePreview{
		Source:      paymentTransaction{ID: 7, AmountCents: 200000, Currency: "EUR", Description: "September rent"},
		Allocations: []paymentAllocation{{TenantID: &tenantID, AmountCents: 100000, AllocationKind: allocationKindRent, Status: allocationStatusConfirmed}},
		TenantNames: map[uint64]string{tenantID: "Aoife Murphy"},
		Obligations: map[uint64]rentObligation{},
	}
	data := transactionRevokePreviewDataFromModel(preview)
	if data.CurrentRemainingAmountDisplay != "EUR 1000.00" || data.RemainingAmountDisplay != "EUR 2000.00" {
		t.Fatalf("preview balances current=%q after=%q", data.CurrentRemainingAmountDisplay, data.RemainingAmountDisplay)
	}
}

func TestHistoricalPayerPreviewTemplateConfirmsOneRowAtATime(t *testing.T) {
	var body strings.Builder
	err := payerPreviewTemplate.Execute(&body, payerPreviewPageData{Rows: []payerPreviewRow{{
		TransactionID: "7", PayerName: "Aoife Murphy", AmountDisplay: "EUR 950.00", ProposedTenantName: "Aoife Murphy", ProposedPeriod: "2026-09", CanConfirm: true,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	page := body.String()
	if !strings.Contains(page, "/billing/payer/confirm") || strings.Count(page, "/billing/payer/confirm") != 1 {
		t.Fatalf("historical preview should render one confirm form per row: %s", page)
	}
}

func TestRentDashboardTemplateRendersMonthlyStatus(t *testing.T) {
	var body strings.Builder
	err := rentDashboardTemplate.Execute(&body, rentDashboardPageData{
		workspaceShell: workspaceShell{Username: "ddrzh", Environment: "sandbox"},
		Period:         "2026-09",
		PeriodLabel:    "2026年9月",
		ExpectedTotal:  "EUR 950.00",
		Rows: []rentDashboardRow{{
			ObligationID:   11,
			TenantName:     "Aoife Murphy",
			RoomAddress:    "Room A12",
			DueDate:        "2026-09-05",
			ExpectedAmount: "EUR 950.00",
			Status:         "paid",
			StatusLabel:    "已缴清",
			Payments: []rentPaymentDetail{{
				AmountDisplay:      "EUR 950.00",
				DateDisplay:        "04 Sep 2026 08:00",
				Description:        "September rent",
				Reference:          "rent-2026-09",
				ConfirmationSource: "auto_id",
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"2026-09", "租客缴费情况", "Aoife Murphy", "月度总览", "银行流水", `onchange="this.form.submit()"`, "收款明细", "September rent"} {
		if !strings.Contains(body.String(), expected) {
			t.Fatalf("dashboard missing %q: %s", expected, body.String())
		}
	}
	for _, unwanted := range []string{"参考号", "rent-2026-09"} {
		if strings.Contains(body.String(), unwanted) {
			t.Fatalf("dashboard payment detail still renders %q: %s", unwanted, body.String())
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

func TestFiltersFromQuerySupportsPendingOnly(t *testing.T) {
	query := url.Values{}
	query.Set("period", "2026-09")
	query.Set("pending", "1")
	filters := filtersFromQuery(query)
	if filters.PeriodMonth != "2026-09" || !filters.PendingOnly {
		t.Fatalf("unexpected pending filters: %+v", filters)
	}
}

func TestFiltersFromQuerySeparatesArrivalAndRentPeriods(t *testing.T) {
	query := url.Values{
		"arrival_from": {"2026-08-01"},
		"arrival_to":   {"2026-09-01"},
		"payer":        {"Aoife"},
		"tenant_id":    {"7"},
		"rent_period":  {"2026-07"},
		"allocation":   {allocationKindDeposit},
		"sort":         {"amount_asc"},
		"page":         {"2"},
		"page_size":    {"25"},
		"pending":      {"1"},
	}
	filters := filtersFromQuery(query)
	if filters.ArrivalFrom != "2026-08-01" || filters.ArrivalTo != "2026-09-01" || filters.PeriodMonth != "" || filters.RentPeriod != "2026-07" || filters.Payer != "Aoife" || filters.TenantID != 7 || filters.AllocationKind != allocationKindDeposit || filters.Sort != "amount_asc" || filters.Page != 2 || filters.PageSize != 25 || !filters.PendingOnly {
		t.Fatalf("filters=%+v want separate arrival/rent periods and all query controls", filters)
	}
	if err := validateTransactionFilters(filters); err != nil {
		t.Fatal(err)
	}
}

func TestValidateTransactionFiltersRejectsUnsafeSortAndInvalidDateRange(t *testing.T) {
	if err := validateTransactionFilters(transactionFilters{Sort: "amount desc; DROP TABLE payment_transactions"}); err == nil {
		t.Fatal("unsafe sort should be rejected")
	}
	if err := validateTransactionFilters(transactionFilters{ArrivalFrom: "2026-09-02", ArrivalTo: "2026-09-01"}); err == nil {
		t.Fatal("reversed arrival date range should be rejected")
	}
}

func TestTransactionPageRowDisplaysInternalAndProviderIdentifiers(t *testing.T) {
	providerID := "bank-tx-7"
	parsed := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	row := transactionPageRowFromModel(paymentTransaction{
		ID: 7, ProviderTransactionID: &providerID, Direction: "income", AmountCents: 95000, Currency: "EUR",
		TransactionTime: &parsed, ParsedPeriodMonth: &parsed, MatchStatus: "partial",
	})
	if row.InternalID != "7" || row.ProviderTransactionID != providerID || row.ParsedPeriodDisplay != "2026-07" || row.MatchStatusLabel != "部分关联" {
		t.Fatalf("page row=%+v want both identifiers and parsed period", row)
	}
}

func TestFallbackTransactionPageRowsFiltersPendingTransactionsByPeriod(t *testing.T) {
	result := demoResult{Accounts: []demoAccount{{
		Account: account{AccountID: "acct-1", DisplayName: "Rent account", Currency: "EUR"},
		Transactions: json.RawMessage(`{"results":[
			{"transaction_id":"sept-income","provider_transaction_id":"sept-income","timestamp":"2026-09-09T00:00:00Z","amount":950,"transaction_type":"CREDIT"},
			{"transaction_id":"oct-income","provider_transaction_id":"oct-income","timestamp":"2026-10-09T00:00:00Z","amount":950,"transaction_type":"CREDIT"},
			{"transaction_id":"sept-expense","provider_transaction_id":"sept-expense","timestamp":"2026-09-10T00:00:00Z","amount":-25,"transaction_type":"DEBIT"}
		]}`),
	}}}

	rows := fallbackTransactionPageRows(result, transactionFilters{PeriodMonth: "2026-09", PendingOnly: true})
	if len(rows) != 1 || rows[0].TransactionID != "sept-income" {
		t.Fatalf("unexpected pending rows: %+v", rows)
	}
}

func TestRentDashboardTemplateLinksPendingCountToSelectedPeriod(t *testing.T) {
	var body strings.Builder
	if err := rentDashboardTemplate.Execute(&body, rentDashboardPageData{
		Period:       "2026-09",
		PeriodLabel:  "2026年9月",
		PendingCount: 4,
	}); err != nil {
		t.Fatal(err)
	}
	page := body.String()
	if !strings.Contains(page, `href="/billing?period=2026-09&amp;pending=1"`) {
		t.Fatalf("pending count is not linked to the selected period: %s", page)
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
		Status:           "active",
		MonthlyRentCents: 100000,
		Currency:         "EUR",
		RentStartDate:    rentStart,
		RentEndDate:      &rentEnd,
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
		MonthlyRentCents: 100000,
		Currency:         "EUR",
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

func TestParseReferencedPeriodUsesYearFromCompactEnglishMonthYear(t *testing.T) {
	txTime := time.Date(2027, 9, 2, 8, 0, 0, 0, time.UTC)
	cases := map[string]string{
		"REMAIN50 *MOBI RENT JULY26": "2026-07-01",
		"RENT SEP26":                 "2026-09-01",
	}
	for text, wantText := range cases {
		t.Run(text, func(t *testing.T) {
			got, ok := parseReferencedPeriod(text, txTime)
			if !ok {
				t.Fatal("expected compact English month-year reference")
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

func TestDecideRentMatchUsesRememberedPayerNameAutomatically(t *testing.T) {
	tenantRow := tenant{ID: 7, Name: "张三", PayerNameHint: ptrString("ZHANG SAN")}
	obligation := rentObligation{ID: 11, TenantID: 7, PeriodMonth: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), ExpectedAmountCents: 95000}
	tx := paymentTransactionInput{
		Direction:       "income",
		AmountCents:     95000,
		PayerName:       " zhang   san ",
		TransactionTime: ptrTime(time.Date(2026, 9, 4, 8, 0, 0, 0, time.UTC)),
	}

	decision := decideRentMatch(tx, []tenant{tenantRow}, []rentObligation{obligation})
	if decision.Status != "matched" || decision.ConfirmationSource != "auto_name" {
		t.Fatalf("unexpected remembered-name decision: %+v", decision)
	}
}

func TestDecideRentMatchOverpaymentCapsAtTenantResponsibility(t *testing.T) {
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
	if decision.Status != "partial" || decision.AllocationAmountCents != 95000 || decision.Reason == "" {
		t.Fatalf("unexpected overpayment decision: %+v", decision)
	}
}

func TestDecideStrictRentMatchUsesOnlyPayerRelationsAndExplicitPeriod(t *testing.T) {
	payerID := "payer-123"
	parsedPeriod := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	tenants := []tenant{{ID: 7, Name: "Aoife Murphy"}}
	payers := []tenantPayer{{
		TenantID:            7,
		PayerID:             &payerID,
		PayerNameOriginal:   "Aoife Murphy",
		PayerNameNormalized: "aoife murphy",
	}}
	obligations := []rentObligation{{
		ID:                  11,
		TenantID:            7,
		PeriodMonth:         parsedPeriod,
		ExpectedAmountCents: 95000,
		Currency:            "EUR",
	}}
	decision := decideStrictRentMatch(paymentTransactionInput{
		Direction:         "income",
		AmountCents:       95000,
		Currency:          "EUR",
		PayerID:           payerID,
		PayerName:         "Aoife Murphy",
		ParsedPeriodMonth: &parsedPeriod,
	}, payers, tenants, obligations)
	if decision.Status != "matched" || decision.ConfirmationSource != "auto_id" || decision.RentObligationID != 11 {
		t.Fatalf("strict decision=%+v want exact payer-id match", decision)
	}

	withoutPeriod := paymentTransactionInput{
		Direction:   "income",
		AmountCents: 95000,
		Currency:    "EUR",
		PayerID:     payerID,
		PayerName:   "Aoife Murphy",
	}
	decision = decideStrictRentMatch(withoutPeriod, payers, tenants, obligations)
	if decision.Status != "candidate" || decision.TenantID != 7 || decision.Reason != "explicit rent period is missing" {
		t.Fatalf("missing-period decision=%+v want candidate without auto allocation", decision)
	}
}

func TestDecideStrictRentMatchRejectsSharedPayerAndNonEUR(t *testing.T) {
	sharedID := "shared-payer"
	payers := []tenantPayer{
		{TenantID: 7, PayerID: &sharedID, PayerNameNormalized: "mike"},
		{TenantID: 8, PayerID: &sharedID, PayerNameNormalized: "mike"},
	}
	tenants := []tenant{{ID: 7}, {ID: 8}}
	period := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	tx := paymentTransactionInput{Direction: "income", AmountCents: 95000, Currency: "EUR", PayerID: sharedID, ParsedPeriodMonth: &period}
	decision := decideStrictRentMatch(tx, payers, tenants, nil)
	if decision.Status != "needs_review" || decision.Reason != "multiple tenants for payer id" {
		t.Fatalf("shared payer decision=%+v want needs_review", decision)
	}

	uniqueID := "unique-payer"
	decision = decideStrictRentMatch(paymentTransactionInput{Direction: "income", AmountCents: 95000, Currency: "GBP", PayerID: uniqueID, ParsedPeriodMonth: &period}, []tenantPayer{{TenantID: 7, PayerID: &uniqueID, PayerNameNormalized: "aoife"}}, tenants[:1], []rentObligation{{ID: 12, TenantID: 7, PeriodMonth: period, ExpectedAmountCents: 95000, Currency: "EUR"}})
	if decision.Status != "needs_review" || decision.Reason != "currency mismatch" {
		t.Fatalf("non-EUR decision=%+v want currency review", decision)
	}
}

func TestSelectObligationRequiresUserChoiceWhenReferencedMonthIsPaid(t *testing.T) {
	tenantID := uint64(7)
	tx := paymentTransactionInput{
		Direction:       "income",
		AmountCents:     100000,
		Description:     "REMAINS 50 *MOBI RENT 4/26",
		TransactionTime: ptrTime(time.Date(2026, 4, 30, 23, 0, 0, 0, time.UTC)),
	}
	obligations := []rentObligation{
		{ID: 31, TenantID: tenantID, PeriodMonth: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), ExpectedAmountCents: 105000},
		{ID: 41, TenantID: tenantID, PeriodMonth: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC), ExpectedAmountCents: 105000, PaidAmountCents: 105000},
		{ID: 51, TenantID: tenantID, PeriodMonth: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), ExpectedAmountCents: 105000},
	}

	got, ok := selectObligationForTransaction(tx, tenantID, obligations)
	if ok {
		t.Fatalf("unexpected automatic allocation to obligation %d (%s)", got.ID, got.PeriodMonth.Format("2006-01"))
	}
}

func ptrTime(t time.Time) *time.Time {
	return &t
}

func ptrUint64(value uint64) *uint64 {
	return &value
}

func TestSummarizeBankSyncAccountsMarksPartialWhenOneAccountFails(t *testing.T) {
	status := summarizeBankSyncAccounts([]bankSyncAccountResult{
		{Status: bankSyncAccountSucceeded},
		{Status: bankSyncAccountFailed, ErrorMessage: "transactions unavailable"},
	})
	if status != bankSyncStatusPartial {
		t.Fatalf("status=%q want %q", status, bankSyncStatusPartial)
	}
}

func TestInitialSyncWindowDefaultsToOneYear(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	from, to, err := syncRequestWindow(bankSyncModeInitialYear, "", now)
	if err != nil {
		t.Fatal(err)
	}
	if from != "2025-09-16" || !to.Equal(now) {
		t.Fatalf("window=(%q,%s) want (2025-09-16,%s)", from, to, now)
	}
}

func TestFormatBankSyncCoverageShowsActualAccountCoverage(t *testing.T) {
	from := time.Date(2025, 9, 16, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	coveredFrom := time.Date(2025, 10, 1, 0, 0, 0, 0, time.UTC)
	coveredTo := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	run := bankSyncRun{ID: 9, Mode: bankSyncModeInitialYear, Status: bankSyncStatusPartial, RequestedFrom: from, RequestedTo: to}
	coverage := formatBankSyncCoverage(run, []bankSyncRunAccount{
		{Status: bankSyncAccountSucceeded, CoveredFrom: &coveredFrom, CoveredTo: &coveredTo},
		{Status: bankSyncAccountFailed},
	})
	for _, expected := range []string{"部分成功", "初次同步一年", "覆盖 2025-10-01 至 2026-09-15", "账户 1/2"} {
		if !strings.Contains(coverage, expected) {
			t.Fatalf("coverage=%q missing %q", coverage, expected)
		}
	}
}

func TestRefreshSyncWindowUsesProviderSafeLookback(t *testing.T) {
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	from, to, err := syncRequestWindow(bankSyncModeRefresh90d, "2026-01-01", now)
	if err != nil {
		t.Fatal(err)
	}
	if from != "2026-06-11" || !to.Equal(now) {
		t.Fatalf("window=(%q,%s) want (2026-06-11,%s)", from, to, now)
	}
}

func TestPaymentTransactionFromInputPreservesParsedPeriodFacts(t *testing.T) {
	parsed := time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)
	input := paymentTransactionInput{
		Direction:          "income",
		AmountCents:        200000,
		Currency:           "EUR",
		ParsedPeriodMonth:  &parsed,
		ParsedPeriodSource: "description",
		ParsedPeriodNote:   "JULY26",
	}

	row := paymentTransactionFromInput(7, input)
	if row.ParsedPeriodMonth == nil || !row.ParsedPeriodMonth.Equal(parsed) {
		t.Fatalf("parsed period=%v want %s", row.ParsedPeriodMonth, parsed.Format(dateLayout))
	}
	if row.ParsedPeriodSource != "description" || row.ParsedPeriodNote != "JULY26" {
		t.Fatalf("parsed facts not preserved: source=%q note=%q", row.ParsedPeriodSource, row.ParsedPeriodNote)
	}
}

func TestValidateTransactionAllocationDraftsAcceptsMixedUsesWithinSourceBudget(t *testing.T) {
	source := paymentTransaction{UserID: 7, Direction: "income", AmountCents: 200000, Currency: "EUR"}
	tenantID := uint64(11)
	obligation := rentObligation{
		ID:                  21,
		UserID:              7,
		TenantID:            tenantID,
		ExpectedAmountCents: 100000,
		Currency:            "EUR",
	}
	drafts := []transactionAllocationDraft{
		{TenantID: tenantID, RentObligationID: obligation.ID, AmountCents: 100000, Kind: allocationKindRent},
		{TenantID: tenantID, AmountCents: 60000, Kind: allocationKindDeposit},
		{TenantID: tenantID, AmountCents: 40000, Kind: allocationKindOther, Note: "cleaning fee"},
	}

	if err := validateTransactionAllocationDrafts(source, nil, drafts, map[uint64]rentObligation{obligation.ID: obligation}); err != nil {
		t.Fatalf("mixed allocation drafts rejected: %v", err)
	}
}

func TestValidateTransactionAllocationDraftsRejectsOverBudgetAsOneBatch(t *testing.T) {
	source := paymentTransaction{UserID: 7, Direction: "income", AmountCents: 120000, Currency: "EUR"}
	tenantID := uint64(11)
	drafts := []transactionAllocationDraft{
		{TenantID: tenantID, AmountCents: 100000, Kind: allocationKindDeposit},
		{TenantID: tenantID, AmountCents: 30000, Kind: allocationKindOther, Note: "fee"},
	}

	if err := validateTransactionAllocationDrafts(source, nil, drafts, nil); err == nil {
		t.Fatal("over-budget allocation batch should be rejected")
	}
}

func TestSummarizeTransactionAllocationsReportsPartialRemainderAndKinds(t *testing.T) {
	source := paymentTransaction{AmountCents: 120000}
	allocations := []paymentAllocation{
		{AmountCents: 100000, AllocationKind: allocationKindRent, Status: allocationStatusConfirmed},
	}

	summary := summarizeTransactionAllocations(source, allocations)
	if summary.AllocatedCents != 100000 || summary.RemainingCents != 20000 {
		t.Fatalf("summary amounts=(%d,%d) want (100000,20000)", summary.AllocatedCents, summary.RemainingCents)
	}
	if summary.Status != "partial" || summary.KindCents[allocationKindRent] != 100000 {
		t.Fatalf("summary=%+v want partial rent projection", summary)
	}
}

func TestProjectTransactionMatchPreservesAllocationStateAndActionReason(t *testing.T) {
	tenantID := uint64(17)
	source := paymentTransaction{AmountCents: 120000}
	allocations := []paymentAllocation{{
		TenantID:       &tenantID,
		AmountCents:    80000,
		AllocationKind: allocationKindRent,
		Status:         allocationStatusConfirmed,
	}}

	projected := projectTransactionMatch(source, allocations, "", "")
	if projected.Status != "partial" || projected.MatchedTenantID == nil || *projected.MatchedTenantID != tenantID {
		t.Fatalf("allocation projection=%+v want partial tenant %d", projected, tenantID)
	}

	ignored := projectTransactionMatch(source, nil, transactionActionIgnore, "duplicate payment")
	if ignored.Status != "ignored" || ignored.MatchedTenantID != nil || ignored.Reason != "duplicate payment" {
		t.Fatalf("ignore projection=%+v want ignored without tenant", ignored)
	}

	restored := projectTransactionMatch(source, nil, transactionActionRestore, "review again")
	if restored.Status != "unmatched" || restored.MatchedTenantID != nil || restored.Reason != "review again" {
		t.Fatalf("restore projection=%+v want unmatched", restored)
	}

	revoked := projectTransactionMatch(source, []paymentAllocation{{
		TenantID:       &tenantID,
		AmountCents:    120000,
		AllocationKind: allocationKindRent,
		Status:         allocationStatusVoided,
	}}, transactionActionRevokeAllocations, "wrong month")
	if revoked.Status != "unmatched" || revoked.MatchedTenantID != nil || revoked.Reason != "wrong month" {
		t.Fatalf("revoke projection=%+v want unmatched", revoked)
	}
}

func TestNormalizeTransactionActionReasonRejectsBlankAndTruncates(t *testing.T) {
	if _, err := normalizeTransactionActionReason("  "); err == nil {
		t.Fatal("blank action reason should be rejected")
	}
	longReason := strings.Repeat("字", 600)
	reason, err := normalizeTransactionActionReason(longReason)
	if err != nil {
		t.Fatal(err)
	}
	if len([]rune(reason)) != 512 {
		t.Fatalf("reason rune length=%d want 512", len([]rune(reason)))
	}
}

func ptrString(value string) *string {
	return &value
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
