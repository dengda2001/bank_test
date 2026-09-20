package main

import (
	"context"
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

// pageActionView is the stable action contract shared by desktop and mobile
// renderers. A renderer can choose a button, drawer or sheet without parsing a
// human-facing label to discover whether a reason or confirmation is needed.
type pageActionView struct {
	Key                  string
	Method               string
	URL                  string
	Label                string
	RequiresReason       bool
	RequiresConfirmation bool
	Enabled              bool
	DisabledReason       string
}

type propertyPageRow struct {
	ID                    uint64
	Mark                  string
	Name                  string
	CityRegion            string
	Address               string
	Timezone              string
	Notes                 string
	ResponsibilityCount   int
	Status                string
	StatusLabel           string
	CollectionStatus      string
	CollectionStatusLabel string
	RoomCount             int
	ActiveRoomCount       int
	ExpectedCents         int64
	PaidCents             int64
	BalanceCents          int64
	ExpenseCents          int64
	OtherIncomeCents      int64
	ExpectedAmount        string
	PaidAmount            string
	BalanceAmount         string
	ExpenseAmount         string
	OtherIncomeAmount     string
	NetAmount             string
	CollectionPercent     int
	Actions               []pageActionView
}

type propertyPageData struct {
	workspaceShell
	Period           string
	PeriodLabel      string
	PeriodOptions    []pagePeriodOption
	Rows             []propertyPageRow
	Form             propertyPageForm
	ShowForm         bool
	Message          string
	Error            string
	StatusFilter     string
	CollectionFilter string
	Search           string
}

type propertyPageForm struct {
	ID         uint64
	Name       string
	CityRegion string
	Address    string
	Timezone   string
	Notes      string
}

type propertyDetailPageData struct {
	workspaceShell
	Period         string
	PeriodLabel    string
	ListStatus     string
	ListSearch     string
	ListCollection string
	Property       propertyPageRow
	Rooms          []roomPageRow
	Expenses       []expenseRecord
	ExpenseDrawer  *expenseDrawerData
	RoomDrawer     *roomCreateDrawerData
	RoomOpenURL    string
	Editing        bool
	Error          string
	Message        string
}

type roomPageRow struct {
	ID                    uint64
	PropertyID            uint64
	PropertyName          string
	RoomLabel             string
	RoomType              string
	Capacity              int
	Notes                 string
	Status                string
	StatusLabel           string
	CollectionStatus      string
	CollectionStatusLabel string
	ActiveFrom            string
	InactiveFrom          string
	TenantNames           []string
	MonthlyRentCents      int64
	MonthlyRent           string
	Currency              string
	DueDay                int
	ExpectedCents         int64
	PaidCents             int64
	BalanceCents          int64
	ExpectedAmount        string
	PaidAmount            string
	BalanceAmount         string
	Actions               []pageActionView
}

type roomPageData struct {
	workspaceShell
	Period           string
	PeriodLabel      string
	PeriodOptions    []pagePeriodOption
	Rows             []roomPageRow
	Properties       []propertyPageRow
	PropertyOptions  []propertyPageRow
	Form             roomPageForm
	ShowForm         bool
	Message          string
	Error            string
	PropertyID       uint64
	StatusFilter     string
	CollectionFilter string
	Search           string
	Drawer           *roomCreateDrawerData
}

type roomCreateDrawerData struct {
	Period               string
	StatusFilter         string
	CollectionFilter     string
	Search               string
	PropertyID           uint64
	Properties           []propertyPageRow
	Form                 roomPageForm
	ReturnURL            string
	ReturnContext        string
	ReturnPropertyID     uint64
	ReturnPeriod         string
	ReturnListStatus     string
	ReturnListSearch     string
	ReturnListCollection string
	ErrorMessage         string
}

type pagePeriodOption struct {
	Value string
	Label string
}

func pagePeriodOptions(period time.Time) []pagePeriodOption {
	options := make([]pagePeriodOption, 0, 12)
	for offset := 0; offset < 12; offset++ {
		month := monthStart(period).AddDate(0, -offset, 0)
		options = append(options, pagePeriodOption{Value: month.Format("2006-01"), Label: formatMonthLabel(month)})
	}
	return options
}

type roomPageForm struct {
	ID               uint64
	PropertyID       uint64
	RoomLabel        string
	RoomType         string
	Capacity         int
	MonthlyRentValue string
	DueDay           int
	Notes            string
	ActiveFrom       string
}

type roomEditPageData struct {
	workspaceShell
	Form       roomPageForm
	Properties []propertyPageRow
	Error      string
}

type tenancyPartyView struct {
	TenantID            uint64
	TenantName          string
	ResponsibilityCents int64
	Responsibility      string
	JoinedAt            string
	LeftAt              string
}

type tenancyPageRow struct {
	ID               uint64
	RecordLabel      string
	RoomID           uint64
	RoomLabel        string
	PropertyID       uint64
	PropertyName     string
	StartDate        string
	ContractDate     string
	MoveInDate       string
	EndDate          string
	MonthlyRent      string
	MonthlyRentCents int64
	Currency         string
	DueDay           int
	Status           string
	StatusLabel      string
	TenantID         uint64
	TenantName       string
	Responsibility   string
	Parties          []tenancyPartyView
}

type tenancyPageData struct {
	workspaceShell
	Rows         []tenancyPageRow
	TableRows    []tenancyPageRow
	Period       string
	StatusFilter string
	Search       string
	ShowCreate   bool
	Rooms        []tenancyRoomOption
	Tenants      []tenancyTenantOption
	Form         tenancyFormData
	Message      string
	Error        string
}

type cashReceiptPageRow struct {
	ID            uint64
	ReceiptNumber string
	TenantID      uint64
	TenantName    string
	RoomLabel     string
	ObligationID  uint64
	Period        string
	AmountCents   int64
	Amount        string
	Currency      string
	ReceivedAt    string
	Status        string
	StatusLabel   string
	Note          string
	VoidReason    string
	VoidURL       string
}

type cashReceiptPageData struct {
	workspaceShell
	Rows         []cashReceiptPageRow
	Period       string
	StatusFilter string
	Search       string
	ShowForm     bool
	Form         cashReceiptFormData
	Drawer       *cashReceiptDrawerData
	Message      string
	Error        string
}

type bankPageAccount struct {
	AccountID        string
	AccountName      string
	Status           string
	StatusLabel      string
	CoveredFrom      string
	CoveredTo        string
	TransactionCount int
	Error            string
}

type bankPageRun struct {
	ID            uint64
	Mode          string
	Status        string
	StatusLabel   string
	RequestedFrom string
	RequestedTo   string
	StartedAt     string
	FinishedAt    string
	Error         string
	Accounts      []bankPageAccount
}

type bankPageData struct {
	workspaceShell
	Connected              bool
	Provider               string
	Environment            string
	SavedAt                string
	LastSyncAt             string
	SyncStatus             string
	SyncStatusLabel        string
	SyncCoverage           string
	LastSuccessfulCoverage string
	Runs                   []bankPageRun
	Message                string
	Error                  string
}

func pageStatusLabel(status string) string {
	switch status {
	case "active":
		return "有效"
	case "inactive":
		return "已停用"
	case "vacant":
		return "空置"
	case "open":
		return "待收"
	case "partial":
		return "部分收款"
	case "paid":
		return "已收齐"
	case "overdue":
		return "逾期"
	case "needs_review":
		return "待处理"
	case bankSyncStatusRunning:
		return "同步中"
	case bankSyncStatusSucceeded:
		return "同步成功"
	case bankSyncStatusFailed:
		return "同步失败"
	case "not_connected":
		return "未连接"
	case cashReceiptStatusConfirmed:
		return "已确认"
	case cashReceiptStatusVoided:
		return "已作废"
	default:
		return firstNonEmpty(status, "未知")
	}
}

func collectionStatus(expectedCents, paidCents int64) (string, string) {
	switch {
	case expectedCents <= 0:
		return "vacant", "无应收"
	case paidCents <= 0:
		return "overdue", "未收"
	case paidCents < expectedCents:
		return "partial", "部分未收"
	default:
		return "paid", "已收齐"
	}
}

func propertyMark(name string) string {
	fields := strings.Fields(strings.TrimSpace(name))
	if len(fields) == 0 {
		return "R"
	}
	var mark strings.Builder
	for _, character := range fields[0] {
		if character < '0' || character > '9' {
			break
		}
		mark.WriteRune(character)
	}
	if mark.Len() > 0 {
		return mark.String()
	}
	for _, character := range fields[0] {
		return strings.ToUpper(string(character))
	}
	return "R"
}

func matchesCollectionFilter(status, filter string) bool {
	switch filter {
	case "unpaid":
		return status == "overdue" || status == "partial"
	case "paid":
		return status == "paid"
	default:
		return true
	}
}

func bankStatusLabel(status string) string {
	if status == "partial" {
		return "部分成功"
	}
	return pageStatusLabel(status)
}

func pageDate(value *time.Time) string {
	if value == nil || value.IsZero() {
		return ""
	}
	return value.UTC().Format(dateLayout)
}

func pageDateTime(value *time.Time) string {
	if value == nil || value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func pageCurrencyAmount(cents int64, currency string) string {
	return formatMoney(centsToMoney(cents), firstNonEmpty(currency, ledgerCurrencyEUR), 2)
}

func canonicalPageShell(a *app, r *http.Request, active string, note string) workspaceShell {
	compactTitle := map[string]string{
		"rent-dashboard": "本月收租",
		"bills":          "账单",
		"transactions":   "流水",
		"dunning":        "催收任务",
		"properties":     "对象管理",
		"rooms":          "对象管理",
		"tenants":        "对象管理",
		"tenancies":      "租约",
		"cash-receipts":  "现金补录",
		"expenses":       "费用支出",
		"bank":           "银行设置",
		"more":           "更多",
	}[active]
	return a.fillWorkspaceShell(r, workspaceShell{ActivePage: active, Username: a.displayUsername(r), Environment: a.cfg.Environment, FootNote: note, CompactTitle: compactTitle, ShowNavCounts: true})
}

func bankRefreshRedirect(r *http.Request, query string) string {
	path := "/billing"
	if r != nil && strings.HasPrefix(r.URL.Path, "/bank/") {
		path = "/bank"
	}
	return path + "?" + query
}

func (a *app) scopedPageUser(w http.ResponseWriter, r *http.Request) (uint64, bool) {
	if !a.requireAuth(w, r) {
		return 0, false
	}
	userID, ok := a.currentUserID(r)
	if !ok || a.db == nil {
		http.Error(w, "database session required", http.StatusServiceUnavailable)
		return 0, false
	}
	return userID, true
}

// Canonical aliases intentionally call the existing read/action handlers. This
// keeps one accounting implementation while giving the desktop and mobile
// workspaces stable URLs.
func (a *app) handleBills(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if _, ok := a.scopedPageUser(w, r); !ok {
		return
	}
	a.renderRentDashboard(w, r, nil)
}

func (a *app) handleTransactions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if _, ok := a.scopedPageUser(w, r); !ok {
		return
	}
	a.handleBilling(w, r)
}

func (a *app) handleDunningPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if _, ok := a.scopedPageUser(w, r); !ok {
		return
	}
	period, err := parsePeriodMonth(strings.TrimSpace(r.URL.Query().Get("period")))
	if err != nil {
		http.Error(w, "period is invalid", http.StatusBadRequest)
		return
	}
	filters, err := rentDashboardFiltersFromQuery(r.URL.Query())
	if err != nil {
		filters = defaultRentDashboardFilters()
	}
	a.renderRentDashboard(w, r, &dunningDashboardAction{Kind: dunningActionPage, Period: period, Filters: filters})
}

func (a *app) handleRoomRoute(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/rooms/"), "/")
	if path == "" {
		a.handleRooms(w, r)
		return
	}
	if r.Method == http.MethodGet {
		a.handleRoomDetail(w, r)
		return
	}
	if r.Method == http.MethodPost {
		a.handleRoomMutation(w, r, path)
		return
	}
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func (a *app) handleRoomEdit(w http.ResponseWriter, r *http.Request, pathID string) {
	if !a.requireAuth(w, r) {
		return
	}
	roomID, err := parsePositiveUint(pathID)
	if err != nil || a.db == nil {
		http.NotFound(w, r)
		return
	}
	userID, ok := a.currentUserID(r)
	if !ok {
		http.Error(w, "database session required", http.StatusServiceUnavailable)
		return
	}
	repo := newLandlordRentRepository(a.db)
	roomRow, err := repo.findRoom(r.Context(), userID, roomID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	properties, err := repo.listProperties(r.Context(), userID, propertyQuery{Status: "active"})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	propertyRows := make([]propertyPageRow, 0, len(properties))
	for _, propertyRow := range properties {
		propertyRows = append(propertyRows, propertyPageRow{ID: propertyRow.ID, Name: propertyRow.Name, Address: propertyAddress(propertyRow), Status: propertyRow.Status, StatusLabel: pageStatusLabel(propertyRow.Status)})
	}
	data := roomEditPageData{workspaceShell: canonicalPageShell(a, r, "rooms", "房间编辑"), Form: roomPageForm{ID: roomRow.ID, PropertyID: roomRow.PropertyID, RoomLabel: roomRow.RoomLabel, RoomType: roomRow.RoomType, Capacity: roomRow.Capacity, Notes: stringValue(roomRow.Notes), ActiveFrom: roomRow.ActiveFrom.Format("2006-01")}, Properties: propertyRows, Error: r.URL.Query().Get("error")}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := roomEditPageTemplate.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (a *app) handlePropertyRoute(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/properties/"), "/")
	if path == "" {
		a.handleProperties(w, r)
		return
	}
	if !a.requireAuth(w, r) {
		return
	}
	propertyID, err := strconv.ParseUint(path, 10, 64)
	if err != nil || propertyID == 0 {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodGet {
		a.handlePropertyDetail(w, r, propertyID)
		return
	}
	if r.Method == http.MethodPost {
		a.handlePropertyMutation(w, r, propertyID)
		return
	}
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func (a *app) handleCashReceipts(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		a.handleCashReceiptList(w, r)
		return
	}
	a.handleCashReceiptCreate(w, r)
}

func propertyActions(id uint64, active bool) []pageActionView {
	rowID := strconv.FormatUint(id, 10)
	actions := []pageActionView{{Key: "edit", Method: http.MethodPost, URL: "/properties/" + rowID, Label: "编辑", Enabled: true}}
	if active {
		actions = append(actions, pageActionView{Key: "deactivate", Method: http.MethodPost, URL: "/properties/" + rowID, Label: "停用", RequiresReason: false, RequiresConfirmation: true, Enabled: true})
	}
	return actions
}

func roomActions(id uint64, active bool) []pageActionView {
	rowID := strconv.FormatUint(id, 10)
	actions := []pageActionView{{Key: "edit", Method: http.MethodPost, URL: "/rooms/" + rowID, Label: "编辑", Enabled: true}}
	if active {
		actions = append(actions, pageActionView{Key: "deactivate", Method: http.MethodPost, URL: "/rooms/" + rowID, Label: "停用", RequiresConfirmation: true, Enabled: true})
	}
	return actions
}

func (a *app) loadPropertyPage(ctx context.Context, userID uint64, period time.Time, statusFilter string) (propertyPageData, error) {
	statusFilter = firstNonEmpty(strings.TrimSpace(statusFilter), "active")
	if statusFilter != "all" && statusFilter != "active" && statusFilter != "inactive" {
		return propertyPageData{}, errors.New("property status filter is invalid")
	}
	properties, err := newLandlordRentRepository(a.db).listProperties(ctx, userID, propertyQuery{Status: func() string {
		if statusFilter == "all" {
			return ""
		}
		return statusFilter
	}()})
	if err != nil {
		return propertyPageData{}, err
	}
	rooms, err := newLandlordRentRepository(a.db).listRooms(ctx, userID, roomQuery{})
	if err != nil {
		return propertyPageData{}, err
	}
	financial := make(map[uint64]rentWorkspacePropertyRow)
	responsibilityCount := make(map[uint64]int)
	if activeRows, loadErr := newRentWorkspaceService(a.db).load(ctx, userID, func() rentWorkspaceFilters {
		filters := defaultRentWorkspaceFilters(period)
		filters.PageSize = rentWorkspaceMaxPageSize
		return filters
	}()); loadErr == nil {
		for _, row := range activeRows.PropertyRows {
			financial[row.PropertyID] = row
		}
		for _, row := range activeRows.TenantRows {
			responsibilityCount[row.PropertyID]++
		}
	}
	roomCount := make(map[uint64]int)
	activeRoomCount := make(map[uint64]int)
	for _, row := range rooms {
		roomCount[row.PropertyID]++
		if row.Status == "active" && roomActiveInMonth(row, period) {
			activeRoomCount[row.PropertyID]++
		}
	}
	rows := make([]propertyPageRow, 0, len(properties))
	for _, row := range properties {
		financialRow := financial[row.ID]
		currency := ledgerCurrencyEUR
		collectionState, collectionLabel := collectionStatus(financialRow.ExpectedCents, financialRow.PaidCents)
		rows = append(rows, propertyPageRow{
			ID: row.ID, Mark: propertyMark(row.Name), Name: row.Name, CityRegion: row.CityRegion, Address: propertyAddress(row), Timezone: row.Timezone, Notes: stringValue(row.Notes), Status: row.Status,
			ResponsibilityCount: responsibilityCount[row.ID],
			StatusLabel:         pageStatusLabel(row.Status), CollectionStatus: collectionState, CollectionStatusLabel: collectionLabel, RoomCount: roomCount[row.ID], ActiveRoomCount: activeRoomCount[row.ID],
			ExpectedCents: financialRow.ExpectedCents, PaidCents: financialRow.PaidCents, BalanceCents: financialRow.BalanceCents, ExpenseCents: financialRow.ExpenseCents, OtherIncomeCents: financialRow.OtherIncomeCents,
			ExpectedAmount: pageCurrencyAmount(financialRow.ExpectedCents, currency), PaidAmount: pageCurrencyAmount(financialRow.PaidCents, currency), BalanceAmount: pageCurrencyAmount(financialRow.BalanceCents, currency), ExpenseAmount: pageCurrencyAmount(financialRow.ExpenseCents, currency), OtherIncomeAmount: pageCurrencyAmount(financialRow.OtherIncomeCents, currency), CollectionPercent: financialRow.CollectionPercent,
			NetAmount: pageCurrencyAmount(financialRow.NetCents, currency),
			Actions:   propertyActions(row.ID, row.Status == "active"),
		})
	}
	return propertyPageData{Period: monthStart(period).Format("2006-01"), PeriodLabel: formatMonthLabel(period), Rows: rows, StatusFilter: statusFilter}, nil
}

func filterPropertyPageRows(rows []propertyPageRow, search string) []propertyPageRow {
	needle := strings.ToLower(strings.TrimSpace(search))
	if needle == "" {
		return rows
	}
	filtered := make([]propertyPageRow, 0, len(rows))
	for _, row := range rows {
		if strings.Contains(strings.ToLower(row.Name+" "+row.CityRegion), needle) || strings.Contains(strings.ToLower(row.Address), needle) {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

func filterPropertyCollectionRows(rows []propertyPageRow, filter string) []propertyPageRow {
	if filter == "" || filter == "all" {
		return rows
	}
	filtered := make([]propertyPageRow, 0, len(rows))
	for _, row := range rows {
		if matchesCollectionFilter(row.CollectionStatus, filter) {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

func filterRoomCollectionRows(rows []roomPageRow, filter string) []roomPageRow {
	if filter == "" || filter == "all" {
		return rows
	}
	filtered := make([]roomPageRow, 0, len(rows))
	for _, row := range rows {
		if matchesCollectionFilter(row.CollectionStatus, filter) {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

func filterRoomPageRows(rows []roomPageRow, search, status string) []roomPageRow {
	needle := strings.ToLower(strings.TrimSpace(search))
	if status == "" {
		status = "all"
	}
	filtered := make([]roomPageRow, 0, len(rows))
	for _, row := range rows {
		if status != "all" && row.Status != status {
			continue
		}
		if needle != "" {
			match := strings.Contains(strings.ToLower(row.RoomLabel), needle) || strings.Contains(strings.ToLower(row.PropertyName), needle)
			for _, tenantName := range row.TenantNames {
				if strings.Contains(strings.ToLower(tenantName), needle) {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}
		filtered = append(filtered, row)
	}
	return filtered
}

func (a *app) handleProperties(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		a.handlePropertyMutation(w, r, 0)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := a.scopedPageUser(w, r)
	if !ok {
		return
	}
	period, err := parsePeriodMonth(strings.TrimSpace(r.URL.Query().Get("period")))
	if err != nil {
		http.Error(w, "period is invalid", http.StatusBadRequest)
		return
	}
	data, err := a.loadPropertyPage(r.Context(), userID, period, r.URL.Query().Get("status"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	data.workspaceShell = canonicalPageShell(a, r, "properties", "房产与房间管理")
	data.PeriodOptions = pagePeriodOptions(period)
	data.Search = strings.TrimSpace(r.URL.Query().Get("search"))
	data.Rows = filterPropertyPageRows(data.Rows, data.Search)
	data.CollectionFilter = firstNonEmpty(strings.TrimSpace(r.URL.Query().Get("collection")), "all")
	if data.CollectionFilter != "all" && data.CollectionFilter != "unpaid" && data.CollectionFilter != "paid" {
		http.Error(w, "property collection filter is invalid", http.StatusBadRequest)
		return
	}
	data.Rows = filterPropertyCollectionRows(data.Rows, data.CollectionFilter)
	data.ShowForm = r.URL.Query().Get("add") == "1"
	data.Form = propertyPageForm{Timezone: "Europe/Dublin"}
	data.Message, data.Error = r.URL.Query().Get("message"), r.URL.Query().Get("error")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := propertyPageTemplate.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (a *app) handlePropertyDetail(w http.ResponseWriter, r *http.Request, propertyID uint64) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := a.scopedPageUser(w, r)
	if !ok {
		return
	}
	propertyRow, err := newLandlordRentRepository(a.db).findProperty(r.Context(), userID, propertyID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	period, err := parsePeriodMonth(strings.TrimSpace(r.URL.Query().Get("period")))
	if err != nil {
		http.Error(w, "period is invalid", http.StatusBadRequest)
		return
	}
	rooms, err := a.loadRoomRows(r.Context(), userID, period, propertyID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	expenses, err := newExpenseService(a.db).listExpenses(r.Context(), userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	filteredExpenses := make([]expenseRecord, 0)
	for _, expense := range expenses {
		if expense.PropertyID == strconv.FormatUint(propertyID, 10) {
			filteredExpenses = append(filteredExpenses, expense)
		}
	}
	listStatus := strings.TrimSpace(r.URL.Query().Get("list_status"))
	if listStatus != "all" && listStatus != "inactive" {
		listStatus = "active"
	}
	listCollection := firstNonEmpty(strings.TrimSpace(r.URL.Query().Get("list_collection")), "all")
	if listCollection != "all" && listCollection != "unpaid" && listCollection != "paid" {
		listCollection = "all"
	}
	data := propertyDetailPageData{workspaceShell: canonicalPageShell(a, r, "properties", "房产详情"), Period: period.Format("2006-01"), PeriodLabel: formatMonthLabel(period), ListStatus: listStatus, ListSearch: strings.TrimSpace(r.URL.Query().Get("list_search")), ListCollection: listCollection, Property: propertyPageRow{ID: propertyRow.ID, Mark: propertyMark(propertyRow.Name), Name: propertyRow.Name, CityRegion: propertyRow.CityRegion, Address: propertyAddress(propertyRow), Timezone: propertyRow.Timezone, Notes: stringValue(propertyRow.Notes), Status: propertyRow.Status, StatusLabel: pageStatusLabel(propertyRow.Status), Actions: propertyActions(propertyRow.ID, propertyRow.Status == "active")}, Rooms: rooms, Expenses: filteredExpenses, Editing: r.URL.Query().Get("edit") == "1", Message: r.URL.Query().Get("message"), Error: r.URL.Query().Get("error")}
	data.RoomOpenURL = propertyRoomOpenURL(r)
	if r.URL.Query().Get("room") == "1" || r.URL.Query().Get("room_error") != "" {
		returnURL := propertyDetailReturnURL(r)
		form := roomPageForm{PropertyID: propertyID, Capacity: 1, DueDay: 1, ActiveFrom: period.Format("2006-01")}
		data.RoomDrawer = &roomCreateDrawerData{
			Period: period.Format("2006-01"), StatusFilter: "all", CollectionFilter: "all", PropertyID: propertyID,
			Properties: []propertyPageRow{{ID: propertyRow.ID, Name: propertyRow.Name}}, Form: form, ReturnURL: returnURL,
			ReturnContext: "property", ReturnPropertyID: propertyID, ReturnPeriod: period.Format("2006-01"),
			ReturnListStatus: listStatus, ReturnListSearch: strings.TrimSpace(r.URL.Query().Get("list_search")), ReturnListCollection: listCollection,
			ErrorMessage: roomMutationErrorMessage(r.URL.Query().Get("room_error")),
		}
	}
	if r.URL.Query().Get("expense") == "1" || isExpenseFormError(r.URL.Query().Get("error")) {
		returnURL := expenseFormReturnURL(r)
		expenseDrawer, drawerErr := a.loadExpenseDrawerData(r.Context(), userID, period.Format("2006-01"), propertyID, 0, returnURL, r.URL.Query().Get("error"))
		if drawerErr != nil {
			http.Error(w, drawerErr.Error(), http.StatusInternalServerError)
			return
		}
		data.ExpenseDrawer = expenseDrawer
	}
	if summary, summaryErr := a.loadPropertyPage(r.Context(), userID, period, "all"); summaryErr == nil {
		for _, row := range summary.Rows {
			if row.ID == propertyID {
				data.Property = row
				break
			}
		}
	}
	data.Property.RoomCount, data.Property.ActiveRoomCount = len(rooms), 0
	data.CompactTitle = data.Property.Name
	for _, room := range rooms {
		if room.Status == "active" {
			data.Property.ActiveRoomCount++
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := propertyDetailPageTemplate.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func propertyDetailReturnURL(r *http.Request) string {
	query := cloneQueryValues(r.URL.Query())
	for _, key := range []string{"edit", "expense", "room", "room_error", "message", "error", "cash"} {
		query.Del(key)
	}
	return r.URL.Path + cashReceiptEncodedQuery(query)
}

func propertyRoomOpenURL(r *http.Request) string {
	base := propertyDetailReturnURL(r)
	target, err := url.ParseRequestURI(base)
	if err != nil {
		return base
	}
	query := target.Query()
	query.Set("room", "1")
	target.RawQuery = query.Encode()
	return target.RequestURI()
}

func (a *app) handlePropertyMutation(w http.ResponseWriter, r *http.Request, propertyID uint64) {
	if !a.requireAuth(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := a.currentUserID(r)
	if !ok || a.db == nil {
		http.Error(w, "database session required", http.StatusServiceUnavailable)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/properties?error=invalid_form", http.StatusFound)
		return
	}
	if propertyID == 0 {
		if raw := strings.TrimSpace(r.Form.Get("property_id")); raw != "" {
			propertyID, _ = strconv.ParseUint(raw, 10, 64)
		}
	}
	action := firstNonEmpty(strings.TrimSpace(r.Form.Get("action")), "save")
	service := newLandlordDomainService(a.db)
	var err error
	switch action {
	case "edit":
		if propertyID == 0 {
			redirectPropertyList(w, r, "", "property_action_failed", false)
			return
		}
		redirectPropertyMutation(w, r, propertyID, "", true)
		return
	case "deactivate":
		effective, parseErr := parsePeriodMonth(strings.TrimSpace(r.Form.Get("effective_month")))
		if parseErr != nil {
			err = parseErr
		} else {
			err = service.deactivateProperty(r.Context(), userID, propertyID, effective)
		}
	case "save":
		input := propertyInput{Name: r.Form.Get("name"), CityRegion: r.Form.Get("city_region"), Address: r.Form.Get("address"), Timezone: r.Form.Get("timezone"), Notes: r.Form.Get("notes")}
		if propertyID == 0 {
			_, err = service.createProperty(r.Context(), userID, input)
		} else {
			_, err = service.updateProperty(r.Context(), userID, propertyID, input)
		}
	default:
		err = errors.New("property action is invalid")
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		if action == "save" && propertyID != 0 {
			redirectPropertyMutation(w, r, propertyID, "property_action_failed", true)
			return
		}
		redirectPropertyList(w, r, "", "property_action_failed", action == "save")
		return
	}
	if action == "save" && propertyID != 0 {
		redirectPropertyMutation(w, r, propertyID, "property_saved", false)
		return
	}
	if action == "deactivate" {
		redirectPropertyList(w, r, "property_deactivated", "", false)
		return
	}
	redirectPropertyList(w, r, "property_saved", "", false)
}

func redirectPropertyMutation(w http.ResponseWriter, r *http.Request, propertyID uint64, message string, editing bool) {
	query := url.Values{}
	if message != "" {
		query.Set("message", message)
	}
	if period, err := parsePeriodMonth(strings.TrimSpace(r.Form.Get("period"))); err == nil {
		query.Set("period", period.Format("2006-01"))
	}
	listStatus := strings.TrimSpace(r.Form.Get("list_status"))
	if listStatus == "all" || listStatus == "inactive" {
		query.Set("list_status", listStatus)
	} else if listStatus == "active" {
		query.Set("list_status", "active")
	}
	if listSearch := strings.TrimSpace(r.Form.Get("list_search")); listSearch != "" {
		query.Set("list_search", listSearch)
	}
	listCollection := strings.TrimSpace(r.Form.Get("list_collection"))
	if listCollection == "unpaid" || listCollection == "paid" {
		query.Set("list_collection", listCollection)
	}
	if editing {
		query.Set("edit", "1")
	}
	http.Redirect(w, r, "/properties/"+strconv.FormatUint(propertyID, 10)+"?"+query.Encode(), http.StatusFound)
}

func redirectPropertyList(w http.ResponseWriter, r *http.Request, message, errorCode string, showForm bool) {
	query := url.Values{}
	query.Set("period", validatedPeriodValue(r.Form.Get("period")))
	status := strings.TrimSpace(r.Form.Get("status"))
	if status != "all" && status != "inactive" {
		status = "active"
	}
	query.Set("status", status)
	if search := strings.TrimSpace(r.Form.Get("search")); search != "" {
		query.Set("search", search)
	}
	collection := strings.TrimSpace(r.Form.Get("collection"))
	if collection == "unpaid" || collection == "paid" {
		query.Set("collection", collection)
	}
	if message != "" {
		query.Set("message", message)
	}
	if errorCode != "" {
		query.Set("error", errorCode)
	}
	if showForm {
		query.Set("add", "1")
	}
	http.Redirect(w, r, "/properties?"+query.Encode(), http.StatusFound)
}

func validatedPeriodValue(value string) string {
	period, err := parsePeriodMonth(strings.TrimSpace(value))
	if err != nil {
		return time.Now().Format("2006-01")
	}
	return period.Format("2006-01")
}

func redirectRoomList(w http.ResponseWriter, r *http.Request, message, errorCode string, showForm bool) {
	query := url.Values{}
	query.Set("period", validatedPeriodValue(r.Form.Get("period")))
	status := strings.TrimSpace(r.Form.Get("filter_status"))
	if status != "active" && status != "inactive" {
		status = "all"
	}
	query.Set("status", status)
	if propertyID, err := parseOptionalUint(r.Form.Get("filter_property_id")); err == nil && propertyID > 0 {
		query.Set("property_id", strconv.FormatUint(propertyID, 10))
	}
	if search := strings.TrimSpace(r.Form.Get("search")); search != "" {
		query.Set("search", search)
	}
	collection := strings.TrimSpace(r.Form.Get("collection"))
	if collection == "unpaid" || collection == "paid" {
		query.Set("collection", collection)
	}
	if message != "" {
		query.Set("message", message)
	}
	if errorCode != "" {
		query.Set("error", errorCode)
	}
	if showForm {
		query.Set("add", "1")
	}
	http.Redirect(w, r, "/rooms?"+query.Encode(), http.StatusFound)
}

func redirectRoomEdit(w http.ResponseWriter, r *http.Request, roomID uint64, errorCode string) {
	query := url.Values{"edit": []string{"1"}}
	if period, err := parsePeriodMonth(strings.TrimSpace(r.Form.Get("period"))); err == nil {
		query.Set("period", period.Format("2006-01"))
	}
	query.Set("error", errorCode)
	copyRoomReturnContext(query, r.Form)
	http.Redirect(w, r, "/rooms/"+strconv.FormatUint(roomID, 10)+"?"+query.Encode(), http.StatusFound)
}

func roomListURL(period string, propertyID uint64, status, search string, collection ...string) string {
	query := url.Values{"period": []string{validatedPeriodValue(period)}}
	if propertyID > 0 {
		query.Set("property_id", strconv.FormatUint(propertyID, 10))
	}
	if status != "active" && status != "inactive" {
		status = "all"
	}
	query.Set("status", status)
	if search = strings.TrimSpace(search); search != "" {
		query.Set("search", search)
	}
	if len(collection) > 0 && (collection[0] == "unpaid" || collection[0] == "paid") {
		query.Set("collection", collection[0])
	}
	return "/rooms?" + query.Encode()
}

func (a *app) loadRoomRows(ctx context.Context, userID uint64, period time.Time, propertyID uint64) ([]roomPageRow, error) {
	repo := newLandlordRentRepository(a.db)
	rooms, err := repo.listRooms(ctx, userID, roomQuery{PropertyID: propertyID})
	if err != nil {
		return nil, err
	}
	properties, err := repo.listProperties(ctx, userID, propertyQuery{})
	if err != nil {
		return nil, err
	}
	propertyByID := make(map[uint64]property, len(properties))
	for _, row := range properties {
		propertyByID[row.ID] = row
	}
	financial := make(map[uint64]rentWorkspaceRoomRow)
	filters := defaultRentWorkspaceFilters(period)
	filters.View, filters.PageSize, filters.PropertyID = rentWorkspaceViewRooms, rentWorkspaceMaxPageSize, propertyID
	if workspace, loadErr := newRentWorkspaceService(a.db).load(ctx, userID, filters); loadErr == nil {
		for _, row := range workspace.RoomRows {
			financial[row.RoomID] = row
		}
	}
	rows := make([]roomPageRow, 0, len(rooms))
	for _, roomRow := range rooms {
		propertyRow := propertyByID[roomRow.PropertyID]
		agreements, err := repo.listTenancyAgreements(ctx, userID, agreementQuery{RoomID: roomRow.ID, Status: "active"})
		if err != nil {
			return nil, err
		}
		var agreement *tenancyAgreement
		if len(agreements) > 0 {
			agreement = &agreements[0]
		}
		partyNames := make([]string, 0)
		if agreement != nil {
			parties, partyErr := repo.listAgreementParties(ctx, userID, agreementPartyQuery{AgreementID: agreement.ID, Status: "active"})
			if partyErr != nil {
				return nil, partyErr
			}
			for _, party := range parties {
				var tenantRow tenant
				if tenantErr := a.db.WithContext(ctx).Where("id = ? AND user_id = ?", party.TenantID, userID).First(&tenantRow).Error; tenantErr == nil {
					partyNames = append(partyNames, firstNonEmpty(tenantRow.DisplayAlias, tenantRow.Name))
				}
			}
		}
		financialRow := financial[roomRow.ID]
		currency := firstNonEmpty(func() string {
			if agreement != nil {
				return agreement.Currency
			}
			return ""
		}(), ledgerCurrencyEUR)
		collectionState, collectionLabel := collectionStatus(financialRow.ExpectedCents, financialRow.PaidCents)
		pageRow := roomPageRow{ID: roomRow.ID, PropertyID: roomRow.PropertyID, PropertyName: propertyRow.Name, RoomLabel: roomRow.RoomLabel, RoomType: roomRow.RoomType, Capacity: roomRow.Capacity, Notes: stringValue(roomRow.Notes), Status: roomRow.Status, StatusLabel: pageStatusLabel(roomRow.Status), CollectionStatus: collectionState, CollectionStatusLabel: collectionLabel, ActiveFrom: roomRow.ActiveFrom.Format(dateLayout), InactiveFrom: pageDate(roomRow.InactiveFrom), TenantNames: partyNames, Currency: currency, ExpectedCents: financialRow.ExpectedCents, PaidCents: financialRow.PaidCents, BalanceCents: financialRow.BalanceCents, ExpectedAmount: pageCurrencyAmount(financialRow.ExpectedCents, currency), PaidAmount: pageCurrencyAmount(financialRow.PaidCents, currency), BalanceAmount: pageCurrencyAmount(financialRow.BalanceCents, currency), Actions: roomActions(roomRow.ID, roomRow.Status == "active")}
		if agreement != nil {
			pageRow.MonthlyRentCents, pageRow.DueDay = agreement.MonthlyRentCents, agreement.DueDay
			pageRow.MonthlyRent = pageCurrencyAmount(agreement.MonthlyRentCents, agreement.Currency)
		} else if roomRow.MonthlyRentCents > 0 {
			pageRow.MonthlyRentCents, pageRow.DueDay = roomRow.MonthlyRentCents, roomRow.DueDay
			pageRow.MonthlyRent = pageCurrencyAmount(roomRow.MonthlyRentCents, ledgerCurrencyEUR)
		}
		rows = append(rows, pageRow)
	}
	return rows, nil
}

func (a *app) handleRooms(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		a.handleRoomMutation(w, r, "")
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := a.scopedPageUser(w, r)
	if !ok {
		return
	}
	period, err := parsePeriodMonth(strings.TrimSpace(r.URL.Query().Get("period")))
	if err != nil {
		http.Error(w, "period is invalid", http.StatusBadRequest)
		return
	}
	propertyID, err := parseOptionalUint(r.URL.Query().Get("property_id"))
	if err != nil {
		http.Error(w, "property_id is invalid", http.StatusBadRequest)
		return
	}
	statusFilter := firstNonEmpty(strings.TrimSpace(r.URL.Query().Get("status")), "all")
	if statusFilter != "all" && statusFilter != "active" && statusFilter != "inactive" {
		http.Error(w, "room status filter is invalid", http.StatusBadRequest)
		return
	}
	rows, err := a.loadRoomRows(r.Context(), userID, period, propertyID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	rows = filterRoomPageRows(rows, search, statusFilter)
	collectionFilter := firstNonEmpty(strings.TrimSpace(r.URL.Query().Get("collection")), "all")
	if collectionFilter != "all" && collectionFilter != "unpaid" && collectionFilter != "paid" {
		http.Error(w, "room collection filter is invalid", http.StatusBadRequest)
		return
	}
	rows = filterRoomCollectionRows(rows, collectionFilter)
	repo := newLandlordRentRepository(a.db)
	properties, err := repo.listProperties(r.Context(), userID, propertyQuery{Status: "active"})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	allProperties, err := repo.listProperties(r.Context(), userID, propertyQuery{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	propertyRows := make([]propertyPageRow, 0, len(properties))
	for _, row := range properties {
		propertyRows = append(propertyRows, propertyPageRow{ID: row.ID, Name: row.Name, CityRegion: row.CityRegion, Address: propertyAddress(row), Timezone: row.Timezone, Notes: stringValue(row.Notes), Status: row.Status, StatusLabel: pageStatusLabel(row.Status)})
	}
	propertyOptions := make([]propertyPageRow, 0, len(allProperties))
	for _, row := range allProperties {
		propertyOptions = append(propertyOptions, propertyPageRow{ID: row.ID, Name: row.Name, CityRegion: row.CityRegion, Address: propertyAddress(row), Timezone: row.Timezone, Notes: stringValue(row.Notes), Status: row.Status, StatusLabel: pageStatusLabel(row.Status)})
	}
	data := roomPageData{workspaceShell: canonicalPageShell(a, r, "rooms", "房间与房产绑定"), Period: period.Format("2006-01"), PeriodLabel: formatMonthLabel(period), PeriodOptions: pagePeriodOptions(period), Rows: rows, Properties: propertyRows, PropertyOptions: propertyOptions, PropertyID: propertyID, StatusFilter: statusFilter, CollectionFilter: collectionFilter, Search: search, ShowForm: r.URL.Query().Get("add") == "1", Form: roomPageForm{PropertyID: propertyID, Capacity: 1, DueDay: 1, ActiveFrom: period.Format("2006-01")}, Message: r.URL.Query().Get("message"), Error: r.URL.Query().Get("error")}
	if data.ShowForm {
		returnURL := roomListURL(data.Period, propertyID, statusFilter, search, collectionFilter)
		data.Drawer = &roomCreateDrawerData{Period: data.Period, StatusFilter: statusFilter, CollectionFilter: collectionFilter, Search: search, PropertyID: propertyID, Properties: propertyRows, Form: data.Form, ReturnURL: returnURL, ErrorMessage: roomMutationErrorMessage(data.Error)}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := roomPageTemplate.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (a *app) handleRoomMutation(w http.ResponseWriter, r *http.Request, pathID string) {
	if !a.requireAuth(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := a.currentUserID(r)
	if !ok || a.db == nil {
		http.Error(w, "database session required", http.StatusServiceUnavailable)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/rooms?error=invalid_form", http.StatusFound)
		return
	}
	returnPropertyID := uint64(0)
	if r.Form.Get("return_context") == "property" {
		var parseReturnErr error
		returnPropertyID, parseReturnErr = parsePositiveUint(r.Form.Get("return_property_id"))
		if parseReturnErr != nil {
			http.Error(w, "room return property is invalid", http.StatusBadRequest)
			return
		}
		if _, err := newLandlordRentRepository(a.db).findProperty(r.Context(), userID, returnPropertyID); errors.Is(err, gorm.ErrRecordNotFound) {
			http.NotFound(w, r)
			return
		} else if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	redirectPropertyRoom := func(message, errorCode string, open bool) bool {
		if returnPropertyID == 0 {
			return false
		}
		target, valid := roomPropertyReturnURL(r.Form, message, errorCode, open)
		if !valid {
			return false
		}
		http.Redirect(w, r, target, http.StatusFound)
		return true
	}
	roomID := uint64(0)
	if pathID != "" {
		parsedRoomID, err := parsePositiveUint(pathID)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		roomID = parsedRoomID
	} else if raw := strings.TrimSpace(r.Form.Get("room_id")); raw != "" {
		parsedRoomID, err := parsePositiveUint(raw)
		if err != nil {
			redirectRoomList(w, r, "", "room_action_failed", true)
			return
		}
		roomID = parsedRoomID
	}
	action := firstNonEmpty(strings.TrimSpace(r.Form.Get("action")), "save")
	propertyID, parseErr := parsePositiveUint(r.Form.Get("property_id"))
	if parseErr != nil && action != "deactivate" {
		if roomID != 0 {
			redirectRoomEdit(w, r, roomID, "room_action_failed")
		} else if redirectPropertyRoom("", "room_action_failed", true) {
			return
		} else {
			redirectRoomList(w, r, "", "room_action_failed", true)
		}
		return
	}
	service := newLandlordDomainService(a.db)
	var err error
	switch action {
	case "deactivate":
		effective, effectiveErr := parsePeriodMonth(strings.TrimSpace(r.Form.Get("effective_month")))
		if effectiveErr != nil {
			err = effectiveErr
		} else {
			err = service.deactivateRoom(r.Context(), userID, roomID, effective)
		}
	case "save":
		capacity := 1
		if strings.TrimSpace(r.Form.Get("capacity")) != "" {
			raw := strings.TrimSpace(r.Form.Get("capacity"))
			capacity, err = strconv.Atoi(raw)
		}
		var monthlyRentCents int64
		if err == nil {
			monthlyRentCents, err = parseOptionalRoomRentCents(r.Form.Get("monthly_rent"))
		}
		var dueDay int
		if err == nil {
			dueDay, err = parseOptionalRoomDueDay(r.Form.Get("due_day"))
		}
		if err == nil && roomID == 0 {
			activeFrom, activeErr := parsePeriodMonth(strings.TrimSpace(r.Form.Get("active_from")))
			if activeErr != nil {
				err = activeErr
			} else {
				_, err = service.createRoom(r.Context(), userID, roomInput{PropertyID: propertyID, RoomLabel: r.Form.Get("room_label"), RoomType: r.Form.Get("room_type"), Capacity: capacity, MonthlyRentCents: monthlyRentCents, DueDay: dueDay, Notes: r.Form.Get("notes"), ActiveFrom: activeFrom})
			}
		} else if err == nil {
			period, periodErr := parsePeriodMonth(strings.TrimSpace(r.Form.Get("period")))
			if periodErr != nil {
				err = periodErr
			} else {
				_, err = service.updateRoom(r.Context(), userID, roomID, roomInput{PropertyID: propertyID, RoomLabel: r.Form.Get("room_label"), RoomType: r.Form.Get("room_type"), Capacity: capacity, MonthlyRentCents: monthlyRentCents, DueDay: dueDay, Notes: r.Form.Get("notes"), EffectiveMonth: period})
			}
		}
	default:
		err = errors.New("room action is invalid")
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		errorCode := "room_action_failed"
		if errors.Is(err, errArrangementHistoryLocked) {
			errorCode = "room_arrangement_locked"
		}
		if roomID != 0 {
			redirectRoomEdit(w, r, roomID, errorCode)
		} else if redirectPropertyRoom("", errorCode, true) {
			return
		} else {
			redirectRoomList(w, r, "", errorCode, true)
		}
		return
	}
	if roomID != 0 {
		query := url.Values{"message": []string{"room_saved"}}
		if period, periodErr := parsePeriodMonth(strings.TrimSpace(r.Form.Get("period"))); periodErr == nil {
			query.Set("period", period.Format("2006-01"))
		}
		copyRoomReturnContext(query, r.Form)
		http.Redirect(w, r, "/rooms/"+strconv.FormatUint(roomID, 10)+"?"+query.Encode(), http.StatusFound)
		return
	}
	if redirectPropertyRoom("room_saved", "", false) {
		return
	}
	redirectRoomList(w, r, "room_saved", "", false)
}

func roomPropertyReturnURL(values url.Values, message, errorCode string, open bool) (string, bool) {
	propertyID, err := parsePositiveUint(values.Get("return_property_id"))
	if err != nil || propertyID == 0 {
		return "", false
	}
	period := validatedPeriodValue(values.Get("return_period"))
	query := url.Values{"period": []string{period}}
	status := strings.TrimSpace(values.Get("return_list_status"))
	if status != "active" && status != "inactive" {
		status = "all"
	}
	query.Set("list_status", status)
	if search := strings.TrimSpace(values.Get("return_list_search")); search != "" {
		query.Set("list_search", search)
	}
	collection := strings.TrimSpace(values.Get("return_list_collection"))
	if collection == "unpaid" || collection == "paid" {
		query.Set("list_collection", collection)
	}
	if open {
		query.Set("room", "1")
	}
	if errorCode != "" {
		query.Set("room_error", errorCode)
	}
	if message != "" {
		query.Set("message", message)
	}
	return "/properties/" + strconv.FormatUint(propertyID, 10) + "?" + query.Encode(), true
}

func parseOptionalRoomRentCents(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	parts := strings.Split(raw, ".")
	if len(parts) > 2 {
		return 0, errors.New("monthly room rent must be a positive amount")
	}
	wholeText := parts[0]
	if wholeText == "" {
		wholeText = "0"
	}
	whole, err := strconv.ParseInt(wholeText, 10, 64)
	if err != nil || whole < 0 {
		return 0, errors.New("monthly room rent must be a positive amount")
	}
	var fraction int64
	if len(parts) == 2 {
		if parts[1] == "" || len(parts[1]) > 2 {
			return 0, errors.New("monthly room rent supports at most two decimal places")
		}
		fraction, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			return 0, errors.New("monthly room rent must be a positive amount")
		}
		if len(parts[1]) == 1 {
			fraction *= 10
		}
	}
	const maxInt64 = int64(^uint64(0) >> 1)
	if whole > (maxInt64-fraction)/100 {
		return 0, errors.New("monthly room rent exceeds the supported range")
	}
	cents := whole*100 + fraction
	if cents <= 0 {
		return 0, errors.New("monthly room rent must be at least one cent")
	}
	return cents, nil
}

func parseOptionalRoomDueDay(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	dueDay, err := strconv.Atoi(raw)
	if err != nil || dueDay < 1 || dueDay > 31 {
		return 0, errors.New("due day must be between 1 and 31")
	}
	return dueDay, nil
}

func roomMutationErrorMessage(errorCode string) string {
	switch errorCode {
	case "room_arrangement_locked":
		return "所选月份的账单已生成，月租和账单日无法修改。本次资料未保存。"
	case "room_action_failed":
		return "房间资料未能保存，请检查输入内容和租约状态。"
	default:
		return ""
	}
}

func copyRoomReturnContext(target, source url.Values) {
	if source.Get("from") == "rooms" {
		target.Set("from", "rooms")
	}
	if propertyID, err := parseOptionalUint(source.Get("return_property_id")); err == nil && propertyID > 0 {
		target.Set("return_property_id", strconv.FormatUint(propertyID, 10))
	}
	status := strings.TrimSpace(source.Get("return_status"))
	if status == "active" || status == "inactive" {
		target.Set("return_status", status)
	} else if status == "all" {
		target.Set("return_status", "all")
	}
	if search := strings.TrimSpace(source.Get("return_search")); search != "" {
		target.Set("return_search", search)
	}
	collection := strings.TrimSpace(source.Get("return_collection"))
	if collection == "unpaid" || collection == "paid" {
		target.Set("return_collection", collection)
	}
}

func (a *app) handleTenancies(w http.ResponseWriter, r *http.Request) {
	a.serveTenanciesPage(w, r)
}

func (a *app) handleMore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if _, ok := a.scopedPageUser(w, r); !ok {
		return
	}
	data := morePageData{workspaceShell: canonicalPageShell(a, r, "more", "全部功能")}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := morePageTemplate.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func bankSyncRunView(run bankSyncRun, accounts []bankSyncRunAccount) bankPageRun {
	view := bankPageRun{ID: run.ID, Mode: run.Mode, Status: run.Status, StatusLabel: bankStatusLabel(run.Status), RequestedFrom: run.RequestedFrom.Format(dateLayout), RequestedTo: run.RequestedTo.Format(dateLayout), StartedAt: run.StartedAt.UTC().Format(time.RFC3339), FinishedAt: pageDateTime(run.FinishedAt), Error: stringValue(run.ErrorMessage)}
	view.Accounts = make([]bankPageAccount, 0, len(accounts))
	for _, account := range accounts {
		view.Accounts = append(view.Accounts, bankPageAccount{AccountID: account.AccountID, AccountName: account.AccountName, Status: account.Status, StatusLabel: bankStatusLabel(account.Status), CoveredFrom: pageDate(account.CoveredFrom), CoveredTo: pageDate(account.CoveredTo), TransactionCount: account.TransactionCount, Error: stringValue(account.ErrorMessage)})
	}
	return view
}

func (a *app) loadBankPage(ctx context.Context, userID uint64) (bankPageData, error) {
	data := bankPageData{Provider: "truelayer", Environment: a.cfg.Environment}
	var connection bankConnection
	if err := a.db.WithContext(ctx).Where("user_id = ? AND provider = ? AND environment = ?", userID, "truelayer", a.cfg.Environment).First(&connection).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return bankPageData{}, err
	}
	if connection.ID != 0 {
		data.Connected = strings.TrimSpace(connection.RefreshTokenCiphertext) != ""
		data.SavedAt, data.LastSyncAt = pageDateTime(connection.SavedAt), pageDateTime(connection.LastSyncAt)
	}
	if data.Connected {
		data.SyncStatus, _ = latestBankSyncStatus(ctx, a.db, userID)
		data.SyncCoverage, _ = latestBankSyncCoverage(ctx, a.db, userID)
		data.LastSuccessfulCoverage, _ = latestSuccessfulBankSyncCoverage(ctx, a.db, userID)
	}
	data.SyncStatusLabel = pageStatusLabel(firstNonEmpty(data.SyncStatus, "not_connected"))
	var runs []bankSyncRun
	if err := a.db.WithContext(ctx).Where("user_id = ?", userID).Order("started_at DESC, id DESC").Limit(10).Find(&runs).Error; err != nil {
		return bankPageData{}, err
	}
	data.Runs = make([]bankPageRun, 0, len(runs))
	for _, run := range runs {
		var accounts []bankSyncRunAccount
		if err := a.db.WithContext(ctx).Where("user_id = ? AND bank_sync_run_id = ?", userID, run.ID).Order("account_name ASC, id ASC").Find(&accounts).Error; err != nil {
			return bankPageData{}, err
		}
		data.Runs = append(data.Runs, bankSyncRunView(run, accounts))
	}
	return data, nil
}

func (a *app) handleBank(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := a.scopedPageUser(w, r)
	if !ok {
		return
	}
	data, err := a.loadBankPage(r.Context(), userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data.workspaceShell = canonicalPageShell(a, r, "bank", "单一银行授权与同步状态")
	data.Message, data.Error = r.URL.Query().Get("message"), r.URL.Query().Get("error")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := bankPageTemplate.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

type manualBalancePreviewPageData struct {
	workspaceShell
	TenantName   string
	Period       string
	Expected     string
	Paid         string
	Remaining    string
	ObligationID uint64
	Reason       string
	Error        string
}

func (a *app) handleManualBalancePreview(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := a.currentUserID(r)
	if !ok || a.db == nil {
		http.Error(w, "database session required", http.StatusServiceUnavailable)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	obligationID, err := parsePositiveUint(r.Form.Get("obligation_id"))
	if err != nil {
		http.Error(w, "obligation_id is invalid", http.StatusBadRequest)
		return
	}
	var obligation rentObligation
	if err := a.db.WithContext(r.Context()).Where("id = ? AND user_id = ?", obligationID, userID).First(&obligation).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var tenantRow tenant
	if err := a.db.WithContext(r.Context()).Where("id = ? AND user_id = ?", obligation.TenantID, userID).First(&tenantRow).Error; err != nil {
		http.NotFound(w, r)
		return
	}
	allocations, err := newLandlordRentRepository(a.db).listPaymentAllocations(r.Context(), userID, paymentAllocationQuery{RentObligationID: obligation.ID})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var cash []cashReceipt
	if err := a.db.WithContext(r.Context()).Where("user_id = ? AND rent_obligation_id = ?", userID, obligation.ID).Find(&cash).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	projected := projectRentObligation(obligation, allocations, cash, time.Now().UTC())
	remaining := maxInt64(projected.ExpectedAmountCents-projected.PaidAmountCents, 0)
	data := manualBalancePreviewPageData{workspaceShell: canonicalPageShell(a, r, "bills", "人工平账确认"), TenantName: tenantRow.Name, Period: obligation.PeriodMonth.Format("2006-01"), Expected: pageCurrencyAmount(projected.ExpectedAmountCents, obligation.Currency), Paid: pageCurrencyAmount(projected.PaidAmountCents, obligation.Currency), Remaining: pageCurrencyAmount(remaining, obligation.Currency), ObligationID: obligation.ID, Reason: strings.TrimSpace(r.Form.Get("reason")), Error: r.URL.Query().Get("error")}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := manualBalancePreviewTemplate.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

var propertyPageTemplate = newEmbeddedWorkspacePageTemplate("properties-page", nil, "web/templates/pages/properties.html")

var propertyDetailPageTemplate = newEmbeddedWorkspacePageTemplate("property-detail-page", nil, "web/templates/pages/property-detail.html")

var roomPageTemplate = newEmbeddedWorkspacePageTemplate("rooms-page", template.FuncMap{"roomPageDrawer": roomPageDrawer}, "web/templates/pages/rooms.html")

func roomPageDrawer(data roomPageData) *roomCreateDrawerData {
	if data.Drawer != nil {
		return data.Drawer
	}
	returnURL := roomListURL(data.Period, data.PropertyID, data.StatusFilter, data.Search, data.CollectionFilter)
	return &roomCreateDrawerData{Period: data.Period, StatusFilter: data.StatusFilter, CollectionFilter: data.CollectionFilter, Search: data.Search, PropertyID: data.PropertyID, Properties: data.Properties, Form: data.Form, ReturnURL: returnURL, ErrorMessage: roomMutationErrorMessage(data.Error)}
}

var roomEditPageTemplate = newWorkspacePageTemplate("room-edit-page", nil, `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>RentOps Room Edit</title><style>`+workspacePageCSS+`</style></head><body><div class="app">{{template "workspace-nav" .}}<main class="content"><header class="topbar"><div><div class="brand-title">资产管理</div><h1>编辑房间</h1><div class="tiny">更新房产绑定与生效月份</div></div><a class="btn" href="/rooms">返回房间</a></header>{{if .Error}}<div class="notice error">{{.Error}}</div>{{end}}<section class="panel surface entity-form" aria-labelledby="room-form-title"><div class="panel-head"><div><h2 id="room-form-title">房间资料</h2><p class="tiny">房间必须绑定一个有效房产。</p></div></div><form method="post" action="/rooms/{{.Form.ID}}"><input type="hidden" name="action" value="save"><div class="form-grid"><div class="form-field"><label for="room-label">房间名称</label><input id="room-label" name="room_label" value="{{.Form.RoomLabel}}" maxlength="191" required></div><div class="form-field"><label for="room-property">所属房产</label><select id="room-property" name="property_id" required><option value="">请选择房产</option>{{range .Properties}}<option value="{{.ID}}"{{if eq $.Form.PropertyID .ID}} selected{{end}}>{{.Name}}</option>{{end}}</select></div><div class="form-field"><label for="room-active-from">生效月份</label><input id="room-active-from" name="active_from" type="month" value="{{.Form.ActiveFrom}}" required></div></div><div class="drawer-actions"><button class="btn primary" type="submit">保存房间</button></div></form></section></main></div></body></html>`)

var tenancyPageTemplate = newEmbeddedWorkspacePageTemplate("tenancies-page", nil, "web/templates/pages/tenancies.html")

var legacyCashReceiptPageTemplate = newWorkspacePageTemplate("cash-receipts-legacy", nil, `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>RentOps Cash Receipts</title><style>`+workspacePageCSS+`</style></head><body><div class="app">{{template "workspace-nav" .}}<main class="content"><header class="topbar"><div><div class="brand-title">现金补录</div><h1>现金收款记录</h1><div class="tiny">现金不会伪装成银行流水；作废必须保留原因</div></div><a class="btn primary" href="/cash-receipts/new">新建现金收款</a></header>{{if eq .Message "cash_receipt_saved"}}<div class="notice ok" data-toast>现金收款已登记。</div>{{end}}{{if eq .Message "cash_receipt_voided"}}<div class="notice ok" data-toast>现金收款已撤销，原始收据仍保留。</div>{{end}}{{if .Error}}<div class="notice error">{{.Error}}</div>{{end}}<section class="panel surface"><div class="panel-head"><h2>收款列表</h2><form method="get" action="/cash-receipts"><select name="status"><option value=""{{if eq .StatusFilter ""}} selected{{end}}>全部状态</option><option value="confirmed"{{if eq .StatusFilter "confirmed"}} selected{{end}}>已确认</option><option value="voided"{{if eq .StatusFilter "voided"}} selected{{end}}>已作废</option></select></form></div>{{if .Rows}}<div class="table-wrap"><table><thead><tr><th>收据</th><th>租客／月份</th><th>金额</th><th>收款日期</th><th>状态</th><th>作废原因</th></tr></thead><tbody>{{range .Rows}}<tr data-page="cash-receipts" data-receipt-id="{{.ID}}"><td class="mono">{{.ReceiptNumber}}</td><td><a href="/tenants/{{.TenantID}}">{{.TenantName}}</a><br><span class="tiny">{{.Period}}</span></td><td class="amount">{{.Amount}}</td><td>{{.ReceivedAt}}</td><td><span class="status {{.Status}}">{{.StatusLabel}}</span>{{if eq .Status "confirmed"}}<br><a class="btn subtle" href="{{.VoidURL}}">撤销</a>{{end}}</td><td>{{if .VoidReason}}{{.VoidReason}}{{else}}—{{end}}</td></tr>{{end}}</tbody></table></div>{{else}}<div class="empty">暂无现金收款记录。</div>{{end}}</section></main></div></body></html>`)

var bankPageCSS = `
  .account-actions { display: flex; flex-wrap: wrap; align-items: center; gap: 10px; padding: 16px 20px; }
  .account-actions form { margin: 0; }
  .account-actions .tiny { flex: 1 1 200px; margin: 0; }
  @media (max-width: 640px) {
    .account-actions { display: grid; grid-template-columns: minmax(0,1fr); }
    .account-actions form { display: grid; }
    .account-actions .btn { width: 100%; min-height: 44px; }
  }
`

var bankPageTemplate = newWorkspacePageTemplate("bank-page", nil, `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>RentOps Bank</title><style>`+workspacePageCSS+bankPageCSS+`</style></head><body><div class="app">{{template "workspace-nav" .}}<main class="content"><header class="topbar"><div><div class="brand-title">银行设置</div><h1>银行连接与同步</h1><div class="tiny">每个账号只保留一个授权，授权下可同步多个实际账户</div></div><div class="actions"><a class="btn primary" href="/bank/connect">添加银行账户</a></div></header>{{if eq .Message "refreshed"}}<div class="notice ok" data-toast>银行数据已刷新。</div>{{end}}{{if .Error}}<div class="notice error">{{.Error}}</div>{{end}}<section class="summary"><div class="panel metric"><div class="label">授权</div><strong>{{if .Connected}}已连接{{else}}未连接{{end}}</strong><span>{{.Provider}} · {{.Environment}}</span></div><div class="panel metric"><div class="label">同步状态</div><strong>{{.SyncStatusLabel}}</strong><span>{{.LastSyncAt}}</span></div><div class="panel metric"><div class="label">最近覆盖</div><strong>{{if .SyncCoverage}}已记录{{else}}暂无{{end}}</strong><span>{{.SyncCoverage}}</span></div></section><section class="panel surface bank-account-actions"><div class="panel-head"><div><h2>账户操作</h2><p class="tiny">系统只读取账户与流水，不发起付款。</p></div><span class="tiny">{{if .Connected}}已连接{{else}}未连接{{end}}</span></div><div class="account-actions">{{if .Connected}}<form method="post" action="/bank/sync"><button class="btn primary" type="submit">立即同步</button></form><a class="btn" href="/bank/connect">重新授权</a>{{else}}<a class="btn primary" href="/bank/connect">连接银行</a><p class="tiny">还没有银行授权，添加银行账户后可在此同步。</p>{{end}}</div></section><section class="panel surface"><div class="panel-head"><h2>同步运行</h2><span class="tiny">refresh token 流程</span></div>{{if .Runs}}{{range .Runs}}<article class="panel" data-page="bank" data-sync-run-id="{{.ID}}"><div class="panel-head"><div><strong>{{.StartedAt}}</strong><div class="tiny">{{.RequestedFrom}} 至 {{.RequestedTo}} · {{.Mode}}</div></div><span class="status {{.Status}}">{{.StatusLabel}}</span></div>{{if .Error}}<div class="notice error">{{.Error}}</div>{{end}}{{if .Accounts}}<div class="table-wrap"><table><thead><tr><th>实际账户</th><th>覆盖范围</th><th>流水数</th><th>状态</th></tr></thead><tbody>{{range .Accounts}}<tr><td class="mono">{{.AccountName}}</td><td>{{.CoveredFrom}} 至 {{.CoveredTo}}</td><td>{{.TransactionCount}}</td><td><span class="status {{.Status}}">{{.StatusLabel}}</span>{{if .Error}}<br><span class="tiny">{{.Error}}</span>{{end}}</td></tr>{{end}}</tbody></table></div>{{end}}</article>{{end}}{{else}}<div class="empty">暂无同步记录。连接银行后点击同步。</div>{{end}}</section></main></div></body></html>`)

var manualBalancePreviewTemplate = newWorkspacePageTemplate("manual-balance-preview-page", nil, `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>人工平账确认</title><style>`+workspacePageCSS+`</style></head><body><div class="app">{{template "workspace-nav" .}}<main class="content"><header class="topbar"><div><div class="brand-title">账单调整</div><h1>人工平账确认</h1><div class="tiny">确认前请核对本次调整影响</div></div><a class="btn" href="/bills">返回账单</a></header><section class="panel surface"><dl class="facts"><dt>租客</dt><dd>{{.TenantName}}</dd><dt>租金月份</dt><dd>{{.Period}}</dd><dt>应收</dt><dd class="amount">{{.Expected}}</dd><dt>当前已收</dt><dd class="amount">{{.Paid}}</dd><dt>本次人工调整</dt><dd class="amount">{{.Remaining}}</dd></dl><form method="post" action="/bills/settle"><input type="hidden" name="obligation_id" value="{{.ObligationID}}"><label for="manual-balance-reason">调整原因（必填）</label><textarea id="manual-balance-reason" name="reason" maxlength="512" required>{{.Reason}}</textarea><button class="btn primary" type="submit">确认人工平账</button></form></section></main></div></body></html>`)
