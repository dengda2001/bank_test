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
	defaultScopes = "info accounts balance transactions offline_access"
	stateTTL      = 15 * time.Minute
	sessionTTL    = 12 * time.Hour
	dateLayout    = "2006-01-02"
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

	result, err := a.fetchDemoResult(r.Context(), token.AccessToken)
	if err != nil {
		http.Error(w, "data fetch failed: "+err.Error(), http.StatusBadGateway)
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

		q := txQuery(a.cfg.From, time.Now())
		var txs json.RawMessage
		if err := a.getJSON(ctx, accessToken, "/data/v1/accounts/"+url.PathEscape(acct.AccountID)+"/transactions", q, &txs); err != nil {
			item.Errors = append(item.Errors, "transactions: "+err.Error())
		} else {
			item.Transactions = txs
		}

		result.Accounts = append(result.Accounts, item)
	}
	return result, nil
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
				Reference:     firstNonEmpty(tx.Reference, metaString(tx.Meta, "payment_reference"), metaString(tx.Meta, "reference"), metaString(tx.Meta, "remittance_information"), metaString(tx.Meta, "remittanceInformation")),
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
				row.StatusLabel = "Confirmed payer"
			} else if row.PayerNameKind == "inferred" {
				row.Status = "inferred_payer"
				row.StatusLabel = "Inferred payer"
			} else {
				row.Status = "unknown_payer"
				row.StatusLabel = "Unknown payer"
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
	if name := firstNonEmpty(tx.PayerName, tx.RemitterName, tx.CounterpartyName, metaString(tx.Meta, "payer_name"), metaString(tx.Meta, "remitter_name"), metaString(tx.Meta, "counterparty_name")); name != "" {
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
		if row.PayerNameKind == "unknown" || row.PayerID == "unknown" {
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
      --paper: #f7f8f5; --panel: #ffffff; --ink: #1d2522; --muted: #66736e;
      --line: #d9dfd9; --line-strong: #b9c4bd; --teal: #0f766e;
      --green: #257a4b; --green-soft: #e8f4ed; --amber: #a45f08;
      --amber-soft: #fff0d8; --red: #af3333; --red-soft: #fae5e3;
      --shadow: 0 18px 45px rgba(26, 39, 34, 0.10);
      --sans: "IBM Plex Sans", "Aptos", "Segoe UI", system-ui, sans-serif;
      --mono: "IBM Plex Mono", "SFMono-Regular", Consolas, monospace;
    }
    * { box-sizing: border-box; }
    body { margin: 0; background: var(--paper); color: var(--ink); font-family: var(--sans); }
    button, input { font: inherit; }
    .app { min-height: 100vh; display: grid; grid-template-columns: 236px minmax(0, 1fr); }
    aside { background: #17201d; color: #eef5f1; padding: 20px 16px; display: flex; flex-direction: column; gap: 24px; }
    .brand { display: flex; gap: 10px; align-items: center; padding-bottom: 14px; border-bottom: 1px solid rgba(255,255,255,.12); }
    .mark { width: 34px; height: 34px; background: #f7f8f5; color: #17201d; display: grid; place-items: center; font: 700 20px Georgia, serif; }
    .brand-name { font: 700 22px Georgia, serif; }
    .nav { display: grid; gap: 5px; }
    .nav div { color: rgba(238,245,241,.64); padding: 10px 9px; border-radius: 6px; }
    .nav .active { color: #fff; background: rgba(255,255,255,.10); }
    .side-foot { margin-top: auto; border-top: 1px solid rgba(255,255,255,.12); padding-top: 14px; color: rgba(238,245,241,.72); font-size: 13px; }
    main { min-width: 0; padding: 24px; }
    .topbar { display: flex; justify-content: space-between; gap: 18px; align-items: flex-start; margin-bottom: 18px; }
    .kicker { color: var(--muted); font-size: 13px; margin-bottom: 7px; }
    h1 { margin: 0; font: 700 32px/1.05 Georgia, serif; }
    .actions { display: flex; gap: 9px; flex-wrap: wrap; justify-content: flex-end; }
    .btn {
      height: 38px; border: 1px solid var(--line-strong); background: var(--panel);
      color: var(--ink); border-radius: 6px; padding: 0 12px; display: inline-flex;
      align-items: center; cursor: pointer; text-decoration: none;
    }
    .btn.primary { border-color: #0f665f; background: var(--teal); color: #fff; }
    .btn.danger { border-color: #d8aaa6; color: var(--red); }
    .summary { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 10px; margin-bottom: 14px; }
    .metric { background: #fff; border: 1px solid var(--line); border-radius: 8px; padding: 14px; box-shadow: 0 10px 30px rgba(26,39,34,.04); }
    .metric label { display: block; color: var(--muted); font-size: 12px; margin-bottom: 7px; }
    .metric strong { font: 700 26px Georgia, serif; }
    .metric span { display: block; color: var(--muted); font-size: 12px; margin-top: 7px; }
    .notice { border: 1px solid var(--line); background: #fff; border-radius: 8px; padding: 12px 14px; margin-bottom: 14px; color: var(--muted); }
    .notice.error { border-color: #e9aaa5; background: var(--red-soft); color: var(--red); }
    .notice.ok { border-color: #b8d9c3; background: var(--green-soft); color: var(--green); }
    .surface { background: #fff; border: 1px solid var(--line); border-radius: 8px; box-shadow: var(--shadow); overflow: hidden; }
    .surface-head { padding: 14px 16px; border-bottom: 1px solid var(--line); display: flex; justify-content: space-between; gap: 12px; align-items: center; }
    .title { font-weight: 700; }
    .tiny { color: var(--muted); font-size: 12px; }
    .table-wrap { overflow-x: auto; }
    table { width: 100%; min-width: 1040px; border-collapse: collapse; }
    th { text-align: left; color: var(--muted); font-size: 12px; padding: 10px 9px; background: #fafbf8; border-bottom: 1px solid var(--line); }
    td { padding: 11px 9px; border-bottom: 1px solid #edf0ec; font-size: 13px; vertical-align: top; }
    .mono { font-family: var(--mono); font-size: 12px; overflow-wrap: anywhere; }
    .money { font-variant-numeric: tabular-nums; white-space: nowrap; font-weight: 700; }
    .payer { display: grid; gap: 3px; }
    .payer span { color: var(--muted); }
    .status { display: inline-flex; height: 25px; align-items: center; border-radius: 999px; padding: 0 8px; font-size: 12px; font-weight: 700; white-space: nowrap; }
    .confirmed_payer { color: var(--green); background: var(--green-soft); }
    .inferred_payer { color: var(--amber); background: var(--amber-soft); }
    .unknown_payer { color: var(--red); background: var(--red-soft); }
    .empty { padding: 34px 18px; color: var(--muted); text-align: center; }
    @media (max-width: 900px) {
      .app { grid-template-columns: 1fr; }
      aside { display: none; }
      main { padding: 16px; }
      .topbar { display: grid; }
      .actions { justify-content: flex-start; }
      .summary { grid-template-columns: repeat(2, minmax(0, 1fr)); }
    }
    @media (max-width: 520px) { .summary { grid-template-columns: 1fr; } h1 { font-size: 29px; } }
  </style>
</head>
<body>
  <div class="app">
    <aside>
      <div class="brand">
        <div class="mark">R</div>
        <div>
          <div class="brand-name">RentOps</div>
          <div class="tiny">Tenant billing</div>
        </div>
      </div>
      <div class="nav">
        <div class="active">Bank income</div>
        <div>Bills</div>
        <div>Tenants</div>
        <div>Properties</div>
      </div>
      <div class="side-foot">
        <strong>{{.Username}}</strong><br>
        Demo administrator
      </div>
    </aside>
    <main>
      <header class="topbar">
        <div>
          <div class="kicker">{{.Environment}} environment · manual refresh · {{.LastSync}}</div>
          <h1>Bank income transactions</h1>
        </div>
        <div class="actions">
          {{if .Connected}}
            <a class="btn" href="/refresh">Manual refresh</a>
          {{else}}
            <a class="btn primary" href="/login">Bind bank account</a>
          {{end}}
          <form method="post" action="/logout"><button class="btn danger" type="submit">Sign out</button></form>
        </div>
      </header>

      {{if .NeedsReconnect}}<div class="notice error">Bank access needs a new authorization. Bind the bank account again to continue refreshing transactions.</div>{{end}}
      {{if eq .Message "bank_connected"}}<div class="notice ok">Bank account connected. Latest transactions were fetched.</div>{{end}}
      {{if eq .Message "refreshed"}}<div class="notice ok">Bank data refreshed with saved login.</div>{{end}}

      <section class="summary">
        <div class="metric"><label>Income total</label><strong>{{.IncomeTotal}}</strong><span>from latest sync</span></div>
        <div class="metric"><label>Income rows</label><strong>{{.IncomeCount}}</strong><span>credits and positive amounts</span></div>
        <div class="metric"><label>Unknown payer</label><strong>{{.UnknownPayerCount}}</strong><span>id or name missing</span></div>
        <div class="metric"><label>Bank connection</label><strong>{{if .Connected}}Connected{{else}}Not bound{{end}}</strong><span>{{.TokenFile}}</span></div>
      </section>

      <section class="surface">
        <div class="surface-head">
          <div>
            <div class="title">Income transaction list</div>
            <div class="tiny">Payer fields can be confirmed by bank data, inferred from text, or unknown.</div>
          </div>
          {{if .Connected}}<span class="status confirmed_payer">Saved login available</span>{{else}}<span class="status unknown_payer">Bind required</span>{{end}}
        </div>
        {{if .Rows}}
        <div class="table-wrap">
          <table>
            <thead>
              <tr>
                <th>Transaction ID</th>
                <th>Payer</th>
                <th>Payer/source ID</th>
                <th>Date</th>
                <th>Amount</th>
                <th>Receiving account</th>
                <th>Description / reference</th>
                <th>Status</th>
              </tr>
            </thead>
            <tbody>
              {{range .Rows}}
              <tr>
                <td class="mono">{{.TransactionID}}</td>
                <td><div class="payer"><strong>{{.PayerName}}</strong><span>{{.PayerNameKind}}</span></div></td>
                <td class="mono">{{.PayerID}}<br>{{.SourceID}}</td>
                <td>{{.DateDisplay}}</td>
                <td class="money">{{.AmountDisplay}}</td>
                <td>{{.AccountName}}<br><span class="mono">{{.AccountID}}</span></td>
                <td>{{.Description}}<br><span class="mono">REF: {{.Reference}}</span></td>
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
</body>
</html>
`))
