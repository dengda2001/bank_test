package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	defaultScopes = "info accounts balance transactions"
	stateTTL      = 15 * time.Minute
	dateLayout    = "2006-01-02"
)

type config struct {
	Address      string
	ClientID     string
	ClientSecret string
	RedirectURI  string
	Environment  string
	AuthBaseURL  string
	AuthURL      string
	APIBaseURL   string
	Scopes       []string
	Providers    string
	ProviderID   string
	From         string
	LogFile      string
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresIn    int    `json:"expires_in,omitempty"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope,omitempty"`
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
	mux.HandleFunc("/login", a.handleLogin)
	mux.HandleFunc("/callback", a.handleCallback)

	log.Printf("TrueLayer demo listening on http://localhost%s", cfg.Address)
	log.Printf("Redirect URI must be registered in TrueLayer Console: %s", cfg.RedirectURI)
	log.Fatal(http.ListenAndServe(cfg.Address, mux))
}

func loadConfig() (config, error) {
	env := strings.ToLower(strings.TrimSpace(getenv("TL_ENV", "sandbox")))
	cfg := config{
		Address:      getenv("TL_ADDR", ":8080"),
		ClientID:     strings.TrimSpace(os.Getenv("TL_CLIENT_ID")),
		ClientSecret: strings.TrimSpace(os.Getenv("TL_CLIENT_SECRET")),
		RedirectURI:  strings.TrimSpace(getenv("TL_REDIRECT_URI", "http://localhost:8080/callback")),
		Environment:  env,
		AuthURL:      strings.TrimSpace(os.Getenv("TL_AUTH_URL")),
		Scopes:       splitWords(getenv("TL_SCOPES", defaultScopes)),
		Providers:    strings.TrimSpace(os.Getenv("TL_PROVIDERS")),
		ProviderID:   strings.TrimSpace(os.Getenv("TL_PROVIDER_ID")),
		From:         strings.TrimSpace(os.Getenv("TL_FROM")),
		LogFile:      strings.TrimSpace(getenv("TL_LOG_FILE", "bank-data.jsonl")),
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
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := indexTemplate.Execute(w, a.cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (a *app) handleLogin(w http.ResponseWriter, r *http.Request) {
	state, err := randomState()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	issuedStates.record(state)
	http.Redirect(w, r, a.authURL(state), http.StatusFound)
}

func (a *app) handleCallback(w http.ResponseWriter, r *http.Request) {
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

	result, err := a.fetchDemoResult(ctx, token.AccessToken)
	if err != nil {
		http.Error(w, "data fetch failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	if err := a.appendDemoResultLog(result); err != nil {
		http.Error(w, "log write failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
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
// bound is never taken from config: it is always "now in UTC minus one hour",
// so it is guaranteed to be in the past and TrueLayer can never reject it as
// an invalid (future) date range. A configured `from` in the future is
// meaningless and dropped; unparseable values are passed through unchanged.
func txQuery(from string, now time.Time) url.Values {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	q := url.Values{}
	if from != "" {
		if d, err := time.Parse(dateLayout, from); err != nil || !d.After(today) {
			q.Set("from", from)
		}
	}
	q.Set("to", now.UTC().Add(-time.Hour).Format(time.RFC3339))
	return q
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

var indexTemplate = template.Must(template.New("index").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>TrueLayer Ireland Demo</title>
  <style>
    body { font-family: system-ui, sans-serif; max-width: 760px; margin: 48px auto; line-height: 1.5; color: #17202a; }
    code { background: #f3f5f7; padding: 2px 5px; border-radius: 4px; }
    a.button { display: inline-block; padding: 10px 14px; border-radius: 6px; background: #0b5fff; color: white; text-decoration: none; }
    dl { display: grid; grid-template-columns: 160px 1fr; gap: 8px 16px; }
  </style>
</head>
<body>
  <h1>TrueLayer Ireland Demo</h1>
  <p>This local demo starts a TrueLayer OAuth flow, then reads accounts, balances, and transactions after user consent.</p>
  <p><a class="button" href="/login">Connect bank</a></p>
  <dl>
    <dt>Environment</dt><dd><code>{{.Environment}}</code></dd>
    <dt>Redirect URI</dt><dd><code>{{.RedirectURI}}</code></dd>
    <dt>Scopes</dt><dd><code>{{range $i, $s := .Scopes}}{{if $i}} {{end}}{{$s}}{{end}}</code></dd>
    <dt>Providers</dt><dd><code>{{if .Providers}}{{.Providers}}{{else}}not set{{end}}</code></dd>
    <dt>Provider ID</dt><dd><code>{{if .ProviderID}}{{.ProviderID}}{{else}}not set{{end}}</code></dd>
  </dl>
</body>
</html>
`))
