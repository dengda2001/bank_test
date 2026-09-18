package main

import (
	"context"
	"errors"
	"html/template"
	"net/http"
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
	ID                uint64
	Name              string
	Address           string
	Status            string
	StatusLabel       string
	RoomCount         int
	ActiveRoomCount   int
	ExpectedCents     int64
	PaidCents         int64
	BalanceCents      int64
	ExpenseCents      int64
	ExpectedAmount    string
	PaidAmount        string
	BalanceAmount     string
	ExpenseAmount     string
	CollectionPercent int
	Actions           []pageActionView
}

type propertyPageData struct {
	workspaceShell
	Period       string
	PeriodLabel  string
	Rows         []propertyPageRow
	Message      string
	Error        string
	StatusFilter string
}

type propertyDetailPageData struct {
	workspaceShell
	Property propertyPageRow
	Rooms    []roomPageRow
	Expenses []expenseRecord
	Editing  bool
	Error    string
	Message  string
}

type roomPageRow struct {
	ID               uint64
	PropertyID       uint64
	PropertyName     string
	RoomLabel        string
	Status           string
	StatusLabel      string
	ActiveFrom       string
	InactiveFrom     string
	TenantNames      []string
	MonthlyRentCents int64
	MonthlyRent      string
	Currency         string
	DueDay           int
	ExpectedCents    int64
	PaidCents        int64
	BalanceCents     int64
	ExpectedAmount   string
	PaidAmount       string
	BalanceAmount    string
	Actions          []pageActionView
}

type roomPageData struct {
	workspaceShell
	Period      string
	PeriodLabel string
	Rows        []roomPageRow
	Properties  []propertyPageRow
	Message     string
	Error       string
	PropertyID  uint64
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
	RoomID           uint64
	RoomLabel        string
	PropertyID       uint64
	PropertyName     string
	StartDate        string
	EndDate          string
	MonthlyRent      string
	MonthlyRentCents int64
	Currency         string
	DueDay           int
	Status           string
	StatusLabel      string
	Parties          []tenancyPartyView
}

type tenancyPageData struct {
	workspaceShell
	Rows    []tenancyPageRow
	Message string
	Error   string
}

type cashReceiptPageRow struct {
	ID            uint64
	ReceiptNumber string
	TenantID      uint64
	TenantName    string
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
	StatusFilter string
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
	return workspaceShell{ActivePage: active, Username: a.displayUsername(r), Environment: a.cfg.Environment, FootNote: note, ShowNavCounts: true, NavLabel: "主导航"}
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
	a.handleRentDashboard(w, r)
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
	a.renderRentDashboard(w, r, &dunningDashboardAction{Kind: dunningActionConfig, Period: period, Filters: filters})
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
	if activeRows, loadErr := newRentWorkspaceService(a.db).load(ctx, userID, func() rentWorkspaceFilters {
		filters := defaultRentWorkspaceFilters(period)
		filters.PageSize = rentWorkspaceMaxPageSize
		return filters
	}()); loadErr == nil {
		for _, row := range activeRows.PropertyRows {
			financial[row.PropertyID] = row
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
		rows = append(rows, propertyPageRow{
			ID: row.ID, Name: row.Name, Address: propertyAddress(row), Status: row.Status,
			StatusLabel: pageStatusLabel(row.Status), RoomCount: roomCount[row.ID], ActiveRoomCount: activeRoomCount[row.ID],
			ExpectedCents: financialRow.ExpectedCents, PaidCents: financialRow.PaidCents, BalanceCents: financialRow.BalanceCents, ExpenseCents: financialRow.ExpenseCents,
			ExpectedAmount: pageCurrencyAmount(financialRow.ExpectedCents, currency), PaidAmount: pageCurrencyAmount(financialRow.PaidCents, currency), BalanceAmount: pageCurrencyAmount(financialRow.BalanceCents, currency), ExpenseAmount: pageCurrencyAmount(financialRow.ExpenseCents, currency), CollectionPercent: financialRow.CollectionPercent,
			Actions: propertyActions(row.ID, row.Status == "active"),
		})
	}
	return propertyPageData{Period: monthStart(period).Format("2006-01"), PeriodLabel: formatMonthLabel(period), Rows: rows, StatusFilter: statusFilter}, nil
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
	data := propertyDetailPageData{workspaceShell: canonicalPageShell(a, r, "properties", "房产详情"), Property: propertyPageRow{ID: propertyRow.ID, Name: propertyRow.Name, Address: propertyAddress(propertyRow), Status: propertyRow.Status, StatusLabel: pageStatusLabel(propertyRow.Status), Actions: propertyActions(propertyRow.ID, propertyRow.Status == "active")}, Rooms: rooms, Expenses: filteredExpenses, Editing: r.URL.Query().Get("edit") == "1", Message: r.URL.Query().Get("message"), Error: r.URL.Query().Get("error")}
	if summary, summaryErr := a.loadPropertyPage(r.Context(), userID, period, "all"); summaryErr == nil {
		for _, row := range summary.Rows {
			if row.ID == propertyID {
				data.Property = row
				break
			}
		}
	}
	data.Property.RoomCount, data.Property.ActiveRoomCount = len(rooms), 0
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
			http.Redirect(w, r, "/properties?error=property_action_failed", http.StatusFound)
			return
		}
		http.Redirect(w, r, "/properties/"+strconv.FormatUint(propertyID, 10)+"?edit=1", http.StatusFound)
		return
	case "deactivate":
		effective, parseErr := parsePeriodMonth(strings.TrimSpace(r.Form.Get("effective_month")))
		if parseErr != nil {
			err = parseErr
		} else {
			err = service.deactivateProperty(r.Context(), userID, propertyID, effective)
		}
	case "save":
		input := propertyInput{Name: r.Form.Get("name"), Address: r.Form.Get("address")}
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
		http.Redirect(w, r, "/properties?error=property_action_failed", http.StatusFound)
		return
	}
	http.Redirect(w, r, "/properties?message=property_saved", http.StatusFound)
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
		pageRow := roomPageRow{ID: roomRow.ID, PropertyID: roomRow.PropertyID, PropertyName: propertyRow.Name, RoomLabel: roomRow.RoomLabel, Status: roomRow.Status, StatusLabel: pageStatusLabel(roomRow.Status), ActiveFrom: roomRow.ActiveFrom.Format(dateLayout), InactiveFrom: pageDate(roomRow.InactiveFrom), TenantNames: partyNames, Currency: currency, ExpectedCents: financialRow.ExpectedCents, PaidCents: financialRow.PaidCents, BalanceCents: financialRow.BalanceCents, ExpectedAmount: pageCurrencyAmount(financialRow.ExpectedCents, currency), PaidAmount: pageCurrencyAmount(financialRow.PaidCents, currency), BalanceAmount: pageCurrencyAmount(financialRow.BalanceCents, currency), Actions: roomActions(roomRow.ID, roomRow.Status == "active")}
		if agreement != nil {
			pageRow.MonthlyRentCents, pageRow.DueDay = agreement.MonthlyRentCents, agreement.DueDay
			pageRow.MonthlyRent = pageCurrencyAmount(agreement.MonthlyRentCents, agreement.Currency)
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
	rows, err := a.loadRoomRows(r.Context(), userID, period, propertyID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	properties, err := newLandlordRentRepository(a.db).listProperties(r.Context(), userID, propertyQuery{Status: "active"})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	propertyRows := make([]propertyPageRow, 0, len(properties))
	for _, row := range properties {
		propertyRows = append(propertyRows, propertyPageRow{ID: row.ID, Name: row.Name, Address: propertyAddress(row), Status: row.Status, StatusLabel: pageStatusLabel(row.Status)})
	}
	data := roomPageData{workspaceShell: canonicalPageShell(a, r, "rooms", "房间与房产绑定"), Period: period.Format("2006-01"), PeriodLabel: formatMonthLabel(period), Rows: rows, Properties: propertyRows, PropertyID: propertyID, Message: r.URL.Query().Get("message"), Error: r.URL.Query().Get("error")}
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
	roomID := uint64(0)
	if pathID != "" {
		roomID, _ = strconv.ParseUint(pathID, 10, 64)
	} else if raw := strings.TrimSpace(r.Form.Get("room_id")); raw != "" {
		roomID, _ = strconv.ParseUint(raw, 10, 64)
	}
	propertyID, parseErr := parsePositiveUint(r.Form.Get("property_id"))
	if parseErr != nil && strings.TrimSpace(r.Form.Get("action")) != "deactivate" {
		http.Redirect(w, r, "/rooms?error=room_action_failed", http.StatusFound)
		return
	}
	action := firstNonEmpty(strings.TrimSpace(r.Form.Get("action")), "save")
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
		activeFrom, activeErr := parsePeriodMonth(strings.TrimSpace(r.Form.Get("active_from")))
		if activeErr != nil {
			err = activeErr
		} else if roomID == 0 {
			_, err = service.createRoom(r.Context(), userID, roomInput{PropertyID: propertyID, RoomLabel: r.Form.Get("room_label"), ActiveFrom: activeFrom})
		} else {
			_, err = service.updateRoom(r.Context(), userID, roomID, roomInput{PropertyID: propertyID, RoomLabel: r.Form.Get("room_label"), ActiveFrom: activeFrom})
		}
	default:
		err = errors.New("room action is invalid")
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Redirect(w, r, "/rooms?error=room_action_failed", http.StatusFound)
		return
	}
	http.Redirect(w, r, "/rooms?message=room_saved", http.StatusFound)
}

func (a *app) handleTenancies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := a.scopedPageUser(w, r)
	if !ok {
		return
	}
	repo := newLandlordRentRepository(a.db)
	agreements, err := repo.listTenancyAgreements(r.Context(), userID, agreementQuery{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	rooms, err := repo.listRooms(r.Context(), userID, roomQuery{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	properties, err := repo.listProperties(r.Context(), userID, propertyQuery{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	parties, err := repo.listAgreementParties(r.Context(), userID, agreementPartyQuery{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var tenants []tenant
	if err := a.db.WithContext(r.Context()).Where("user_id = ?", userID).Find(&tenants).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	roomByID := make(map[uint64]room, len(rooms))
	for _, row := range rooms {
		roomByID[row.ID] = row
	}
	propertyByID := make(map[uint64]property, len(properties))
	for _, row := range properties {
		propertyByID[row.ID] = row
	}
	tenantByID := make(map[uint64]tenant, len(tenants))
	for _, row := range tenants {
		tenantByID[row.ID] = row
	}
	partiesByAgreement := make(map[uint64][]tenancyPartyView)
	for _, party := range parties {
		tenantRow := tenantByID[party.TenantID]
		partiesByAgreement[party.AgreementID] = append(partiesByAgreement[party.AgreementID], tenancyPartyView{TenantID: party.TenantID, TenantName: firstNonEmpty(tenantRow.DisplayAlias, tenantRow.Name, "未绑定租客"), ResponsibilityCents: party.ResponsibilityCents, Responsibility: pageCurrencyAmount(party.ResponsibilityCents, "EUR"), JoinedAt: pageDate(party.JoinedAt), LeftAt: pageDate(party.LeftAt)})
	}
	rows := make([]tenancyPageRow, 0, len(agreements))
	for _, agreement := range agreements {
		roomRow := roomByID[agreement.RoomID]
		propertyRow := propertyByID[roomRow.PropertyID]
		rows = append(rows, tenancyPageRow{ID: agreement.ID, RoomID: agreement.RoomID, RoomLabel: roomRow.RoomLabel, PropertyID: propertyRow.ID, PropertyName: propertyRow.Name, StartDate: agreement.StartDate.Format(dateLayout), EndDate: pageDate(agreement.EndDate), MonthlyRentCents: agreement.MonthlyRentCents, MonthlyRent: pageCurrencyAmount(agreement.MonthlyRentCents, agreement.Currency), Currency: agreement.Currency, DueDay: agreement.DueDay, Status: agreement.Status, StatusLabel: pageStatusLabel(agreement.Status), Parties: partiesByAgreement[agreement.ID]})
	}
	data := tenancyPageData{workspaceShell: canonicalPageShell(a, r, "tenancies", "自动生成的租住安排"), Rows: rows, Message: r.URL.Query().Get("message"), Error: r.URL.Query().Get("error")}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tenancyPageTemplate.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (a *app) handleCashReceiptList(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.scopedPageUser(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	statusFilter := strings.TrimSpace(r.URL.Query().Get("status"))
	if statusFilter != "" && statusFilter != cashReceiptStatusConfirmed && statusFilter != cashReceiptStatusVoided {
		http.Error(w, "cash receipt status is invalid", http.StatusBadRequest)
		return
	}
	query := a.db.WithContext(r.Context()).Where("user_id = ?", userID)
	if statusFilter != "" {
		query = query.Where("status = ?", statusFilter)
	}
	var receipts []cashReceipt
	if err := query.Order("received_at DESC, id DESC").Find(&receipts).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var tenants []tenant
	if err := a.db.WithContext(r.Context()).Where("user_id = ?", userID).Find(&tenants).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tenantByID := make(map[uint64]tenant, len(tenants))
	for _, row := range tenants {
		tenantByID[row.ID] = row
	}
	rows := make([]cashReceiptPageRow, 0, len(receipts))
	for _, receipt := range receipts {
		row := cashReceiptPageRow{ID: receipt.ID, ReceiptNumber: receipt.ReceiptNumber, TenantID: receipt.TenantID, TenantName: firstNonEmpty(tenantByID[receipt.TenantID].Name, "未绑定租客"), ObligationID: receipt.RentObligationID, AmountCents: receipt.AmountCents, Amount: pageCurrencyAmount(receipt.AmountCents, receipt.Currency), Currency: receipt.Currency, ReceivedAt: receipt.ReceivedAt.Format(dateLayout), Status: receipt.Status, StatusLabel: pageStatusLabel(receipt.Status), Note: receipt.Note, VoidReason: stringValue(receipt.VoidReason), VoidURL: "/cash-receipts/void?receipt_id=" + strconv.FormatUint(receipt.ID, 10)}
		var obligation rentObligation
		if err := a.db.WithContext(r.Context()).Where("id = ? AND user_id = ?", receipt.RentObligationID, userID).First(&obligation).Error; err == nil {
			row.Period = obligation.PeriodMonth.Format("2006-01")
		}
		rows = append(rows, row)
	}
	data := cashReceiptPageData{workspaceShell: canonicalPageShell(a, r, "cash-receipts", "现金收款记录"), Rows: rows, StatusFilter: statusFilter, Message: r.URL.Query().Get("message"), Error: r.URL.Query().Get("error")}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := cashReceiptPageTemplate.Execute(w, data); err != nil {
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

var propertyPageTemplate = newWorkspacePageTemplate("properties-page", template.FuncMap{"actionMethod": func(action pageActionView) string { return action.Method }}, `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>RentOps Properties</title><style>`+workspacePageCSS+`</style></head><body><div class="app">{{template "workspace-nav" .}}<main class="content"><header class="topbar"><div><div class="brand-title">资产管理</div><h1>房产</h1><div class="tiny">{{.PeriodLabel}} · {{len .Rows}} 套</div></div><a class="btn primary" href="/properties?add=1">新建房产</a></header>{{if .Message}}<div class="notice ok">{{.Message}}</div>{{end}}{{if .Error}}<div class="notice error">{{.Error}}</div>{{end}}<section class="panel surface"><div class="panel-head"><h2>房产列表</h2><form method="get" action="/properties"><label class="tiny">状态<select name="status"><option value="active"{{if eq .StatusFilter "active"}} selected{{end}}>有效</option><option value="inactive"{{if eq .StatusFilter "inactive"}} selected{{end}}>已停用</option><option value="all"{{if eq .StatusFilter "all"}} selected{{end}}>全部</option></select></label></form></div>{{if .Rows}}<div class="table-wrap"><table><thead><tr><th>房产</th><th>房间</th><th>本月应收</th><th>已收</th><th>支出</th><th>状态</th><th>操作</th></tr></thead><tbody>{{range .Rows}}<tr data-page="properties" data-property-id="{{.ID}}"><td><a href="/properties/{{.ID}}"><strong>{{.Name}}</strong></a><br><span class="tiny">{{.Address}}</span></td><td>{{.ActiveRoomCount}} / {{.RoomCount}}</td><td class="amount">{{.ExpectedAmount}}</td><td class="amount">{{.PaidAmount}}</td><td class="amount">{{.ExpenseAmount}}</td><td><span class="status {{.Status}}">{{.StatusLabel}}</span></td><td>{{range .Actions}}<form method="post" action="{{.URL}}" style="display:inline"><input type="hidden" name="action" value="{{.Key}}"><button class="btn subtle" type="submit"{{if .RequiresConfirmation}} data-confirm="true"{{end}}>{{.Label}}</button></form>{{end}}</td></tr>{{end}}</tbody></table></div>{{else}}<div class="empty">还没有房产，先创建一套房产。</div>{{end}}</section></main></div></body></html>`)

var propertyDetailPageTemplate = newWorkspacePageTemplate("property-detail-page", nil, `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>RentOps Property</title><style>`+workspacePageCSS+`</style></head><body><div class="app">{{template "workspace-nav" .}}<main class="content"><header class="topbar"><div><div class="brand-title">房产详情</div><h1>{{.Property.Name}}</h1><div class="tiny">{{.Property.Address}} · {{.Property.StatusLabel}}</div></div><a class="btn" href="/properties">返回房产</a></header>{{if .Message}}<div class="notice ok">{{.Message}}</div>{{end}}{{if .Error}}<div class="notice error">{{.Error}}</div>{{end}}{{if .Editing}}<section class="panel surface"><div class="panel-head"><h2>编辑房产</h2></div><form method="post" action="/properties/{{.Property.ID}}"><input type="hidden" name="action" value="save"><label for="property-name">名称</label><input id="property-name" name="name" value="{{.Property.Name}}" required maxlength="191"><label for="property-address">地址</label><input id="property-address" name="address" value="{{.Property.Address}}" maxlength="512"><button class="btn primary" type="submit">保存房产</button></form></section>{{end}}<section class="summary"><div class="panel metric"><div class="label">房间</div><strong>{{.Property.ActiveRoomCount}} / {{.Property.RoomCount}}</strong><span>有效／全部</span></div><div class="panel metric"><div class="label">本月应收</div><strong>{{.Property.ExpectedAmount}}</strong></div><div class="panel metric"><div class="label">已收</div><strong>{{.Property.PaidAmount}}</strong></div><div class="panel metric"><div class="label">支出</div><strong>{{.Property.ExpenseAmount}}</strong></div></section><section class="panel surface"><div class="panel-head"><h2>房间</h2><a class="btn primary" href="/rooms?property_id={{.Property.ID}}&amp;add=1">新建房间</a></div>{{if .Rooms}}<div class="table-wrap"><table><thead><tr><th>房间</th><th>入住人</th><th>月租</th><th>状态</th><th>操作</th></tr></thead><tbody>{{range .Rooms}}<tr><td><a href="/rooms/{{.ID}}"><strong>{{.RoomLabel}}</strong></a><br><span class="tiny">{{.PropertyName}}</span></td><td>{{range $index, $name := .TenantNames}}{{if $index}}, {{end}}{{$name}}{{else}}空置{{end}}</td><td class="amount">{{.MonthlyRent}}</td><td><span class="status {{.Status}}">{{.StatusLabel}}</span></td><td><a class="btn subtle" href="/rooms/{{.ID}}">详情</a></td></tr>{{end}}</tbody></table></div>{{else}}<div class="empty">该房产暂无房间。</div>{{end}}</section><section class="panel surface"><div class="panel-head"><h2>关联支出</h2><span class="tiny">旧未关联记录不会被伪造迁移</span></div>{{if .Expenses}}<div class="table-wrap"><table><thead><tr><th>日期</th><th>描述</th><th>类别</th><th>金额</th><th>发票</th></tr></thead><tbody>{{range .Expenses}}<tr><td>{{.DateDisplay}}</td><td>{{.Description}}</td><td>{{.Category}}</td><td class="amount">{{.AmountDisplay}}</td><td>{{if .InvoiceURL}}<a href="{{.InvoiceURL}}" target="_blank" rel="noopener noreferrer">查看链接</a>{{else}}未提供{{end}}</td></tr>{{end}}</tbody></table></div>{{else}}<div class="empty">该房产暂无关联支出。</div>{{end}}</section></main></div></body></html>`)

var roomPageTemplate = newWorkspacePageTemplate("rooms-page", nil, `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>RentOps Rooms</title><style>`+workspacePageCSS+`</style></head><body><div class="app">{{template "workspace-nav" .}}<main class="content"><header class="topbar"><div><div class="brand-title">资产管理</div><h1>房间</h1><div class="tiny">{{.PeriodLabel}} · 房产绑定与入住安排</div></div><a class="btn primary" href="/rooms?add=1">新建房间</a></header>{{if .Message}}<div class="notice ok">{{.Message}}</div>{{end}}{{if .Error}}<div class="notice error">{{.Error}}</div>{{end}}<section class="panel surface"><div class="panel-head"><h2>房间列表</h2></div>{{if .Rows}}<div class="table-wrap"><table><thead><tr><th>房间</th><th>房产</th><th>入住人</th><th>固定月租</th><th>本月余额</th><th>状态</th><th>操作</th></tr></thead><tbody>{{range .Rows}}<tr data-page="rooms" data-room-id="{{.ID}}"><td><a href="/rooms/{{.ID}}?period={{$.Period}}"><strong>{{.RoomLabel}}</strong></a><br><span class="tiny">{{.ActiveFrom}}</span></td><td><a href="/properties/{{.PropertyID}}">{{.PropertyName}}</a></td><td>{{range $index, $name := .TenantNames}}{{if $index}}, {{end}}{{$name}}{{else}}空置{{end}}</td><td class="amount">{{.MonthlyRent}}</td><td class="amount">{{.BalanceAmount}}</td><td><span class="status {{.Status}}">{{.StatusLabel}}</span></td><td><a class="btn subtle" href="/rooms/{{.ID}}">详情</a></td></tr>{{end}}</tbody></table></div>{{else}}<div class="empty">还没有房间，请先创建房产。</div>{{end}}</section></main></div></body></html>`)

var tenancyPageTemplate = newWorkspacePageTemplate("tenancies-page", nil, `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>RentOps Tenancies</title><style>`+workspacePageCSS+`</style></head><body><div class="app">{{template "workspace-nav" .}}<main class="content"><header class="topbar"><div><div class="brand-title">租住安排</div><h1>自动生成的租住安排</h1><div class="tiny">只读历史视图；新安排从租客入住配置生成</div></div><a class="btn" href="/tenants">管理租客</a></header>{{if .Message}}<div class="notice ok">{{.Message}}</div>{{end}}{{if .Error}}<div class="notice error">{{.Error}}</div>{{end}}<section class="panel surface">{{if .Rows}}<div class="table-wrap"><table><thead><tr><th>房产／房间</th><th>租客责任</th><th>总月租</th><th>租期</th><th>账单日</th><th>状态</th></tr></thead><tbody>{{range .Rows}}<tr data-page="tenancies" data-tenancy-id="{{.ID}}"><td><a href="/rooms/{{.RoomID}}"><strong>{{.PropertyName}}</strong><br>{{.RoomLabel}}</a></td><td>{{range .Parties}}<div>{{.TenantName}} · {{.Responsibility}}</div>{{else}}未绑定租客{{end}}</td><td class="amount">{{.MonthlyRent}}</td><td class="mono">{{.StartDate}}{{if .EndDate}} 至 {{.EndDate}}{{end}}</td><td>每月 {{.DueDay}} 日</td><td><span class="status {{.Status}}">{{.StatusLabel}}</span></td></tr>{{end}}</tbody></table></div>{{else}}<div class="empty">暂无租住安排。新租客可先不绑定房间。</div>{{end}}</section></main></div></body></html>`)

var cashReceiptPageTemplate = newWorkspacePageTemplate("cash-receipts-page", nil, `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>RentOps Cash Receipts</title><style>`+workspacePageCSS+`</style></head><body><div class="app">{{template "workspace-nav" .}}<main class="content"><header class="topbar"><div><div class="brand-title">现金补录</div><h1>现金收款记录</h1><div class="tiny">现金不会伪装成银行流水；作废必须保留原因</div></div><a class="btn primary" href="/cash-receipts/new">新建现金收款</a></header>{{if .Message}}<div class="notice ok">{{.Message}}</div>{{end}}{{if .Error}}<div class="notice error">{{.Error}}</div>{{end}}<section class="panel surface"><div class="panel-head"><h2>收款列表</h2><form method="get" action="/cash-receipts"><select name="status"><option value=""{{if eq .StatusFilter ""}} selected{{end}}>全部状态</option><option value="confirmed"{{if eq .StatusFilter "confirmed"}} selected{{end}}>已确认</option><option value="voided"{{if eq .StatusFilter "voided"}} selected{{end}}>已作废</option></select></form></div>{{if .Rows}}<div class="table-wrap"><table><thead><tr><th>收据</th><th>租客／月份</th><th>金额</th><th>收款日期</th><th>状态</th><th>作废原因</th></tr></thead><tbody>{{range .Rows}}<tr data-page="cash-receipts" data-receipt-id="{{.ID}}"><td class="mono">{{.ReceiptNumber}}</td><td><a href="/tenants/{{.TenantID}}">{{.TenantName}}</a><br><span class="tiny">{{.Period}}</span></td><td class="amount">{{.Amount}}</td><td>{{.ReceivedAt}}</td><td><span class="status {{.Status}}">{{.StatusLabel}}</span>{{if eq .Status "confirmed"}}<br><a class="btn subtle" href="{{.VoidURL}}">撤销</a>{{end}}</td><td>{{if .VoidReason}}{{.VoidReason}}{{else}}—{{end}}</td></tr>{{end}}</tbody></table></div>{{else}}<div class="empty">暂无现金收款记录。</div>{{end}}</section></main></div></body></html>`)

var bankPageTemplate = newWorkspacePageTemplate("bank-page", nil, `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>RentOps Bank</title><style>`+workspacePageCSS+`</style></head><body><div class="app">{{template "workspace-nav" .}}<main class="content"><header class="topbar"><div><div class="brand-title">银行设置</div><h1>银行连接与同步</h1><div class="tiny">每个账号只保留一个授权，授权下可同步多个实际账户</div></div><div class="actions"><a class="btn" href="/bank/connect">{{if .Connected}}重新授权{{else}}连接银行{{end}}</a>{{if .Connected}}<form method="post" action="/bank/sync"><button class="btn primary" type="submit">同步银行流水</button></form>{{end}}</div></header>{{if .Message}}<div class="notice ok">{{.Message}}</div>{{end}}{{if .Error}}<div class="notice error">{{.Error}}</div>{{end}}<section class="summary"><div class="panel metric"><div class="label">授权</div><strong>{{if .Connected}}已连接{{else}}未连接{{end}}</strong><span>{{.Provider}} · {{.Environment}}</span></div><div class="panel metric"><div class="label">同步状态</div><strong>{{.SyncStatusLabel}}</strong><span>{{.LastSyncAt}}</span></div><div class="panel metric"><div class="label">最近覆盖</div><strong>{{if .SyncCoverage}}已记录{{else}}暂无{{end}}</strong><span>{{.SyncCoverage}}</span></div></section><section class="panel surface"><div class="panel-head"><h2>同步运行</h2><span class="tiny">refresh token 流程</span></div>{{if .Runs}}{{range .Runs}}<article class="panel" data-page="bank" data-sync-run-id="{{.ID}}"><div class="panel-head"><div><strong>{{.StartedAt}}</strong><div class="tiny">{{.RequestedFrom}} 至 {{.RequestedTo}} · {{.Mode}}</div></div><span class="status {{.Status}}">{{.StatusLabel}}</span></div>{{if .Error}}<div class="notice error">{{.Error}}</div>{{end}}{{if .Accounts}}<div class="table-wrap"><table><thead><tr><th>实际账户</th><th>覆盖范围</th><th>流水数</th><th>状态</th></tr></thead><tbody>{{range .Accounts}}<tr><td class="mono">{{.AccountName}}</td><td>{{.CoveredFrom}} 至 {{.CoveredTo}}</td><td>{{.TransactionCount}}</td><td><span class="status {{.Status}}">{{.StatusLabel}}</span>{{if .Error}}<br><span class="tiny">{{.Error}}</span>{{end}}</td></tr>{{end}}</tbody></table></div>{{end}}</article>{{end}}{{else}}<div class="empty">暂无同步记录。连接银行后点击同步。</div>{{end}}</section></main></div></body></html>`)

var manualBalancePreviewTemplate = newWorkspacePageTemplate("manual-balance-preview-page", nil, `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>人工平账确认</title><style>`+workspacePageCSS+`</style></head><body><div class="app">{{template "workspace-nav" .}}<main class="content"><header class="topbar"><div><div class="brand-title">账单调整</div><h1>人工平账确认</h1><div class="tiny">确认前请核对本次调整影响</div></div><a class="btn" href="/bills">返回账单</a></header><section class="panel surface"><dl class="facts"><dt>租客</dt><dd>{{.TenantName}}</dd><dt>租金月份</dt><dd>{{.Period}}</dd><dt>应收</dt><dd class="amount">{{.Expected}}</dd><dt>当前已收</dt><dd class="amount">{{.Paid}}</dd><dt>本次人工调整</dt><dd class="amount">{{.Remaining}}</dd></dl><form method="post" action="/bills/settle"><input type="hidden" name="obligation_id" value="{{.ObligationID}}"><label for="manual-balance-reason">调整原因（必填）</label><textarea id="manual-balance-reason" name="reason" maxlength="512" required>{{.Reason}}</textarea><button class="btn primary" type="submit">确认人工平账</button></form></section></main></div></body></html>`)
