package main

import (
	"context"
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"sort"
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
	SearchTerms           []string
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
	Sort             string
	SortLinks        map[string]tableSortLink
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
	IsDeleted      bool
	Period         string
	PeriodLabel    string
	ListStatus     string
	ListSearch     string
	ListCollection string
	ListSort       string
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
	TenantNames           []string
	SearchTerms           []string
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
	Sort             string
	SortLinks        map[string]tableSortLink
	Drawer           *roomCreateDrawerData
}

type roomCreateDrawerData struct {
	Period               string
	StatusFilter         string
	CollectionFilter     string
	Search               string
	Sort                 string
	PropertyID           uint64
	Properties           []propertyPageRow
	Tenants              []roomRentPlanTenantOption
	Form                 roomPageForm
	ReturnURL            string
	ReturnContext        string
	ReturnPropertyID     uint64
	ReturnPeriod         string
	ReturnListStatus     string
	ReturnListSearch     string
	ReturnListCollection string
	ReturnListSort       string
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
	ID             uint64
	PropertyID     uint64
	RoomLabel      string
	RoomType       string
	Capacity       int
	Notes          string
	MonthlyRent    string
	EffectiveMonth string
	DueDay         int
}

func defaultRoomCreateForm(propertyID uint64) roomPageForm {
	return roomPageForm{
		PropertyID:     propertyID,
		Capacity:       1,
		EffectiveMonth: dublinCurrentMonth(time.Now()).Format("2006-01"),
		DueDay:         1,
	}
}

func withRoomCreateDefaults(form roomPageForm) roomPageForm {
	if form.Capacity == 0 {
		form.Capacity = 1
	}
	if form.EffectiveMonth == "" {
		form.EffectiveMonth = dublinCurrentMonth(time.Now()).Format("2006-01")
	}
	if form.DueDay == 0 {
		form.DueDay = 1
	}
	return form
}

type roomEditPageData struct {
	workspaceShell
	Form       roomPageForm
	Properties []propertyPageRow
	Error      string
}

type cashReceiptPageRow struct {
	ID            uint64
	ReceiptNumber string
	TenantID      uint64
	TenantName    string
	PayerName     string
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
	Sort         string
	SortLinks    map[string]tableSortLink
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

func assetStatusLabel(status string) string {
	if status == "active" {
		return "在用"
	}
	return pageStatusLabel(status)
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
	path := "/transactions"
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
	http.Redirect(w, r, legacyBillsWorkspaceURL(r.URL.Query()), http.StatusFound)
}

// handleBillingAlias is the one URL outside the app that still has to resolve:
// the bank authorization callback lands on /billing?message=bank_connected, and
// old bookmarks point here too. The page that used to live at /billing was a
// second, older transaction table and is gone; this forwards instead. The query
// goes over verbatim because /transactions reads the same keys the old page did
// (message, error, reconnect, period, match_status...), so a bookmarked filter
// keeps working. /bills is the same shape (see handleBills).
func (a *app) handleBillingAlias(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if _, ok := a.scopedPageUser(w, r); !ok {
		return
	}
	target := "/transactions"
	if raw := r.URL.RawQuery; raw != "" {
		target += "?" + raw
	}
	http.Redirect(w, r, target, http.StatusFound)
}

func legacyBillsWorkspaceURL(query url.Values) string {
	period, err := parsePeriodMonth(strings.TrimSpace(query.Get("period")))
	if err != nil {
		period = monthStart(time.Now().UTC())
	}
	filters := defaultRentWorkspaceFilters(period)
	if parsed, parseErr := rentWorkspaceFiltersFromQuery(query); parseErr == nil {
		filters = parsed
	}
	filters.View = rentWorkspaceViewTenants
	target, err := url.Parse(rentWorkspaceURL(filters, filters.Page))
	if err != nil {
		return "/rent-dashboard?view=tenants"
	}
	values := target.Query()
	for _, key := range []string{"message", "error"} {
		if value := strings.TrimSpace(query.Get(key)); value != "" {
			values.Set(key, value)
		}
	}
	target.RawQuery = values.Encode()
	return target.String()
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
	if strings.HasSuffix(path, "/rent-plan") {
		roomIDText := strings.TrimSuffix(path, "/rent-plan")
		if r.Method == http.MethodPost {
			a.handleRoomRentPlanMutation(w, r, roomIDText)
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if strings.Contains(path, "/") {
		http.NotFound(w, r)
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

func (a *app) handleRoomRentPlanMutation(w http.ResponseWriter, r *http.Request, pathID string) {
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
	roomID, err := parsePositiveUint(pathID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		a.redirectRoomRentPlanMutation(w, r, roomID, "", "rent_plan_invalid")
		return
	}
	period := validatedPeriodValue(r.Form.Get("period"))
	version, versionErr := strconv.ParseUint(strings.TrimSpace(r.Form.Get("plan_version")), 10, 64)
	if versionErr != nil {
		a.redirectRoomRentPlanMutation(w, r, roomID, period, "rent_plan_stale")
		return
	}
	service := newRoomRentPlanService(a.db)
	switch strings.TrimSpace(r.Form.Get("action")) {
	case "save":
		effectiveMonth, parseErr := parsePeriodMonth(strings.TrimSpace(r.Form.Get("effective_month")))
		monthlyRent, rentErr := parseOptionalRentPlanAmountCents(r.Form.Get("monthly_rent"))
		dueDay, dueErr := strconv.Atoi(strings.TrimSpace(r.Form.Get("due_day")))
		if amount, numericErr := strconv.ParseFloat(strings.TrimSpace(r.Form.Get("monthly_rent")), 64); numericErr == nil && amount <= 0 {
			a.redirectRoomRentPlanMutation(w, r, roomID, period, "rent_plan_zero_rent")
			return
		}
		if parseErr != nil || rentErr != nil || dueErr != nil {
			a.redirectRoomRentPlanMutation(w, r, roomID, period, "rent_plan_invalid")
			return
		}
		splitEvenly := r.Form.Get("split_evenly") == "1"
		tenants := r.Form["tenant_id"]
		responsibilities := r.Form["responsibility"]
		members := make([]RoomRentPlanMemberInput, 0, len(tenants))
		for index, rawTenantID := range tenants {
			rawAmount := ""
			if index < len(responsibilities) {
				rawAmount = responsibilities[index]
			}
			if strings.TrimSpace(rawTenantID) == "" && strings.TrimSpace(rawAmount) == "" {
				continue
			}
			tenantID, idErr := parsePositiveUint(rawTenantID)
			amountCents := int64(0)
			if !splitEvenly {
				amountCents, rentErr = parseOptionalRentPlanAmountCents(rawAmount)
			}
			if idErr != nil || rentErr != nil {
				a.redirectRoomRentPlanMutation(w, r, roomID, period, "rent_plan_invalid")
				return
			}
			members = append(members, RoomRentPlanMemberInput{TenantID: tenantID, ResponsibilityCents: amountCents})
		}
		_, _, err = service.SaveRoomRentPlan(r.Context(), SaveRoomRentPlanCommand{
			UserID: userID, RoomID: roomID, EffectiveMonth: effectiveMonth,
			MonthlyRentCents: monthlyRent, Currency: ledgerCurrencyEUR, DueDay: dueDay,
			Members: members, ExpectedTimelineVersion: version,
		})
		if err == nil {
			a.redirectRoomRentPlanMutation(w, r, roomID, period, "rent_plan_saved")
			return
		}
	case "end":
		vacantFrom, parseErr := parsePeriodMonth(strings.TrimSpace(r.Form.Get("vacant_from_month")))
		if parseErr != nil {
			a.redirectRoomRentPlanMutation(w, r, roomID, period, "rent_plan_invalid")
			return
		}
		_, err = service.EndRoomRentPlan(r.Context(), EndRoomRentPlanCommand{
			UserID: userID, RoomID: roomID, VacantFromMonth: vacantFrom,
			ExpectedTimelineVersion: version,
		})
		if err == nil {
			a.redirectRoomRentPlanMutation(w, r, roomID, period, "rent_plan_ended")
			return
		}
	default:
		err = ErrInvalidRentPlan
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		http.NotFound(w, r)
		return
	}
	errorCode := "rent_plan_failed"
	switch {
	case errors.Is(err, ErrInvalidRentPlan):
		errorCode = "rent_plan_invalid"
	case errors.Is(err, ErrRentPlanFactsLocked):
		errorCode = "rent_plan_locked"
	case errors.Is(err, ErrStaleRentPlanTimeline):
		errorCode = "rent_plan_stale"
	case errors.Is(err, ErrRentPlanTimelineConflict):
		errorCode = "rent_plan_conflict"
	case errors.Is(err, ErrTenantRoomMonthConflict):
		errorCode = "tenant_room_month_conflict"
	}
	a.redirectRoomRentPlanMutation(w, r, roomID, period, errorCode)
}

func (a *app) redirectRoomRentPlanMutation(w http.ResponseWriter, r *http.Request, roomID uint64, period, result string) {
	query := url.Values{}
	query.Set("period", validatedPeriodValue(period))
	query.Set("rent", "1")
	if result == "rent_plan_saved" || result == "rent_plan_ended" {
		query.Set("message", result)
	} else if result != "" {
		query.Set("rent_error", result)
	}
	copyRoomReturnContext(query, r.Form)
	http.Redirect(w, r, "/rooms/"+strconv.FormatUint(roomID, 10)+"?"+query.Encode(), http.StatusFound)
}

func rentPlanErrorMessage(errorCode string) string {
	switch errorCode {
	case "rent_plan_invalid":
		return "入住与租金未保存。请检查月份、缴租日和租客；如果填写了个人月租，所有租客金额之和须等于房间月租。"
	case "rent_plan_zero_rent":
		return "房间月租必须大于 0。若房间从某月起空置，请在下方选择该月份并点击「结束入住」。"
	case "rent_plan_locked":
		return "所选月份及之后已有收款、平账或催收记录，入住与租金计划不能重算。"
	case "rent_plan_stale":
		return "计划已被其他操作更新，请刷新后重新编辑。"
	case "rent_plan_conflict":
		return "房间计划时间线存在重叠，请刷新后重试。"
	case "tenant_room_month_conflict":
		return "该租客从所选月份起已安排在其他房间。请先到原房间的「入住与租金」移除该租客，再回来保存。"
	case "rent_plan_failed":
		return "入住与租金计划保存失败，请稍后重试。"
	default:
		return ""
	}
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
	properties, err := repo.listProperties(r.Context(), userID, propertyQuery{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	propertyRows := roomEditPropertyOptions(properties, roomRow.PropertyID)
	data := roomEditPageData{workspaceShell: canonicalPageShell(a, r, "rooms", "房间编辑"), Form: roomPageForm{ID: roomRow.ID, PropertyID: roomRow.PropertyID, RoomLabel: roomRow.RoomLabel, RoomType: roomRow.RoomType, Capacity: roomRow.Capacity, Notes: stringValue(roomRow.Notes)}, Properties: propertyRows, Error: r.URL.Query().Get("error")}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := roomEditPageTemplate.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func roomEditPropertyOptions(properties []property, currentPropertyID uint64) []propertyPageRow {
	options := make([]propertyPageRow, 0, len(properties))
	for _, row := range properties {
		if row.Status != "active" && row.ID != currentPropertyID {
			continue
		}
		options = append(options, propertyPageRow{ID: row.ID, Name: row.Name, Address: propertyAddress(row), Status: row.Status, StatusLabel: assetStatusLabel(row.Status)})
	}
	return options
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

type monthlyObjectTenant struct {
	RoomID       uint64
	Name         string
	DisplayAlias string
}

// The selected rent-plan month, rather than current asset or tenant status,
// determines which tenants can be found on the object lists.
func (a *app) monthlyObjectTenants(ctx context.Context, userID uint64, period time.Time) (map[uint64][]monthlyObjectTenant, error) {
	var matches []monthlyObjectTenant
	month := monthStart(period)
	err := a.db.WithContext(ctx).Table("room_rent_plan_members AS member").
		Select("plan.room_id, tenant.name, tenant.display_alias").
		Joins("JOIN room_rent_plans AS plan ON plan.id = member.room_rent_plan_id AND plan.user_id = member.user_id").
		Joins("JOIN rooms AS room ON room.id = plan.room_id AND room.user_id = plan.user_id").
		Joins("JOIN tenants AS tenant ON tenant.id = member.tenant_id AND tenant.user_id = member.user_id").
		Where("member.user_id = ? AND plan.effective_from_month <= ? AND (plan.effective_to_month IS NULL OR plan.effective_to_month >= ?)", userID, month, month).
		Order("plan.room_id ASC, member.id ASC").
		Scan(&matches).Error
	if err != nil {
		return nil, err
	}
	byRoom := make(map[uint64][]monthlyObjectTenant)
	for _, match := range matches {
		byRoom[match.RoomID] = append(byRoom[match.RoomID], match)
	}
	return byRoom, nil
}

func (a *app) loadPropertyPage(ctx context.Context, userID uint64, period time.Time, statusFilter string) (propertyPageData, error) {
	return a.loadPropertyPageWithAssetHistory(ctx, userID, period, statusFilter, false)
}

func (a *app) loadPropertyPageWithAssetHistory(ctx context.Context, userID uint64, period time.Time, statusFilter string, includeDeleted bool) (propertyPageData, error) {
	statusFilter = firstNonEmpty(strings.TrimSpace(statusFilter), "active")
	if statusFilter != "all" && statusFilter != "active" && statusFilter != "inactive" {
		return propertyPageData{}, errors.New("property status filter is invalid")
	}
	properties, err := newLandlordRentRepository(a.db).listProperties(ctx, userID, propertyQuery{IncludeDeleted: includeDeleted, Status: func() string {
		if statusFilter == "all" {
			return ""
		}
		return statusFilter
	}()})
	if err != nil {
		return propertyPageData{}, err
	}
	rooms, err := newLandlordRentRepository(a.db).listRooms(ctx, userID, roomQuery{IncludeDeleted: includeDeleted})
	if err != nil {
		return propertyPageData{}, err
	}
	monthTenants, err := a.monthlyObjectTenants(ctx, userID, period)
	if err != nil {
		return propertyPageData{}, err
	}
	financial := make(map[uint64]rentWorkspacePropertyRow)
	responsibilityCount := make(map[uint64]int)
	if activeRows, loadErr := newRentWorkspaceService(a.db).loadWithAssetHistory(ctx, userID, func() rentWorkspaceFilters {
		filters := defaultRentWorkspaceFilters(period)
		filters.PageSize = rentWorkspaceMaxPageSize
		return filters
	}(), includeDeleted); loadErr == nil {
		for _, row := range activeRows.PropertyRows {
			financial[row.PropertyID] = row
		}
		for _, row := range activeRows.TenantRows {
			responsibilityCount[row.PropertyID]++
		}
	}
	roomCount := make(map[uint64]int)
	activeRoomCount := make(map[uint64]int)
	searchTerms := make(map[uint64][]string)
	for _, row := range rooms {
		roomCount[row.PropertyID]++
		searchTerms[row.PropertyID] = append(searchTerms[row.PropertyID], row.RoomLabel)
		for _, tenantRow := range monthTenants[row.ID] {
			searchTerms[row.PropertyID] = append(searchTerms[row.PropertyID], tenantRow.Name, tenantRow.DisplayAlias)
		}
		if row.Status == "active" {
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
			StatusLabel:         assetStatusLabel(row.Status), CollectionStatus: collectionState, CollectionStatusLabel: collectionLabel, RoomCount: roomCount[row.ID], ActiveRoomCount: activeRoomCount[row.ID],
			ExpectedCents: financialRow.ExpectedCents, PaidCents: financialRow.PaidCents, BalanceCents: financialRow.BalanceCents, ExpenseCents: financialRow.ExpenseCents, OtherIncomeCents: financialRow.OtherIncomeCents,
			ExpectedAmount: pageCurrencyAmount(financialRow.ExpectedCents, currency), PaidAmount: pageCurrencyAmount(financialRow.PaidCents, currency), BalanceAmount: pageCurrencyAmount(financialRow.BalanceCents, currency), ExpenseAmount: pageCurrencyAmount(financialRow.ExpenseCents, currency), OtherIncomeAmount: pageCurrencyAmount(financialRow.OtherIncomeCents, currency), CollectionPercent: financialRow.CollectionPercent,
			NetAmount:   pageCurrencyAmount(financialRow.NetCents, currency),
			Actions:     propertyActions(row.ID, row.Status == "active" && row.DeletedAt == nil),
			SearchTerms: searchTerms[row.ID],
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
		if strings.Contains(strings.ToLower(row.Name+" "+row.CityRegion), needle) || strings.Contains(strings.ToLower(row.Address), needle) || containsSearchTerm(row.SearchTerms, needle) {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

func containsSearchTerm(terms []string, needle string) bool {
	for _, term := range terms {
		if strings.Contains(strings.ToLower(term), needle) {
			return true
		}
	}
	return false
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

const (
	propertyPageDefaultSort = "name_asc"
	roomPageDefaultSort     = "room_asc"
)

func validPropertyPageSort(value string) bool {
	switch value {
	case "", "name_asc", "name_desc", "address_asc", "address_desc", "rooms_asc", "rooms_desc", "responsibilities_asc", "responsibilities_desc", "expected_asc", "expected_desc", "paid_asc", "paid_desc", "status_asc", "status_desc":
		return true
	default:
		return false
	}
}

func sortPropertyPageRows(rows []propertyPageRow, sortValue string) []propertyPageRow {
	if sortValue == "" {
		sortValue = propertyPageDefaultSort
	}
	sorted := append([]propertyPageRow(nil), rows...)
	sort.SliceStable(sorted, func(i, j int) bool {
		left, right := sorted[i], sorted[j]
		leftName, rightName := strings.ToLower(left.Name), strings.ToLower(right.Name)
		switch sortValue {
		case "name_asc":
			if leftName != rightName {
				return leftName < rightName
			}
		case "name_desc":
			if leftName != rightName {
				return leftName > rightName
			}
		case "address_asc", "address_desc":
			leftAddress, rightAddress := strings.ToLower(firstNonEmpty(left.CityRegion, left.Address)), strings.ToLower(firstNonEmpty(right.CityRegion, right.Address))
			if leftAddress != rightAddress {
				return leftAddress < rightAddress == (sortValue == "address_asc")
			}
		case "rooms_asc", "rooms_desc":
			if left.RoomCount != right.RoomCount {
				return left.RoomCount < right.RoomCount == (sortValue == "rooms_asc")
			}
		case "responsibilities_asc", "responsibilities_desc":
			if left.ResponsibilityCount != right.ResponsibilityCount {
				return left.ResponsibilityCount < right.ResponsibilityCount == (sortValue == "responsibilities_asc")
			}
		case "expected_asc", "expected_desc":
			if left.ExpectedCents != right.ExpectedCents {
				return left.ExpectedCents < right.ExpectedCents == (sortValue == "expected_asc")
			}
		case "paid_asc", "paid_desc":
			if left.PaidCents != right.PaidCents {
				return left.PaidCents < right.PaidCents == (sortValue == "paid_asc")
			}
		case "status_asc", "status_desc":
			leftStatus, rightStatus := strings.ToLower(left.CollectionStatusLabel), strings.ToLower(right.CollectionStatusLabel)
			if leftStatus != rightStatus {
				return leftStatus < rightStatus == (sortValue == "status_asc")
			}
		}
		if leftName != rightName {
			return leftName < rightName
		}
		return left.ID < right.ID
	})
	return sorted
}

func propertyListURL(period, status, search, collection, sortValue string) string {
	query := url.Values{"period": []string{validatedPeriodValue(period)}}
	if status != "all" && status != "active" && status != "inactive" {
		status = "active"
	}
	query.Set("status", status)
	if search = strings.TrimSpace(search); search != "" {
		query.Set("search", search)
	}
	if collection == "unpaid" || collection == "paid" {
		query.Set("collection", collection)
	}
	if sortValue != "" && sortValue != propertyPageDefaultSort {
		query.Set("sort", sortValue)
	}
	return "/properties?" + query.Encode()
}

func propertyPageSortLinks(period, status, search, collection, current string) map[string]tableSortLink {
	activeSort := normalisedSort(current, propertyPageDefaultSort)
	sortURL := func(sortValue string) string {
		return propertyListURL(period, status, search, collection, sortValue)
	}
	return map[string]tableSortLink{
		"name":             sortLinkFor(sortURL, activeSort, "name_asc", "name_desc"),
		"address":          sortLinkFor(sortURL, activeSort, "address_asc", "address_desc"),
		"rooms":            sortLinkFor(sortURL, activeSort, "rooms_asc", "rooms_desc"),
		"responsibilities": sortLinkFor(sortURL, activeSort, "responsibilities_asc", "responsibilities_desc"),
		"expected":         sortLinkFor(sortURL, activeSort, "expected_asc", "expected_desc"),
		"paid":             sortLinkFor(sortURL, activeSort, "paid_asc", "paid_desc"),
		"status":           sortLinkFor(sortURL, activeSort, "status_asc", "status_desc"),
	}
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
			match = match || containsSearchTerm(row.TenantNames, needle) || containsSearchTerm(row.SearchTerms, needle)
			if !match {
				continue
			}
		}
		filtered = append(filtered, row)
	}
	return filtered
}

func validRoomPageSort(value string) bool {
	switch value {
	case "", "room_asc", "room_desc", "property_asc", "property_desc", "tenants_asc", "tenants_desc", "expected_asc", "expected_desc", "paid_asc", "paid_desc", "count_asc", "count_desc", "status_asc", "status_desc":
		return true
	default:
		return false
	}
}

func sortRoomPageRows(rows []roomPageRow, sortValue string) []roomPageRow {
	if sortValue == "" {
		sortValue = roomPageDefaultSort
	}
	sorted := append([]roomPageRow(nil), rows...)
	sort.SliceStable(sorted, func(i, j int) bool {
		left, right := sorted[i], sorted[j]
		leftRoom, rightRoom := strings.ToLower(left.RoomLabel), strings.ToLower(right.RoomLabel)
		switch sortValue {
		case "room_asc":
			if leftRoom != rightRoom {
				return leftRoom < rightRoom
			}
		case "room_desc":
			if leftRoom != rightRoom {
				return leftRoom > rightRoom
			}
		case "property_asc", "property_desc":
			leftProperty, rightProperty := strings.ToLower(left.PropertyName), strings.ToLower(right.PropertyName)
			if leftProperty != rightProperty {
				return leftProperty < rightProperty == (sortValue == "property_asc")
			}
		case "tenants_asc", "tenants_desc":
			leftTenants, rightTenants := strings.ToLower(strings.Join(left.TenantNames, " ")), strings.ToLower(strings.Join(right.TenantNames, " "))
			if leftTenants != rightTenants {
				return leftTenants < rightTenants == (sortValue == "tenants_asc")
			}
		case "expected_asc", "expected_desc":
			if left.ExpectedCents != right.ExpectedCents {
				return left.ExpectedCents < right.ExpectedCents == (sortValue == "expected_asc")
			}
		case "paid_asc", "paid_desc":
			if left.PaidCents != right.PaidCents {
				return left.PaidCents < right.PaidCents == (sortValue == "paid_asc")
			}
		case "count_asc", "count_desc":
			if len(left.TenantNames) != len(right.TenantNames) {
				return len(left.TenantNames) < len(right.TenantNames) == (sortValue == "count_asc")
			}
		case "status_asc", "status_desc":
			leftStatus, rightStatus := strings.ToLower(left.CollectionStatusLabel), strings.ToLower(right.CollectionStatusLabel)
			if leftStatus != rightStatus {
				return leftStatus < rightStatus == (sortValue == "status_asc")
			}
		}
		if leftRoom != rightRoom {
			return leftRoom < rightRoom
		}
		return left.ID < right.ID
	})
	return sorted
}

func roomPageSortLinks(period string, propertyID uint64, status, search, collection, current string) map[string]tableSortLink {
	activeSort := normalisedSort(current, roomPageDefaultSort)
	sortURL := func(sortValue string) string {
		return roomListURL(period, propertyID, status, search, collection, sortValue)
	}
	return map[string]tableSortLink{
		"room":     sortLinkFor(sortURL, activeSort, "room_asc", "room_desc"),
		"property": sortLinkFor(sortURL, activeSort, "property_asc", "property_desc"),
		"tenants":  sortLinkFor(sortURL, activeSort, "tenants_asc", "tenants_desc"),
		"expected": sortLinkFor(sortURL, activeSort, "expected_asc", "expected_desc"),
		"paid":     sortLinkFor(sortURL, activeSort, "paid_asc", "paid_desc"),
		"count":    sortLinkFor(sortURL, activeSort, "count_asc", "count_desc"),
		"status":   sortLinkFor(sortURL, activeSort, "status_asc", "status_desc"),
	}
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
	data.Sort = strings.TrimSpace(r.URL.Query().Get("sort"))
	if !validPropertyPageSort(data.Sort) {
		http.Error(w, "property sort is invalid", http.StatusBadRequest)
		return
	}
	data.Rows = sortPropertyPageRows(data.Rows, data.Sort)
	data.SortLinks = propertyPageSortLinks(data.Period, data.StatusFilter, data.Search, data.CollectionFilter, data.Sort)
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
	rooms, err := a.loadRoomRowsWithAssetHistory(r.Context(), userID, period, propertyID, propertyRow.DeletedAt != nil)
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
	listSort := strings.TrimSpace(r.URL.Query().Get("list_sort"))
	if !validPropertyPageSort(listSort) {
		listSort = ""
	}
	data := propertyDetailPageData{workspaceShell: canonicalPageShell(a, r, "properties", "房产详情"), IsDeleted: propertyRow.DeletedAt != nil, Period: period.Format("2006-01"), PeriodLabel: formatMonthLabel(period), ListStatus: listStatus, ListSearch: strings.TrimSpace(r.URL.Query().Get("list_search")), ListCollection: listCollection, ListSort: listSort, Property: propertyPageRow{ID: propertyRow.ID, Mark: propertyMark(propertyRow.Name), Name: propertyRow.Name, CityRegion: propertyRow.CityRegion, Address: propertyAddress(propertyRow), Timezone: propertyRow.Timezone, Notes: stringValue(propertyRow.Notes), Status: propertyRow.Status, StatusLabel: assetStatusLabel(propertyRow.Status), Actions: propertyActions(propertyRow.ID, propertyRow.Status == "active" && propertyRow.DeletedAt == nil)}, Rooms: rooms, Expenses: filteredExpenses, Editing: r.URL.Query().Get("edit") == "1" && propertyRow.DeletedAt == nil, Message: r.URL.Query().Get("message"), Error: r.URL.Query().Get("error")}
	data.RoomOpenURL = propertyRoomOpenURL(r)
	if propertyRow.DeletedAt == nil && (r.URL.Query().Get("room") == "1" || r.URL.Query().Get("room_error") != "") {
		returnURL := propertyDetailReturnURL(r)
		form := defaultRoomCreateForm(propertyID)
		formMonth, _ := parsePeriodMonth(form.EffectiveMonth)
		tenantOptions, tenantErr := loadRoomTenantOptions(r.Context(), a.db, userID, 0, formMonth)
		if tenantErr != nil {
			http.Error(w, tenantErr.Error(), http.StatusInternalServerError)
			return
		}
		data.RoomDrawer = &roomCreateDrawerData{
			Period: period.Format("2006-01"), StatusFilter: "all", CollectionFilter: "all", PropertyID: propertyID,
			Properties: []propertyPageRow{{ID: propertyRow.ID, Name: propertyRow.Name}}, Tenants: tenantOptions, Form: form, ReturnURL: returnURL,
			ReturnContext: "property", ReturnPropertyID: propertyID, ReturnPeriod: period.Format("2006-01"),
			ReturnListStatus: listStatus, ReturnListSearch: strings.TrimSpace(r.URL.Query().Get("list_search")), ReturnListCollection: listCollection, ReturnListSort: listSort,
			ErrorMessage: roomMutationErrorMessage(r.URL.Query().Get("room_error")),
		}
	}
	if propertyRow.DeletedAt == nil && (r.URL.Query().Get("expense") == "1" || isExpenseFormError(r.URL.Query().Get("error"))) {
		returnURL := expenseFormReturnURL(r)
		expenseDrawer, drawerErr := a.loadExpenseDrawerData(r.Context(), userID, period.Format("2006-01"), propertyID, 0, returnURL, r.URL.Query().Get("error"))
		if drawerErr != nil {
			http.Error(w, drawerErr.Error(), http.StatusInternalServerError)
			return
		}
		data.ExpenseDrawer = expenseDrawer
	}
	if summary, summaryErr := a.loadPropertyPageWithAssetHistory(r.Context(), userID, period, "all", propertyRow.DeletedAt != nil); summaryErr == nil {
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
		redirectPropertyMutation(w, r, propertyID, "", "", true)
		return
	case "deactivate":
		err = service.deactivateProperty(r.Context(), userID, propertyID)
	case "delete":
		err = service.deleteProperty(r.Context(), userID, propertyID)
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
			redirectPropertyMutation(w, r, propertyID, "", "property_action_failed", true)
			return
		}
		if action == "delete" && propertyID != 0 && errors.Is(err, errPropertyDeletionBlocked) {
			redirectPropertyMutation(w, r, propertyID, "", "property_delete_blocked", false)
			return
		}
		redirectPropertyList(w, r, "", "property_action_failed", action == "save")
		return
	}
	if action == "save" && propertyID != 0 {
		redirectPropertyMutation(w, r, propertyID, "property_saved", "", false)
		return
	}
	if action == "deactivate" {
		redirectPropertyList(w, r, "property_deactivated", "", false)
		return
	}
	if action == "delete" {
		redirectPropertyList(w, r, "property_deleted", "", false)
		return
	}
	redirectPropertyList(w, r, "property_saved", "", false)
}

func redirectPropertyMutation(w http.ResponseWriter, r *http.Request, propertyID uint64, message, errorCode string, editing bool) {
	query := url.Values{}
	if message != "" {
		query.Set("message", message)
	}
	if errorCode != "" {
		query.Set("error", errorCode)
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
	if listSort := strings.TrimSpace(r.Form.Get("list_sort")); validPropertyPageSort(listSort) && listSort != "" && listSort != propertyPageDefaultSort {
		query.Set("list_sort", listSort)
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
	if sortValue := strings.TrimSpace(r.Form.Get("sort")); validPropertyPageSort(sortValue) && sortValue != "" && sortValue != propertyPageDefaultSort {
		query.Set("sort", sortValue)
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
	if sortValue := strings.TrimSpace(r.Form.Get("sort")); validRoomPageSort(sortValue) && sortValue != "" && sortValue != roomPageDefaultSort {
		query.Set("sort", sortValue)
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

func redirectRoomDetail(w http.ResponseWriter, r *http.Request, roomID uint64, message, errorCode string) {
	query := url.Values{}
	if period, err := parsePeriodMonth(strings.TrimSpace(r.Form.Get("period"))); err == nil {
		query.Set("period", period.Format("2006-01"))
	}
	if message != "" {
		query.Set("message", message)
	}
	if errorCode != "" {
		query.Set("error", errorCode)
	}
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
	if len(collection) > 1 && collection[1] != "" && collection[1] != roomPageDefaultSort {
		query.Set("sort", collection[1])
	}
	return "/rooms?" + query.Encode()
}

func (a *app) loadRoomRows(ctx context.Context, userID uint64, period time.Time, propertyID uint64) ([]roomPageRow, error) {
	return a.loadRoomRowsWithAssetHistory(ctx, userID, period, propertyID, false)
}

func (a *app) loadRoomRowsWithAssetHistory(ctx context.Context, userID uint64, period time.Time, propertyID uint64, includeDeleted bool) ([]roomPageRow, error) {
	repo := newLandlordRentRepository(a.db)
	rooms, err := repo.listRooms(ctx, userID, roomQuery{PropertyID: propertyID, IncludeDeleted: includeDeleted})
	if err != nil {
		return nil, err
	}
	properties, err := repo.listProperties(ctx, userID, propertyQuery{IncludeDeleted: includeDeleted})
	if err != nil {
		return nil, err
	}
	monthTenants, err := a.monthlyObjectTenants(ctx, userID, period)
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
	if workspace, loadErr := newRentWorkspaceService(a.db).loadWithAssetHistory(ctx, userID, filters, includeDeleted); loadErr == nil {
		for _, row := range workspace.RoomRows {
			financial[row.RoomID] = row
		}
	}
	rows := make([]roomPageRow, 0, len(rooms))
	for _, roomRow := range rooms {
		propertyRow := propertyByID[roomRow.PropertyID]
		plans, err := repo.listRoomRentPlans(ctx, userID, roomRentPlanQuery{RoomID: roomRow.ID, EffectiveFromMonthOnOrBefore: &period, EffectiveToMonthOnOrAfter: &period})
		if err != nil {
			return nil, err
		}
		var plan *roomRentPlan
		if len(plans) > 0 {
			plan = &plans[0]
		}
		partyNames := make([]string, 0, len(monthTenants[roomRow.ID]))
		searchTerms := make([]string, 0, len(monthTenants[roomRow.ID])*2)
		for _, tenantRow := range monthTenants[roomRow.ID] {
			partyNames = append(partyNames, firstNonEmpty(tenantRow.DisplayAlias, tenantRow.Name))
			searchTerms = append(searchTerms, tenantRow.Name, tenantRow.DisplayAlias)
		}
		financialRow := financial[roomRow.ID]
		currency := firstNonEmpty(func() string {
			if plan != nil {
				return plan.Currency
			}
			return ""
		}(), ledgerCurrencyEUR)
		collectionState, collectionLabel := collectionStatus(financialRow.ExpectedCents, financialRow.PaidCents)
		pageRow := roomPageRow{ID: roomRow.ID, PropertyID: roomRow.PropertyID, PropertyName: propertyRow.Name, RoomLabel: roomRow.RoomLabel, RoomType: roomRow.RoomType, Capacity: roomRow.Capacity, Notes: stringValue(roomRow.Notes), Status: roomRow.Status, StatusLabel: assetStatusLabel(roomRow.Status), CollectionStatus: collectionState, CollectionStatusLabel: collectionLabel, TenantNames: partyNames, SearchTerms: searchTerms, Currency: currency, ExpectedCents: financialRow.ExpectedCents, PaidCents: financialRow.PaidCents, BalanceCents: financialRow.BalanceCents, ExpectedAmount: pageCurrencyAmount(financialRow.ExpectedCents, currency), PaidAmount: pageCurrencyAmount(financialRow.PaidCents, currency), BalanceAmount: pageCurrencyAmount(financialRow.BalanceCents, currency), Actions: roomActions(roomRow.ID, roomRow.Status == "active" && roomRow.DeletedAt == nil && propertyRow.DeletedAt == nil)}
		if plan != nil {
			pageRow.MonthlyRentCents, pageRow.DueDay = plan.MonthlyRentCents, plan.DueDay
			pageRow.MonthlyRent = pageCurrencyAmount(plan.MonthlyRentCents, plan.Currency)
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
	sortValue := strings.TrimSpace(r.URL.Query().Get("sort"))
	if !validRoomPageSort(sortValue) {
		http.Error(w, "room sort is invalid", http.StatusBadRequest)
		return
	}
	rows = sortRoomPageRows(rows, sortValue)
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
		propertyRows = append(propertyRows, propertyPageRow{ID: row.ID, Name: row.Name, CityRegion: row.CityRegion, Address: propertyAddress(row), Timezone: row.Timezone, Notes: stringValue(row.Notes), Status: row.Status, StatusLabel: assetStatusLabel(row.Status)})
	}
	propertyOptions := make([]propertyPageRow, 0, len(allProperties))
	for _, row := range allProperties {
		propertyOptions = append(propertyOptions, propertyPageRow{ID: row.ID, Name: row.Name, CityRegion: row.CityRegion, Address: propertyAddress(row), Timezone: row.Timezone, Notes: stringValue(row.Notes), Status: row.Status, StatusLabel: assetStatusLabel(row.Status)})
	}
	data := roomPageData{workspaceShell: canonicalPageShell(a, r, "rooms", "房间与房产绑定"), Period: period.Format("2006-01"), PeriodLabel: formatMonthLabel(period), PeriodOptions: pagePeriodOptions(period), Rows: rows, Properties: propertyRows, PropertyOptions: propertyOptions, PropertyID: propertyID, StatusFilter: statusFilter, CollectionFilter: collectionFilter, Search: search, Sort: sortValue, SortLinks: roomPageSortLinks(period.Format("2006-01"), propertyID, statusFilter, search, collectionFilter, sortValue), ShowForm: r.URL.Query().Get("add") == "1", Form: defaultRoomCreateForm(propertyID), Message: r.URL.Query().Get("message"), Error: r.URL.Query().Get("error")}
	if data.ShowForm {
		returnURL := roomListURL(data.Period, propertyID, statusFilter, search, collectionFilter, sortValue)
		formMonth, _ := parsePeriodMonth(data.Form.EffectiveMonth)
		tenantOptions, tenantErr := loadRoomTenantOptions(r.Context(), a.db, userID, 0, formMonth)
		if tenantErr != nil {
			http.Error(w, tenantErr.Error(), http.StatusInternalServerError)
			return
		}
		data.Drawer = &roomCreateDrawerData{Period: data.Period, StatusFilter: statusFilter, CollectionFilter: collectionFilter, Search: search, Sort: sortValue, PropertyID: propertyID, Properties: propertyRows, Tenants: tenantOptions, Form: data.Form, ReturnURL: returnURL, ErrorMessage: roomMutationErrorMessage(data.Error)}
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
	if parseErr != nil && action != "deactivate" && action != "delete" {
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
	createdRoomID := uint64(0)
	switch action {
	case "deactivate":
		err = service.deactivateRoom(r.Context(), userID, roomID, time.Time{})
	case "delete":
		err = service.deleteRoom(r.Context(), userID, roomID)
	case "save", "save_and_setup":
		capacity := 1
		if roomID != 0 {
			capacity = 0
		}
		if strings.TrimSpace(r.Form.Get("capacity")) != "" {
			raw := strings.TrimSpace(r.Form.Get("capacity"))
			capacity, err = strconv.Atoi(raw)
		}
		if err == nil && roomID == 0 {
			effectiveMonth, monthErr := parsePeriodMonth(strings.TrimSpace(r.Form.Get("effective_month")))
			monthlyRent, rentErr := parseOptionalRentPlanAmountCents(r.Form.Get("monthly_rent"))
			dueDay, dueErr := strconv.Atoi(strings.TrimSpace(r.Form.Get("due_day")))
			if monthErr != nil || rentErr != nil || dueErr != nil {
				err = ErrInvalidRentPlan
				break
			}
			selectedTenantID := uint64(0)
			if action == "save_and_setup" {
				switch firstNonEmpty(strings.TrimSpace(r.Form.Get("tenant_choice")), "new") {
				case "existing":
					selectedTenantID, err = parsePositiveUint(r.Form.Get("tenant_id"))
					if err != nil {
						err = ErrInvalidRentPlan
					}
				case "new":
				default:
					err = ErrInvalidRentPlan
				}
				if err != nil {
					break
				}
			}
			var created room
			created, _, err = service.createRoomWithRentPlanAndTenant(r.Context(), userID, roomInput{PropertyID: propertyID, RoomLabel: r.Form.Get("room_label"), RoomType: r.Form.Get("room_type"), Capacity: capacity, Notes: r.Form.Get("notes")}, roomRentPlanSetupInput{EffectiveMonth: effectiveMonth, MonthlyRentCents: monthlyRent, Currency: ledgerCurrencyEUR, DueDay: dueDay}, selectedTenantID)
			createdRoomID = created.ID
		} else if err == nil {
			_, err = service.updateRoom(r.Context(), userID, roomID, roomInput{PropertyID: propertyID, RoomLabel: r.Form.Get("room_label"), RoomType: r.Form.Get("room_type"), Capacity: capacity, Notes: r.Form.Get("notes")})
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
		if errors.Is(err, ErrRoomPropertyLocked) {
			errorCode = "room_property_locked"
		}
		if errors.Is(err, ErrTenantRoomMonthConflict) {
			errorCode = "tenant_room_conflict"
		}
		if errors.Is(err, ErrInvalidRentPlan) || errors.Is(err, gorm.ErrRecordNotFound) {
			errorCode = "tenant_room_invalid"
		}
		if action == "delete" && errors.Is(err, errRoomDeletionBlocked) {
			redirectRoomDetail(w, r, roomID, "", "room_delete_blocked")
			return
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
	if action == "save_and_setup" && createdRoomID != 0 && firstNonEmpty(strings.TrimSpace(r.Form.Get("tenant_choice")), "new") == "new" {
		query := url.Values{
			"period":     []string{strings.TrimSpace(r.Form.Get("effective_month"))},
			"tenant_add": []string{"1"},
		}
		http.Redirect(w, r, "/rooms/"+strconv.FormatUint(createdRoomID, 10)+"?"+query.Encode(), http.StatusFound)
		return
	}
	if action == "save_and_setup" && createdRoomID != 0 {
		http.Redirect(w, r, "/rooms/"+strconv.FormatUint(createdRoomID, 10)+"?period="+url.QueryEscape(strings.TrimSpace(r.Form.Get("effective_month")))+"&rent=1&message=room_saved", http.StatusFound)
		return
	}
	if action == "delete" {
		redirectRoomList(w, r, "room_deleted", "", false)
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
	if sortValue := strings.TrimSpace(values.Get("return_list_sort")); validPropertyPageSort(sortValue) && sortValue != "" && sortValue != propertyPageDefaultSort {
		query.Set("list_sort", sortValue)
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

func parseOptionalRentPlanAmountCents(raw string) (int64, error) {
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

func roomMutationErrorMessage(errorCode string) string {
	switch errorCode {
	case "tenant_room_conflict":
		return "该租客从所选月份起已在其他房间入住。请先去原房间的「入住与租金」解绑，再回来选择。"
	case "tenant_room_invalid":
		return "请检查所选租客、月份和租金；租客可能已停用或不可用。"
	case "room_property_locked":
		return "房间已有租金记录，不能更改所属房产。"
	case "room_action_failed":
		return "房间资料未能保存，请检查输入内容。"
	case "room_delete_blocked":
		return "请先在「入住与租金」中选择结束入住的月份，再隐藏房间。历史记录会保留。"
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
	if sortValue := strings.TrimSpace(source.Get("return_sort")); validRoomPageSort(sortValue) && sortValue != "" && sortValue != roomPageDefaultSort {
		target.Set("return_sort", sortValue)
	}
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
	returnURL := roomListURL(data.Period, data.PropertyID, data.StatusFilter, data.Search, data.CollectionFilter, data.Sort)
	return &roomCreateDrawerData{Period: data.Period, StatusFilter: data.StatusFilter, CollectionFilter: data.CollectionFilter, Search: data.Search, Sort: data.Sort, PropertyID: data.PropertyID, Properties: data.Properties, Form: withRoomCreateDefaults(data.Form), ReturnURL: returnURL, ErrorMessage: roomMutationErrorMessage(data.Error)}
}

var roomEditPageTemplate = newWorkspacePageTemplate("room-edit-page", nil, `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>RentOps Room Edit</title><style>`+workspacePageCSS+`</style></head><body><div class="app">{{template "workspace-nav" .}}<main class="content"><header class="topbar"><div><div class="brand-title">资产管理</div><h1>编辑房间</h1><div class="tiny">房间资料与入住租金计划分开维护</div></div><a class="btn" href="/rooms">返回房间</a></header>{{if .Error}}<div class="notice error">{{.Error}}</div>{{end}}<section class="panel surface entity-form" aria-labelledby="room-form-title"><div class="panel-head"><div><h2 id="room-form-title">房间资料</h2><p class="tiny">房间必须绑定一个房产。</p></div></div><form method="post" action="/rooms/{{.Form.ID}}"><input type="hidden" name="action" value="save"><div class="form-grid"><div class="form-field"><label for="room-label">房间名称</label><input id="room-label" name="room_label" value="{{.Form.RoomLabel}}" maxlength="191" required></div><div class="form-field"><label for="room-property">所属房产</label><select id="room-property" name="property_id" required><option value="">请选择房产</option>{{range .Properties}}<option value="{{.ID}}"{{if eq $.Form.PropertyID .ID}} selected{{end}}>{{.Name}}</option>{{end}}</select></div></div><div class="drawer-actions"><button class="btn primary" type="submit">保存房间</button></div></form></section></main></div></body></html>`)

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
