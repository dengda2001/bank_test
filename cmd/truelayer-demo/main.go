package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultScopes       = "info accounts balance transactions offline_access"
	stateTTL            = 15 * time.Minute
	sessionTTL          = 12 * time.Hour
	refreshLookbackDays = 90
	dateLayout          = "2006-01-02"
)

type config struct {
	Address       string
	ClientID      string
	ClientSecret  string
	RedirectURI   string
	Environment   string
	AuthBaseURL   string
	AuthURL       string
	APIBaseURL    string
	Scopes        []string
	Providers     string
	ProviderID    string
	From          string
	LogFile       string
	TokenFile     string
	AdminUsername string
	AdminPassword string
	SessionSecret string
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresIn    int    `json:"expires_in,omitempty"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope,omitempty"`
}

type storedToken struct {
	RefreshToken string `json:"refresh_token"`
	SavedAt      string `json:"saved_at"`
}

type accountList struct {
	Results []account `json:"results"`
}

type account struct {
	AccountID   string          `json:"account_id"`
	AccountType string          `json:"account_type"`
	DisplayName string          `json:"display_name"`
	Currency    string          `json:"currency"`
	Provider    json.RawMessage `json:"provider,omitempty"`
}

type demoAccount struct {
	Account      account         `json:"account"`
	Balance      json.RawMessage `json:"balance,omitempty"`
	Transactions json.RawMessage `json:"transactions,omitempty"`
	Errors       []string        `json:"errors,omitempty"`
}

type demoResult struct {
	Environment string        `json:"environment"`
	FetchedAt   string        `json:"fetched_at"`
	Accounts    []demoAccount `json:"accounts"`
}

type incomeTransaction struct {
	TransactionID string
	SourceID      string
	PayerID       string
	PayerName     string
	PayerNameKind string
	Timestamp     string
	DateDisplay   string
	Amount        float64
	AmountDisplay string
	Currency      string
	AccountID     string
	AccountName   string
	Description   string
	Reference     string
	Status        string
	StatusLabel   string
}

type billingPageData struct {
	Username          string
	Environment       string
	Connected         bool
	NeedsReconnect    bool
	LastSync          string
	Message           string
	Error             string
	Rows              []incomeTransaction
	IncomeTotal       string
	IncomeCount       int
	UnknownPayerCount int
	TokenFile         string
}

type app struct {
	cfg        config
	httpClient *http.Client
}

// issuedStates tracks states this server has handed to TrueLayer so the
// /callback handler can verify a state came from a login it started — without
// relying on the login cookie. That cookie is bound to whatever browser opened
// /login; on iOS the OAuth return can be handed off to a different browser
// (e.g. WeChat in-app browser -> Safari), which has no copy of the cookie but
// does carry the state in the URL. See TestLoginThenCallbackVerifiesAcrossBrowser.
var issuedStates = newStateStore()

type stateStore struct {
	mu    sync.Mutex
	items map[string]time.Time // issued state -> expiry
}

func newStateStore() *stateStore {
	return &stateStore{items: make(map[string]time.Time)}
}

func (s *stateStore) record(state string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for k, exp := range s.items { // opportunistic purge of expired states
		if !now.Before(exp) {
			delete(s.items, k)
		}
	}
	s.items[state] = now.Add(stateTTL)
}

// consume returns true only if state was issued recently and unused; the state
// is removed whether valid or expired so it can never be replayed.
func (s *stateStore) consume(state string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	exp, ok := s.items[state]
	if !ok {
		return false
	}
	delete(s.items, state)
	return time.Now().Before(exp)
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatal(err)
	}

	a := &app{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", a.handleIndex)
	mux.HandleFunc("/login-local", a.handleLocalLogin)
	mux.HandleFunc("/logout", a.handleLogout)
	mux.HandleFunc("/billing", a.handleBilling)
	mux.HandleFunc("/login", a.handleLogin)
	mux.HandleFunc("/callback", a.handleCallback)
	mux.HandleFunc("/refresh", a.handleRefresh)

	log.Printf("TrueLayer demo listening on http://localhost%s", cfg.Address)
	log.Printf("Redirect URI must be registered in TrueLayer Console: %s", cfg.RedirectURI)
	log.Fatal(http.ListenAndServe(cfg.Address, mux))
}

func loadConfig() (config, error) {
	env := strings.ToLower(strings.TrimSpace(getenv("TL_ENV", "sandbox")))
	cfg := config{
		Address:       getenv("TL_ADDR", ":8080"),
		ClientID:      strings.TrimSpace(os.Getenv("TL_CLIENT_ID")),
		ClientSecret:  strings.TrimSpace(os.Getenv("TL_CLIENT_SECRET")),
		RedirectURI:   strings.TrimSpace(getenv("TL_REDIRECT_URI", "http://localhost:8080/callback")),
		Environment:   env,
		AuthURL:       strings.TrimSpace(os.Getenv("TL_AUTH_URL")),
		Scopes:        splitWords(getenv("TL_SCOPES", defaultScopes)),
		Providers:     strings.TrimSpace(os.Getenv("TL_PROVIDERS")),
		ProviderID:    strings.TrimSpace(os.Getenv("TL_PROVIDER_ID")),
		From:          strings.TrimSpace(os.Getenv("TL_FROM")),
		LogFile:       strings.TrimSpace(getenv("TL_LOG_FILE", "bank-data.jsonl")),
		TokenFile:     strings.TrimSpace(getenv("TL_TOKEN_FILE", "truelayer-token.json")),
		AdminUsername: strings.TrimSpace(getenv("APP_ADMIN_USERNAME", "ddrzh")),
		AdminPassword: getenv("APP_ADMIN_PASSWORD", "ddrzh512"),
		SessionSecret: strings.TrimSpace(os.Getenv("APP_SESSION_SECRET")),
	}
	if cfg.SessionSecret == "" {
		secret, err := randomState()
		if err != nil {
			return config{}, fmt.Errorf("generate session secret: %w", err)
		}
		cfg.SessionSecret = secret
	}

	switch env {
	case "live":
		cfg.AuthBaseURL = "https://auth.truelayer.com"
		cfg.APIBaseURL = "https://api.truelayer.com"
	case "sandbox":
		cfg.AuthBaseURL = "https://auth.truelayer-sandbox.com"
		cfg.APIBaseURL = "https://api.truelayer-sandbox.com"
	default:
		return config{}, fmt.Errorf("TL_ENV must be sandbox or live, got %q", env)
	}

	if v := strings.TrimSpace(os.Getenv("TL_AUTH_BASE_URL")); v != "" {
		cfg.AuthBaseURL = strings.TrimRight(v, "/")
	}
	if v := strings.TrimSpace(os.Getenv("TL_API_BASE_URL")); v != "" {
		cfg.APIBaseURL = strings.TrimRight(v, "/")
	}
	if cfg.ClientID == "" {
		return config{}, errors.New("TL_CLIENT_ID is required")
	}
	if cfg.ClientSecret == "" {
		return config{}, errors.New("TL_CLIENT_SECRET is required")
	}
	if _, err := url.ParseRequestURI(cfg.RedirectURI); err != nil {
		return config{}, fmt.Errorf("TL_REDIRECT_URI is not a valid URI: %w", err)
	}
	if len(cfg.Scopes) == 0 {
		return config{}, errors.New("TL_SCOPES must contain at least one scope")
	}
	return cfg, nil
}

func splitWords(s string) []string {
	return strings.Fields(strings.ReplaceAll(s, ",", " "))
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func (a *app) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if a.isAuthenticated(r) {
		http.Redirect(w, r, "/billing", http.StatusFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	data := struct {
		Error    string
		Username string
	}{
		Error:    r.URL.Query().Get("error"),
		Username: a.cfg.AdminUsername,
	}
	if err := loginTemplate.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (a *app) handleLocalLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	if r.Form.Get("username") != a.cfg.AdminUsername || r.Form.Get("password") != a.cfg.AdminPassword {
		http.Redirect(w, r, "/?error=invalid_login", http.StatusFound)
		return
	}
	http.SetCookie(w, sessionCookie(a.cfg, time.Now().Add(sessionTTL)))
	http.Redirect(w, r, "/billing", http.StatusFound)
}

func (a *app) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	http.SetCookie(w, expiredSessionCookie())
	http.Redirect(w, r, "/", http.StatusFound)
}

func (a *app) handleBilling(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	result, _ := a.loadLatestDemoResult()
	rows := normalizeIncomeTransactions(result)
	data := billingPageData{
		Username:          a.cfg.AdminUsername,
		Environment:       a.cfg.Environment,
		Connected:         a.hasStoredToken(),
		NeedsReconnect:    r.URL.Query().Get("reconnect") == "1",
		LastSync:          result.FetchedAt,
		Message:           r.URL.Query().Get("message"),
		Error:             r.URL.Query().Get("error"),
		Rows:              rows,
		IncomeTotal:       formatMoney(sumIncome(rows), "EUR", 2),
		IncomeCount:       len(rows),
		UnknownPayerCount: countUnknownPayers(rows),
		TokenFile:         a.cfg.TokenFile,
	}
	if data.LastSync == "" {
		data.LastSync = "No sync yet"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := billingTemplate.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (a *app) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	state, err := randomState()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	issuedStates.record(state)
	http.Redirect(w, r, a.authURL(state), http.StatusFound)
}

func (a *app) handleCallback(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	ctx := r.Context()
	if errText := r.URL.Query().Get("error"); errText != "" {
		http.Error(w, "TrueLayer returned error: "+errText, http.StatusBadRequest)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}
	if err := verifyState(r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	token, err := a.exchangeCode(ctx, code)
	if err != nil {
		http.Error(w, "token exchange failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	if token.RefreshToken != "" {
		if err := a.saveStoredToken(token); err != nil {
			http.Error(w, "token save failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	result, err := a.fetchDemoResult(ctx, token.AccessToken)
	if err != nil {
		http.Error(w, "data fetch failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	if err := a.appendDemoResultLog(result); err != nil {
		http.Error(w, "log write failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/billing?message=bank_connected", http.StatusFound)
}

func (a *app) handleRefresh(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stored, err := a.loadStoredToken()
	if err != nil {
		http.Redirect(w, r, "/billing?reconnect=1&error=no_saved_login", http.StatusFound)
		return
	}

	token, err := a.refreshAccessToken(r.Context(), stored.RefreshToken)
	if err != nil {
		http.Redirect(w, r, "/billing?reconnect=1&error=refresh_failed", http.StatusFound)
		return
	}
	if token.RefreshToken != "" && token.RefreshToken != stored.RefreshToken {
		if err := a.saveStoredToken(token); err != nil {
			http.Error(w, "token save failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	from := refreshTransactionFrom(a.cfg.From, time.Now())
	result, err := a.fetchDemoResultWithOptions(r.Context(), token.AccessToken, from, true)
	if err != nil {
		http.Redirect(w, r, "/billing?error=data_fetch_failed", http.StatusFound)
		return
	}
	if err := a.appendDemoResultLog(result); err != nil {
		http.Error(w, "log write failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/billing?message=refreshed", http.StatusFound)
}

func (a *app) requireAuth(w http.ResponseWriter, r *http.Request) bool {
	if a.isAuthenticated(r) {
		return true
	}
	http.Redirect(w, r, "/", http.StatusFound)
	return false
}

func (a *app) isAuthenticated(r *http.Request) bool {
	c, err := r.Cookie("rentops_session")
	if err != nil {
		return false
	}
	parts := strings.Split(c.Value, "|")
	if len(parts) != 3 {
		return false
	}
	if parts[0] != a.cfg.AdminUsername {
		return false
	}
	expiresUnix, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || time.Now().After(time.Unix(expiresUnix, 0)) {
		return false
	}
	want := sessionSignature(a.cfg, parts[0], parts[1])
	return hmac.Equal([]byte(parts[2]), []byte(want))
}

func sessionCookie(cfg config, expires time.Time) *http.Cookie {
	expiresUnix := strconv.FormatInt(expires.Unix(), 10)
	value := cfg.AdminUsername + "|" + expiresUnix + "|" + sessionSignature(cfg, cfg.AdminUsername, expiresUnix)
	return &http.Cookie{
		Name:     "rentops_session",
		Value:    value,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
}

func expiredSessionCookie() *http.Cookie {
	return &http.Cookie{
		Name:     "rentops_session",
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
}

func sessionSignature(cfg config, username, expiresUnix string) string {
	mac := hmac.New(sha256.New, []byte(cfg.SessionSecret))
	io.WriteString(mac, username)
	io.WriteString(mac, "|")
	io.WriteString(mac, expiresUnix)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (a *app) hasStoredToken() bool {
	_, err := a.loadStoredToken()
	return err == nil
}

func (a *app) authURL(state string) string {
	raw := strings.TrimRight(a.cfg.AuthBaseURL, "/") + "/"
	if a.cfg.AuthURL != "" {
		raw = a.cfg.AuthURL
	}
	u, err := url.Parse(raw)
	if err != nil {
		panic(err)
	}
	q := u.Query()
	if a.cfg.AuthURL == "" {
		u.Path = "/"
		q.Set("response_type", "code")
		q.Set("client_id", a.cfg.ClientID)
		q.Set("redirect_uri", a.cfg.RedirectURI)
		q.Set("scope", strings.Join(a.cfg.Scopes, " "))
	}
	q.Set("state", state)
	if a.cfg.AuthURL == "" && a.cfg.Providers != "" {
		q.Set("providers", a.cfg.Providers)
	}
	if a.cfg.AuthURL == "" && a.cfg.ProviderID != "" {
		q.Set("provider_id", a.cfg.ProviderID)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func randomState() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

func verifyState(r *http.Request) error {
	state := r.URL.Query().Get("state")
	if state == "" {
		return errors.New("missing state")
	}
	if !issuedStates.consume(state) {
		return errors.New("missing or expired state")
	}
	return nil
}

func (a *app) exchangeCode(ctx context.Context, code string) (tokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", a.cfg.ClientID)
	form.Set("client_secret", a.cfg.ClientSecret)
	form.Set("redirect_uri", a.cfg.RedirectURI)
	form.Set("code", code)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(a.cfg.AuthBaseURL, "/")+"/connect/token", strings.NewReader(form.Encode()))
	if err != nil {
		return tokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var token tokenResponse
	if err := a.doJSON(req, &token); err != nil {
		return tokenResponse{}, err
	}
	if token.AccessToken == "" {
		return tokenResponse{}, errors.New("empty access_token in token response")
	}
	return token, nil
}

func (a *app) refreshAccessToken(ctx context.Context, refreshToken string) (tokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("client_id", a.cfg.ClientID)
	form.Set("client_secret", a.cfg.ClientSecret)
	form.Set("refresh_token", refreshToken)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(a.cfg.AuthBaseURL, "/")+"/connect/token", strings.NewReader(form.Encode()))
	if err != nil {
		return tokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var token tokenResponse
	if err := a.doJSON(req, &token); err != nil {
		return tokenResponse{}, err
	}
	if token.AccessToken == "" {
		return tokenResponse{}, errors.New("empty access_token in token response")
	}
	return token, nil
}

func (a *app) fetchDemoResult(ctx context.Context, accessToken string) (demoResult, error) {
	return a.fetchDemoResultWithOptions(ctx, accessToken, a.cfg.From, false)
}

func (a *app) fetchDemoResultWithOptions(ctx context.Context, accessToken, from string, failOnTransactionError bool) (demoResult, error) {
	var accounts accountList
	if err := a.getJSON(ctx, accessToken, "/data/v1/accounts", nil, &accounts); err != nil {
		return demoResult{}, err
	}

	result := demoResult{
		Environment: a.cfg.Environment,
		FetchedAt:   time.Now().UTC().Format(time.RFC3339),
		Accounts:    make([]demoAccount, 0, len(accounts.Results)),
	}
	for _, acct := range accounts.Results {
		item := demoAccount{Account: acct}

		var balance json.RawMessage
		if err := a.getJSON(ctx, accessToken, "/data/v1/accounts/"+url.PathEscape(acct.AccountID)+"/balance", nil, &balance); err != nil {
			item.Errors = append(item.Errors, "balance: "+err.Error())
		} else {
			item.Balance = balance
		}

		q := txQuery(from, time.Now())
		var txs json.RawMessage
		if err := a.getJSON(ctx, accessToken, "/data/v1/accounts/"+url.PathEscape(acct.AccountID)+"/transactions", q, &txs); err != nil {
			item.Errors = append(item.Errors, "transactions: "+err.Error())
			if failOnTransactionError {
				return demoResult{}, fmt.Errorf("transactions fetch failed")
			}
		} else {
			item.Transactions = txs
		}

		result.Accounts = append(result.Accounts, item)
	}
	return result, nil
}

func refreshTransactionFrom(configuredFrom string, now time.Time) string {
	today := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
	cutoff := today.AddDate(0, 0, -refreshLookbackDays)
	d, err := time.Parse(dateLayout, configuredFrom)
	if configuredFrom == "" || err != nil || d.After(today) || d.Before(cutoff) {
		return cutoff.Format(dateLayout)
	}
	return configuredFrom
}

// txQuery builds the from/to query for the transactions request. The `to`
// bound is always the current UTC time. A configured `from` in the future is
// meaningless and dropped; unparseable values are passed through unchanged.
func txQuery(from string, now time.Time) url.Values {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	q := url.Values{}
	if from != "" {
		if d, err := time.Parse(dateLayout, from); err != nil || !d.After(today) {
			q.Set("from", from)
		}
	}
	q.Set("to", now.UTC().Format(time.RFC3339))
	return q
}

func (a *app) saveStoredToken(token tokenResponse) error {
	if a.cfg.TokenFile == "" || token.RefreshToken == "" {
		return nil
	}
	dir := filepath.Dir(a.cfg.TokenFile)
	if dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}

	stored := storedToken{
		RefreshToken: token.RefreshToken,
		SavedAt:      time.Now().UTC().Format(time.RFC3339),
	}
	body, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	if err := os.WriteFile(a.cfg.TokenFile, body, 0o600); err != nil {
		return err
	}
	return os.Chmod(a.cfg.TokenFile, 0o600)
}

func (a *app) loadStoredToken() (storedToken, error) {
	if a.cfg.TokenFile == "" {
		return storedToken{}, errors.New("TL_TOKEN_FILE is empty")
	}
	body, err := os.ReadFile(a.cfg.TokenFile)
	if err != nil {
		return storedToken{}, err
	}
	var stored storedToken
	if err := json.Unmarshal(body, &stored); err != nil {
		return storedToken{}, err
	}
	if strings.TrimSpace(stored.RefreshToken) == "" {
		return storedToken{}, errors.New("stored refresh_token is empty")
	}
	return stored, nil
}

func (a *app) loadLatestDemoResult() (demoResult, error) {
	if a.cfg.LogFile == "" {
		return demoResult{}, errors.New("TL_LOG_FILE is empty")
	}
	body, err := os.ReadFile(a.cfg.LogFile)
	if err != nil {
		return demoResult{}, err
	}
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		var result demoResult
		if err := json.Unmarshal([]byte(line), &result); err != nil {
			return demoResult{}, err
		}
		return result, nil
	}
	return demoResult{}, errors.New("no logged bank data")
}

type rawTransaction struct {
	TransactionID                   string         `json:"transaction_id"`
	NormalisedProviderTransactionID string         `json:"normalised_provider_transaction_id"`
	ProviderTransactionID           string         `json:"provider_transaction_id"`
	Timestamp                       string         `json:"timestamp"`
	Description                     string         `json:"description"`
	Amount                          float64        `json:"amount"`
	Currency                        string         `json:"currency"`
	TransactionType                 string         `json:"transaction_type"`
	TransactionCategory             string         `json:"transaction_category"`
	MerchantName                    string         `json:"merchant_name"`
	PayerID                         string         `json:"payer_id"`
	PayerName                       string         `json:"payer_name"`
	RemitterID                      string         `json:"remitter_id"`
	RemitterName                    string         `json:"remitter_name"`
	CounterpartyID                  string         `json:"counterparty_id"`
	CounterpartyName                string         `json:"counterparty_name"`
	Reference                       string         `json:"reference"`
	Meta                            map[string]any `json:"meta"`
}

type rawTransactionList struct {
	Results []rawTransaction `json:"results"`
}

func normalizeIncomeTransactions(result demoResult) []incomeTransaction {
	var rows []incomeTransaction
	for _, acct := range result.Accounts {
		var txs rawTransactionList
		if len(acct.Transactions) == 0 {
			continue
		}
		if err := json.Unmarshal(acct.Transactions, &txs); err != nil {
			continue
		}
		for _, tx := range txs.Results {
			if !isIncome(tx) {
				continue
			}
			row := incomeTransaction{
				TransactionID: tx.TransactionID,
				SourceID:      firstNonEmpty(tx.NormalisedProviderTransactionID, tx.ProviderTransactionID, metaString(tx.Meta, "normalised_provider_transaction_id"), metaString(tx.Meta, "provider_transaction_id"), metaString(tx.Meta, "bank_transaction_id")),
				PayerID:       firstNonEmpty(tx.PayerID, tx.RemitterID, tx.CounterpartyID, metaString(tx.Meta, "payer_id"), metaString(tx.Meta, "remitter_id"), metaString(tx.Meta, "counterparty_id")),
				Timestamp:     tx.Timestamp,
				DateDisplay:   formatTimestamp(tx.Timestamp),
				Amount:        tx.Amount,
				Currency:      firstNonEmpty(tx.Currency, acct.Account.Currency),
				AccountID:     acct.Account.AccountID,
				AccountName:   firstNonEmpty(acct.Account.DisplayName, acct.Account.AccountID),
				Description:   tx.Description,
				Reference:     firstNonEmpty(tx.Reference, metaString(tx.Meta, "payment_reference"), metaString(tx.Meta, "reference"), metaString(tx.Meta, "provider_reference"), metaString(tx.Meta, "remittance_information"), metaString(tx.Meta, "remittanceInformation")),
				Status:        "needs_review",
				StatusLabel:   "Needs review",
			}
			row.PayerName, row.PayerNameKind = payerName(tx)
			if row.TransactionID == "" {
				row.TransactionID = "unknown"
			}
			if row.SourceID == "" {
				row.SourceID = "unknown"
			}
			if row.PayerID == "" {
				row.PayerID = "unknown"
			}
			if row.PayerName == "" {
				row.PayerName = "Unknown payer"
				row.PayerNameKind = "unknown"
			}
			if row.Reference == "" {
				row.Reference = "unknown"
			}
			row.AmountDisplay = formatMoney(row.Amount, row.Currency, 2)
			if row.PayerNameKind == "confirmed" {
				row.Status = "confirmed_payer"
				row.StatusLabel = "Confirmed sender"
			} else if row.PayerNameKind == "inferred" {
				row.Status = "inferred_payer"
				row.StatusLabel = "Inferred sender"
			} else {
				row.Status = "unknown_payer"
				row.StatusLabel = "Unknown sender"
			}
			rows = append(rows, row)
		}
	}
	return rows
}

func isIncome(tx rawTransaction) bool {
	t := strings.ToUpper(strings.TrimSpace(tx.TransactionType))
	if t == "CREDIT" {
		return true
	}
	if t == "DEBIT" {
		return false
	}
	return tx.Amount > 0
}

func payerName(tx rawTransaction) (string, string) {
	if name := firstNonEmpty(tx.PayerName, tx.RemitterName, tx.CounterpartyName, metaString(tx.Meta, "payer_name"), metaString(tx.Meta, "remitter_name"), metaString(tx.Meta, "counterparty_name"), metaString(tx.Meta, "counter_party_preferred_name"), metaString(tx.Meta, "counterPartyPreferredName")); name != "" {
		return name, "confirmed"
	}
	if name := firstNonEmpty(tx.MerchantName, tx.Description); name != "" {
		return name, "inferred"
	}
	return "", "unknown"
}

func metaString(meta map[string]any, key string) string {
	if len(meta) == 0 {
		return ""
	}
	v, ok := meta[key]
	if !ok {
		return ""
	}
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case float64:
		if math.Trunc(x) == x {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	default:
		return ""
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

func formatTimestamp(value string) string {
	if value == "" {
		return "Unknown"
	}
	layouts := []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"}
	for _, layout := range layouts {
		if ts, err := time.Parse(layout, value); err == nil {
			return ts.Format("02 Jan 2006 15:04")
		}
	}
	return value
}

func formatMoney(amount float64, currency string, decimals int) string {
	if currency == "" {
		currency = "EUR"
	}
	return fmt.Sprintf("%s %.*f", currency, decimals, amount)
}

func sumIncome(rows []incomeTransaction) float64 {
	var total float64
	for _, row := range rows {
		total += row.Amount
	}
	return total
}

func countUnknownPayers(rows []incomeTransaction) int {
	var count int
	for _, row := range rows {
		if row.PayerNameKind == "unknown" {
			count++
		}
	}
	return count
}

func (a *app) appendDemoResultLog(result demoResult) error {
	if a.cfg.LogFile == "" {
		return nil
	}
	dir := filepath.Dir(a.cfg.LogFile)
	if dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	f, err := os.OpenFile(a.cfg.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	return enc.Encode(result)
}

func (a *app) getJSON(ctx context.Context, accessToken, path string, q url.Values, out any) error {
	u, err := url.Parse(strings.TrimRight(a.cfg.APIBaseURL, "/") + path)
	if err != nil {
		return err
	}
	if len(q) > 0 {
		u.RawQuery = q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	return a.doJSON(req, out)
}

func (a *app) doJSON(req *http.Request, out any) error {
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("%s: %s", resp.Status, compactBody(body))
	}
	if len(body) == 0 {
		return nil
	}
	return json.NewDecoder(bytes.NewReader(body)).Decode(out)
}

func compactBody(body []byte) string {
	s := strings.Join(strings.Fields(string(body)), " ")
	if len(s) > 800 {
		return s[:800] + "..."
	}
	return s
}

var loginTemplate = template.Must(template.New("login").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>RentOps Login</title>
  <style>
    * { box-sizing: border-box; }
    body {
      margin: 0;
      min-height: 100vh;
      display: grid;
      place-items: center;
      background: #f7f8f5;
      color: #1d2522;
      font-family: "IBM Plex Sans", "Aptos", "Segoe UI", system-ui, sans-serif;
    }
    .shell {
      width: min(420px, calc(100vw - 32px));
      background: #fff;
      border: 1px solid #d9dfd9;
      border-radius: 8px;
      box-shadow: 0 18px 45px rgba(26, 39, 34, 0.10);
      padding: 28px;
    }
    .brand { display: flex; align-items: center; gap: 10px; margin-bottom: 28px; }
    .mark {
      width: 36px; height: 36px; display: grid; place-items: center;
      background: #17201d; color: #fff; font: 700 21px Georgia, serif;
    }
    h1 { margin: 0; font: 700 28px Georgia, serif; }
    p { margin: 8px 0 22px; color: #66736e; }
    label { display: block; margin: 14px 0 6px; font-size: 13px; font-weight: 700; }
    input {
      width: 100%; height: 42px; border: 1px solid #b9c4bd; border-radius: 6px;
      padding: 0 11px; font: inherit; background: #fbfcfa;
    }
    button {
      width: 100%; height: 42px; margin-top: 18px; border: 1px solid #0f665f;
      border-radius: 6px; background: #0f766e; color: #fff; font: inherit;
      font-weight: 700; cursor: pointer;
    }
    .error {
      border: 1px solid #e9aaa5; background: #fff5f3; color: #af3333;
      border-radius: 6px; padding: 10px 12px; font-size: 13px; margin-bottom: 16px;
    }
    .hint { margin-top: 16px; color: #66736e; font-size: 12px; }
  </style>
</head>
<body>
  <main class="shell">
    <div class="brand">
      <div class="mark">R</div>
      <div>
        <h1>RentOps</h1>
        <p>Billing workspace</p>
      </div>
    </div>
    {{if .Error}}<div class="error">Invalid username or password.</div>{{end}}
    <form method="post" action="/login-local">
      <label for="username">Username</label>
      <input id="username" name="username" autocomplete="username" value="{{.Username}}">
      <label for="password">Password</label>
      <input id="password" name="password" type="password" autocomplete="current-password">
      <button type="submit">Sign in</button>
    </form>
    <div class="hint">Demo login is configured with APP_ADMIN_USERNAME and APP_ADMIN_PASSWORD.</div>
  </main>
</body>
</html>
`))

var billingTemplate = template.Must(template.New("billing").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>RentOps Billing</title>
  <style>
    :root {
      --background-deep: #020203;
      --background-base: #050506;
      --background-elevated: #0a0a0c;
      --surface: rgba(255,255,255,0.05);
      --surface-strong: rgba(255,255,255,0.08);
      --foreground: #ededef;
      --foreground-muted: #8a8f98;
      --foreground-subtle: rgba(255,255,255,0.62);
      --accent: #5e6ad2;
      --accent-bright: #6872d9;
      --accent-glow: rgba(94,106,210,0.30);
      --positive: #7dd3a8;
      --warning: #e9b872;
      --danger: #ff8b86;
      --border: rgba(255,255,255,0.06);
      --border-hover: rgba(255,255,255,0.12);
      --shadow-card: 0 0 0 1px rgba(255,255,255,0.06), 0 20px 70px rgba(0,0,0,0.48), 0 0 70px rgba(94,106,210,0.08);
      --shadow-button: 0 0 0 1px rgba(94,106,210,0.50), 0 8px 26px rgba(94,106,210,0.28), inset 0 1px 0 rgba(255,255,255,0.22);
      --sans: "Inter", "Geist Sans", "Aptos", "Segoe UI", system-ui, sans-serif;
      --mono: "IBM Plex Mono", "SFMono-Regular", Consolas, monospace;
      --ease: cubic-bezier(0.16, 1, 0.3, 1);
    }
    * { box-sizing: border-box; }
    html { background: var(--background-deep); }
    body {
      margin: 0;
      min-height: 100vh;
      color: var(--foreground);
      font-family: var(--sans);
      background:
        radial-gradient(ellipse at top, #111225 0%, var(--background-base) 48%, var(--background-deep) 100%);
      overflow-x: hidden;
    }
    body::before {
      content: "";
      position: fixed;
      inset: 0;
      pointer-events: none;
      background-image:
        linear-gradient(rgba(255,255,255,0.022) 1px, transparent 1px),
        linear-gradient(90deg, rgba(255,255,255,0.022) 1px, transparent 1px);
      background-size: 64px 64px;
      mask-image: radial-gradient(circle at top, black, transparent 75%);
      opacity: 0.55;
    }
    body::after {
      content: "";
      position: fixed;
      inset: 0;
      pointer-events: none;
      opacity: 0.035;
      background-image: url("data:image/svg+xml,%3Csvg viewBox='0 0 160 160' xmlns='http://www.w3.org/2000/svg'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='.9' numOctaves='4' stitchTiles='stitch'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23n)' opacity='.45'/%3E%3C/svg%3E");
    }
    button, input { font: inherit; }
    a { color: inherit; }
    .ambient {
      position: fixed;
      inset: 0;
      z-index: 0;
      pointer-events: none;
      overflow: hidden;
    }
    .blob {
      position: absolute;
      border-radius: 999px;
      filter: blur(130px);
      opacity: 0.58;
      animation: float 9s ease-in-out infinite;
      transform: translateZ(0);
    }
    .blob.primary {
      width: 980px;
      height: 720px;
      left: 18%;
      top: -330px;
      background: rgba(94,106,210,0.26);
    }
    .blob.secondary {
      width: 560px;
      height: 760px;
      left: -230px;
      top: 220px;
      background: rgba(151,83,210,0.16);
      animation-delay: -2s;
    }
    .blob.tertiary {
      width: 620px;
      height: 620px;
      right: -240px;
      top: 180px;
      background: rgba(74,112,255,0.13);
      animation-delay: -4s;
    }
    @keyframes float {
      0%, 100% { transform: translateY(0) rotate(0deg); }
      50% { transform: translateY(-20px) rotate(1deg); }
    }
    .app {
      position: relative;
      z-index: 1;
      width: min(1480px, 100%);
      margin: 0 auto;
      padding: 24px;
    }
    .shell {
      min-height: calc(100vh - 48px);
      border: 1px solid var(--border);
      border-radius: 24px;
      background: linear-gradient(180deg, rgba(255,255,255,0.075), rgba(255,255,255,0.026));
      box-shadow: var(--shadow-card);
      backdrop-filter: blur(24px);
      overflow: hidden;
    }
    .topbar {
      display: flex;
      justify-content: space-between;
      gap: 18px;
      align-items: center;
      padding: 18px 22px;
      border-bottom: 1px solid var(--border);
      background: rgba(5,5,6,0.62);
    }
    .brand {
      display: flex;
      align-items: center;
      gap: 12px;
      min-width: 0;
    }
    .mark {
      width: 38px;
      height: 38px;
      border-radius: 12px;
      display: grid;
      place-items: center;
      color: white;
      font: 700 17px var(--mono);
      background: linear-gradient(145deg, rgba(104,114,217,0.95), rgba(94,106,210,0.55));
      box-shadow: var(--shadow-button);
    }
    .brand-title { font-weight: 650; letter-spacing: -0.01em; }
    .brand-meta {
      color: var(--foreground-muted);
      font: 500 11px var(--mono);
      text-transform: uppercase;
      letter-spacing: 0.12em;
      margin-top: 2px;
    }
    .actions {
      display: flex;
      gap: 10px;
      flex-wrap: wrap;
      justify-content: flex-end;
    }
    .btn {
      position: relative;
      min-height: 38px;
      border: 0;
      border-radius: 10px;
      padding: 0 14px;
      display: inline-flex;
      align-items: center;
      justify-content: center;
      gap: 8px;
      color: var(--foreground);
      background: rgba(255,255,255,0.055);
      box-shadow: inset 0 1px 0 rgba(255,255,255,0.10), 0 0 0 1px rgba(255,255,255,0.08);
      cursor: pointer;
      text-decoration: none;
      transition: transform 220ms var(--ease), background 220ms var(--ease), box-shadow 220ms var(--ease);
    }
    .btn:hover { transform: translateY(-2px); background: rgba(255,255,255,0.09); box-shadow: inset 0 1px 0 rgba(255,255,255,0.14), 0 0 0 1px rgba(255,255,255,0.13), 0 12px 34px rgba(0,0,0,0.24); }
    .btn:active { transform: scale(0.98); }
    .btn:focus-visible { outline: 2px solid rgba(104,114,217,0.9); outline-offset: 3px; }
    .btn.primary { background: var(--accent); color: white; box-shadow: var(--shadow-button); }
    .btn.primary:hover { background: var(--accent-bright); box-shadow: 0 0 0 1px rgba(104,114,217,0.65), 0 12px 34px rgba(94,106,210,0.36), inset 0 1px 0 rgba(255,255,255,0.24); }
    .btn.danger { color: #ffd8d6; background: rgba(255,139,134,0.08); }
    .content {
      display: grid;
      gap: 18px;
      padding: 24px;
    }
    .hero {
      display: grid;
      grid-template-columns: minmax(0, 1fr) 340px;
      gap: 18px;
      align-items: stretch;
    }
    .panel {
      position: relative;
      overflow: hidden;
      border: 1px solid var(--border);
      border-radius: 20px;
      background: linear-gradient(180deg, rgba(255,255,255,0.078), rgba(255,255,255,0.026));
      box-shadow: var(--shadow-card);
      transition: transform 240ms var(--ease), border-color 240ms var(--ease), box-shadow 240ms var(--ease);
    }
    .panel::before {
      content: "";
      position: absolute;
      inset: 0;
      pointer-events: none;
      background: radial-gradient(300px circle at var(--mx, 50%) var(--my, 0%), rgba(94,106,210,0.15), transparent 58%);
      opacity: 0;
      transition: opacity 220ms var(--ease);
    }
    .panel:hover { border-color: var(--border-hover); box-shadow: 0 0 0 1px rgba(255,255,255,0.08), 0 28px 90px rgba(0,0,0,0.55), 0 0 90px rgba(94,106,210,0.12); }
    .panel:hover::before { opacity: 1; }
    .intro { padding: 34px; min-height: 270px; }
    .eyebrow {
      display: inline-flex;
      gap: 8px;
      align-items: center;
      margin-bottom: 16px;
      color: #c5c9ff;
      font: 700 11px var(--mono);
      text-transform: uppercase;
      letter-spacing: 0.16em;
    }
    .eyebrow::before {
      content: "";
      width: 7px;
      height: 7px;
      border-radius: 999px;
      background: var(--accent-bright);
      box-shadow: 0 0 18px var(--accent-glow);
    }
    h1 {
      max-width: 760px;
      margin: 0;
      font-size: clamp(42px, 6vw, 78px);
      line-height: 0.95;
      letter-spacing: -0.035em;
      font-weight: 650;
      background: linear-gradient(180deg, #fff, rgba(255,255,255,0.92) 44%, rgba(255,255,255,0.62));
      -webkit-background-clip: text;
      background-clip: text;
      color: transparent;
    }
    .lead {
      max-width: 680px;
      margin: 18px 0 0;
      color: var(--foreground-muted);
      font-size: 15px;
      line-height: 1.8;
    }
    .sync-card {
      padding: 24px;
      display: grid;
      align-content: space-between;
      min-height: 270px;
    }
    .connection-row {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 14px;
      margin-bottom: 22px;
    }
    .label {
      color: var(--foreground-muted);
      font: 700 11px var(--mono);
      text-transform: uppercase;
      letter-spacing: 0.12em;
    }
    .sync-time {
      margin-top: 8px;
      color: var(--foreground);
      font-size: 18px;
      line-height: 1.35;
    }
    .token-path {
      margin-top: 18px;
      color: var(--foreground-muted);
      font: 12px/1.5 var(--mono);
      overflow-wrap: anywhere;
    }
    .status {
      display: inline-flex;
      min-height: 27px;
      align-items: center;
      border-radius: 999px;
      padding: 0 10px;
      border: 1px solid rgba(255,255,255,0.09);
      font-size: 12px;
      font-weight: 650;
      white-space: nowrap;
      background: rgba(255,255,255,0.055);
    }
    .confirmed_payer { color: #cbffe1; border-color: rgba(125,211,168,0.30); background: rgba(125,211,168,0.10); }
    .inferred_payer { color: #ffe0a7; border-color: rgba(233,184,114,0.34); background: rgba(233,184,114,0.10); }
    .unknown_payer { color: #ffd0ce; border-color: rgba(255,139,134,0.34); background: rgba(255,139,134,0.10); }
    .summary {
      display: grid;
      grid-template-columns: 1.1fr 0.8fr 0.8fr 1fr;
      gap: 12px;
    }
    .metric {
      padding: 18px;
      min-height: 124px;
    }
    .metric strong {
      display: block;
      margin-top: 13px;
      font-size: clamp(26px, 3vw, 38px);
      line-height: 1;
      letter-spacing: -0.025em;
      font-weight: 650;
    }
    .metric span {
      display: block;
      margin-top: 10px;
      color: var(--foreground-muted);
      font-size: 12px;
      line-height: 1.55;
    }
    .notice {
      padding: 13px 15px;
      border-radius: 14px;
      border: 1px solid var(--border);
      color: var(--foreground-subtle);
      background: rgba(255,255,255,0.045);
      box-shadow: inset 0 1px 0 rgba(255,255,255,0.08);
    }
    .notice.error { border-color: rgba(255,139,134,0.24); background: rgba(255,139,134,0.08); color: #ffd0ce; }
    .notice.ok { border-color: rgba(125,211,168,0.26); background: rgba(125,211,168,0.08); color: #cbffe1; }
    .surface { overflow: hidden; }
    .surface-head {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 14px;
      padding: 18px 20px;
      border-bottom: 1px solid var(--border);
      background: rgba(5,5,6,0.42);
    }
    .title {
      color: var(--foreground);
      font-size: 17px;
      font-weight: 650;
      letter-spacing: -0.01em;
    }
    .tiny {
      margin-top: 4px;
      color: var(--foreground-muted);
      font-size: 12px;
      line-height: 1.55;
    }
    .table-wrap { overflow-x: auto; }
    table {
      width: 100%;
      min-width: 1120px;
      border-collapse: collapse;
    }
    th {
      text-align: left;
      color: var(--foreground-muted);
      font: 700 11px var(--mono);
      text-transform: uppercase;
      letter-spacing: 0.10em;
      padding: 13px 14px;
      border-bottom: 1px solid var(--border);
      background: rgba(255,255,255,0.024);
    }
    td {
      padding: 16px 14px;
      border-bottom: 1px solid rgba(255,255,255,0.045);
      color: var(--foreground-subtle);
      font-size: 13px;
      line-height: 1.45;
      vertical-align: top;
    }
    tr { transition: background 180ms var(--ease); }
    tbody tr:hover { background: rgba(255,255,255,0.035); }
    .mono {
      font-family: var(--mono);
      font-size: 11px;
      color: var(--foreground-muted);
      overflow-wrap: anywhere;
    }
    .amount {
      color: var(--foreground);
      font-size: 15px;
      font-weight: 700;
      font-variant-numeric: tabular-nums;
      white-space: nowrap;
    }
    .sender {
      display: grid;
      gap: 8px;
      min-width: 190px;
    }
    .sender strong {
      color: var(--foreground);
      font-size: 14px;
      letter-spacing: -0.01em;
    }
    .sender-note {
      color: var(--foreground-muted);
      font: 700 11px var(--mono);
      text-transform: uppercase;
      letter-spacing: 0.10em;
    }
    .description {
      max-width: 280px;
      color: var(--foreground);
    }
    .empty {
      padding: 64px 20px;
      color: var(--foreground-muted);
      text-align: center;
      line-height: 1.7;
    }
    @media (max-width: 1040px) {
      .hero { grid-template-columns: 1fr; }
      .summary { grid-template-columns: repeat(2, minmax(0, 1fr)); }
      .sync-card { min-height: auto; }
    }
    @media (max-width: 700px) {
      .app { padding: 12px; }
      .shell { min-height: calc(100vh - 24px); border-radius: 18px; }
      .topbar, .surface-head { align-items: flex-start; flex-direction: column; }
      .actions { width: 100%; justify-content: stretch; }
      .actions .btn, .actions form { flex: 1 1 auto; }
      .actions form .btn { width: 100%; }
      .content { padding: 14px; }
      .intro { padding: 24px; min-height: auto; }
      .summary { grid-template-columns: 1fr; }
      table { min-width: 940px; }
    }
    @media (prefers-reduced-motion: reduce) {
      *, *::before, *::after { animation-duration: 0.01ms !important; animation-iteration-count: 1 !important; scroll-behavior: auto !important; transition-duration: 0.01ms !important; }
    }
  </style>
</head>
<body>
  <div class="ambient" aria-hidden="true">
    <div class="blob primary"></div>
    <div class="blob secondary"></div>
    <div class="blob tertiary"></div>
  </div>
  <div class="app">
    <div class="shell">
      <header class="topbar">
        <div class="brand">
          <div class="mark">R</div>
          <div>
            <div class="brand-title">RentOps</div>
            <div class="brand-meta">Bank income workspace</div>
          </div>
        </div>
        <div class="actions">
          {{if .Connected}}
            <a class="btn primary" href="/refresh" aria-label="Refresh bank income transactions">Refresh bank data</a>
          {{else}}
            <a class="btn primary" href="/login" aria-label="Bind bank account">Bind bank account</a>
          {{end}}
          <form method="post" action="/logout"><button class="btn danger" type="submit">Sign out</button></form>
        </div>
      </header>

      <main class="content">
        <section class="hero" aria-labelledby="page-title">
          <div class="panel intro" data-spotlight>
            <div class="eyebrow">{{.Environment}} environment</div>
            <h1 id="page-title">Bank income transactions</h1>
            <p class="lead">A focused bank statement view for incoming rent payments. Sender names are marked as confirmed when provided by the bank payload, including preferred counterparty names from transaction metadata.</p>
          </div>
          <aside class="panel sync-card" data-spotlight aria-label="Bank connection status">
            <div>
              <div class="connection-row">
                <div>
                  <div class="label">Last sync</div>
                  <div class="sync-time">{{.LastSync}}</div>
                </div>
                {{if .Connected}}<span class="status confirmed_payer">Connected</span>{{else}}<span class="status unknown_payer">Not bound</span>{{end}}
              </div>
              <div class="label">Saved login</div>
              <div class="token-path">{{.TokenFile}}</div>
            </div>
          </aside>
        </section>

        {{if .NeedsReconnect}}<div class="notice error">Bank access needs a new authorization. Bind the bank account again to continue refreshing transactions.</div>{{end}}
        {{if eq .Error "data_fetch_failed"}}<div class="notice error">Bank data refresh failed. The app did not save this refresh; bind the bank account again if the bank requires new authorization.</div>{{end}}
        {{if eq .Message "bank_connected"}}<div class="notice ok">Bank account connected. Latest transactions were fetched.</div>{{end}}
        {{if eq .Message "refreshed"}}<div class="notice ok">Bank data refreshed with saved login.</div>{{end}}

        <section class="summary" aria-label="Bank income summary">
          <div class="panel metric" data-spotlight><div class="label">Income total</div><strong>{{.IncomeTotal}}</strong><span>credits and positive amounts from the latest sync</span></div>
          <div class="panel metric" data-spotlight><div class="label">Income rows</div><strong>{{.IncomeCount}}</strong><span>normalized bank statement entries</span></div>
          <div class="panel metric" data-spotlight><div class="label">Unknown sender</div><strong>{{.UnknownPayerCount}}</strong><span>missing sender id or name</span></div>
          <div class="panel metric" data-spotlight><div class="label">Operator</div><strong>{{.Username}}</strong><span>demo administrator</span></div>
        </section>

        <section class="panel surface" data-spotlight aria-labelledby="statement-title">
          <div class="surface-head">
            <div>
              <h2 class="title" id="statement-title">Statement entries</h2>
              <div class="tiny">Confirmed sender names come from payer, remitter, counterparty, or preferred counterparty fields in the bank JSON.</div>
            </div>
            {{if .Connected}}<span class="status confirmed_payer">Saved login available</span>{{else}}<span class="status unknown_payer">Bind required</span>{{end}}
          </div>
          {{if .Rows}}
          <div class="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Sender</th>
                  <th>Amount</th>
                  <th>Date</th>
                  <th>Reference</th>
                  <th>Receiving account</th>
                  <th>Transaction</th>
                  <th>Status</th>
                </tr>
              </thead>
              <tbody>
                {{range .Rows}}
                <tr>
                  <td>
                    <div class="sender">
                      <strong>{{.PayerName}}</strong>
                      <span class="sender-note">{{if eq .PayerNameKind "confirmed"}}Confirmed sender{{else if eq .PayerNameKind "inferred"}}Inferred from text{{else}}Unknown sender{{end}}</span>
                      <span class="mono">Sender ID: {{.PayerID}}</span>
                    </div>
                  </td>
                  <td><span class="amount">{{.AmountDisplay}}</span></td>
                  <td>{{.DateDisplay}}</td>
                  <td><div class="description">{{.Description}}</div><div class="mono">REF: {{.Reference}}</div></td>
                  <td>{{.AccountName}}<br><span class="mono">{{.AccountID}}</span></td>
                  <td><span class="mono">{{.TransactionID}}<br>Source: {{.SourceID}}</span></td>
                  <td><span class="status {{.Status}}">{{.StatusLabel}}</span></td>
                </tr>
                {{end}}
              </tbody>
            </table>
          </div>
          {{else}}
            <div class="empty">No income transactions are available yet. Bind a bank account or refresh with saved login.</div>
          {{end}}
        </section>
      </main>
    </div>
  </div>
  <script>
    for (const el of document.querySelectorAll("[data-spotlight]")) {
      el.addEventListener("pointermove", (event) => {
        const rect = el.getBoundingClientRect();
        el.style.setProperty("--mx", String(event.clientX - rect.left) + "px");
        el.style.setProperty("--my", String(event.clientY - rect.top) + "px");
      });
    }
  </script>
</body>
</html>
`))
