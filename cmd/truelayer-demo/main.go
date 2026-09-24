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
	"sort"
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
	MySQLDSN               string
	DatabaseURL            string
	MigrationsDir          string
	BankTokenEncryptionKey string
	AllowPlaintextTokens   bool
	AdminUsername          string
	AdminPassword          string
	SessionSecret          string
	DunningSMTPHost        string
	DunningSMTPPort        int
	DunningSMTPUsername    string
	DunningSMTPPassword    string
	DunningSMTPFrom        string
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

type transactionListPageData struct {
	workspaceShell
	CanonicalPath      string
	Connected          bool
	NeedsReconnect     bool
	LastSync           string
	Message            string
	Error              string
	TransactionRows    []transactionPageRow
	ExpenseDrawer      *expenseDrawerData
	ExpenseLinkDrawer  *transactionExpenseDrawerData
	CashReceiptDrawer  *cashReceiptDrawerData
	MatchReview        *transactionMatchReviewData
	ExpenseOpenURL     string
	CashReceiptOpenURL string
	TenantOptions      []billingTenantOption
	ArrivalFromFilter  string
	ArrivalToFilter    string
	PayerFilter        string
	TenantFilter       uint64
	DirectionFilter    string
	MatchStatusFilter  string
	PeriodFilter       string
	RentPeriodFilter   string
	AllocationFilter   string
	SortFilter         string
	SortLinks          map[string]tableSortLink
	// MatchStatusSelection is what the 匹配状态 dropdown shows; it can be the
	// synthetic "pending" option even though MatchStatusFilter itself is empty.
	MatchStatusSelection string
	TransactionScope     string
	Page                 int
	PageSize             int
	TotalTransactions    int64
	TotalPages           int
	Pagination           []paginationLink
	PreviousPageURL      string
	NextPageURL          string
	PendingCount         int
	TokenFile            string
}

type billingTenantOption struct {
	ID   uint64
	Name string
}

// tenantOptionsFromRows builds the tenant picker shared by the transactions
// page and the transaction detail page. Callers are expected to have already
// ordered the rows by name.
func tenantOptionsFromRows(rows []tenant) []billingTenantOption {
	options := make([]billingTenantOption, 0, len(rows))
	for _, row := range rows {
		options = append(options, billingTenantOption{ID: row.ID, Name: row.Name})
	}
	return options
}

type billingMonthOption struct {
	Period    string
	Label     string
	Expected  string
	Paid      string
	Remaining string
}

type billingRentMatchOption struct {
	RentObligationID uint64
	TenantID         uint64
	TenantName       string
	Period           string
	PeriodLabel      string
	// Expected is the month's full 应交房租 and Remaining is what is still 未收。
	// 匹配时房东要拿流水金额跟这两个数对一眼，所以两个都带出来：只有未收的话，
	// 一笔部分缴过的月份看起来跟没缴过一样。
	Expected  string
	Remaining string
	Label     string
}

type tenantRecord struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	DisplayAlias  string `json:"display_alias,omitempty"`
	Email         string `json:"email,omitempty"`
	PayerID       string `json:"payer_id,omitempty"`
	PayerNameHint string `json:"payer_name_hint,omitempty"`
	Status        string `json:"status,omitempty"`
	CreatedAt     string `json:"created_at"`

	BillingHistory []tenantBillingMonth `json:"-"`
	ListDetailURL  string               `json:"-"`
	ListEditURL    string               `json:"-"`
}

type expenseRecord struct {
	ID                   string  `json:"id"`
	PropertyID           string  `json:"property_id,omitempty"`
	RoomID               string  `json:"room_id,omitempty"`
	PropertyName         string  `json:"-"`
	RoomLabel            string  `json:"-"`
	Description          string  `json:"description"`
	Category             string  `json:"category"`
	Amount               float64 `json:"amount"`
	Currency             string  `json:"currency"`
	ExpenseDate          string  `json:"expense_date"`
	PaymentMethod        string  `json:"payment_method"`
	RoomHint             string  `json:"room_hint,omitempty"`
	TenantHint           string  `json:"tenant_hint,omitempty"`
	InvoiceURL           string  `json:"invoice_url,omitempty"`
	InvoiceLinked        bool    `json:"-"`
	InvoiceNumber        string  `json:"-"`
	InvoiceVendor        string  `json:"-"`
	InvoiceDateDisplay   string  `json:"-"`
	InvoiceAmountDisplay string  `json:"-"`
	InvoiceDownloadURL   string  `json:"-"`
	InvoiceActionURL     string  `json:"-"`
	CreatedAt            string  `json:"created_at"`

	AmountDisplay string `json:"-"`
	DateDisplay   string `json:"-"`
	PeriodDisplay string `json:"-"`
}

type tenantPageData struct {
	workspaceShell
	PageKey       string
	CanonicalPath string
	Message       string
	Error         string
	Rows          []tenantRecord
	Search        string
	Sort          string
	SortLinks     map[string]tableSortLink
	FilteredCount int
	ActiveCount   int
	ShowForm      bool
	Editing       bool
	Form          tenantRecord
	ReturnURL     string
	PostReturnURL string
	ErrorMessage  string

	TenantProperties            []tenantPropertyAssignmentOption
	TenantRooms                 []tenantRoomAssignmentOption
	TenantAssignmentPropertyID  uint64
	TenantAssignmentRoomID      uint64
	TenantArrangementStartMonth string
	TenantRoomLocked            bool
}

type tenantPropertyAssignmentOption struct {
	ID   uint64
	Name string
}

type tenantRoomAssignmentOption struct {
	ID              uint64
	PropertyID      uint64
	PropertyName    string
	RoomLabel       string
	RentPlanVersion uint64
	PlansJSON       string
}

type tenantRoomPlanWire struct {
	EffectiveFromMonth string                     `json:"effective_from_month"`
	EffectiveToMonth   string                     `json:"effective_to_month,omitempty"`
	MonthlyRentCents   int64                      `json:"monthly_rent_cents"`
	Currency           string                     `json:"currency"`
	DueDay             int                        `json:"due_day"`
	Members            []tenantRoomPlanMemberWire `json:"members"`
}

type tenantRoomPlanMemberWire struct {
	TenantID            uint64 `json:"tenant_id"`
	TenantName          string `json:"tenant_name"`
	ResponsibilityCents int64  `json:"responsibility_cents"`
}

type expensePageData struct {
	workspaceShell
	PageKey       string
	CanonicalPath string
	Message       string
	Error         string
	Rows          []expenseRecord
	FilteredCount int
	Period        string
	StatusFilter  string
	Search        string
	Sort          string
	SortLinks     map[string]tableSortLink
	ShowForm      bool
	ExpenseDrawer *expenseDrawerData
	InvoiceForm   *expenseInvoiceFormView
	ExpenseTotal  string
	Today         string
	Properties    []expensePropertyOption
	Rooms         []expenseRoomOption
}

type expensePropertyOption struct {
	ID   uint64
	Name string
}

type expenseRoomOption struct {
	ID         uint64
	PropertyID uint64
	Label      string
}

type rentDashboardPageData struct {
	workspaceShell
	PageKey                    string
	CanonicalPath              string
	Period                     string
	PeriodLabel                string
	PreviousPeriod             string
	NextPeriod                 string
	Rows                       []rentDashboardRow
	SearchFilter               string
	StatusFilter               string
	SortFilter                 string
	TenantSort                 tableSortLink
	DueSort                    tableSortLink
	AmountSort                 tableSortLink
	StatusSort                 tableSortLink
	Page                       int
	PageSize                   int
	FilteredCount              int
	TotalRows                  int
	TotalPages                 int
	Pagination                 []paginationLink
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
	CollectionPercent          int
	PendingCount               int
	PendingTotal               string
	OtherIncomeTotal           string
	OtherIncomeCount           int
	SyncCoverage               string
	SyncStatus                 string
	LastSuccessfulSyncCoverage string
	Dunning                    dunningDrawerData
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
	AmountCents        int64
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
	dunningMailer   dunningMailDelivery
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
		dunningMailer:   newSMTPDunningDelivery(dunningSMTPConfigFromConfig(cfg)),
	}
	if err := a.auth.seedDefaultUser(context.Background()); err != nil {
		log.Fatal(err)
	}

	mux := newAppMux(a)

	log.Printf("TrueLayer demo listening on http://localhost%s", cfg.Address)
	log.Printf("Redirect URI must be registered in TrueLayer Console: %s", cfg.RedirectURI)
	log.Fatal(http.ListenAndServe(cfg.Address, mux))
}

// newAppMux is the single route registry used by the live server and by
// handler tests. Canonical Figma routes are registered alongside the legacy
// paths so existing bookmarks and form actions remain valid.
func newAppMux(a *app) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/static/", embeddedWebStaticHandler())
	mux.HandleFunc("/", a.handleIndex)
	mux.HandleFunc("/login-local", a.handleLocalLogin)
	mux.HandleFunc("/logout", a.handleLogout)
	mux.HandleFunc("/rent-dashboard", a.handleRentDashboard)
	mux.HandleFunc("/rent-dashboard/settle", a.handleDashboardManualBalance)
	mux.HandleFunc("/bills", a.handleBills)
	mux.HandleFunc("/bills/generate", a.handleBillsGenerate)
	mux.HandleFunc("/bills/settle/preview", a.handleManualBalancePreview)
	mux.HandleFunc("/bills/settle", a.handleDashboardManualBalance)
	// /billing 只转发：银行授权回调落在这里，老书签也指向它。它自己不再渲染页面。
	mux.HandleFunc("/billing", a.handleBillingAlias)
	mux.HandleFunc("/transactions", a.handleTransactions)
	mux.HandleFunc("/transactions/confirm", a.handleRentMatchConfirmation)
	mux.HandleFunc("/transactions/confirm-batch", a.handleRentMatchBatchConfirmation)
	mux.HandleFunc("/transactions/rematch", a.handleTransactionRematch)
	mux.HandleFunc("/transactions/allocate", a.handleTransactionAllocation)
	mux.HandleFunc("/transactions/ignore", a.handleTransactionIgnore)
	mux.HandleFunc("/transactions/defer", a.handleTransactionDefer)
	mux.HandleFunc("/transactions/undefer", a.handleTransactionUndefer)
	mux.HandleFunc("/transactions/restore", a.handleTransactionRestore)
	mux.HandleFunc("/transactions/revoke", a.handleTransactionRevoke)
	mux.HandleFunc("/transactions/revoke-allocation", a.handleTransactionAllocationRevoke)
	mux.HandleFunc("/transactions/expense-link", a.handleTransactionExpenseLink)
	mux.HandleFunc("/transactions/payer/preview", a.handlePayerPreview)
	mux.HandleFunc("/transactions/payer/confirm", a.handlePayerConfirm)
	mux.HandleFunc("/rooms", a.handleRooms)
	mux.HandleFunc("/rooms/", a.handleRoomRoute)
	mux.HandleFunc("/properties", a.handleProperties)
	mux.HandleFunc("/properties/", a.handlePropertyRoute)
	mux.HandleFunc("/more", a.handleMore)
	mux.HandleFunc("/dunning", a.handleDunningPage)
	mux.HandleFunc("/dunning/config", a.handleDunningConfig)
	mux.HandleFunc("/dunning/preview", a.handleDunningPreview)
	mux.HandleFunc("/dunning/send", a.handleDunningSend)
	mux.HandleFunc("/tenants", a.handleTenants)
	mux.HandleFunc("/tenants/", a.handleTenantSubroute)
	mux.HandleFunc("/cash-receipts/new", a.handleCashReceiptNew)
	mux.HandleFunc("/cash-receipts/preview", a.handleCashReceiptPreview)
	mux.HandleFunc("/cash-receipts/void", a.handleCashReceiptVoid)
	mux.HandleFunc("/cash-receipts", a.handleCashReceipts)
	mux.HandleFunc("/expenses", a.handleExpenses)
	mux.HandleFunc("/expenses/invoices", a.handleExpenseInvoice)
	mux.HandleFunc("/expenses/invoices/", a.handleExpenseInvoiceFile)
	mux.HandleFunc("/bank", a.handleBank)
	mux.HandleFunc("/bank/connect", a.handleLogin)
	mux.HandleFunc("/bank/sync", a.handleRefresh)
	mux.HandleFunc("/login", a.handleLogin)
	mux.HandleFunc("/callback", a.handleCallback)
	mux.HandleFunc("/refresh", a.handleRefresh)
	return mux
}

func loadConfig() (config, error) {
	env := strings.ToLower(strings.TrimSpace(getenv("TL_ENV", "sandbox")))
	dunningSMTPPort := 587
	if value := strings.TrimSpace(os.Getenv("DUNNING_SMTP_PORT")); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 65535 {
			return config{}, errors.New("DUNNING_SMTP_PORT must be between 1 and 65535")
		}
		dunningSMTPPort = parsed
	}
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
		MySQLDSN:               strings.TrimSpace(os.Getenv("MYSQL_DSN")),
		DatabaseURL:            strings.TrimSpace(os.Getenv("DATABASE_URL")),
		MigrationsDir:          strings.TrimSpace(getenv("MIGRATIONS_DIR", "migrations")),
		BankTokenEncryptionKey: strings.TrimSpace(os.Getenv("BANK_TOKEN_ENCRYPTION_KEY")),
		AllowPlaintextTokens:   os.Getenv("ALLOW_PLAINTEXT_TOKENS") == "1",
		AdminUsername:          strings.TrimSpace(getenv("APP_ADMIN_USERNAME", "ddrzh")),
		AdminPassword:          getenv("APP_ADMIN_PASSWORD", "ddrzh512"),
		SessionSecret:          strings.TrimSpace(os.Getenv("APP_SESSION_SECRET")),
		DunningSMTPHost:        strings.TrimSpace(os.Getenv("DUNNING_SMTP_HOST")),
		DunningSMTPPort:        dunningSMTPPort,
		DunningSMTPUsername:    strings.TrimSpace(os.Getenv("DUNNING_SMTP_USERNAME")),
		DunningSMTPPassword:    os.Getenv("DUNNING_SMTP_PASSWORD"),
		DunningSMTPFrom:        strings.TrimSpace(os.Getenv("DUNNING_SMTP_FROM")),
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

// handleTransactions renders the transaction list (and, with ?detail=, a single
// transaction's page). It was called handleBilling while /billing was still a
// second route onto the same page; that alias is gone.
func (a *app) handleTransactions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !a.requireAuth(w, r) {
		return
	}
	if _, ok := a.scopedPageUser(w, r); !ok {
		return
	}
	if detailKey := strings.TrimSpace(r.URL.Query().Get("detail")); detailKey != "" {
		a.renderTransactionDetail(w, r, detailKey)
		return
	}
	filters, transactionScope := transactionListFiltersFromQuery(r.URL.Query())
	filterError := ""
	if err := validateTransactionFilters(filters); err != nil {
		filterError = "invalid_filter"
		filters = transactionFilters{Page: 1, PageSize: 10}
	}
	var rows []transactionPageRow
	var lastSync string
	var tenantCount, expenseCount int
	var pendingCount int
	var totalTransactions int64
	var tenantOptions []billingTenantOption
	if userID, ok := a.currentUserID(r); ok && a.db != nil {
		rentPeriod := monthStart(time.Now().UTC())
		if filters.RentPeriod != "" {
			if parsed, parseErr := parsePeriodMonth(filters.RentPeriod); parseErr == nil {
				rentPeriod = parsed
			}
		}
		if err := newMonthlyRentFactsService(a.db).ensureMonthlyRentFacts(r.Context(), userID, rentPeriod, rentFactsIntentRead); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
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
		tenantOptions = tenantOptionsFromRows(tenantRows)
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
	}
	setTransactionDetailLinks(r.URL.Query(), rows)
	for index := range rows {
		rows[index].PayerSearchURL = transactionNameSearchURL(r.URL.Query(), rows[index].PayerName)
		if rows[index].MatchedTenantName != "" {
			rows[index].TenantSearchURL = transactionNameSearchURL(r.URL.Query(), rows[index].MatchedTenantName)
		}
		if rows[index].Direction == "income" && rows[index].MatchStatus != "ignored" {
			if id, err := strconv.ParseUint(rows[index].InternalID, 10, 64); err == nil {
				rows[index].MatchURL = transactionReviewURL(r.URL.Query(), id)
			}
		} else if rows[index].Direction == "expense" && (rows[index].Source == "truelayer" || rows[index].ExpenseLinked) {
			if id, err := strconv.ParseUint(rows[index].InternalID, 10, 64); err == nil {
				rows[index].ExpenseLinkURL = transactionExpenseActionURL(r.URL.Query(), id)
			}
		}
	}
	transactionReturnURL := transactionListURL(r.URL.Query())
	for index := range rows {
		rows[index].ReturnURL = transactionReturnURL
	}
	page := filters.Page
	if page <= 0 {
		page = 1
	}
	pageSize := filters.PageSize
	if pageSize <= 0 {
		pageSize = 10
	}
	totalPages := 0
	if totalTransactions > 0 {
		totalPages = int((totalTransactions + int64(pageSize) - 1) / int64(pageSize))
	}
	previousPageURL, nextPageURL := "", ""
	if page > 1 {
		previousPageURL = transactionPageURL(r.URL.Query(), page-1)
	}
	if totalPages > 0 && page < totalPages {
		nextPageURL = transactionPageURL(r.URL.Query(), page+1)
	}
	connected := a.hasStoredToken()
	if userID, ok := a.currentUserID(r); ok && a.bankConnections != nil {
		connected, _ = a.bankConnections.hasRefreshToken(r.Context(), userID)
	}
	data := transactionListPageData{
		workspaceShell: a.fillWorkspaceShell(r, workspaceShell{
			ActivePage:    "transactions",
			Username:      a.displayUsername(r),
			Environment:   a.cfg.Environment,
			FootNote:      "银行流水与租金关联",
			CompactTitle:  "流水",
			ShowNavCounts: true,
			IncomeCount:   int(totalTransactions),
			TenantCount:   tenantCount,
			ExpenseCount:  expenseCount,
		}),
		CanonicalPath:     "/transactions",
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
		SortLinks:         transactionSortLinks(r.URL.Query(), filters.Sort),

		MatchStatusSelection: matchStatusSelection(filters),
		TransactionScope:     transactionScope,
		Page:                 page,
		PageSize:             pageSize,
		TotalTransactions:    totalTransactions,
		TotalPages:           totalPages,
		Pagination:           paginationLinks(page, totalPages, func(target int) string { return transactionPageURL(r.URL.Query(), target) }),
		PreviousPageURL:      previousPageURL,
		NextPageURL:          nextPageURL,
		PendingCount:         pendingCount,
		TokenFile:            a.cfg.TokenFile,
	}
	data.ExpenseOpenURL = transactionDrawerURL(r, "expense")
	data.CashReceiptOpenURL = transactionDrawerURL(r, "cash")
	if matchKey := strings.TrimSpace(r.URL.Query().Get("match")); matchKey != "" {
		matchID, parseErr := parsePositiveUint(matchKey)
		if parseErr != nil {
			http.NotFound(w, r)
			return
		}
		var tenantID uint64
		if selected := r.URL.Query().Get("match_tenant"); selected != "" {
			tenantID, parseErr = parsePositiveUint(selected)
			if parseErr != nil {
				http.NotFound(w, r)
				return
			}
		}
		historyPage := 1
		if selected := r.URL.Query().Get("match_history_page"); selected != "" {
			historyPage, parseErr = strconv.Atoi(selected)
			if parseErr != nil || historyPage < 1 || historyPage > 100000 {
				http.NotFound(w, r)
				return
			}
		}
		userID, _ := a.currentUserID(r)
		if a.db == nil || userID == 0 {
			http.NotFound(w, r)
			return
		}
		review, reviewErr := newTransactionService(a.db).transactionMatchReviewForMonthWithOrigin(r.Context(), userID, matchID, tenantID, historyPage, strings.TrimSpace(r.URL.Query().Get("match_month")), r.URL.Query().Get("match_origin") != "roommate")
		if errors.Is(reviewErr, gorm.ErrRecordNotFound) {
			http.NotFound(w, r)
			return
		}
		if reviewErr != nil {
			http.Error(w, reviewErr.Error(), http.StatusInternalServerError)
			return
		}
		review.setURLs(r.URL.Query())
		data.MatchReview = &review
	}
	if r.URL.Query().Get("expense") == "1" || isExpenseFormError(r.URL.Query().Get("error")) {
		if userID, ok := a.currentUserID(r); ok {
			drawer, drawerErr := a.loadExpenseDrawerData(r.Context(), userID, monthStart(time.Now().UTC()).Format("2006-01"), 0, 0, expenseFormReturnURL(r), r.URL.Query().Get("error"))
			if drawerErr != nil {
				http.Error(w, drawerErr.Error(), http.StatusInternalServerError)
				return
			}
			data.ExpenseDrawer = drawer
		}
	}
	if rawID := strings.TrimSpace(r.URL.Query().Get("expense_link")); rawID != "" {
		transactionID, parseErr := parsePositiveUint(rawID)
		if parseErr != nil {
			http.NotFound(w, r)
			return
		}
		userID, _ := a.currentUserID(r)
		drawer, drawerErr := a.loadTransactionExpenseDrawer(r.Context(), userID, transactionID, r.URL.Query())
		if errors.Is(drawerErr, gorm.ErrRecordNotFound) {
			http.NotFound(w, r)
			return
		}
		if drawerErr != nil {
			http.Error(w, drawerErr.Error(), http.StatusInternalServerError)
			return
		}
		data.ExpenseLinkDrawer = drawer
	}
	if drawer := cashReceiptDrawerFromRequest(r); drawer != nil {
		data.CashReceiptDrawer = drawer
	} else if r.URL.Query().Get("cash") == "1" {
		if userID, ok := a.currentUserID(r); ok {
			returnURL, _, _, valid := cashReceiptHostReturnURL("/transactions?" + r.URL.Query().Encode())
			if valid {
				drawer, drawerErr := a.loadCashReceiptHostDrawer(r.Context(), r, userID, 0, time.Time{}, returnURL, nil)
				if drawerErr != nil {
					http.Error(w, drawerErr.Error(), http.StatusInternalServerError)
					return
				}
				data.CashReceiptDrawer = drawer
			}
		}
	}
	if data.LastSync == "" {
		data.LastSync = "尚未同步"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := transactionListTemplate.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func transactionListFiltersFromQuery(query url.Values) (transactionFilters, string) {
	filters := filtersFromQuery(query)
	// Older links can name one unfinished stored status. The visible list now
	// offers one 待处理 group, so those links follow that same group.
	switch filters.MatchStatus {
	case "candidate", "needs_review", "unmatched", "partial":
		filters.MatchStatus = ""
		filters.PendingOnly = true
	}
	scope := firstNonEmpty(strings.TrimSpace(query.Get("scope")), matchStatusSelection(filters))
	if scope == "all" {
		filters.PendingOnly = false
		filters.MatchStatus = ""
	} else if scope == "" {
		scope = "all"
	}
	return filters, scope
}

func rememberPayerPreference(values url.Values) bool {
	choices, provided := values["remember_payer"]
	if !provided {
		return true
	}
	for _, choice := range choices {
		if choice == "1" {
			return true
		}
	}
	return false
}

func transactionDrawerURL(r *http.Request, drawer string) string {
	values := url.Values{}
	for key, items := range r.URL.Query() {
		if key == "cash" || key == "expense" || key == "expense_link" || key == "error" || key == "message" || key == "detail" {
			continue
		}
		values[key] = append([]string(nil), items...)
	}
	values.Set(drawer, "1")
	return "/transactions?" + values.Encode()
}

func transactionPageURL(query url.Values, page int) string {
	values := url.Values{}
	for key, items := range query {
		values[key] = append([]string(nil), items...)
	}
	values.Set("page", strconv.Itoa(page))
	return "/transactions?" + values.Encode()
}

const transactionDefaultSort = "arrival_desc"

func transactionSortLinks(query url.Values, current string) map[string]tableSortLink {
	activeSort := normalisedSort(current, transactionDefaultSort)
	sortURL := func(sortValue string) string {
		values := url.Values{}
		for key, items := range query {
			values[key] = append([]string(nil), items...)
		}
		values.Set("sort", sortValue)
		values.Set("page", "1")
		return "/transactions?" + values.Encode()
	}
	return map[string]tableSortLink{
		"arrival":     sortLinkFor(sortURL, activeSort, "arrival_desc", "arrival_asc"),
		"payer":       sortLinkFor(sortURL, activeSort, "payer_asc", "payer_desc"),
		"object":      sortLinkFor(sortURL, activeSort, "object_asc", "object_desc"),
		"amount":      sortLinkFor(sortURL, activeSort, "amount_desc", "amount_asc"),
		"rent_period": sortLinkFor(sortURL, activeSort, "rent_period_asc", "rent_period_desc"),
		"reason":      sortLinkFor(sortURL, activeSort, "reason_asc", "reason_desc"),
		"status":      sortLinkFor(sortURL, activeSort, "status_asc", "status_desc"),
	}
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
		redirectTransactionResult(w, r, "error", "invalid_confirmation")
		return
	}
	transactionID, err := transactionID(r.Form.Get("transaction_id"))
	if err != nil {
		redirectTransactionResult(w, r, "error", "invalid_confirmation")
		return
	}
	rememberPayer := rememberPayerPreference(r.Form)
	service := newTransactionService(a.db)
	if value := strings.TrimSpace(r.Form.Get("rent_obligation_id")); value != "" {
		rentObligationID, parseErr := parsePositiveUint(value)
		if parseErr != nil {
			redirectTransactionConfirmationResult(w, r, a.db, userID, transactionID, "error", "invalid_confirmation")
			return
		}
		if err := service.confirmRentMatchToObligation(r.Context(), userID, transactionID, rentObligationID, rememberPayer); err != nil {
			redirectTransactionConfirmationResult(w, r, a.db, userID, transactionID, "error", transactionFailureCode(err, "confirmation_failed"))
			return
		}
		redirectTransactionResult(w, r, "message", "rent_confirmed")
		return
	}
	tenantID, err := strconv.ParseUint(r.Form.Get("tenant_id"), 10, 64)
	if err != nil || tenantID == 0 {
		redirectTransactionConfirmationResult(w, r, a.db, userID, transactionID, "error", "invalid_confirmation")
		return
	}
	var period *time.Time
	if value := strings.TrimSpace(r.Form.Get("period")); value != "" {
		parsed, err := parsePeriodMonth(value)
		if err != nil {
			redirectTransactionConfirmationResult(w, r, a.db, userID, transactionID, "error", "invalid_confirmation")
			return
		}
		period = &parsed
	}
	if err := service.confirmRentMatch(r.Context(), userID, transactionID, tenantID, period, rememberPayer); err != nil {
		redirectTransactionConfirmationResult(w, r, a.db, userID, transactionID, "error", transactionFailureCode(err, "confirmation_failed"))
		return
	}
	redirectTransactionResult(w, r, "message", "rent_confirmed")
}

func (a *app) requestCounts(ctx context.Context, r *http.Request) (tenantCount, transactionCount, expenseCount int, err error) {
	userID, ok := a.currentUserID(r)
	if !ok || a.db == nil {
		return 0, 0, 0, errors.New("database session required")
	}
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

// tenantHistoryMonths is how far back the row expansion on the tenant list
// looks. It includes the current month, so 6 covers this month plus the previous
// five — enough to show a tenant who is behind across a quarter.
const tenantHistoryMonths = 6

func (a *app) handleTenants(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	userID, ok := a.scopedPageUser(w, r)
	if !ok {
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
	tenantCount := len(tenants)
	activeCount := 0
	for _, record := range tenants {
		if record.Status == "active" {
			activeCount++
		}
	}
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	sortValue := strings.TrimSpace(r.URL.Query().Get("sort"))
	if !validTenantPageSort(sortValue) {
		http.Error(w, "tenant sort is invalid", http.StatusBadRequest)
		return
	}
	tenants = filterTenantRecords(tenants, search)
	tenants = sortTenantRecords(tenants, sortValue)
	for index := range tenants {
		tenants[index].ListDetailURL = tenantListDetailURL(tenants[index].ID, search, sortValue)
		tenants[index].ListEditURL = tenantListEditURL(tenants[index].ID, search, sortValue)
	}
	if userID, ok := a.currentUserID(r); ok && a.db != nil {
		history, err := newObligationService(a.db).listTenantBillingHistory(r.Context(), userID, monthStart(time.Now().UTC()), tenantHistoryMonths)
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
	formRecord := tenantRecord{Status: "active"}
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
	showForm := editing || r.URL.Query().Get("add") == "1" || r.URL.Query().Get("error") != ""
	assignmentPropertyID, assignmentRoomID, arrangementStartMonth, assignmentErr := tenantAssignmentSelectionFromQuery(r.URL.Query())
	if assignmentErr != nil {
		http.Error(w, "tenant room selection is invalid", http.StatusBadRequest)
		return
	}
	returnRoomID, returnRoomErr := parseOptionalUint(r.URL.Query().Get("return_room_id"))
	if returnRoomErr != nil || (returnRoomID != 0 && (editing || returnRoomID != assignmentRoomID)) {
		http.Error(w, "tenant room return is invalid", http.StatusBadRequest)
		return
	}
	returnURL := tenantListURL(search, sortValue)
	postReturnQuery := url.Values{}
	if search != "" {
		postReturnQuery.Set("search", search)
	}
	if sortValue != "" && sortValue != tenantPageDefaultSort {
		postReturnQuery.Set("sort", sortValue)
	}
	if editing {
		postReturnQuery.Set("edit", formRecord.ID)
	} else if showForm {
		postReturnQuery.Set("add", "1")
		if assignmentPropertyID != 0 {
			postReturnQuery.Set("property_id", strconv.FormatUint(assignmentPropertyID, 10))
		}
		if assignmentRoomID != 0 {
			postReturnQuery.Set("room_id", strconv.FormatUint(assignmentRoomID, 10))
		}
		postReturnQuery.Set("arrangement_start_month", arrangementStartMonth)
		if returnRoomID != 0 {
			postReturnQuery.Set("return_room_id", strconv.FormatUint(returnRoomID, 10))
		}
	}
	postReturnURL := "/tenants"
	if encoded := postReturnQuery.Encode(); encoded != "" {
		postReturnURL += "?" + encoded
	}
	data := tenantPageData{
		workspaceShell: a.fillWorkspaceShell(r, workspaceShell{
			ActivePage:    "tenants",
			Username:      a.displayUsername(r),
			Environment:   a.cfg.Environment,
			FootNote:      "租客资料",
			CompactTitle:  "对象管理",
			ShowNavCounts: true,
			TenantCount:   tenantCount,
			IncomeCount:   incomeCount,
			ExpenseCount:  expenseCount,
		}),
		PageKey:                     "tenants",
		CanonicalPath:               "/tenants",
		Message:                     r.URL.Query().Get("message"),
		Error:                       r.URL.Query().Get("error"),
		Rows:                        tenants,
		Search:                      search,
		Sort:                        sortValue,
		SortLinks:                   tenantPageSortLinks(search, sortValue),
		FilteredCount:               len(tenants),
		ActiveCount:                 activeCount,
		ShowForm:                    showForm,
		Editing:                     editing,
		Form:                        formRecord,
		ReturnURL:                   returnURL,
		PostReturnURL:               postReturnURL,
		ErrorMessage:                tenantFormErrorMessage(r.URL.Query().Get("error")),
		TenantAssignmentPropertyID:  assignmentPropertyID,
		TenantAssignmentRoomID:      assignmentRoomID,
		TenantArrangementStartMonth: arrangementStartMonth,
		TenantRoomLocked:            returnRoomID != 0,
	}
	if showForm && !editing {
		properties, rooms, loadErr := a.loadTenantRoomAssignmentOptions(r.Context(), userID)
		if loadErr != nil {
			http.Error(w, loadErr.Error(), http.StatusInternalServerError)
			return
		}
		data.TenantProperties, data.TenantRooms = properties, rooms
		if returnRoomID != 0 {
			found := false
			for _, roomOption := range rooms {
				if roomOption.ID == returnRoomID && roomOption.PropertyID == assignmentPropertyID {
					found = true
					break
				}
			}
			if !found {
				http.NotFound(w, r)
				return
			}
			data.ReturnURL = "/rooms/" + strconv.FormatUint(returnRoomID, 10) + "?period=" + url.QueryEscape(arrangementStartMonth) + "&rent=1"
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tenantTemplate.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (a *app) createTenant(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, tenantFormRedirectURL("/tenants?add=1", "", "invalid_form"), http.StatusFound)
		return
	}
	returnTo := strings.TrimSpace(r.Form.Get("return_to"))
	if returnTo == "" {
		if tenantID := strings.TrimSpace(r.Form.Get("tenant_id")); tenantID != "" {
			returnTo = "/tenants?edit=" + url.QueryEscape(tenantID)
		} else {
			returnTo = "/tenants?add=1"
		}
	}
	if returnRoomID, valid := tenantReturnRoomID(returnTo); valid && r.Form.Get("room_id") != strconv.FormatUint(returnRoomID, 10) {
		http.Redirect(w, r, tenantFormRedirectURL(returnTo, "", "tenant_room_invalid"), http.StatusFound)
		return
	}
	if _, valid := tenantReturnRoomID(returnTo); valid && strings.TrimSpace(r.Form.Get("tenant_id")) != "" {
		http.Redirect(w, r, tenantFormRedirectURL(returnTo, "", "tenant_room_invalid"), http.StatusFound)
		return
	}
	if err := a.persistTenantRecord(r.Context(), r, r.Form); err != nil {
		errorCode := ""
		switch {
		case errors.Is(err, errInvalidTenantInput):
			errorCode = "invalid_tenant"
		case errors.Is(err, errInvalidTenantRoomPlan), errors.Is(err, ErrInvalidRentPlan):
			errorCode = "tenant_room_invalid"
		case errors.Is(err, ErrStaleRentPlanTimeline):
			errorCode = "tenant_room_stale"
		case errors.Is(err, ErrTenantRoomMonthConflict):
			errorCode = "tenant_room_conflict"
		case errors.Is(err, ErrRentPlanFactsLocked):
			errorCode = "tenant_room_locked"
		case errors.Is(err, gorm.ErrRecordNotFound):
			errorCode = "tenant_room_unavailable"
		}
		if errorCode != "" {
			http.Redirect(w, r, tenantFormRedirectURL(returnTo, "", errorCode), http.StatusFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	message := "tenant_added"
	if strings.TrimSpace(r.Form.Get("tenant_id")) != "" {
		message = "tenant_updated"
	}
	if message == "tenant_added" {
		if roomReturnURL, valid := tenantCreatedRoomReturnURL(returnTo, r.Form); valid {
			http.Redirect(w, r, roomReturnURL, http.StatusFound)
			return
		}
	}
	http.Redirect(w, r, tenantFormRedirectURL(returnTo, message, ""), http.StatusFound)
}

func tenantCreatedRoomReturnURL(returnTo string, form url.Values) (string, bool) {
	roomID, valid := tenantReturnRoomID(returnTo)
	assignedRoomID, assignedErr := parsePositiveUint(form.Get("room_id"))
	month, monthErr := parsePeriodMonth(form.Get("arrangement_start_month"))
	if !valid || assignedErr != nil || monthErr != nil || roomID != assignedRoomID {
		return "", false
	}
	return "/rooms/" + strconv.FormatUint(roomID, 10) + "?period=" + month.Format("2006-01") + "&rent=1&message=tenant_added", true
}

func tenantReturnRoomID(returnTo string) (uint64, bool) {
	target, err := url.ParseRequestURI(strings.TrimSpace(returnTo))
	if err != nil || target.IsAbs() || target.Host != "" || strings.HasPrefix(target.Path, "//") {
		return 0, false
	}
	if target.Path == "/tenants" {
		roomID, err := parsePositiveUint(target.Query().Get("return_room_id"))
		return roomID, err == nil
	}
	if !strings.HasPrefix(target.Path, "/rooms/") || target.Query().Get("tenant_add") != "1" {
		return 0, false
	}
	roomID, err := parsePositiveUint(strings.TrimPrefix(target.Path, "/rooms/"))
	if err != nil {
		return 0, false
	}
	month, err := parsePeriodMonth(target.Query().Get("period"))
	return roomID, err == nil && month.Format("2006-01") == target.Query().Get("period")
}

func tenantFormRedirectURL(returnTo, message, errorCode string) string {
	target, err := url.ParseRequestURI(strings.TrimSpace(returnTo))
	if err != nil || target.IsAbs() || target.Host != "" || strings.HasPrefix(target.Path, "//") {
		target = &url.URL{Path: "/tenants"}
	}
	validPath := target.Path == "/tenants"
	if !validPath && strings.HasPrefix(target.Path, "/tenants/") {
		id := strings.TrimPrefix(target.Path, "/tenants/")
		if id != "" && !strings.Contains(id, "/") {
			_, parseErr := strconv.ParseUint(id, 10, 64)
			validPath = parseErr == nil
		}
	}
	if !validPath {
		_, validPath = tenantReturnRoomID(returnTo)
	}
	if !validPath {
		target = &url.URL{Path: "/tenants"}
	}
	query := target.Query()
	query.Del("message")
	query.Del("error")
	if errorCode != "" {
		query.Set("error", errorCode)
		if target.Path == "/tenants" {
			if query.Get("edit") == "" {
				query.Set("add", "1")
			}
		} else if strings.HasPrefix(target.Path, "/tenants/") {
			query.Set("edit", "1")
		}
	} else {
		query.Del("add")
		query.Del("edit")
		if message != "" {
			query.Set("message", message)
		}
	}
	target.RawQuery = query.Encode()
	return target.RequestURI()
}

func filterTenantRecords(rows []tenantRecord, search string) []tenantRecord {
	search = strings.ToLower(strings.TrimSpace(search))
	if search == "" {
		return rows
	}
	filtered := make([]tenantRecord, 0, len(rows))
	for _, row := range rows {
		fields := []string{row.Name, row.DisplayAlias, row.Email, row.PayerNameHint, row.PayerID}
		for _, field := range fields {
			if strings.Contains(strings.ToLower(field), search) {
				filtered = append(filtered, row)
				break
			}
		}
	}
	return filtered
}

const tenantPageDefaultSort = "name_asc"

func validTenantPageSort(value string) bool {
	switch value {
	case "", "name_asc", "name_desc", "payer_asc", "payer_desc", "status_asc", "status_desc", "created_asc", "created_desc":
		return true
	default:
		return false
	}
}

func sortTenantRecords(rows []tenantRecord, sortValue string) []tenantRecord {
	if sortValue == "" {
		sortValue = tenantPageDefaultSort
	}
	sorted := append([]tenantRecord(nil), rows...)
	sort.SliceStable(sorted, func(i, j int) bool {
		left, right := sorted[i], sorted[j]
		leftName, rightName := strings.ToLower(firstNonEmpty(left.DisplayAlias, left.Name)), strings.ToLower(firstNonEmpty(right.DisplayAlias, right.Name))
		switch sortValue {
		case "name_asc", "name_desc":
			if leftName != rightName {
				return leftName < rightName == (sortValue == "name_asc")
			}
		case "payer_asc", "payer_desc":
			leftPayer, rightPayer := strings.ToLower(left.PayerNameHint), strings.ToLower(right.PayerNameHint)
			if leftPayer != rightPayer {
				return leftPayer < rightPayer == (sortValue == "payer_asc")
			}
		case "status_asc", "status_desc":
			if left.Status != right.Status {
				return left.Status < right.Status == (sortValue == "status_asc")
			}
		case "created_asc", "created_desc":
			if left.CreatedAt != right.CreatedAt {
				return left.CreatedAt < right.CreatedAt == (sortValue == "created_asc")
			}
		}
		if leftName != rightName {
			return leftName < rightName
		}
		return left.ID < right.ID
	})
	return sorted
}

func tenantListURL(search, sortValue string) string {
	query := url.Values{}
	if search = strings.TrimSpace(search); search != "" {
		query.Set("search", search)
	}
	if sortValue != "" && sortValue != tenantPageDefaultSort {
		query.Set("sort", sortValue)
	}
	if encoded := query.Encode(); encoded != "" {
		return "/tenants?" + encoded
	}
	return "/tenants"
}

func tenantListDetailURL(tenantID, search, sortValue string) string {
	query := url.Values{}
	if search = strings.TrimSpace(search); search != "" {
		query.Set("search", search)
	}
	if sortValue != "" && sortValue != tenantPageDefaultSort {
		query.Set("sort", sortValue)
	}
	if encoded := query.Encode(); encoded != "" {
		return "/tenants/" + url.PathEscape(tenantID) + "?" + encoded
	}
	return "/tenants/" + url.PathEscape(tenantID)
}

func tenantListEditURL(tenantID, search, sortValue string) string {
	query := url.Values{"edit": []string{tenantID}}
	if search = strings.TrimSpace(search); search != "" {
		query.Set("search", search)
	}
	if sortValue != "" && sortValue != tenantPageDefaultSort {
		query.Set("sort", sortValue)
	}
	return "/tenants?" + query.Encode()
}

func tenantPageSortLinks(search, current string) map[string]tableSortLink {
	activeSort := normalisedSort(current, tenantPageDefaultSort)
	sortURL := func(sortValue string) string { return tenantListURL(search, sortValue) }
	return map[string]tableSortLink{
		"name":    sortLinkFor(sortURL, activeSort, "name_asc", "name_desc"),
		"payer":   sortLinkFor(sortURL, activeSort, "payer_asc", "payer_desc"),
		"status":  sortLinkFor(sortURL, activeSort, "status_asc", "status_desc"),
		"created": sortLinkFor(sortURL, activeSort, "created_asc", "created_desc"),
	}
}

var errInvalidTenantInput = errors.New("invalid tenant input")
var errInvalidTenantRoomPlan = errors.New("invalid tenant room plan")

func (a *app) listTenantRecords(ctx context.Context, r *http.Request) ([]tenantRecord, error) {
	userID, ok := a.currentUserID(r)
	if !ok || a.db == nil {
		return nil, errors.New("database session required")
	}
	return newTenantService(a.db).listTenants(ctx, userID)
}

func (a *app) persistTenantRecord(ctx context.Context, r *http.Request, values formValues) error {
	userID, ok := a.currentUserID(r)
	if !ok || a.db == nil {
		return errors.New("database session required")
	}
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
		form, ok := values.(url.Values)
		if !ok {
			return errInvalidTenantRoomPlan
		}
		assignment, hasAssignment, assignmentErr := tenantRoomPlanAssignmentInputFromForm(form)
		if assignmentErr != nil {
			return errInvalidTenantRoomPlan
		}
		if hasAssignment {
			_, err = service.createTenantWithRoomPlan(ctx, userID, input, assignment)
		} else {
			_, err = service.createTenant(ctx, userID, input)
		}
	}
	if err != nil {
		if isValidationError(err) {
			return errInvalidTenantInput
		}
		return err
	}
	return nil
}

func tenantAssignmentSelectionFromQuery(values url.Values) (propertyID, roomID uint64, arrangementMonth string, err error) {
	if propertyID, err = parseOptionalUint(values.Get("property_id")); err != nil {
		return 0, 0, "", err
	}
	if roomID, err = parseOptionalUint(values.Get("room_id")); err != nil {
		return 0, 0, "", err
	}
	arrangementMonth = dublinCurrentMonth(time.Now()).Format("2006-01")
	if raw := strings.TrimSpace(values.Get("arrangement_start_month")); raw != "" {
		month, monthErr := parsePeriodMonth(raw)
		if monthErr != nil {
			return 0, 0, "", monthErr
		}
		arrangementMonth = month.Format("2006-01")
	}
	return propertyID, roomID, arrangementMonth, nil
}

func (a *app) loadTenantRoomAssignmentOptions(ctx context.Context, userID uint64) ([]tenantPropertyAssignmentOption, []tenantRoomAssignmentOption, error) {
	repo := newLandlordRentRepository(a.db)
	properties, err := repo.listProperties(ctx, userID, propertyQuery{Status: "active"})
	if err != nil {
		return nil, nil, err
	}
	rooms, err := repo.listRooms(ctx, userID, roomQuery{Status: "active"})
	if err != nil {
		return nil, nil, err
	}
	plans, err := repo.listRoomRentPlans(ctx, userID, roomRentPlanQuery{})
	if err != nil {
		return nil, nil, err
	}
	members, err := repo.listRoomRentPlanMembers(ctx, userID, roomRentPlanMemberQuery{})
	if err != nil {
		return nil, nil, err
	}
	var tenants []tenant
	if err := a.db.WithContext(ctx).Where("user_id = ? AND status = ?", userID, "active").Find(&tenants).Error; err != nil {
		return nil, nil, err
	}

	propertyNameByID := make(map[uint64]string, len(properties))
	propertyOptions := make([]tenantPropertyAssignmentOption, 0, len(properties))
	for _, row := range properties {
		propertyNameByID[row.ID] = row.Name
		propertyOptions = append(propertyOptions, tenantPropertyAssignmentOption{ID: row.ID, Name: row.Name})
	}
	tenantNameByID := make(map[uint64]string, len(tenants))
	for _, row := range tenants {
		tenantNameByID[row.ID] = firstNonEmpty(row.DisplayAlias, row.Name)
	}
	membersByPlanID := make(map[uint64][]tenantRoomPlanMemberWire)
	for _, row := range members {
		membersByPlanID[row.RoomRentPlanID] = append(membersByPlanID[row.RoomRentPlanID], tenantRoomPlanMemberWire{
			TenantID: row.TenantID, TenantName: firstNonEmpty(tenantNameByID[row.TenantID], "已停用租客"), ResponsibilityCents: row.ResponsibilityCents,
		})
	}
	plansByRoomID := make(map[uint64][]tenantRoomPlanWire)
	for _, row := range plans {
		wire := tenantRoomPlanWire{
			EffectiveFromMonth: row.EffectiveFromMonth.Format("2006-01"), MonthlyRentCents: row.MonthlyRentCents,
			Currency: row.Currency, DueDay: row.DueDay, Members: membersByPlanID[row.ID],
		}
		if row.EffectiveToMonth != nil {
			wire.EffectiveToMonth = row.EffectiveToMonth.Format("2006-01")
		}
		plansByRoomID[row.RoomID] = append(plansByRoomID[row.RoomID], wire)
	}
	options := make([]tenantRoomAssignmentOption, 0, len(rooms))
	for _, row := range rooms {
		plansJSON, marshalErr := json.Marshal(plansByRoomID[row.ID])
		if marshalErr != nil {
			return nil, nil, marshalErr
		}
		options = append(options, tenantRoomAssignmentOption{
			ID: row.ID, PropertyID: row.PropertyID, PropertyName: propertyNameByID[row.PropertyID], RoomLabel: row.RoomLabel,
			RentPlanVersion: row.RentPlanVersion, PlansJSON: string(plansJSON),
		})
	}
	return propertyOptions, options, nil
}

type tenantRoomPlanMemberForm struct {
	TenantID            uint64 `json:"tenant_id"`
	ResponsibilityCents int64  `json:"responsibility_cents"`
}

func tenantRoomPlanAssignmentInputFromForm(values url.Values) (tenantRoomPlanAssignmentInput, bool, error) {
	rawRoomID := strings.TrimSpace(values.Get("room_id"))
	if rawRoomID == "" {
		return tenantRoomPlanAssignmentInput{}, false, nil
	}
	roomID, roomErr := parsePositiveUint(rawRoomID)
	propertyID, propertyErr := parsePositiveUint(values.Get("property_id"))
	effectiveMonth, monthErr := parsePeriodMonth(strings.TrimSpace(values.Get("arrangement_start_month")))
	version, versionErr := strconv.ParseUint(strings.TrimSpace(values.Get("plan_version")), 10, 64)
	newResponsibility, responsibilityErr := parseOptionalRentPlanAmountCents(values.Get("new_responsibility"))
	if roomErr != nil || propertyErr != nil || monthErr != nil || versionErr != nil || responsibilityErr != nil {
		return tenantRoomPlanAssignmentInput{}, false, errInvalidTenantRoomPlan
	}
	rawPlan := strings.TrimSpace(values.Get("room_plan"))
	if len(rawPlan) > 64*1024 {
		return tenantRoomPlanAssignmentInput{}, false, errInvalidTenantRoomPlan
	}
	members := make([]tenantRoomPlanMemberForm, 0)
	if rawPlan != "" {
		decoder := json.NewDecoder(strings.NewReader(rawPlan))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&members); err != nil {
			return tenantRoomPlanAssignmentInput{}, false, errInvalidTenantRoomPlan
		}
		var extra any
		if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
			return tenantRoomPlanAssignmentInput{}, false, errInvalidTenantRoomPlan
		}
	}
	assignment := tenantRoomPlanAssignmentInput{
		PropertyID: propertyID, RoomID: roomID, EffectiveMonth: effectiveMonth, ExpectedTimelineVersion: version,
		ExistingMembers: make([]RoomRentPlanMemberInput, 0, len(members)), NewTenantResponsibilityCents: newResponsibility,
	}
	for _, member := range members {
		assignment.ExistingMembers = append(assignment.ExistingMembers, RoomRentPlanMemberInput{TenantID: member.TenantID, ResponsibilityCents: member.ResponsibilityCents})
	}
	if err := validateTenantRoomPlanAssignment(assignment); err != nil {
		return tenantRoomPlanAssignmentInput{}, false, errInvalidTenantRoomPlan
	}
	return assignment, true, nil
}

func tenantFormErrorMessage(errorCode string) string {
	switch errorCode {
	case "invalid_tenant":
		return "请检查租客姓名、邮箱和付款人资料。"
	case "tenant_room_invalid":
		return "请检查所选房产、房间、月份与租金金额。"
	case "tenant_room_stale":
		return "房间的入住或租金已更新，请重新打开表单后再保存。"
	case "tenant_room_conflict":
		return "该租客从所选月份起已安排在其他房间。请先到原房间的「入住与租金」移除该租客。"
	case "tenant_room_locked":
		return "所选月份的租金已锁定，不能调整入住人员。"
	case "tenant_room_unavailable":
		return "所选房间或该月份的租金规则已不可用，请重新选择。"
	default:
		return "请检查租客姓名、邮箱、付款人名称及房间租金资料。"
	}
}

func (a *app) handleExpenses(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	if _, ok := a.scopedPageUser(w, r); !ok {
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
	period, err := parsePeriodMonth(strings.TrimSpace(r.URL.Query().Get("period")))
	if err != nil {
		http.Error(w, "period is invalid", http.StatusBadRequest)
		return
	}
	statusFilter := firstNonEmpty(strings.TrimSpace(r.URL.Query().Get("status")), "all")
	if statusFilter != "all" && statusFilter != "invoice_linked" && statusFilter != "invoice_missing" {
		http.Error(w, "expense status is invalid", http.StatusBadRequest)
		return
	}
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	sortValue := strings.TrimSpace(r.URL.Query().Get("sort"))
	if !validExpensePageSort(sortValue) {
		http.Error(w, "expense sort is invalid", http.StatusBadRequest)
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
	var expenseProperties []expensePropertyOption
	var expenseRooms []expenseRoomOption
	propertyNames := make(map[string]string)
	roomNames := make(map[string]string)
	if userID, ok := a.currentUserID(r); ok && a.db != nil {
		properties, propertyErr := newLandlordRentRepository(a.db).listProperties(r.Context(), userID, propertyQuery{})
		if propertyErr != nil {
			http.Error(w, propertyErr.Error(), http.StatusInternalServerError)
			return
		}
		for _, row := range properties {
			propertyNames[strconv.FormatUint(row.ID, 10)] = row.Name
			if row.Status == "active" {
				expenseProperties = append(expenseProperties, expensePropertyOption{ID: row.ID, Name: row.Name})
			}
		}
		rooms, roomErr := newLandlordRentRepository(a.db).listRooms(r.Context(), userID, roomQuery{})
		if roomErr != nil {
			http.Error(w, roomErr.Error(), http.StatusInternalServerError)
			return
		}
		for _, row := range rooms {
			roomNames[strconv.FormatUint(row.ID, 10)] = row.RoomLabel
			if row.Status == "active" {
				expenseRooms = append(expenseRooms, expenseRoomOption{ID: row.ID, PropertyID: row.PropertyID, Label: row.RoomLabel})
			}
		}
	}
	for index := range expenses {
		expenses[index].PropertyName = propertyNames[expenses[index].PropertyID]
		expenses[index].RoomLabel = roomNames[expenses[index].RoomID]
	}
	var invoiceForm *expenseInvoiceFormView
	if userID, ok := a.currentUserID(r); ok && a.db != nil {
		expenseIDs := make([]uint64, 0, len(expenses))
		expenseByID := make(map[string]expenseRecord, len(expenses))
		for _, expense := range expenses {
			id, parseErr := strconv.ParseUint(expense.ID, 10, 64)
			if parseErr != nil || id == 0 {
				continue
			}
			expenseIDs = append(expenseIDs, id)
			expenseByID[expense.ID] = expense
		}
		currentInvoices, invoiceErr := a.loadCurrentExpenseInvoices(r.Context(), userID, expenseIDs)
		if invoiceErr != nil {
			http.Error(w, invoiceErr.Error(), http.StatusInternalServerError)
			return
		}
		for index := range expenses {
			expense := &expenses[index]
			expenseID, parseErr := strconv.ParseUint(expense.ID, 10, 64)
			invoice, found := currentInvoices[expenseID]
			if parseErr == nil && found {
				expense.InvoiceLinked = true
				expense.InvoiceNumber = invoice.InvoiceNumber
				expense.InvoiceVendor = invoice.Vendor
				expense.InvoiceDateDisplay = invoice.InvoiceDate.Format(dateLayout)
				expense.InvoiceAmountDisplay = formatMoney(centsToMoney(invoice.AmountCents), invoice.Currency, 2)
				expense.InvoiceDownloadURL = fmt.Sprintf("/expenses/invoices/%d", invoice.ID)
			} else if strings.TrimSpace(expense.InvoiceURL) != "" {
				expense.InvoiceLinked = true
				expense.InvoiceDownloadURL = expense.InvoiceURL
			}
			expense.InvoiceActionURL = expenseInvoiceActionURL(expense.ID, period.Format("2006-01"), statusFilter, search, sortValue)
		}
		if rawID := strings.TrimSpace(r.URL.Query().Get("invoice")); rawID != "" {
			if _, parseErr := strconv.ParseUint(rawID, 10, 64); parseErr != nil {
				http.Error(w, "expense invoice target is invalid", http.StatusBadRequest)
				return
			}
			selected, found := expenseByID[rawID]
			if !found {
				http.NotFound(w, r)
				return
			}
			invoiceForm, invoiceErr = a.expenseInvoiceForm(r.Context(), userID, selected, period.Format("2006-01"), statusFilter, search, sortValue)
			if invoiceErr != nil {
				http.Error(w, invoiceErr.Error(), http.StatusInternalServerError)
				return
			}
		}
	}
	for index := range expenses {
		if !expenses[index].InvoiceLinked && strings.TrimSpace(expenses[index].InvoiceURL) != "" {
			expenses[index].InvoiceLinked = true
			expenses[index].InvoiceDownloadURL = expenses[index].InvoiceURL
		}
		if expenses[index].InvoiceActionURL == "" {
			expenses[index].InvoiceActionURL = expenseInvoiceActionURL(expenses[index].ID, period.Format("2006-01"), statusFilter, search, sortValue)
		}
	}
	totalExpenseCount := len(expenses)
	expenses = filterExpensePageRows(expenses, period, statusFilter, search)
	expenses = sortExpenseRecords(expenses, sortValue)
	showExpenseForm := r.URL.Query().Get("add") == "1" || (r.URL.Query().Get("error") != "" && r.URL.Query().Get("error") != "invalid_invoice")
	var expenseDrawer *expenseDrawerData
	if showExpenseForm {
		selectedPropertyID, _ := parseOptionalUint(r.URL.Query().Get("property_id"))
		selectedRoomID, _ := parseOptionalUint(r.URL.Query().Get("room_id"))
		for _, option := range expenseRooms {
			if option.ID == selectedRoomID {
				if selectedPropertyID == 0 || selectedPropertyID == option.PropertyID {
					selectedPropertyID = option.PropertyID
				} else {
					selectedRoomID = 0
				}
				break
			}
		}
		returnURL := expenseFormReturnURL(r)
		expenseDrawer = &expenseDrawerData{Period: period.Format("2006-01"), Today: time.Now().UTC().Format(dateLayout), Properties: expenseProperties, Rooms: expenseRooms, SelectedPropertyID: selectedPropertyID, SelectedRoomID: selectedRoomID, ReturnURL: returnURL, PostReturnURL: returnURL, Error: r.URL.Query().Get("error")}
	}
	data := expensePageData{
		workspaceShell: a.fillWorkspaceShell(r, workspaceShell{
			ActivePage:    "expenses",
			Username:      a.displayUsername(r),
			Environment:   a.cfg.Environment,
			FootNote:      "支出资料",
			CompactTitle:  "费用支出",
			ShowNavCounts: true,
			TenantCount:   tenantCount,
			IncomeCount:   incomeCount,
			ExpenseCount:  totalExpenseCount,
		}),
		PageKey:       "expenses",
		CanonicalPath: "/expenses",
		Message:       r.URL.Query().Get("message"),
		Error:         r.URL.Query().Get("error"),
		Rows:          expenses,
		FilteredCount: len(expenses),
		Period:        period.Format("2006-01"),
		StatusFilter:  statusFilter,
		Search:        search,
		Sort:          sortValue,
		SortLinks:     expensePageSortLinks(period.Format("2006-01"), statusFilter, search, sortValue),
		ShowForm:      showExpenseForm,
		ExpenseDrawer: expenseDrawer,
		InvoiceForm:   invoiceForm,
		ExpenseTotal:  formatMoney(sumExpenses(expenses), "EUR", 2),
		Today:         time.Now().UTC().Format(dateLayout),
		Properties:    expenseProperties,
		Rooms:         expenseRooms,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := expenseTemplate.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (a *app) createExpense(w http.ResponseWriter, r *http.Request) {
	parseErr := r.ParseForm()
	returnTo := strings.TrimSpace(r.Form.Get("return_to"))
	if returnTo == "" {
		returnTo = expensePageURL(r.Form, "", "", false)
	}
	if parseErr != nil {
		http.Redirect(w, r, expenseFormRedirectURL(returnTo, "", "invalid_form"), http.StatusFound)
		return
	}
	if err := a.persistExpenseRecord(r.Context(), r, r.Form); err != nil {
		if errors.Is(err, errInvalidExpenseInput) {
			http.Redirect(w, r, expenseFormRedirectURL(returnTo, "", "invalid_expense"), http.StatusFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, expenseFormRedirectURL(returnTo, "expense_added", ""), http.StatusFound)
}

func expensePageURL(values url.Values, message, errorCode string, showForm bool) string {
	query := url.Values{}
	if period := strings.TrimSpace(values.Get("period")); period != "" {
		query.Set("period", validatedPeriodValue(period))
	}
	status := strings.TrimSpace(values.Get("status"))
	if status == "all" || status == "invoice_linked" || status == "invoice_missing" {
		query.Set("status", status)
	}
	if search := strings.TrimSpace(values.Get("search")); search != "" {
		query.Set("search", search)
	}
	if sortValue := strings.TrimSpace(values.Get("sort")); validExpensePageSort(sortValue) && sortValue != "" && sortValue != expensePageDefaultSort {
		query.Set("sort", sortValue)
	}
	if showForm {
		query.Set("add", "1")
	}
	if errorCode != "" {
		if invoiceID := strings.TrimSpace(values.Get("expense_id")); invoiceID != "" {
			if _, err := strconv.ParseUint(invoiceID, 10, 64); err == nil {
				query.Set("invoice", invoiceID)
			}
		}
	}
	if message != "" {
		query.Set("message", message)
	}
	if errorCode != "" {
		query.Set("error", errorCode)
	}
	if len(query) == 0 {
		return "/expenses"
	}
	return "/expenses?" + query.Encode()
}

var errInvalidExpenseInput = errors.New("invalid expense input")

func (a *app) listExpenseRecords(ctx context.Context, r *http.Request) ([]expenseRecord, error) {
	userID, ok := a.currentUserID(r)
	if !ok || a.db == nil {
		return nil, errors.New("database session required")
	}
	return newExpenseService(a.db).listExpenses(ctx, userID)
}

func (a *app) persistExpenseRecord(ctx context.Context, r *http.Request, values formValues) error {
	userID, ok := a.currentUserID(r)
	if !ok || a.db == nil {
		return errors.New("database session required")
	}
	input, err := expenseInputFromForm(values, time.Now())
	if err != nil || input.PropertyID == nil {
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
	// 落点是 /billing 而不是 /transactions：那是外面唯一还认得的 URL（老书签、文档、
	// 我们自己的审计脚本都写着它），由 handleBillingAlias 转发到列表页。
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
			http.Redirect(w, r, bankRefreshRedirect(r, "reconnect=1&error=no_saved_login"), http.StatusFound)
			return
		}
	} else {
		stored, err := a.loadStoredToken()
		if err != nil {
			http.Redirect(w, r, bankRefreshRedirect(r, "reconnect=1&error=no_saved_login"), http.StatusFound)
			return
		}
		refreshToken = stored.RefreshToken
	}

	token, err := a.refreshAccessToken(r.Context(), refreshToken)
	if err != nil {
		http.Redirect(w, r, bankRefreshRedirect(r, "reconnect=1&error=refresh_failed"), http.StatusFound)
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
		http.Redirect(w, r, bankRefreshRedirect(r, "error=data_fetch_failed"), http.StatusFound)
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
		http.Redirect(w, r, bankRefreshRedirect(r, "error=data_fetch_failed"), http.StatusFound)
		return
	}
	if err := a.appendDemoResultLog(result); err != nil {
		http.Error(w, "log write failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if hasUser && a.db != nil {
		if err := newTransactionService(a.db).ingestDemoResult(r.Context(), userID, result); err != nil {
			http.Redirect(w, r, bankRefreshRedirect(r, "error=data_fetch_failed"), http.StatusFound)
			return
		}
		if a.bankConnections != nil && syncResultHasSuccessfulAccount(result) {
			_ = a.bankConnections.markLastSync(r.Context(), userID)
		}
	}
	http.Redirect(w, r, bankRefreshRedirect(r, "message=refreshed"), http.StatusFound)
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

// displayUsername is the account name shown in the page chrome. It comes from
// the signed-in session, so an operator signed in as any account sees their own
// name rather than whichever account APP_ADMIN_USERNAME happens to configure.
// Requests without a valid session (the login screen, legacy cookies) fall back
// to the configured administrator.
func (a *app) displayUsername(r *http.Request) string {
	c, err := r.Cookie("rentops_session")
	if err != nil {
		return a.cfg.AdminUsername
	}
	parts := strings.Split(c.Value, "|")
	if len(parts) != 5 || parts[0] != "v2" {
		return a.cfg.AdminUsername
	}
	expiresUnix, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil || time.Now().After(time.Unix(expiresUnix, 0)) {
		return a.cfg.AdminUsername
	}
	want := userSessionSignature(a.cfg, parts[1], parts[2], parts[3])
	if !hmac.Equal([]byte(parts[4]), []byte(want)) {
		return a.cfg.AdminUsername
	}
	if username := strings.TrimSpace(parts[2]); username != "" {
		return username
	}
	return a.cfg.AdminUsername
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

func prepareExpenses(rows []expenseRecord) {
	for i := range rows {
		rows[i].Currency = firstNonEmpty(rows[i].Currency, "EUR")
		rows[i].AmountDisplay = formatMoney(rows[i].Amount, rows[i].Currency, 2)
		rows[i].DateDisplay = formatDate(rows[i].ExpenseDate)
		if date, err := time.Parse(dateLayout, rows[i].ExpenseDate); err == nil {
			rows[i].PeriodDisplay = date.Format("2006-01")
		} else {
			rows[i].PeriodDisplay = "—"
		}
	}
}

func filterExpensePageRows(rows []expenseRecord, period time.Time, status, search string) []expenseRecord {
	needle := strings.ToLower(strings.TrimSpace(search))
	month := monthStart(period).Format("2006-01")
	filtered := make([]expenseRecord, 0, len(rows))
	for _, row := range rows {
		if row.ExpenseDate != "" {
			if _, err := time.Parse(dateLayout, row.ExpenseDate); err == nil && !strings.HasPrefix(row.ExpenseDate, month) {
				continue
			}
		}
		linked := row.InvoiceLinked || strings.TrimSpace(row.InvoiceURL) != ""
		if status == "invoice_linked" && !linked || status == "invoice_missing" && linked {
			continue
		}
		if needle != "" {
			searchable := strings.ToLower(strings.Join([]string{row.Description, row.Category, row.PropertyName, row.RoomLabel, row.RoomHint, row.TenantHint}, " "))
			if !strings.Contains(searchable, needle) {
				continue
			}
		}
		filtered = append(filtered, row)
	}
	return filtered
}

const expensePageDefaultSort = "date_desc"

func validExpensePageSort(value string) bool {
	switch value {
	case "", "date_asc", "date_desc", "description_asc", "description_desc", "object_asc", "object_desc", "category_asc", "category_desc", "amount_asc", "amount_desc", "invoice_asc", "invoice_desc":
		return true
	default:
		return false
	}
}

func expenseObjectSortValue(row expenseRecord) string {
	return firstNonEmpty(row.PropertyName, row.RoomHint, row.RoomLabel, "房产级支出")
}

func sortExpenseRecords(rows []expenseRecord, sortValue string) []expenseRecord {
	if sortValue == "" {
		sortValue = expensePageDefaultSort
	}
	sorted := append([]expenseRecord(nil), rows...)
	sort.SliceStable(sorted, func(i, j int) bool {
		left, right := sorted[i], sorted[j]
		compareText := func(leftValue, rightValue string, ascending bool) (bool, bool) {
			leftValue, rightValue = strings.ToLower(leftValue), strings.ToLower(rightValue)
			if leftValue == rightValue {
				return false, false
			}
			return true, leftValue < rightValue == ascending
		}
		switch sortValue {
		case "date_asc", "date_desc":
			if left.ExpenseDate != right.ExpenseDate {
				return left.ExpenseDate < right.ExpenseDate == (sortValue == "date_asc")
			}
		case "description_asc", "description_desc":
			if decided, less := compareText(left.Description, right.Description, sortValue == "description_asc"); decided {
				return less
			}
		case "object_asc", "object_desc":
			if decided, less := compareText(expenseObjectSortValue(left), expenseObjectSortValue(right), sortValue == "object_asc"); decided {
				return less
			}
		case "category_asc", "category_desc":
			if decided, less := compareText(left.Category, right.Category, sortValue == "category_asc"); decided {
				return less
			}
		case "amount_asc", "amount_desc":
			if left.Amount != right.Amount {
				return left.Amount < right.Amount == (sortValue == "amount_asc")
			}
		case "invoice_asc", "invoice_desc":
			if left.InvoiceLinked != right.InvoiceLinked {
				return !left.InvoiceLinked == (sortValue == "invoice_asc")
			}
		}
		leftDescription, rightDescription := strings.ToLower(left.Description), strings.ToLower(right.Description)
		if leftDescription != rightDescription {
			return leftDescription < rightDescription
		}
		return left.ID < right.ID
	})
	return sorted
}

func expenseListURL(period, status, search, sortValue string) string {
	query := url.Values{"period": []string{validatedPeriodValue(period)}}
	if status != "" && status != "all" {
		query.Set("status", status)
	}
	if search = strings.TrimSpace(search); search != "" {
		query.Set("search", search)
	}
	if sortValue != "" && sortValue != expensePageDefaultSort {
		query.Set("sort", sortValue)
	}
	return "/expenses?" + query.Encode()
}

func expensePageSortLinks(period, status, search, current string) map[string]tableSortLink {
	activeSort := normalisedSort(current, expensePageDefaultSort)
	sortURL := func(sortValue string) string { return expenseListURL(period, status, search, sortValue) }
	return map[string]tableSortLink{
		"date": sortLinkFor(sortURL, activeSort, "date_desc", "date_asc"), "description": sortLinkFor(sortURL, activeSort, "description_asc", "description_desc"), "object": sortLinkFor(sortURL, activeSort, "object_asc", "object_desc"), "category": sortLinkFor(sortURL, activeSort, "category_asc", "category_desc"), "amount": sortLinkFor(sortURL, activeSort, "amount_desc", "amount_asc"), "invoice": sortLinkFor(sortURL, activeSort, "invoice_asc", "invoice_desc"),
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

func sumExpenses(rows []expenseRecord) float64 {
	var total float64
	for _, row := range rows {
		total += row.Amount
	}
	return total
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
  <title>登录 · RentOps</title>
  <link rel="stylesheet" href="/static/css/workspace.css">
  <link rel="stylesheet" href="/static/css/pages/login.css">
</head>
<body>
  <main class="shell">
    <div class="brand">
      <div class="mark">R</div>
      <div>
        <div class="brand-title">RentOps</div>
        <div class="brand-meta">收租工作区</div>
      </div>
    </div>
    <h1>登录</h1>
    <p class="sub">输入账号和密码，进入收租工作区。</p>
    {{if .Error}}<div class="error" role="alert">用户名或密码错误，请重试。</div>{{end}}
    <form method="post" action="/login-local">
      <label for="username">用户名</label>
      <input id="username" name="username" autocomplete="username" value="{{.Username}}" required>
      <label for="password">密码</label>
      <input id="password" name="password" type="password" autocomplete="current-password" required>
      <button type="submit">登录</button>
    </form>
    <p class="hint">如需开通或重置账号，请联系工作区管理员。</p>
  </main>
</body>
</html>
`))

var workspacePageCSS = embeddedWebText("web/static/css/workspace.css")

var tenantTemplate = newWorkspacePageTemplate("tenants", nil, `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>RentOps Tenants</title>
  <link rel="stylesheet" href="/static/css/pages/object-navigation.css"><link rel="stylesheet" href="/static/css/pages/entity-drawers.css">
  <style>`+workspacePageCSS+`
    .tenant-row, .tenant-month-row { cursor: pointer; }
    .tenant-row:hover, .tenant-row:focus, .tenant-month-row:hover, .tenant-month-row:focus { background: var(--surface-accent); outline: none; }
    .tenant-row td:first-child::after, .tenant-month-row td:first-child::after { content: " +"; margin-left: 6px; color: var(--foreground-muted); font: 700 12px var(--mono); }
    .tenant-row[aria-expanded="true"] td:first-child::after, .tenant-month-row[aria-expanded="true"] td:first-child::after { content: " -"; }
    .status { display: inline-block; border-radius: 999px; padding: 5px 9px; font-size: 12px; white-space: nowrap; }
    .status.open { color: #1e40af; background: #dbeafe; }
    .status.overdue, .status.needs_review { color: #991b1b; background: #fee2e2; }
    .status.partial { color: #92400e; background: #fef3c7; }
    .status.paid { color: #065f46; background: #d1fae5; }
    /* 详情 and 编辑 used to be stacked with a <br>, which read as one broken
       button. They sit side by side in their own cell, sized down so the pair
       does not dominate the row. */
    .row-actions { display: flex; align-items: center; gap: 8px; white-space: nowrap; }
    .row-actions .btn { min-height: 32px; margin-top: 0; padding: 0 12px; font-size: 12px; }
    .tenant-history-row td, .tenant-month-details td { padding: 0; background: var(--surface-muted); }
    .tenant-history { padding: 14px 18px 16px 32px; border-top: 1px solid var(--border); }
    .tenant-history-head { display: flex; align-items: baseline; justify-content: space-between; gap: 12px; margin-bottom: 10px; }
    .tenant-history-head h3 { margin: 0; font-size: 13px; }
    .tenant-history-table { min-width: 620px; background: var(--surface); }
    .tenant-history-table th { padding: 10px 12px; }
    .tenant-history-table td { padding: 12px; }
    .tenant-month-details .payment-list { padding: 12px 16px 14px 28px; }
    .tenant-month-details .payment-item { display: grid; grid-template-columns: 140px 170px minmax(180px, 1fr) minmax(160px, 1fr) 100px; gap: 12px; padding: 9px 0; border-bottom: 1px solid var(--border); color: var(--foreground-subtle); font-size: 12px; }
    .tenant-month-details .payment-item:last-child { border-bottom: 0; }
    .tenant-mobile-list { display: none; }
    .tenant-search { display: flex; align-items: end; flex-wrap: wrap; gap: 10px; padding: 14px 20px; border-bottom: 1px solid var(--border); }
    .tenant-search label { flex: 1 1 280px; max-width: 520px; margin: 0; }
    .tenant-search input { min-height: 40px; }
    .tenant-search .btn { min-height: 40px; margin-top: 0; }
    .tenant-mobile-card { display: grid; gap: 11px; padding: 14px; }
    .tenant-mobile-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 10px; }
    .tenant-mobile-head h3 { margin: 0; font-size: 15px; }
    .tenant-mobile-head p { margin: 3px 0 0; color: var(--foreground-muted); font-size: 11px; }
    .tenant-mobile-rent { display: flex; align-items: baseline; justify-content: space-between; gap: 8px; padding: 10px; border-radius: 8px; background: var(--surface-muted); }
    .tenant-mobile-rent span { color: var(--foreground-muted); font-size: 11px; }
    .tenant-mobile-rent strong { font: 700 15px var(--mono); }
    .tenant-mobile-actions { display: grid; grid-template-columns: 1fr 1fr; gap: 8px; }
    .tenant-mobile-history { border-top: 1px solid var(--border); padding-top: 8px; }
    .tenant-mobile-history summary { min-height: 44px; display: flex; align-items: center; cursor: pointer; font-weight: 700; font-size: 12px; }
    .tenant-mobile-history-row { display: flex; justify-content: space-between; gap: 8px; padding: 8px 0; border-top: 1px solid var(--border); font-size: 11px; }
    .tenant-month-details .payment-item .amount { font-size: 13px; }
    @media (max-width: 760px) { .tenant-month-details .payment-item { grid-template-columns: 1fr 1fr; } .tenant-month-details .payment-item .payment-description { grid-column: 1 / -1; } }
    @media (max-width: 640px) {
      .tenant-table-wrap { display: none; }
      .tenant-mobile-list { display: grid; gap: 9px; }
      .row-actions .btn { min-height: 44px; }
      .tenant-search { padding: 12px 14px; }
      .tenant-search input, .tenant-search .btn { min-height: 44px; }
      /* P1：账单安排列实测只剩 59px，把「每月 1 日 / 2026-07-01 至 2026-09-30」
         断成 5 行。112px 够放两行。表格地板同步抬高，否则从这一列多拿的宽度
         会从别的列抠走；680px 是共享表给无表头页的地板，这里列更多。 */
      .table-wrap > table > thead > tr > th:nth-child(4),
      .table-wrap > table > tbody > tr > td:nth-child(4) { min-width: 112px; }
      table { min-width: 740px; }
    }
  `+`</style>
</head>
<body>
  <div class="app">
    {{template "workspace-nav" .}}
    <main class="content">
      <header class="topbar">
        <div>
          <div class="brand-title">租客管理</div>
          <h1>租客资料</h1>
        </div>
		<div class="actions"><a class="btn primary" href="/tenants?add=1{{if .Search}}&amp;search={{urlquery .Search}}{{end}}{{if .Sort}}&amp;sort={{urlquery .Sort}}{{end}}">添加租客</a><form method="post" action="/logout"><button class="btn danger" type="submit">退出登录</button></form></div>
      </header>
      {{template "workspace-object-tabs" .}}

      {{if eq .Message "tenant_added"}}<div class="notice ok" data-toast>租客资料已保存。</div>{{end}}
      {{if eq .Message "tenant_updated"}}<div class="notice ok" data-toast>租客资料已更新。</div>{{end}}
		{{if eq .Message "tenant_deleted"}}<div class="notice ok" data-toast>租客已删除。</div>{{end}}
		{{if and .Error (not .ShowForm)}}<div class="notice error">请检查租客姓名、邮箱、付款人名称及状态。</div>{{end}}

      <section class="summary" aria-label="Tenant summary">
        <div class="panel metric"><div class="label">租客数量</div><strong>{{.TenantCount}}</strong><span>当前保存的租客</span></div>
        <div class="panel metric"><div class="label">有效档案</div><strong>{{.ActiveCount}}</strong><span>可用于入住与租金安排</span></div>
      </section>

	  {{if .ShowForm}}{{template "tenant-form-drawer" .}}{{end}}
      <section class="panel surface" aria-labelledby="tenant-list-title">
        <form class="tenant-search collection-filters" method="get" action="/tenants"><label class="bills-search" for="tenant-search"><span class="sr-only">搜索</span><input id="tenant-search" type="search" name="search" value="{{.Search}}" placeholder="姓名、邮箱或付款人" aria-label="搜索租客"></label><input type="hidden" name="sort" value="{{.Sort}}"><button class="btn" type="submit">搜索</button>{{if .Search}}<a class="btn subtle" href="/tenants{{if .Sort}}?sort={{urlquery .Sort}}{{end}}">清除</a>{{end}}</form>
        <div class="panel-head"><h2 id="tenant-list-title">租客列表</h2><span class="tiny">{{.FilteredCount}} / {{.TenantCount}} 条记录</span></div>
        {{if .Rows}}
        <div class="tenant-mobile-list" aria-label="移动端租客列表">
          {{range .Rows}}<article class="tenant-mobile-card panel"><div class="tenant-mobile-head"><div><h3>{{if .DisplayAlias}}{{.DisplayAlias}}{{else}}{{.Name}}{{end}}</h3>{{if .DisplayAlias}}<p>{{.Name}}</p>{{end}}<p>{{if .Email}}{{.Email}}{{else}}未填写邮箱{{end}}</p><p>{{if .PayerNameHint}}{{.PayerNameHint}}{{else}}暂无付款识别{{end}}</p></div><span class="status {{.Status}}">{{if eq .Status "active"}}有效{{else}}已停用{{end}}</span></div><div class="tenant-mobile-actions"><a class="btn subtle" href="{{if .ListDetailURL}}{{.ListDetailURL}}{{else}}/tenants/{{.ID}}{{end}}">详情</a><a class="btn subtle" href="{{if .ListEditURL}}{{.ListEditURL}}{{else}}/tenants?edit={{.ID}}{{end}}">编辑档案</a></div>{{if .BillingHistory}}<details class="tenant-mobile-history"><summary>查看最近六个月租金账单</summary>{{range .BillingHistory}}<div class="tenant-mobile-history-row"><span>{{.PeriodLabel}} · {{.StatusLabel}}</span><strong>{{.BalanceAmount}}</strong></div>{{end}}</details>{{end}}</article>{{end}}
        </div>
        <div class="tenant-table-wrap table-wrap">
          <table>
            <thead><tr>{{template "table-sort-heading" (tableSortHeading "租客" (index .SortLinks "name"))}}{{template "table-sort-heading" (tableSortHeading "付款识别" (index .SortLinks "payer"))}}{{template "table-sort-heading" (tableSortHeading "状态" (index .SortLinks "status"))}}{{template "table-sort-heading" (tableSortHeading "创建时间" (index .SortLinks "created"))}}<th>操作</th></tr></thead>
            <tbody>
              {{range .Rows}}
              <tr class="tenant-row" tabindex="0" role="button" aria-expanded="false" aria-controls="tenant-billing-{{.ID}}" data-tenant-target="tenant-billing-{{.ID}}">
                <td><strong>{{if .DisplayAlias}}{{.DisplayAlias}}{{else}}{{.Name}}{{end}}</strong><br>{{if .DisplayAlias}}<span class="tiny">{{.Name}}</span><br>{{end}}<span class="mono">{{.ID}}</span></td>
                <td>{{if .PayerNameHint}}{{.PayerNameHint}}{{else}}暂无付款人名称{{end}}<br><span class="mono">付款人 ID：{{if .PayerID}}{{.PayerID}}{{else}}暂无{{end}}</span></td>
                <td><span class="status {{.Status}}">{{if eq .Status "active"}}有效{{else}}已停用{{end}}</span></td>
                <td class="mono">{{.CreatedAt}}</td>
                <td><div class="row-actions"><a class="btn subtle" href="{{if .ListDetailURL}}{{.ListDetailURL}}{{else}}/tenants/{{.ID}}{{end}}">详情</a><a class="btn subtle" href="{{if .ListEditURL}}{{.ListEditURL}}{{else}}/tenants?edit={{.ID}}{{end}}">编辑档案</a></div></td>
              </tr>
              <tr id="tenant-billing-{{.ID}}" class="tenant-history-row" hidden><td colspan="5"><div class="tenant-history">
                <div class="tenant-history-head"><h3>最近六个月个人租金账单</h3><span class="tiny">账单按月份、房间分别保留</span></div>
                {{if .BillingHistory}}
                <div class="table-wrap"><table class="tenant-history-table">
                  <thead><tr><th>月份</th><th>应收</th><th>实际收</th><th>未收</th><th>状态</th></tr></thead>
                  <tbody>{{range .BillingHistory}}
                    <tr class="tenant-month-row" tabindex="0" role="button" aria-expanded="false" aria-controls="tenant-month-{{.ObligationID}}" data-month-target="tenant-month-{{.ObligationID}}"><td><strong>{{.PeriodLabel}}</strong><br><span class="mono">应缴日 {{.DueDate}}</span></td><td class="amount">{{.ExpectedAmount}}</td><td class="amount">{{.PaidAmount}}</td><td class="amount">{{.BalanceAmount}}</td><td><span class="status {{.Status}}">{{.StatusLabel}}</span></td></tr>
                    <tr id="tenant-month-{{.ObligationID}}" class="tenant-month-details" hidden><td colspan="5"><div class="payment-list"><h3>缴费流水明细</h3>{{if .Payments}}{{range .Payments}}<div class="payment-item"><span class="amount">{{.AmountDisplay}}</span><span class="mono">{{.DateDisplay}}</span><span class="payment-description">{{.Description}}</span><span class="mono">{{.ConfirmationSource}}</span></div>{{end}}{{else}}<div class="tiny">该月暂无已确认缴费。</div>{{end}}</div></td></tr>
                  {{end}}</tbody>
                </table></div>
                {{else}}<div class="empty">最近六个月暂无适用账单。</div>{{end}}
              </div></td></tr>
              {{end}}
            </tbody>
          </table>
        </div>
        {{else}}<div class="empty">{{if .Search}}没有符合“{{.Search}}”的租客。{{else}}还没有租客，请点击“添加租客”创建第一条资料。{{end}}</div>{{end}}
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
`)
