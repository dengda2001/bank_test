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

	"gorm.io/gorm"
)

const (
	defaultScopes       = "info accounts balance transactions offline_access"
	stateTTL            = 15 * time.Minute
	sessionTTL          = 12 * time.Hour
	refreshLookbackDays = 90
	dateLayout          = "2006-01-02"
)

type config struct {
	Address                string
	ClientID               string
	ClientSecret           string
	RedirectURI            string
	Environment            string
	AuthBaseURL            string
	AuthURL                string
	APIBaseURL             string
	Scopes                 []string
	Providers              string
	ProviderID             string
	From                   string
	LogFile                string
	TokenFile              string
	TenantFile             string
	ExpenseFile            string
	MySQLDSN               string
	DatabaseURL            string
	MigrationsDir          string
	BankTokenEncryptionKey string
	AllowPlaintextTokens   bool
	AdminUsername          string
	AdminPassword          string
	SessionSecret          string
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
	SyncRunID   string        `json:"sync_run_id,omitempty"`
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
	ActivePage        string
	Connected         bool
	NeedsReconnect    bool
	LastSync          string
	Message           string
	Error             string
	TransactionRows   []transactionPageRow
	TenantOptions     []billingTenantOption
	ArrivalFromFilter string
	ArrivalToFilter   string
	PayerFilter       string
	TenantFilter      uint64
	DirectionFilter   string
	MatchStatusFilter string
	PeriodFilter      string
	RentPeriodFilter  string
	AllocationFilter  string
	SortFilter        string
	Page              int
	PageSize          int
	TotalTransactions int64
	TotalPages        int
	PreviousPageURL   string
	NextPageURL       string
	PendingFilter     bool
	PendingCount      int
	IncomeCount       int
	TenantCount       int
	ExpenseCount      int
	TokenFile         string
}

type billingTenantOption struct {
	ID   uint64
	Name string
}

type billingMonthOption struct {
	Period    string
	Label     string
	Expected  string
	Paid      string
	Remaining string
}

type tenantRecord struct {
	ID               string  `json:"id"`
	Name             string  `json:"name"`
	DisplayAlias     string  `json:"display_alias,omitempty"`
	Email            string  `json:"email,omitempty"`
	PayerID          string  `json:"payer_id,omitempty"`
	PayerNameHint    string  `json:"payer_name_hint,omitempty"`
	MonthlyRent      float64 `json:"monthly_rent"`
	Currency         string  `json:"currency"`
	IntervalUnit     string  `json:"interval_unit,omitempty"`
	IntervalCount    int     `json:"interval_count,omitempty"`
	BillingStartDate string  `json:"billing_start_date,omitempty"`
	DueDay           int     `json:"due_day,omitempty"`
	RentStartDate    string  `json:"rent_start_date,omitempty"`
	RentEndDate      string  `json:"rent_end_date,omitempty"`
	Status           string  `json:"status,omitempty"`
	RoomLabel        string  `json:"room_label,omitempty"`
	RoomAddress      string  `json:"room_address"`
	PropertyHint     string  `json:"property_hint,omitempty"`
	CreatedAt        string  `json:"created_at"`

	RentDisplay string `json:"-"`

	BillingHistory []tenantBillingMonth `json:"-"`
}

type expenseRecord struct {
	ID            string  `json:"id"`
	Description   string  `json:"description"`
	Category      string  `json:"category"`
	Amount        float64 `json:"amount"`
	Currency      string  `json:"currency"`
	ExpenseDate   string  `json:"expense_date"`
	PaymentMethod string  `json:"payment_method"`
	RoomHint      string  `json:"room_hint,omitempty"`
	TenantHint    string  `json:"tenant_hint,omitempty"`
	CreatedAt     string  `json:"created_at"`

	AmountDisplay string `json:"-"`
	DateDisplay   string `json:"-"`
}

type tenantPageData struct {
	Username     string
	Environment  string
	ActivePage   string
	Message      string
	Error        string
	Rows         []tenantRecord
	TenantCount  int
	RentTotal    string
	TenantFile   string
	ExpenseCount int
	IncomeCount  int
	ShowForm     bool
	Editing      bool
	Form         tenantRecord
}

type expensePageData struct {
	Username     string
	Environment  string
	ActivePage   string
	Message      string
	Error        string
	Rows         []expenseRecord
	ExpenseCount int
	ExpenseTotal string
	ExpenseFile  string
	TenantCount  int
	IncomeCount  int
}

type rentDashboardPageData struct {
	Username                   string
	Environment                string
	ActivePage                 string
	Period                     string
	PeriodLabel                string
	PreviousPeriod             string
	NextPeriod                 string
	Rows                       []rentDashboardRow
	SearchFilter               string
	StatusFilter               string
	SortFilter                 string
	Page                       int
	PageSize                   int
	FilteredCount              int
	TotalRows                  int
	TotalPages                 int
	ExpectedTotal              string
	PaidTotal                  string
	BalanceTotal               string
	ExpenseTotal               string
	OpenCount                  int
	OverdueCount               int
	UnpaidCount                int
	PartialCount               int
	PaidCount                  int
	ReviewCount                int
	TenantCount                int
	IncomeCount                int
	ExpenseCount               int
	CollectionPercent          int
	PendingCount               int
	PendingTotal               string
	OtherIncomeTotal           string
	OtherIncomeCount           int
	SyncCoverage               string
	SyncStatus                 string
	LastSuccessfulSyncCoverage string
	Message                    string
	Error                      string
}

type rentDashboardRow struct {
	TenantID       uint64
	TenantName     string
	RoomLabel      string
	RoomAddress    string
	Period         string
	DueDate        string
	ExpectedAmount string
	PaidAmount     string
	BalanceAmount  string
	Status         string
	StatusLabel    string
	ObligationID   uint64
	Payments       []rentPaymentDetail
	TenantAlias    string
	ExpectedCents  int64
	PaidCents      int64
	DueDateValue   time.Time
}

type rentPaymentDetail struct {
	PaymentID          uint64
	AmountDisplay      string
	DateDisplay        string
	Description        string
	Reference          string
	Source             string
	ConfirmationSource string
}

type app struct {
	cfg             config
	httpClient      *http.Client
	db              *gorm.DB
	auth            *authService
	bankConnections *bankConnectionStore
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
	db, err := initDatabase(cfg)
	if err != nil {
		log.Fatal(err)
	}

	a := &app{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		db:              db,
		auth:            newAuthService(db, cfg),
		bankConnections: newBankConnectionStore(db, cfg),
	}
	if err := a.auth.seedDefaultUser(context.Background()); err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", a.handleIndex)
	mux.HandleFunc("/login-local", a.handleLocalLogin)
	mux.HandleFunc("/logout", a.handleLogout)
	mux.HandleFunc("/rent-dashboard", a.handleRentDashboard)
	mux.HandleFunc("/billing", a.handleBilling)
	mux.HandleFunc("/billing/confirm", a.handleRentMatchConfirmation)
	mux.HandleFunc("/billing/allocate", a.handleTransactionAllocation)
	mux.HandleFunc("/billing/ignore", a.handleTransactionIgnore)
	mux.HandleFunc("/billing/restore", a.handleTransactionRestore)
	mux.HandleFunc("/billing/revoke", a.handleTransactionRevoke)
	mux.HandleFunc("/billing/payer/preview", a.handlePayerPreview)
	mux.HandleFunc("/billing/payer/confirm", a.handlePayerConfirm)
	mux.HandleFunc("/import-legacy", a.handleLegacyImport)
	mux.HandleFunc("/tenants", a.handleTenants)
	mux.HandleFunc("/tenants/", a.handleTenantSubroute)
	mux.HandleFunc("/cash-receipts/new", a.handleCashReceiptNew)
	mux.HandleFunc("/cash-receipts/preview", a.handleCashReceiptPreview)
	mux.HandleFunc("/cash-receipts/void", a.handleCashReceiptVoid)
	mux.HandleFunc("/cash-receipts", a.handleCashReceiptCreate)
	mux.HandleFunc("/expenses", a.handleExpenses)
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
		Address:                getenv("TL_ADDR", ":8080"),
		ClientID:               strings.TrimSpace(os.Getenv("TL_CLIENT_ID")),
		ClientSecret:           strings.TrimSpace(os.Getenv("TL_CLIENT_SECRET")),
		RedirectURI:            strings.TrimSpace(getenv("TL_REDIRECT_URI", "http://localhost:8080/callback")),
		Environment:            env,
		AuthURL:                strings.TrimSpace(os.Getenv("TL_AUTH_URL")),
		Scopes:                 splitWords(getenv("TL_SCOPES", defaultScopes)),
		Providers:              strings.TrimSpace(os.Getenv("TL_PROVIDERS")),
		ProviderID:             strings.TrimSpace(os.Getenv("TL_PROVIDER_ID")),
		From:                   strings.TrimSpace(os.Getenv("TL_FROM")),
		LogFile:                strings.TrimSpace(getenv("TL_LOG_FILE", "bank-data.jsonl")),
		TokenFile:              strings.TrimSpace(getenv("TL_TOKEN_FILE", "truelayer-token.json")),
		TenantFile:             strings.TrimSpace(getenv("RENTOPS_TENANT_FILE", "rentops-tenants.json")),
		ExpenseFile:            strings.TrimSpace(getenv("RENTOPS_EXPENSE_FILE", "rentops-expenses.json")),
		MySQLDSN:               strings.TrimSpace(os.Getenv("MYSQL_DSN")),
		DatabaseURL:            strings.TrimSpace(os.Getenv("DATABASE_URL")),
		MigrationsDir:          strings.TrimSpace(getenv("MIGRATIONS_DIR", "migrations")),
		BankTokenEncryptionKey: strings.TrimSpace(os.Getenv("BANK_TOKEN_ENCRYPTION_KEY")),
		AllowPlaintextTokens:   os.Getenv("ALLOW_PLAINTEXT_TOKENS") == "1",
		AdminUsername:          strings.TrimSpace(getenv("APP_ADMIN_USERNAME", "ddrzh")),
		AdminPassword:          getenv("APP_ADMIN_PASSWORD", "ddrzh512"),
		SessionSecret:          strings.TrimSpace(os.Getenv("APP_SESSION_SECRET")),
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
	if err := validateTokenStorageConfig(cfg); err != nil {
		return config{}, err
	}
	return cfg, nil
}

func splitWords(s string) []string {
	return strings.Fields(strings.ReplaceAll(s, ",", " "))
}

func resolveDatabaseDSN(cfg config) (string, error) {
	if cfg.MySQLDSN != "" {
		return cfg.MySQLDSN, nil
	}
	if cfg.DatabaseURL == "" {
		return "", errors.New("MYSQL_DSN or DATABASE_URL is required")
	}
	u, err := url.Parse(cfg.DatabaseURL)
	if err != nil {
		return "", fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	if u.Scheme != "mysql" {
		return "", fmt.Errorf("DATABASE_URL scheme must be mysql, got %q", u.Scheme)
	}
	username := u.User.Username()
	password, _ := u.User.Password()
	host := u.Host
	dbName := strings.TrimPrefix(u.EscapedPath(), "/")
	if username == "" || host == "" || dbName == "" {
		return "", errors.New("DATABASE_URL must include username, host, and database name")
	}
	auth := username
	if password != "" {
		auth += ":" + password
	}
	q := u.Query()
	return fmt.Sprintf("%s@tcp(%s)/%s?%s", auth, host, dbName, q.Encode()), nil
}

func validateTokenStorageConfig(cfg config) error {
	if cfg.BankTokenEncryptionKey != "" {
		if _, err := tokenEncryptionKeyBytes(cfg.BankTokenEncryptionKey); err != nil {
			return err
		}
		return nil
	}
	if cfg.AllowPlaintextTokens && cfg.Environment != "live" {
		return nil
	}
	return errors.New("BANK_TOKEN_ENCRYPTION_KEY is required for bank token storage")
}

func tokenEncryptionKeyBytes(value string) ([]byte, error) {
	raw := strings.TrimSpace(value)
	if len(raw) == 32 {
		return []byte(raw), nil
	}
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		decoded, err := enc.DecodeString(raw)
		if err == nil && len(decoded) == 32 {
			return decoded, nil
		}
	}
	return nil, errors.New("BANK_TOKEN_ENCRYPTION_KEY must be 32 bytes or base64-encoded 32 bytes")
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
		http.Redirect(w, r, "/rent-dashboard", http.StatusFound)
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
		if a.auth == nil {
			http.Redirect(w, r, "/?error=invalid_login", http.StatusFound)
			return
		}
	}
	if a.auth != nil {
		user, err := a.auth.authenticate(r.Context(), r.Form.Get("username"), r.Form.Get("password"))
		if err != nil {
			http.Redirect(w, r, "/?error=invalid_login", http.StatusFound)
			return
		}
		http.SetCookie(w, userSessionCookie(a.cfg, user.ID, user.Username, time.Now().Add(sessionTTL)))
		http.Redirect(w, r, "/rent-dashboard", http.StatusFound)
		return
	}
	http.SetCookie(w, sessionCookie(a.cfg, time.Now().Add(sessionTTL)))
	http.Redirect(w, r, "/rent-dashboard", http.StatusFound)
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
	filters := filtersFromQuery(r.URL.Query())
	filterError := ""
	if err := validateTransactionFilters(filters); err != nil {
		filterError = "invalid_filter"
		filters = transactionFilters{Page: 1, PageSize: 50}
	}
	var rows []transactionPageRow
	var lastSync string
	var tenantCount, expenseCount int
	var pendingCount int
	var totalTransactions int64
	var tenantOptions []billingTenantOption
	if userID, ok := a.currentUserID(r); ok && a.db != nil {
		var err error
		rows, totalTransactions, err = newTransactionService(a.db).listTransactionPageRowsWithTotal(r.Context(), userID, filters)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		var tenantRows []tenant
		if err := a.db.WithContext(r.Context()).Where("user_id = ?", userID).Order("name ASC").Find(&tenantRows).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		tenantCount = len(tenantRows)
		if coverage, coverageErr := latestBankSyncCoverage(r.Context(), a.db, userID); coverageErr != nil {
			http.Error(w, coverageErr.Error(), http.StatusInternalServerError)
			return
		} else {
			lastSync = coverage
		}
		tenantOptions = make([]billingTenantOption, 0, len(tenantRows))
		for _, tenantRow := range tenantRows {
			tenantOptions = append(tenantOptions, billingTenantOption{ID: tenantRow.ID, Name: tenantRow.Name})
		}
		var count int64
		if err := a.db.WithContext(r.Context()).Model(&manualExpense{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		expenseCount = int(count)
		if pendingCount64, err := newTransactionService(a.db).countPendingTransactionsWithFilters(r.Context(), userID, filters); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		} else {
			pendingCount = int(pendingCount64)
		}
	} else {
		result, _ := a.loadLatestDemoResult()
		lastSync = result.FetchedAt
		rows = fallbackTransactionPageRows(result, filters)
		totalTransactions = int64(len(rows))
		tenants, _ := a.loadTenants()
		expenses, _ := a.loadExpenses()
		tenantCount = len(tenants)
		expenseCount = len(expenses)
	}
	page := filters.Page
	if page <= 0 {
		page = 1
	}
	pageSize := filters.PageSize
	if pageSize <= 0 {
		pageSize = 50
	}
	totalPages := 0
	if totalTransactions > 0 {
		totalPages = int((totalTransactions + int64(pageSize) - 1) / int64(pageSize))
	}
	previousPageURL, nextPageURL := "", ""
	if page > 1 {
		previousPageURL = billingPageURL(r.URL.Query(), page-1)
	}
	if totalPages > 0 && page < totalPages {
		nextPageURL = billingPageURL(r.URL.Query(), page+1)
	}
	connected := a.hasStoredToken()
	if userID, ok := a.currentUserID(r); ok && a.bankConnections != nil {
		connected, _ = a.bankConnections.hasRefreshToken(r.Context(), userID)
	}
	data := billingPageData{
		Username:          a.cfg.AdminUsername,
		Environment:       a.cfg.Environment,
		ActivePage:        "billing",
		Connected:         connected,
		NeedsReconnect:    r.URL.Query().Get("reconnect") == "1",
		LastSync:          lastSync,
		Message:           r.URL.Query().Get("message"),
		Error:             firstNonEmpty(filterError, r.URL.Query().Get("error")),
		TransactionRows:   rows,
		TenantOptions:     tenantOptions,
		ArrivalFromFilter: filters.ArrivalFrom,
		ArrivalToFilter:   filters.ArrivalTo,
		PayerFilter:       filters.Payer,
		TenantFilter:      filters.TenantID,
		DirectionFilter:   filters.Direction,
		MatchStatusFilter: filters.MatchStatus,
		PeriodFilter:      filters.PeriodMonth,
		RentPeriodFilter:  filters.RentPeriod,
		AllocationFilter:  filters.AllocationKind,
		SortFilter:        filters.Sort,
		Page:              page,
		PageSize:          pageSize,
		TotalTransactions: totalTransactions,
		TotalPages:        totalPages,
		PreviousPageURL:   previousPageURL,
		NextPageURL:       nextPageURL,
		PendingFilter:     filters.PendingOnly,
		PendingCount:      pendingCount,
		IncomeCount:       int(totalTransactions),
		TokenFile:         a.cfg.TokenFile,
		TenantCount:       tenantCount,
		ExpenseCount:      expenseCount,
	}
	if data.LastSync == "" {
		data.LastSync = "尚未同步"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := billingTemplate.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func billingPageURL(query url.Values, page int) string {
	values := url.Values{}
	for key, items := range query {
		values[key] = append([]string(nil), items...)
	}
	values.Set("page", strconv.Itoa(page))
	return "/billing?" + values.Encode()
}

func (a *app) handleRentMatchConfirmation(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := a.currentUserID(r)
	if !ok || a.db == nil {
		http.Error(w, "database session required", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/billing?error=invalid_confirmation", http.StatusFound)
		return
	}
	transactionID, err := transactionID(r.Form.Get("transaction_id"))
	if err != nil {
		http.Redirect(w, r, "/billing?error=invalid_confirmation", http.StatusFound)
		return
	}
	tenantID, err := strconv.ParseUint(r.Form.Get("tenant_id"), 10, 64)
	if err != nil || tenantID == 0 {
		http.Redirect(w, r, "/billing?error=invalid_confirmation", http.StatusFound)
		return
	}
	var period *time.Time
	if value := strings.TrimSpace(r.Form.Get("period")); value != "" {
		parsed, err := parsePeriodMonth(value)
		if err != nil {
			http.Redirect(w, r, "/billing?error=invalid_confirmation", http.StatusFound)
			return
		}
		period = &parsed
	}
	rememberPayer := r.Form.Get("remember_payer") != "0"
	if err := newTransactionService(a.db).confirmRentMatch(r.Context(), userID, transactionID, tenantID, period, rememberPayer); err != nil {
		http.Redirect(w, r, "/billing?error=confirmation_failed", http.StatusFound)
		return
	}
	http.Redirect(w, r, "/billing?message=rent_confirmed", http.StatusFound)
}

func (a *app) requestCounts(ctx context.Context, r *http.Request) (tenantCount, transactionCount, expenseCount int, err error) {
	if userID, ok := a.currentUserID(r); ok && a.db != nil {
		var count int64
		if err := a.db.WithContext(ctx).Model(&tenant{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
			return 0, 0, 0, err
		}
		tenantCount = int(count)
		if err := a.db.WithContext(ctx).Model(&paymentTransaction{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
			return 0, 0, 0, err
		}
		transactionCount = int(count)
		if err := a.db.WithContext(ctx).Model(&manualExpense{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
			return 0, 0, 0, err
		}
		return tenantCount, transactionCount, int(count), nil
	}
	tenants, tenantsErr := a.loadTenants()
	if tenantsErr != nil {
		return 0, 0, 0, tenantsErr
	}
	expenses, expensesErr := a.loadExpenses()
	if expensesErr != nil {
		return 0, 0, 0, expensesErr
	}
	result, _ := a.loadLatestDemoResult()
	return len(tenants), len(normalizePaymentTransactions(result)), len(expenses), nil
}

func (a *app) handleTenants(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.Method == http.MethodPost {
		a.createTenant(w, r)
		return
	}
	tenants, err := a.listTenantRecords(r.Context(), r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if userID, ok := a.currentUserID(r); ok && a.db != nil {
		history, err := newObligationService(a.db).listTenantBillingHistory(r.Context(), userID, monthStart(time.Now().UTC()), 3)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for i := range tenants {
			tenantID, parseErr := strconv.ParseUint(tenants[i].ID, 10, 64)
			if parseErr == nil {
				tenants[i].BillingHistory = history[tenantID]
			}
		}
	}
	_, incomeCount, expenseCount, err := a.requestCounts(r.Context(), r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	prepareTenants(tenants)
	formRecord := tenantRecord{Currency: "EUR", IntervalUnit: "month", IntervalCount: 1, Status: "active", DueDay: 1}
	editing := false
	if editID := strings.TrimSpace(r.URL.Query().Get("edit")); editID != "" {
		for _, record := range tenants {
			if record.ID == editID {
				formRecord = record
				editing = true
				break
			}
		}
	}
	showForm := editing || r.URL.Query().Get("add") == "1"
	data := tenantPageData{
		Username:     a.cfg.AdminUsername,
		Environment:  a.cfg.Environment,
		ActivePage:   "tenants",
		Message:      r.URL.Query().Get("message"),
		Error:        r.URL.Query().Get("error"),
		Rows:         tenants,
		TenantCount:  len(tenants),
		RentTotal:    formatMoney(sumTenantRent(tenants), "EUR", 2),
		TenantFile:   a.cfg.TenantFile,
		ExpenseCount: expenseCount,
		IncomeCount:  incomeCount,
		ShowForm:     showForm,
		Editing:      editing,
		Form:         formRecord,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tenantTemplate.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (a *app) createTenant(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/tenants?error=invalid_form", http.StatusFound)
		return
	}
	if err := a.persistTenantRecord(r.Context(), r, r.Form); err != nil {
		if errors.Is(err, errTenantLifecycleConflict) {
			http.Redirect(w, r, "/tenants?error=tenant_has_payments", http.StatusFound)
			return
		}
		if errors.Is(err, errInvalidTenantInput) {
			http.Redirect(w, r, "/tenants?error=invalid_tenant", http.StatusFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	message := "tenant_added"
	if strings.TrimSpace(r.Form.Get("tenant_id")) != "" {
		message = "tenant_updated"
	}
	http.Redirect(w, r, "/tenants?message="+message, http.StatusFound)
}

var errInvalidTenantInput = errors.New("invalid tenant input")

func (a *app) listTenantRecords(ctx context.Context, r *http.Request) ([]tenantRecord, error) {
	if userID, ok := a.currentUserID(r); ok && a.db != nil {
		return newTenantService(a.db).listTenants(ctx, userID)
	}
	return a.loadTenants()
}

func (a *app) persistTenantRecord(ctx context.Context, r *http.Request, values formValues) error {
	if userID, ok := a.currentUserID(r); ok && a.db != nil {
		input, parseErr := tenantInputFromForm(values)
		if parseErr != nil {
			return errInvalidTenantInput
		}
		service := newTenantService(a.db)
		var err error
		if rawID := strings.TrimSpace(values.Get("tenant_id")); rawID != "" {
			var tenantID uint64
			tenantID, err = strconv.ParseUint(rawID, 10, 64)
			if err == nil {
				_, err = service.updateTenant(ctx, userID, tenantID, input)
			}
		} else {
			_, err = service.createTenant(ctx, userID, input)
		}
		if err != nil {
			if isValidationError(err) {
				return errInvalidTenantInput
			}
			return err
		}
		return nil
	}
	name := strings.TrimSpace(values.Get("name"))
	roomAddress := strings.TrimSpace(values.Get("room_address"))
	monthlyRent, err := parsePositiveAmount(values.Get("monthly_rent"))
	if name == "" || roomAddress == "" || err != nil {
		return errInvalidTenantInput
	}
	tenants, err := a.loadTenants()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	updatedID := strings.TrimSpace(values.Get("tenant_id"))
	if updatedID != "" {
		for i := range tenants {
			if tenants[i].ID == updatedID {
				tenants[i].Name = name
				tenants[i].MonthlyRent = monthlyRent
				tenants[i].Currency = firstNonEmpty(strings.ToUpper(strings.TrimSpace(values.Get("currency"))), "EUR")
				tenants[i].RoomAddress = roomAddress
				return a.saveTenants(tenants)
			}
		}
		return errInvalidTenantInput
	}
	tenants = append(tenants, tenantRecord{
		ID:          recordID("tenant", now),
		Name:        name,
		MonthlyRent: monthlyRent,
		Currency:    firstNonEmpty(strings.ToUpper(strings.TrimSpace(values.Get("currency"))), "EUR"),
		RoomAddress: roomAddress,
		CreatedAt:   now.Format(time.RFC3339),
	})
	return a.saveTenants(tenants)
}

func (a *app) handleExpenses(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.Method == http.MethodPost {
		a.createExpense(w, r)
		return
	}
	expenses, err := a.listExpenseRecords(r.Context(), r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tenantCount, incomeCount, _, err := a.requestCounts(r.Context(), r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	prepareExpenses(expenses)
	data := expensePageData{
		Username:     a.cfg.AdminUsername,
		Environment:  a.cfg.Environment,
		ActivePage:   "expenses",
		Message:      r.URL.Query().Get("message"),
		Error:        r.URL.Query().Get("error"),
		Rows:         expenses,
		ExpenseCount: len(expenses),
		ExpenseTotal: formatMoney(sumExpenses(expenses), "EUR", 2),
		ExpenseFile:  a.cfg.ExpenseFile,
		TenantCount:  tenantCount,
		IncomeCount:  incomeCount,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := expenseTemplate.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (a *app) createExpense(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/expenses?error=invalid_form", http.StatusFound)
		return
	}
	if err := a.persistExpenseRecord(r.Context(), r, r.Form); err != nil {
		if errors.Is(err, errInvalidExpenseInput) {
			http.Redirect(w, r, "/expenses?error=invalid_expense", http.StatusFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/expenses?message=expense_added", http.StatusFound)
}

var errInvalidExpenseInput = errors.New("invalid expense input")

func (a *app) listExpenseRecords(ctx context.Context, r *http.Request) ([]expenseRecord, error) {
	if userID, ok := a.currentUserID(r); ok && a.db != nil {
		return newExpenseService(a.db).listExpenses(ctx, userID)
	}
	return a.loadExpenses()
}

func (a *app) persistExpenseRecord(ctx context.Context, r *http.Request, values formValues) error {
	if userID, ok := a.currentUserID(r); ok && a.db != nil {
		input, err := expenseInputFromForm(values, time.Now())
		if err != nil {
			return errInvalidExpenseInput
		}
		if _, err := newExpenseService(a.db).createExpense(ctx, userID, input); err != nil {
			if isValidationError(err) {
				return errInvalidExpenseInput
			}
			return err
		}
		return nil
	}
	description := strings.TrimSpace(values.Get("description"))
	amount, err := parsePositiveAmount(values.Get("amount"))
	if description == "" || err != nil {
		return errInvalidExpenseInput
	}
	expenseDate := strings.TrimSpace(values.Get("expense_date"))
	if _, err := time.Parse(dateLayout, expenseDate); expenseDate == "" || err != nil {
		expenseDate = time.Now().UTC().Format(dateLayout)
	}
	expenses, err := a.loadExpenses()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	expenses = append(expenses, expenseRecord{
		ID:            recordID("expense", now),
		Description:   description,
		Category:      firstNonEmpty(values.Get("category"), "General"),
		Amount:        amount,
		Currency:      firstNonEmpty(strings.ToUpper(strings.TrimSpace(values.Get("currency"))), "EUR"),
		ExpenseDate:   expenseDate,
		PaymentMethod: firstNonEmpty(values.Get("payment_method"), "Manual"),
		RoomHint:      strings.TrimSpace(values.Get("room_hint")),
		TenantHint:    strings.TrimSpace(values.Get("tenant_hint")),
		CreatedAt:     now.Format(time.RFC3339),
	})
	return a.saveExpenses(expenses)
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
		if userID, ok := a.currentUserID(r); ok && a.bankConnections != nil {
			if err := a.bankConnections.saveRefreshToken(ctx, userID, token.RefreshToken); err != nil {
				http.Error(w, "token save failed: "+err.Error(), http.StatusInternalServerError)
				return
			}
		} else if err := a.saveStoredToken(token); err != nil {
			http.Error(w, "token save failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	now := time.Now().UTC()
	from, _, err := syncRequestWindow(bankSyncModeInitialYear, a.cfg.From, now)
	if err != nil {
		http.Error(w, "sync window failed", http.StatusInternalServerError)
		return
	}
	userID, hasUser := a.currentUserID(r)
	var syncStore *bankSyncStore
	var syncRun bankSyncRun
	if hasUser && a.db != nil {
		syncStore, syncRun, err = a.startUserBankSync(ctx, userID, bankSyncModeInitialYear, a.cfg.From, now)
		if err != nil {
			http.Error(w, "sync start failed", http.StatusInternalServerError)
			return
		}
	}
	result, err := a.fetchDemoResultWithOptions(ctx, token.AccessToken, from, false)
	if syncRun.ID != 0 {
		result.SyncRunID = strconv.FormatUint(syncRun.ID, 10)
		if finishErr := syncStore.finishRun(ctx, userID, syncRun.ID, result, err); finishErr != nil {
			http.Error(w, "sync status save failed", http.StatusInternalServerError)
			return
		}
	}
	if err != nil {
		http.Error(w, "data fetch failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	if err := a.appendDemoResultLog(result); err != nil {
		http.Error(w, "log write failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if hasUser && a.db != nil {
		if err := newTransactionService(a.db).ingestDemoResult(ctx, userID, result); err != nil {
			http.Error(w, "transaction ingest failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if a.bankConnections != nil && syncResultHasSuccessfulAccount(result) {
			_ = a.bankConnections.markLastSync(ctx, userID)
		}
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

	var refreshToken string
	if userID, ok := a.currentUserID(r); ok && a.bankConnections != nil {
		var err error
		refreshToken, err = a.bankConnections.loadRefreshToken(r.Context(), userID)
		if err != nil {
			http.Redirect(w, r, "/billing?reconnect=1&error=no_saved_login", http.StatusFound)
			return
		}
	} else {
		stored, err := a.loadStoredToken()
		if err != nil {
			http.Redirect(w, r, "/billing?reconnect=1&error=no_saved_login", http.StatusFound)
			return
		}
		refreshToken = stored.RefreshToken
	}

	token, err := a.refreshAccessToken(r.Context(), refreshToken)
	if err != nil {
		http.Redirect(w, r, "/billing?reconnect=1&error=refresh_failed", http.StatusFound)
		return
	}
	if token.RefreshToken != "" && token.RefreshToken != refreshToken {
		if userID, ok := a.currentUserID(r); ok && a.bankConnections != nil {
			if err := a.bankConnections.saveRefreshToken(r.Context(), userID, token.RefreshToken); err != nil {
				http.Error(w, "token save failed: "+err.Error(), http.StatusInternalServerError)
				return
			}
		} else if err := a.saveStoredToken(token); err != nil {
			http.Error(w, "token save failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	now := time.Now().UTC()
	from, _, err := syncRequestWindow(bankSyncModeRefresh90d, a.cfg.From, now)
	if err != nil {
		http.Redirect(w, r, "/billing?error=data_fetch_failed", http.StatusFound)
		return
	}
	userID, hasUser := a.currentUserID(r)
	var syncStore *bankSyncStore
	var syncRun bankSyncRun
	if hasUser && a.db != nil {
		syncStore, syncRun, err = a.startUserBankSync(r.Context(), userID, bankSyncModeRefresh90d, a.cfg.From, now)
		if err != nil {
			http.Error(w, "sync start failed", http.StatusInternalServerError)
			return
		}
	}
	result, err := a.fetchDemoResultWithOptions(r.Context(), token.AccessToken, from, true)
	if syncRun.ID != 0 {
		result.SyncRunID = strconv.FormatUint(syncRun.ID, 10)
		if finishErr := syncStore.finishRun(r.Context(), userID, syncRun.ID, result, err); finishErr != nil {
			http.Error(w, "sync status save failed", http.StatusInternalServerError)
			return
		}
	}
	if err != nil {
		http.Redirect(w, r, "/billing?error=data_fetch_failed", http.StatusFound)
		return
	}
	if err := a.appendDemoResultLog(result); err != nil {
		http.Error(w, "log write failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if hasUser && a.db != nil {
		if err := newTransactionService(a.db).ingestDemoResult(r.Context(), userID, result); err != nil {
			http.Redirect(w, r, "/billing?error=data_fetch_failed", http.StatusFound)
			return
		}
		if a.bankConnections != nil && syncResultHasSuccessfulAccount(result) {
			_ = a.bankConnections.markLastSync(r.Context(), userID)
		}
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
	if len(parts) == 5 && parts[0] == "v2" {
		expiresUnix, err := strconv.ParseInt(parts[3], 10, 64)
		if err != nil || time.Now().After(time.Unix(expiresUnix, 0)) {
			return false
		}
		want := userSessionSignature(a.cfg, parts[1], parts[2], parts[3])
		return hmac.Equal([]byte(parts[4]), []byte(want))
	}
	if len(parts) != 3 || parts[0] != a.cfg.AdminUsername {
		return false
	}
	expiresUnix, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || time.Now().After(time.Unix(expiresUnix, 0)) {
		return false
	}
	want := sessionSignature(a.cfg, parts[0], parts[1])
	return hmac.Equal([]byte(parts[2]), []byte(want))
}

func (a *app) currentUserID(r *http.Request) (uint64, bool) {
	c, err := r.Cookie("rentops_session")
	if err != nil {
		return 0, false
	}
	parts := strings.Split(c.Value, "|")
	if len(parts) != 5 || parts[0] != "v2" {
		return 0, false
	}
	expiresUnix, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil || time.Now().After(time.Unix(expiresUnix, 0)) {
		return 0, false
	}
	want := userSessionSignature(a.cfg, parts[1], parts[2], parts[3])
	if !hmac.Equal([]byte(parts[4]), []byte(want)) {
		return 0, false
	}
	userID, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil || userID == 0 {
		return 0, false
	}
	return userID, true
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
	successfulTransactionAccounts := 0
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
		} else {
			item.Transactions = txs
			successfulTransactionAccounts++
		}

		result.Accounts = append(result.Accounts, item)
	}
	if failOnTransactionError && len(result.Accounts) > 0 && successfulTransactionAccounts == 0 {
		return result, fmt.Errorf("transactions fetch failed")
	}
	return result, nil
}

// refreshTransactionFrom returns the earliest `from` date (inclusive) the bank
// accepts for a refresh. `from` is date-only (start of UTC day) while `to` is
// the current instant, and TrueLayer's Irish providers reject any window that
// reaches further back than refreshLookbackDays from `now`. Cutting exactly
// refreshLookbackDays from the start of `today` would still overshoot by the
// time elapsed so far today (producing e.g. 91 days back and a 403 SCA Active
// check failed), so we subtract one extra day to stay strictly inside the
// allowed lookback for any time of day.
func refreshTransactionFrom(configuredFrom string, now time.Time) string {
	today := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
	cutoff := today.AddDate(0, 0, -(refreshLookbackDays - 1))
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

func parsePositiveAmount(value string) (float64, error) {
	amount, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return 0, err
	}
	if amount <= 0 {
		return 0, errors.New("amount must be positive")
	}
	return amount, nil
}

func recordID(prefix string, now time.Time) string {
	suffix, err := randomState()
	if err != nil {
		suffix = strconv.FormatInt(now.UnixNano(), 36)
	}
	if len(suffix) > 10 {
		suffix = suffix[:10]
	}
	return prefix + "-" + now.Format("20060102150405") + "-" + suffix
}

func prepareTenants(rows []tenantRecord) {
	for i := range rows {
		rows[i].Currency = firstNonEmpty(rows[i].Currency, "EUR")
		rows[i].RentDisplay = formatMoney(rows[i].MonthlyRent, rows[i].Currency, 2)
	}
}

func prepareExpenses(rows []expenseRecord) {
	for i := range rows {
		rows[i].Currency = firstNonEmpty(rows[i].Currency, "EUR")
		rows[i].AmountDisplay = formatMoney(rows[i].Amount, rows[i].Currency, 2)
		rows[i].DateDisplay = formatDate(rows[i].ExpenseDate)
	}
}

func formatDate(value string) string {
	if value == "" {
		return "Unknown"
	}
	if d, err := time.Parse(dateLayout, value); err == nil {
		return d.Format("02 Jan 2006")
	}
	return value
}

func sumTenantRent(rows []tenantRecord) float64 {
	var total float64
	for _, row := range rows {
		total += row.MonthlyRent
	}
	return total
}

func sumExpenses(rows []expenseRecord) float64 {
	var total float64
	for _, row := range rows {
		total += row.Amount
	}
	return total
}

func (a *app) loadTenants() ([]tenantRecord, error) {
	var rows []tenantRecord
	if err := readJSONFile(a.cfg.TenantFile, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func (a *app) saveTenants(rows []tenantRecord) error {
	return writeJSONFile(a.cfg.TenantFile, rows)
}

func (a *app) loadExpenses() ([]expenseRecord, error) {
	var rows []expenseRecord
	if err := readJSONFile(a.cfg.ExpenseFile, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func (a *app) saveExpenses(rows []expenseRecord) error {
	return writeJSONFile(a.cfg.ExpenseFile, rows)
}

func readJSONFile(path string, out any) error {
	if path == "" {
		return nil
	}
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil
	}
	return json.Unmarshal(body, out)
}

func writeJSONFile(path string, value any) error {
	if path == "" {
		return nil
	}
	dir := filepath.Dir(path)
	if dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	if err := os.WriteFile(path, body, 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
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
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>RentOps Login</title>
  <style>
    :root {
      --background-deep: #020203;
      --background-base: #050506;
      --foreground: #ededef;
      --foreground-muted: #8a8f98;
      --foreground-subtle: rgba(255,255,255,0.62);
      --accent: #5e6ad2;
      --accent-bright: #6872d9;
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
      display: grid;
      place-items: center;
      padding: 24px;
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
      width: 820px;
      height: 640px;
      left: 12%;
      top: -300px;
      background: rgba(94,106,210,0.26);
    }
    .blob.secondary {
      width: 480px;
      height: 700px;
      right: -200px;
      top: 200px;
      background: rgba(151,83,210,0.16);
      animation-delay: -2s;
    }
    .blob.tertiary {
      width: 560px;
      height: 560px;
      left: -220px;
      bottom: -140px;
      background: rgba(74,112,255,0.13);
      animation-delay: -4s;
    }
    @keyframes float {
      0%, 100% { transform: translateY(0) rotate(0deg); }
      50% { transform: translateY(-20px) rotate(1deg); }
    }
    .shell {
      position: relative;
      z-index: 1;
      width: min(424px, 100%);
      padding: 30px;
      border: 1px solid var(--border);
      border-radius: 24px;
      background: linear-gradient(180deg, rgba(255,255,255,0.075), rgba(255,255,255,0.026));
      box-shadow: var(--shadow-card);
      backdrop-filter: blur(24px);
    }
    .brand { display: flex; align-items: center; gap: 12px; margin-bottom: 30px; }
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
    h1 {
      margin: 0;
      font-size: 28px;
      font-weight: 650;
      letter-spacing: -0.03em;
    }
    .sub {
      margin: 10px 0 0;
      color: var(--foreground-muted);
      font-size: 13.5px;
      line-height: 1.6;
    }
    label {
      display: block;
      margin: 18px 0 7px;
      color: var(--foreground-muted);
      font: 700 11px var(--mono);
      text-transform: uppercase;
      letter-spacing: 0.12em;
    }
    input {
      width: 100%;
      height: 44px;
      border: 1px solid var(--border);
      border-radius: 12px;
      padding: 0 13px;
      color: var(--foreground);
      background: rgba(255,255,255,0.05);
      box-shadow: inset 0 1px 0 rgba(255,255,255,0.08);
      outline: none;
      transition: border-color 200ms var(--ease), box-shadow 200ms var(--ease), background 200ms var(--ease);
    }
    input::placeholder { color: var(--foreground-subtle); }
    input:hover { border-color: var(--border-hover); }
    input:focus {
      border-color: rgba(104,114,217,0.85);
      background: rgba(255,255,255,0.07);
      box-shadow: inset 0 1px 0 rgba(255,255,255,0.10), 0 0 0 3px rgba(94,106,210,0.22);
    }
    input:-webkit-autofill,
    input:-webkit-autofill:hover,
    input:-webkit-autofill:focus {
      -webkit-text-fill-color: var(--foreground);
      -webkit-box-shadow: 0 0 0 1000px #0a0a0c inset;
      caret-color: var(--foreground);
    }
    button {
      width: 100%;
      height: 44px;
      margin-top: 22px;
      border: 0;
      border-radius: 12px;
      background: var(--accent);
      color: #fff;
      font: inherit;
      font-weight: 650;
      box-shadow: var(--shadow-button);
      cursor: pointer;
      transition: background 220ms var(--ease), transform 220ms var(--ease), box-shadow 220ms var(--ease);
    }
    button:hover {
      background: var(--accent-bright);
      box-shadow: 0 0 0 1px rgba(104,114,217,0.65), 0 12px 34px rgba(94,106,210,0.36), inset 0 1px 0 rgba(255,255,255,0.24);
      transform: translateY(-1px);
    }
    button:active { transform: scale(0.99); }
    button:focus-visible { outline: 2px solid rgba(104,114,217,0.9); outline-offset: 3px; }
    .error {
      margin-top: 18px;
      padding: 12px 14px;
      border: 1px solid rgba(255,139,134,0.24);
      border-radius: 14px;
      background: rgba(255,139,134,0.08);
      color: #ffd0ce;
      font-size: 13px;
      line-height: 1.6;
    }
    .hint {
      margin-top: 18px;
      color: var(--foreground-muted);
      font: 500 11px/1.7 var(--mono);
    }
    @media (max-width: 520px) {
      body { padding: 16px; }
      .shell { padding: 24px; }
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
  <main class="shell">
    <div class="brand">
      <div class="mark">R</div>
      <div>
        <div class="brand-title">RentOps</div>
        <div class="brand-meta">Bank income workspace</div>
      </div>
    </div>
    <h1>登录</h1>
    <p class="sub">使用演示管理员账号进入银行收入工作台。</p>
    {{if .Error}}<div class="error">用户名或密码错误。</div>{{end}}
    <form method="post" action="/login-local">
      <label for="username">用户名</label>
      <input id="username" name="username" autocomplete="username" value="{{.Username}}">
      <label for="password">密码</label>
      <input id="password" name="password" type="password" autocomplete="current-password">
      <button type="submit">登录</button>
    </form>
    <div class="hint">登录账号由 APP_ADMIN_USERNAME 和 APP_ADMIN_PASSWORD 配置。</div>
  </main>
</body>
</html>
`))

const workspacePageCSS = `
    :root {
      --background-deep: #020203;
      --background-base: #050506;
      --foreground: #ededef;
      --foreground-muted: #8a8f98;
      --foreground-subtle: rgba(255,255,255,0.66);
      --accent: #5e6ad2;
      --accent-bright: #6872d9;
      --positive: #7dd3a8;
      --danger: #ff8b86;
      --warning: #e9b872;
      --border: rgba(255,255,255,0.08);
      --shadow-card: 0 0 0 1px rgba(255,255,255,0.06), 0 20px 70px rgba(0,0,0,0.48);
      --sans: "Inter", "Geist Sans", "Aptos", "Segoe UI", system-ui, sans-serif;
      --mono: "IBM Plex Mono", "SFMono-Regular", Consolas, monospace;
    }
    * { box-sizing: border-box; }
    html { background: var(--background-deep); }
    body {
      margin: 0;
      min-height: 100vh;
      color: var(--foreground);
      font-family: var(--sans);
      background: radial-gradient(ellipse at top, #111225 0%, var(--background-base) 48%, var(--background-deep) 100%);
    }
    button, input, select, textarea { font: inherit; }
    a { color: inherit; }
    .app {
      width: min(1480px, 100%);
      min-height: 100vh;
      margin: 0 auto;
      padding: 24px;
      display: grid;
      grid-template-columns: 236px minmax(0, 1fr);
      gap: 0;
    }
    .sidebar,
    .content {
      border: 1px solid var(--border);
      background: linear-gradient(180deg, rgba(255,255,255,0.075), rgba(255,255,255,0.026));
      box-shadow: var(--shadow-card);
      backdrop-filter: blur(24px);
    }
    .sidebar {
      border-radius: 24px 0 0 24px;
      border-right: 0;
      padding: 18px;
      display: flex;
      flex-direction: column;
      gap: 20px;
    }
    .content {
      min-width: 0;
      border-radius: 0 24px 24px 0;
      padding: 24px;
    }
    .side-brand {
      display: flex;
      align-items: center;
      gap: 12px;
      padding-bottom: 16px;
      border-bottom: 1px solid var(--border);
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
      box-shadow: 0 0 0 1px rgba(94,106,210,0.50), 0 8px 26px rgba(94,106,210,0.28), inset 0 1px 0 rgba(255,255,255,0.22);
    }
    .brand-title { font-weight: 650; letter-spacing: 0; }
    .brand-meta,
    .label {
      color: var(--foreground-muted);
      font: 700 11px var(--mono);
      text-transform: uppercase;
      letter-spacing: 0.10em;
    }
    .nav { display: grid; gap: 7px; }
    .nav a {
      min-height: 40px;
      border-radius: 10px;
      padding: 0 11px;
      display: grid;
      grid-template-columns: 24px 1fr auto;
      gap: 9px;
      align-items: center;
      color: var(--foreground-subtle);
      text-decoration: none;
      font-size: 13px;
    }
    .nav a:hover,
    .nav a.active {
      color: var(--foreground);
      background: rgba(255,255,255,0.075);
    }
    .glyph {
      width: 20px;
      height: 20px;
      border-radius: 7px;
      display: grid;
      place-items: center;
      color: #d8dcff;
      background: rgba(104,114,217,0.18);
      font: 700 11px var(--mono);
    }
    .nav-count {
      min-width: 22px;
      height: 21px;
      border-radius: 999px;
      display: grid;
      place-items: center;
      padding: 0 6px;
      color: var(--foreground-muted);
      background: rgba(255,255,255,0.06);
      font: 700 11px var(--mono);
    }
    .side-foot {
      margin-top: auto;
      padding-top: 16px;
      border-top: 1px solid var(--border);
      color: var(--foreground-muted);
      font-size: 12px;
      line-height: 1.55;
      overflow-wrap: anywhere;
    }
    .topbar {
      display: flex;
      justify-content: space-between;
      align-items: flex-start;
      gap: 16px;
      margin-bottom: 18px;
    }
    h1 {
      margin: 8px 0 0;
      font-size: clamp(32px, 5vw, 56px);
      line-height: 1;
      letter-spacing: 0;
      font-weight: 650;
    }
    h2 { margin: 0; font-size: 17px; letter-spacing: 0; }
    .btn {
      min-height: 38px;
      border: 0;
      border-radius: 10px;
      padding: 0 14px;
      display: inline-flex;
      align-items: center;
      justify-content: center;
      color: var(--foreground);
      background: rgba(255,255,255,0.055);
      box-shadow: inset 0 1px 0 rgba(255,255,255,0.10), 0 0 0 1px rgba(255,255,255,0.08);
      cursor: pointer;
      text-decoration: none;
    }
    .btn.primary { margin-top: 16px; background: var(--accent); color: white; }
    .btn.danger { color: #ffd8d6; background: rgba(255,139,134,0.08); }
    .summary {
      display: grid;
      grid-template-columns: repeat(2, minmax(0, 1fr));
      gap: 12px;
      margin-bottom: 18px;
    }
    .panel {
      border: 1px solid var(--border);
      border-radius: 18px;
      background: linear-gradient(180deg, rgba(255,255,255,0.078), rgba(255,255,255,0.026));
      box-shadow: var(--shadow-card);
    }
    .metric { padding: 18px; min-height: 118px; }
    .metric strong {
      display: block;
      margin-top: 12px;
      font-size: clamp(26px, 3vw, 38px);
      line-height: 1;
      font-weight: 650;
    }
    .metric span,
    .tiny {
      display: block;
      margin-top: 8px;
      color: var(--foreground-muted);
      font-size: 12px;
      line-height: 1.55;
    }
    .notice {
      margin-bottom: 14px;
      padding: 13px 15px;
      border-radius: 14px;
      border: 1px solid var(--border);
      color: var(--foreground-subtle);
      background: rgba(255,255,255,0.045);
    }
    .notice.error { border-color: rgba(255,139,134,0.24); background: rgba(255,139,134,0.08); color: #ffd0ce; }
    .notice.ok { border-color: rgba(125,211,168,0.26); background: rgba(125,211,168,0.08); color: #cbffe1; }
    .grid-two {
      display: grid;
      grid-template-columns: 360px minmax(0, 1fr);
      gap: 18px;
      align-items: start;
    }
    .form { padding: 18px; }
    .panel-head {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 12px;
      margin-bottom: 16px;
    }
    label {
      display: block;
      margin: 13px 0 6px;
      color: var(--foreground-muted);
      font-size: 12px;
      font-weight: 700;
    }
    input, select, textarea {
      width: 100%;
      min-height: 40px;
      border: 1px solid rgba(255,255,255,0.12);
      border-radius: 10px;
      padding: 9px 11px;
      color: var(--foreground);
      background: rgba(255,255,255,0.055);
      outline: none;
      color-scheme: dark;
    }
    select {
      appearance: none;
      padding-right: 34px;
      background-image: linear-gradient(45deg, transparent 50%, var(--foreground-muted) 50%), linear-gradient(135deg, var(--foreground-muted) 50%, transparent 50%);
      background-position: calc(100% - 16px) 17px, calc(100% - 11px) 17px;
      background-size: 5px 5px, 5px 5px;
      background-repeat: no-repeat;
    }
    input[type="date"], input[type="month"] {
      color-scheme: dark;
    }
    input[type="date"]::-webkit-calendar-picker-indicator,
    input[type="month"]::-webkit-calendar-picker-indicator {
      margin-right: -3px;
      padding: 5px;
      border-radius: 7px;
      background-color: rgba(255,255,255,0.08);
      opacity: 0.72;
      filter: invert(1);
      cursor: pointer;
    }
    textarea { resize: vertical; }
    input:focus, select:focus, textarea:focus {
      border-color: rgba(104,114,217,0.85);
      box-shadow: 0 0 0 3px rgba(104,114,217,0.18);
    }
    .surface { overflow: hidden; }
    .surface .panel-head {
      margin: 0;
      padding: 18px 20px;
      border-bottom: 1px solid var(--border);
      background: rgba(5,5,6,0.42);
    }
    .table-wrap { overflow-x: auto; }
    table {
      width: 100%;
      min-width: 760px;
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
    .amount {
      color: var(--foreground);
      font-size: 15px;
      font-weight: 700;
      font-variant-numeric: tabular-nums;
      white-space: nowrap;
    }
    .amount.expense { color: #ffd0ce; }
    .mono {
      font-family: var(--mono);
      font-size: 11px;
      color: var(--foreground-muted);
      overflow-wrap: anywhere;
    }
    .empty {
      padding: 64px 20px;
      color: var(--foreground-muted);
      text-align: center;
      line-height: 1.7;
    }
    @media (max-width: 980px) {
      .app { grid-template-columns: 1fr; padding: 12px; }
      .sidebar { border-radius: 18px 18px 0 0; border-right: 1px solid var(--border); border-bottom: 0; }
      .content { border-radius: 0 0 18px 18px; }
      .grid-two { grid-template-columns: 1fr; }
    }
    @media (max-width: 640px) {
      .content { padding: 14px; }
      .topbar { flex-direction: column; }
      .summary { grid-template-columns: 1fr; }
      h1 { font-size: 34px; }
      table { min-width: 720px; }
    }
`

var tenantTemplate = template.Must(template.New("tenants").Parse(`<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>RentOps Tenants</title>
  <style>` + workspacePageCSS + workspaceCalendarCSS + `
    .tenant-row, .tenant-month-row { cursor: pointer; }
    .tenant-row:hover, .tenant-row:focus, .tenant-month-row:hover, .tenant-month-row:focus { background: rgba(255,255,255,0.035); outline: none; }
    .tenant-row td:first-child::after, .tenant-month-row td:first-child::after { content: " +"; margin-left: 6px; color: var(--foreground-muted); font: 700 12px var(--mono); }
    .tenant-row[aria-expanded="true"] td:first-child::after, .tenant-month-row[aria-expanded="true"] td:first-child::after { content: " -"; }
    .status { display: inline-block; border-radius: 999px; padding: 5px 9px; font-size: 12px; white-space: nowrap; }
    .status.open { color: #d8dcff; background: rgba(104,114,217,0.12); }
    .status.overdue, .status.needs_review { color: #ffd0ce; background: rgba(255,139,134,0.10); }
    .status.partial { color: #ffe0a7; background: rgba(233,184,114,0.10); }
    .status.paid { color: #cbffe1; background: rgba(125,211,168,0.10); }
    .tenant-history-row td, .tenant-month-details td { padding: 0; background: rgba(255,255,255,0.025); }
    .tenant-history { padding: 14px 18px 16px 32px; border-top: 1px solid rgba(255,255,255,0.045); }
    .tenant-history-head { display: flex; align-items: baseline; justify-content: space-between; gap: 12px; margin-bottom: 10px; }
    .tenant-history-head h3 { margin: 0; font-size: 13px; }
    .tenant-history-table { min-width: 620px; background: rgba(255,255,255,0.018); }
    .tenant-history-table th { padding: 10px 12px; }
    .tenant-history-table td { padding: 12px; }
    .tenant-month-details .payment-list { padding: 12px 16px 14px 28px; }
    .tenant-month-details .payment-item { display: grid; grid-template-columns: 140px 170px minmax(180px, 1fr) minmax(160px, 1fr) 100px; gap: 12px; padding: 9px 0; border-bottom: 1px solid rgba(255,255,255,0.04); color: var(--foreground-subtle); font-size: 12px; }
    .tenant-month-details .payment-item:last-child { border-bottom: 0; }
    .tenant-month-details .payment-item .amount { font-size: 13px; }
    @media (max-width: 760px) { .tenant-month-details .payment-item { grid-template-columns: 1fr 1fr; } .tenant-month-details .payment-item .payment-description { grid-column: 1 / -1; } }
  ` + `</style>
  <script>` + workspaceCalendarScript + `</script>
</head>
<body>
  <div class="app">
    <aside class="sidebar" aria-label="Main navigation">
      <div class="side-brand"><div class="mark">R</div><div><div class="brand-title">RentOps</div><div class="brand-meta">{{.Environment}} workspace</div></div></div>
      <nav class="nav">
        <a href="/rent-dashboard"><span class="glyph">总</span><span>月度总览</span><span class="nav-count">{{.TenantCount}}</span></a>
        <a href="/billing"><span class="glyph">流</span><span>银行流水</span><span class="nav-count">{{.IncomeCount}}</span></a>
        <a href="/tenants" class="active"><span class="glyph">租</span><span>租客管理</span><span class="nav-count">{{.TenantCount}}</span></a>
        <a href="/expenses"><span class="glyph">支</span><span>支出记录</span><span class="nav-count">{{.ExpenseCount}}</span></a>
      </nav>
      <div class="side-foot">当前用户：{{.Username}}<br>租客资料：{{.TenantFile}}</div>
    </aside>
    <main class="content">
      <header class="topbar">
        <div>
          <div class="brand-title">租客管理</div>
          <h1>租客资料</h1>
        </div>
        <div class="actions"><a class="btn primary" href="/tenants?add=1">添加租客</a><form method="post" action="/logout"><button class="btn danger" type="submit">退出登录</button></form></div>
      </header>

      {{if eq .Message "tenant_added"}}<div class="notice ok">租客资料已保存。</div>{{end}}
      {{if eq .Message "tenant_updated"}}<div class="notice ok">租客资料已更新。</div>{{end}}
		{{if eq .Error "tenant_has_payments"}}<div class="notice error">无法提前结束租期：结束月之后的账单已有有效收款，请先更正或撤销相关收款。</div>{{else if .Error}}<div class="notice error">请检查租客姓名、邮箱格式、日期关系、月租金额和房间地址。</div>{{end}}

      <section class="summary" aria-label="Tenant summary">
        <div class="panel metric"><div class="label">租客数量</div><strong>{{.TenantCount}}</strong><span>当前保存的租客</span></div>
        <div class="panel metric"><div class="label">月租合计</div><strong>{{.RentTotal}}</strong><span>租客资料中的预期月租</span></div>
      </section>

      {{if .ShowForm}}<section class="panel form tenant-form" aria-labelledby="tenant-form-title">
        <div class="panel-head"><h2 id="tenant-form-title">{{if .Editing}}编辑租客{{else}}添加租客{{end}}</h2><a class="btn subtle" href="/tenants">取消</a></div>
        <form method="post" action="/tenants">
          {{if .Editing}}<input type="hidden" name="tenant_id" value="{{.Form.ID}}">{{end}}
          <label for="name">租客姓名</label>
          <input id="name" name="name" autocomplete="name" value="{{.Form.Name}}" required>
          <label for="display_alias">显示别名</label>
          <input id="display_alias" name="display_alias" value="{{.Form.DisplayAlias}}" placeholder="列表优先显示的称呼">
          <label for="email">邮箱</label>
          <input id="email" name="email" type="email" autocomplete="email" value="{{.Form.Email}}" placeholder="tenant@example.com">
          <label for="payer_id">银行付款人编号</label>
          <input id="payer_id" name="payer_id" value="{{.Form.PayerID}}">
          <label for="payer_name_hint">付款人名称提示</label>
          <input id="payer_name_hint" name="payer_name_hint" value="{{.Form.PayerNameHint}}">
          <label for="monthly_rent">月租金额</label>
          <input id="monthly_rent" name="monthly_rent" type="number" min="0.01" step="0.01" inputmode="decimal" value="{{.Form.MonthlyRent}}" required>
          <label for="currency">币种</label>
          <input id="currency" name="currency" value="{{.Form.Currency}}" maxlength="3">
          <input type="hidden" name="interval_unit" value="month">
          <input type="hidden" name="interval_count" value="1">
          <label for="billing_start_date">计费开始日期</label>
          <input id="billing_start_date" name="billing_start_date" type="date" value="{{.Form.BillingStartDate}}"><span class="tiny">留空则使用租期开始日期；如需延后，请填写租期内日期。</span>
          <label for="due_day">每月应缴日</label>
          <input id="due_day" name="due_day" type="number" min="1" max="31" value="{{.Form.DueDay}}" required>
          <label for="rent_start_date">租期开始日期</label>
          <input id="rent_start_date" name="rent_start_date" type="date" value="{{.Form.RentStartDate}}" required>
          <label for="rent_end_date">租期结束日期</label>
          <input id="rent_end_date" name="rent_end_date" type="date" value="{{.Form.RentEndDate}}">
          <label for="status">状态</label>
          <select id="status" name="status"><option value="active" {{if eq .Form.Status "active"}}selected{{end}}>有效</option><option value="inactive" {{if eq .Form.Status "inactive"}}selected{{end}}>停用</option></select>
          <label for="room_label">房间名称</label>
          <input id="room_label" name="room_label" value="{{.Form.RoomLabel}}">
          <label for="room_address">房间地址</label>
          <textarea id="room_address" name="room_address" rows="4" required>{{.Form.RoomAddress}}</textarea>
          <label for="property_hint">房产备注</label>
          <input id="property_hint" name="property_hint" value="{{.Form.PropertyHint}}">
          <button class="btn primary" type="submit">{{if .Editing}}保存修改{{else}}保存租客{{end}}</button>
        </form>
      </section>{{end}}
      <section class="panel surface" aria-labelledby="tenant-list-title">
        <div class="panel-head"><h2 id="tenant-list-title">租客列表</h2><span class="tiny">{{.TenantCount}} 条记录</span></div>
        {{if .Rows}}
        <div class="table-wrap">
          <table>
            <thead><tr><th>租客</th><th>付款人</th><th>租金</th><th>账单安排</th><th>房间</th><th>创建时间</th><th></th></tr></thead>
            <tbody>
              {{range .Rows}}
              <tr class="tenant-row" tabindex="0" role="button" aria-expanded="false" aria-controls="tenant-billing-{{.ID}}" data-tenant-target="tenant-billing-{{.ID}}">
                <td><strong>{{if .DisplayAlias}}{{.DisplayAlias}}{{else}}{{.Name}}{{end}}</strong><br>{{if .DisplayAlias}}<span class="tiny">{{.Name}}</span><br>{{end}}<span class="mono">{{.ID}}</span></td>
                <td>{{if .PayerNameHint}}{{.PayerNameHint}}{{else}}暂无付款人别名{{end}}<br><span class="mono">编号：{{if .PayerID}}{{.PayerID}}{{else}}暂无{{end}}</span></td>
                <td class="amount">{{.RentDisplay}}</td>
                <td>每月 {{.DueDay}} 日<br><span class="mono">{{.RentStartDate}}{{if .RentEndDate}} 至 {{.RentEndDate}}{{end}}</span></td>
                <td>{{if .RoomLabel}}{{.RoomLabel}}<br>{{end}}{{.RoomAddress}}{{if .PropertyHint}}<br><span class="mono">{{.PropertyHint}}</span>{{end}}</td>
                <td class="mono">{{.CreatedAt}}</td>
                <td><a class="btn subtle" href="/tenants/{{.ID}}">详情</a><br><a class="btn subtle" href="/tenants?edit={{.ID}}">编辑</a></td>
              </tr>
              <tr id="tenant-billing-{{.ID}}" class="tenant-history-row" hidden><td colspan="7"><div class="tenant-history">
                <div class="tenant-history-head"><h3>最近三个月缴费</h3><span class="tiny">应收账单 → 已确认流水</span></div>
                {{if .BillingHistory}}
                <div class="table-wrap"><table class="tenant-history-table">
                  <thead><tr><th>月份</th><th>应收</th><th>实际收</th><th>未收</th><th>状态</th></tr></thead>
                  <tbody>{{range .BillingHistory}}
                    <tr class="tenant-month-row" tabindex="0" role="button" aria-expanded="false" aria-controls="tenant-month-{{.ObligationID}}" data-month-target="tenant-month-{{.ObligationID}}"><td><strong>{{.PeriodLabel}}</strong><br><span class="mono">应缴日 {{.DueDate}}</span></td><td class="amount">{{.ExpectedAmount}}</td><td class="amount">{{.PaidAmount}}</td><td class="amount">{{.BalanceAmount}}</td><td><span class="status {{.Status}}">{{.StatusLabel}}</span></td></tr>
                    <tr id="tenant-month-{{.ObligationID}}" class="tenant-month-details" hidden><td colspan="5"><div class="payment-list"><h3>缴费流水明细</h3>{{if .Payments}}{{range .Payments}}<div class="payment-item"><span class="amount">{{.AmountDisplay}}</span><span class="mono">{{.DateDisplay}}</span><span class="payment-description">{{.Description}}</span><span class="mono">参考号：{{.Reference}}</span><span class="mono">{{.ConfirmationSource}}</span></div>{{end}}{{else}}<div class="tiny">该月暂无已确认缴费。</div>{{end}}</div></td></tr>
                  {{end}}</tbody>
                </table></div>
                {{else}}<div class="empty">最近三个月暂无适用账单。</div>{{end}}
              </div></td></tr>
              {{end}}
            </tbody>
          </table>
        </div>
        {{else}}<div class="empty">还没有租客，请点击“添加租客”创建第一条资料。</div>{{end}}
      </section>
    </main>
  </div>
  <script>
    const toggleDetails = (toggle, details) => {
      const expanded = toggle.getAttribute("aria-expanded") === "true";
      toggle.setAttribute("aria-expanded", String(!expanded));
      details.hidden = expanded;
    };
    for (const row of document.querySelectorAll("[data-tenant-target]")) {
      const details = document.getElementById(row.dataset.tenantTarget);
      if (!details) continue;
      row.addEventListener("click", (event) => {
        if (event.target.closest("a, button, input, select, textarea")) return;
        toggleDetails(row, details);
      });
      row.addEventListener("keydown", (event) => {
        if (event.key === "Enter" || event.key === " ") { event.preventDefault(); toggleDetails(row, details); }
      });
    }
    for (const row of document.querySelectorAll("[data-month-target]")) {
      const details = document.getElementById(row.dataset.monthTarget);
      if (!details) continue;
      row.addEventListener("click", (event) => { event.stopPropagation(); toggleDetails(row, details); });
      row.addEventListener("keydown", (event) => {
        event.stopPropagation();
        if (event.key === "Enter" || event.key === " ") { event.preventDefault(); toggleDetails(row, details); }
      });
    }
  </script>
</body>
</html>
`))

var expenseTemplate = template.Must(template.New("expenses").Parse(`<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>RentOps Expenses</title>
  <style>` + workspacePageCSS + workspaceCalendarCSS + `</style>
  <script>` + workspaceCalendarScript + `</script>
</head>
<body>
  <div class="app">
    <aside class="sidebar" aria-label="Main navigation">
      <div class="side-brand"><div class="mark">R</div><div><div class="brand-title">RentOps</div><div class="brand-meta">{{.Environment}} workspace</div></div></div>
      <nav class="nav">
        <a href="/rent-dashboard"><span class="glyph">总</span><span>月度总览</span><span class="nav-count">{{.TenantCount}}</span></a>
        <a href="/billing"><span class="glyph">流</span><span>银行流水</span><span class="nav-count">{{.IncomeCount}}</span></a>
        <a href="/tenants"><span class="glyph">租</span><span>租客管理</span><span class="nav-count">{{.TenantCount}}</span></a>
        <a href="/expenses" class="active"><span class="glyph">支</span><span>支出记录</span><span class="nav-count">{{.ExpenseCount}}</span></a>
      </nav>
      <div class="side-foot">当前用户：{{.Username}}<br>支出资料：{{.ExpenseFile}}</div>
    </aside>
    <main class="content">
      <header class="topbar">
        <div>
          <div class="brand-title">支出记录</div>
          <h1>房产支出</h1>
        </div>
        <form method="post" action="/logout"><button class="btn danger" type="submit">退出登录</button></form>
      </header>

      {{if eq .Message "expense_added"}}<div class="notice ok">支出记录已保存。</div>{{end}}
      {{if .Error}}<div class="notice error">支出描述和正数金额为必填项。</div>{{end}}

      <section class="summary" aria-label="Expense summary">
        <div class="panel metric"><div class="label">支出笔数</div><strong>{{.ExpenseCount}}</strong><span>手动记录的支出</span></div>
        <div class="panel metric"><div class="label">支出合计</div><strong>{{.ExpenseTotal}}</strong><span>全部已保存支出</span></div>
      </section>

      <section class="grid-two">
        <form class="panel form" method="post" action="/expenses" aria-labelledby="expense-form-title">
          <div class="panel-head"><h2 id="expense-form-title">添加支出</h2><span class="tiny">手动记录</span></div>
          <label for="description">支出描述</label>
          <input id="description" name="description" required>
          <label for="amount">金额</label>
          <input id="amount" name="amount" type="number" min="0.01" step="0.01" inputmode="decimal" required>
          <label for="category">类别</label>
          <select id="category" name="category">
            <option>维修</option>
            <option>水电</option>
            <option>清洁</option>
            <option>保险</option>
            <option>其他</option>
          </select>
          <label for="expense_date">日期</label>
          <input id="expense_date" name="expense_date" type="date">
          <label for="payment_method">付款方式</label>
          <input id="payment_method" name="payment_method" value="手动">
          <label for="room_hint">房间备注</label>
          <input id="room_hint" name="room_hint">
          <label for="tenant_hint">租客备注</label>
          <input id="tenant_hint" name="tenant_hint">
          <input type="hidden" name="currency" value="EUR">
          <button class="btn primary" type="submit">保存支出</button>
        </form>

        <section class="panel surface" aria-labelledby="expense-list-title">
          <div class="panel-head"><h2 id="expense-list-title">支出列表</h2><span class="tiny">{{.ExpenseCount}} 条记录</span></div>
          {{if .Rows}}
          <div class="table-wrap">
            <table>
              <thead><tr><th>描述</th><th>金额</th><th>类别</th><th>日期</th><th>备注</th><th>付款方式</th></tr></thead>
              <tbody>
                {{range .Rows}}
                <tr><td><strong>{{.Description}}</strong><br><span class="mono">{{.ID}}</span></td><td class="amount expense">{{.AmountDisplay}}</td><td>{{.Category}}</td><td>{{.DateDisplay}}</td><td>{{if .RoomHint}}房间：{{.RoomHint}}<br>{{end}}{{if .TenantHint}}租客：{{.TenantHint}}{{else if not .RoomHint}}暂无备注{{end}}</td><td>{{.PaymentMethod}}</td></tr>
                {{end}}
              </tbody>
            </table>
          </div>
          {{else}}<div class="empty">还没有支出记录，请从左侧表单添加第一笔。</div>{{end}}
        </section>
      </section>
    </main>
  </div>
</body>
</html>
`))

var legacyBillingTemplate = template.Must(template.New("billing-legacy").Parse(`<!doctype html>
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
      display: grid;
      grid-template-columns: 236px minmax(0, 1fr);
    }
    .sidebar {
      padding: 18px;
      border-right: 1px solid var(--border);
      background: rgba(5,5,6,0.62);
      display: flex;
      flex-direction: column;
      gap: 20px;
    }
    .side-brand {
      display: flex;
      align-items: center;
      gap: 12px;
      padding-bottom: 16px;
      border-bottom: 1px solid var(--border);
    }
    .nav {
      display: grid;
      gap: 7px;
    }
    .nav a {
      min-height: 40px;
      border-radius: 10px;
      padding: 0 11px;
      display: grid;
      grid-template-columns: 24px 1fr auto;
      gap: 9px;
      align-items: center;
      color: var(--foreground-subtle);
      text-decoration: none;
      font-size: 13px;
      transition: background 180ms var(--ease), color 180ms var(--ease);
    }
    .nav a:hover,
    .nav a.active {
      color: var(--foreground);
      background: rgba(255,255,255,0.075);
    }
    .glyph {
      width: 20px;
      height: 20px;
      border-radius: 7px;
      display: grid;
      place-items: center;
      color: #d8dcff;
      background: rgba(104,114,217,0.18);
      font: 700 11px var(--mono);
    }
    .nav-count {
      min-width: 22px;
      height: 21px;
      border-radius: 999px;
      display: grid;
      place-items: center;
      padding: 0 6px;
      color: var(--foreground-muted);
      background: rgba(255,255,255,0.06);
      font: 700 11px var(--mono);
    }
    .side-foot {
      margin-top: auto;
      padding-top: 16px;
      border-top: 1px solid var(--border);
      color: var(--foreground-muted);
      font-size: 12px;
      line-height: 1.55;
    }
    .pane {
      min-width: 0;
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
      .shell { min-height: calc(100vh - 24px); border-radius: 18px; grid-template-columns: 1fr; }
      .sidebar { border-right: 0; border-bottom: 1px solid var(--border); }
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
      <aside class="sidebar" aria-label="Main navigation">
        <div class="side-brand">
          <div class="mark">R</div>
          <div>
            <div class="brand-title">RentOps</div>
            <div class="brand-meta">{{.Environment}} workspace</div>
          </div>
        </div>
        <nav class="nav">
          <a href="/billing" class="{{if eq .ActivePage "billing"}}active{{end}}"><span class="glyph">IN</span><span>Income</span><span class="nav-count">{{.IncomeCount}}</span></a>
          <a href="/tenants" class="{{if eq .ActivePage "tenants"}}active{{end}}"><span class="glyph">TN</span><span>Tenants</span><span class="nav-count">{{.TenantCount}}</span></a>
          <a href="/expenses" class="{{if eq .ActivePage "expenses"}}active{{end}}"><span class="glyph">EX</span><span>Expenses</span><span class="nav-count">{{.ExpenseCount}}</span></a>
        </nav>
        <div class="side-foot">
          Signed in as {{.Username}}<br>
          Local demo storage only.
        </div>
      </aside>
      <div class="pane">
      <header class="topbar">
        <div>
          <div class="brand-title">Income</div>
          <div class="brand-meta">Bank income workspace</div>
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
