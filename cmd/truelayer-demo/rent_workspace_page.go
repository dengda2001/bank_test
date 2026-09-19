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

type rentWorkspacePageData struct {
	workspaceShell
	Filters          rentWorkspaceFilters
	Period           string
	PeriodLabel      string
	PreviousPeriod   string
	NextPeriod       string
	View             string
	Summary          rentWorkspaceSummary
	PendingCount     int
	PendingItems     []rentWorkspacePendingItem
	PropertyOptions  []rentWorkspacePropertyOption
	PropertyRows     []rentWorkspacePropertyRow
	PropertyTreeRows []rentWorkspacePropertyTreeRow
	RoomRows         []rentWorkspaceRoomRow
	RoomTreeRows     []rentWorkspaceRoomTreeRow
	TenantRows       []rentWorkspaceTenantRow
	TotalRows        int
	FilteredCount    int
	TotalPages       int
	Page             int
	PageSize         int
	Message          string
	Error            string
}

type rentWorkspacePendingItem struct {
	Title     string
	Subtitle  string
	Amount    string
	DetailURL string
	ListURL   string
}

func rentWorkspacePageFromData(a *app, r *http.Request, data rentWorkspaceData, message, pageError string) rentWorkspacePageData {
	period := monthStart(data.Filters.PeriodMonth)
	return rentWorkspacePageData{
		workspaceShell: a.fillWorkspaceShell(r, workspaceShell{
			ActivePage: func() string {
				if r.URL.Path == "/bills" {
					return "bills"
				}
				return "rent-dashboard"
			}(),
			Username:      a.displayUsername(r),
			Environment:   a.cfg.Environment,
			FootNote:      "月度收租工作台",
			CompactTitle:  "本月收租",
			ShowNavCounts: true,
			TenantCount:   len(data.TenantRows),
		}),
		Filters:          data.Filters,
		Period:           period.Format("2006-01"),
		PeriodLabel:      formatMonthLabel(period),
		PreviousPeriod:   period.AddDate(0, -1, 0).Format("2006-01"),
		NextPeriod:       period.AddDate(0, 1, 0).Format("2006-01"),
		View:             data.Filters.View,
		Summary:          data.Summary,
		PropertyOptions:  data.PropertyOptions,
		PropertyRows:     data.PropertyRows,
		PropertyTreeRows: data.PropertyTreeRows,
		RoomRows:         data.RoomRows,
		RoomTreeRows:     data.RoomTreeRows,
		TenantRows:       data.TenantRows,
		TotalRows:        data.TotalRows,
		FilteredCount:    data.FilteredCount,
		TotalPages:       data.TotalPages,
		Page:             data.Page,
		PageSize:         data.PageSize,
		Message:          message,
		Error:            pageError,
	}
}

func (a *app) renderRentWorkspaceDashboard(w http.ResponseWriter, r *http.Request) {
	filters, filterErr := rentWorkspaceFiltersFromQuery(r.URL.Query())
	if filterErr != nil {
		period, _ := parsePeriodMonth(strings.TrimSpace(r.URL.Query().Get("period")))
		if period.IsZero() {
			period = monthStart(time.Now().UTC())
		}
		filters = defaultRentWorkspaceFilters(period)
	}
	userID, ok := a.currentUserID(r)
	if !ok || a.db == nil {
		http.Error(w, "database session required", http.StatusServiceUnavailable)
		return
	}
	data, err := newRentWorkspaceService(a.db).load(r.Context(), userID, filters)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			http.Error(w, "workspace target not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	page := rentWorkspacePageFromData(a, r, data, r.URL.Query().Get("message"), "")
	var pendingTransactions int64
	if err := a.db.WithContext(r.Context()).Model(&paymentTransaction{}).
		Where("user_id = ? AND direction = ? AND match_status IN ?", userID, "income", pendingMatchStatuses).
		Where("transaction_time >= ? AND transaction_time < ?", filters.PeriodMonth, filters.PeriodMonth.AddDate(0, 1, 0)).
		Count(&pendingTransactions).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	page.PendingCount = int(pendingTransactions)
	// The topbar button and the panel below it are two views of one number, so the
	// button takes the panel's count verbatim rather than re-running the query and
	// risking a different answer (the shell's own fill already ran it once).
	page.PendingReviewCount = page.PendingCount
	page.PendingReviewURL = "/rent-dashboard?period=" + url.QueryEscape(filters.PeriodMonth.Format("2006-01")) + "#pending-review"
	var pendingRows []paymentTransaction
	if err := a.db.WithContext(r.Context()).
		Where("user_id = ? AND direction = ? AND match_status IN ?", userID, "income", pendingMatchStatuses).
		Where("transaction_time >= ? AND transaction_time < ?", filters.PeriodMonth, filters.PeriodMonth.AddDate(0, 1, 0)).
		Order("transaction_time ASC, id ASC").Limit(2).Find(&pendingRows).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	page.PendingItems = make([]rentWorkspacePendingItem, 0, len(pendingRows))
	for _, transaction := range pendingRows {
		payer := stringValue(transaction.PayerName)
		if strings.TrimSpace(payer) == "" {
			payer = "付款人待识别"
		}
		subtitle := firstNonEmpty(strings.TrimSpace(transaction.Description), strings.TrimSpace(transaction.Reference), "银行收款需要核对")
		if transaction.TransactionTime != nil {
			subtitle = transaction.TransactionTime.In(time.UTC).Format("01-02") + " · " + subtitle
		}
		page.PendingItems = append(page.PendingItems, rentWorkspacePendingItem{
			Title:     payer,
			Subtitle:  subtitle,
			Amount:    formatMoney(centsToMoney(transaction.AmountCents), firstNonEmpty(transaction.Currency, ledgerCurrencyEUR), 2),
			DetailURL: rentWorkspaceTransactionDetailURL(transaction.ID, filters.PeriodMonth),
			ListURL:   "/transactions?period=" + url.QueryEscape(filters.PeriodMonth.Format("2006-01")) + "&match_status=pending",
		})
	}
	if filterErr != nil {
		page.Error = "invalid_workspace_filter"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := rentWorkspaceTemplate.Execute(w, page); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

type rentRoomDetailPageData struct {
	workspaceShell
	Filters           rentWorkspaceFilters
	ReturnURL         string
	FromList          bool
	ReturnPropertyID  uint64
	ReturnStatus      string
	ReturnSearch      string
	ReturnCollection  string
	Period            string
	PeriodLabel       string
	RoomID            uint64
	RoomLabel         string
	RoomType          string
	Capacity          int
	RoomNotes         string
	ContractDate      string
	MoveInDate        string
	MonthlyRent       string
	MonthlyRentValue  string
	DueDay            int
	RoomActiveFrom    string
	PropertyName      string
	PropertyAddress   string
	Editing           bool
	Form              roomPageForm
	Properties        []propertyPageRow
	Summary           rentWorkspaceRoomRow
	PaymentCount      int
	UnallocatedAmount string
	Tenants           []rentWorkspaceTenantRow
	Expenses          []rentWorkspaceExpenseView
	Message           string
	Error             string
}

type rentWorkspaceExpenseView struct {
	Description string
	Category    string
	ExpenseDate string
	Amount      string
}

func (s *rentWorkspaceService) loadRoomDetail(ctx context.Context, userID, roomID uint64, period time.Time) (rentRoomDetailPageData, error) {
	if userID == 0 || roomID == 0 {
		return rentRoomDetailPageData{}, errors.New("userID and roomID are required")
	}
	var roomRow room
	if err := s.db.WithContext(ctx).Where("user_id = ? AND id = ?", userID, roomID).First(&roomRow).Error; err != nil {
		return rentRoomDetailPageData{}, err
	}
	var propertyRow property
	if err := s.db.WithContext(ctx).Where("user_id = ? AND id = ?", userID, roomRow.PropertyID).First(&propertyRow).Error; err != nil {
		return rentRoomDetailPageData{}, err
	}
	period = monthStart(period)
	var activeAgreement tenancyAgreement
	agreementQuery := s.db.WithContext(ctx).Where("user_id = ? AND room_id = ? AND status = ? AND start_date <= ? AND (end_date IS NULL OR end_date >= ?)", userID, roomID, "active", period.AddDate(0, 1, 0).Add(-time.Nanosecond), period)
	agreementErr := agreementQuery.Order("start_date DESC, id DESC").First(&activeAgreement).Error
	if agreementErr != nil && !errors.Is(agreementErr, gorm.ErrRecordNotFound) {
		return rentRoomDetailPageData{}, agreementErr
	}
	contractDate, moveInDate, monthlyRent, monthlyRentValue, dueDay := "未录入", "未录入", "—", "", roomRow.DueDay
	if dueDay < 1 || dueDay > 31 {
		dueDay = 1
	}
	if roomRow.MonthlyRentCents > 0 {
		monthlyRent = pageCurrencyAmount(roomRow.MonthlyRentCents, ledgerCurrencyEUR)
		monthlyRentValue = strconv.FormatFloat(float64(roomRow.MonthlyRentCents)/100, 'f', 2, 64)
	}
	if agreementErr == nil {
		if activeAgreement.ContractDate != nil {
			contractDate = activeAgreement.ContractDate.Format(dateLayout)
		}
		if activeAgreement.MoveInDate != nil {
			moveInDate = activeAgreement.MoveInDate.Format(dateLayout)
		}
		monthlyRent = pageCurrencyAmount(activeAgreement.MonthlyRentCents, activeAgreement.Currency)
		monthlyRentValue = strconv.FormatFloat(float64(activeAgreement.MonthlyRentCents)/100, 'f', 2, 64)
		dueDay = activeAgreement.DueDay
	}
	filters := defaultRentWorkspaceFilters(period)
	filters.View = rentWorkspaceViewRooms
	filters.PropertyID = propertyRow.ID
	filters.RoomID = roomRow.ID
	data, err := s.load(ctx, userID, filters)
	if err != nil {
		return rentRoomDetailPageData{}, err
	}
	var summary rentWorkspaceRoomRow
	if len(data.RoomRows) > 0 {
		summary = data.RoomRows[0]
	} else {
		summary = rentWorkspaceRoomRow{RoomID: roomRow.ID, PropertyID: propertyRow.ID, PropertyName: propertyRow.Name, RoomLabel: roomRow.RoomLabel, Address: propertyAddress(propertyRow), Status: "vacant", StatusLabel: workspaceStatusLabel("vacant"), ExpectedAmount: "—", PaidAmount: "—", BalanceAmount: "—", DueDate: "—"}
	}
	var expenses []manualExpense
	if err := s.db.WithContext(ctx).Where("user_id = ? AND room_id = ? AND expense_date >= ? AND expense_date < ? AND record_status = ?", userID, roomID, period, period.AddDate(0, 1, 0), obligationRecordActive).Order("expense_date DESC, id DESC").Find(&expenses).Error; err != nil {
		return rentRoomDetailPageData{}, err
	}
	expenseViews := make([]rentWorkspaceExpenseView, len(expenses))
	for index, expense := range expenses {
		expenseViews[index] = rentWorkspaceExpenseView{
			Description: expense.Description,
			Category:    expense.Category,
			ExpenseDate: expense.ExpenseDate.Format("2006-01-02"),
			Amount:      formatMoney(centsToMoney(expense.AmountCents), firstNonEmpty(expense.Currency, ledgerCurrencyEUR), 2),
		}
	}
	paymentIDs := make(map[uint64]struct{})
	obligationIDs := make([]uint64, 0, len(data.TenantRows))
	for _, tenantRow := range data.TenantRows {
		if tenantRow.ObligationID != 0 {
			obligationIDs = append(obligationIDs, tenantRow.ObligationID)
		}
		for _, payment := range tenantRow.Payments {
			if payment.PaymentID != 0 {
				paymentIDs[payment.PaymentID] = struct{}{}
			}
		}
	}
	// "未分配" is money received into this room that no responsibility has claimed,
	// which is a different quantity from "未覆盖" (responsibility no payment covered).
	// It is derived from the source transactions: amount minus effective allocations.
	unallocatedCents, err := s.roomUnallocatedCents(ctx, userID, obligationIDs)
	if err != nil {
		return rentRoomDetailPageData{}, err
	}
	return rentRoomDetailPageData{
		Filters:           filters,
		Period:            period.Format("2006-01"),
		PeriodLabel:       formatMonthLabel(period),
		RoomID:            roomRow.ID,
		RoomLabel:         roomRow.RoomLabel,
		RoomType:          firstNonEmpty(roomRow.RoomType, "未设置"),
		Capacity:          roomRow.Capacity,
		RoomNotes:         stringValue(roomRow.Notes),
		ContractDate:      contractDate,
		MoveInDate:        moveInDate,
		MonthlyRent:       monthlyRent,
		MonthlyRentValue:  monthlyRentValue,
		DueDay:            dueDay,
		RoomActiveFrom:    roomRow.ActiveFrom.Format("2006-01"),
		PropertyName:      propertyRow.Name,
		PropertyAddress:   propertyAddress(propertyRow),
		Summary:           summary,
		PaymentCount:      len(paymentIDs),
		UnallocatedAmount: formatWorkspaceAmount(unallocatedCents),
		Tenants:           data.TenantRows,
		Expenses:          expenseViews,
	}, nil
}

// roomUnallocatedCents sums, over every income transaction that paid into this
// room, the part of the transaction that no effective allocation claimed. A cash
// receipt is bound to a single obligation at creation, so it never contributes.
func (s *rentWorkspaceService) roomUnallocatedCents(ctx context.Context, userID uint64, obligationIDs []uint64) (int64, error) {
	if userID == 0 || len(obligationIDs) == 0 {
		return 0, nil
	}
	transactionIDs := make([]uint64, 0)
	if err := s.db.WithContext(ctx).Model(&paymentAllocation{}).
		Where("user_id = ? AND rent_obligation_id IN ?", userID, obligationIDs).
		Distinct().Pluck("payment_transaction_id", &transactionIDs).Error; err != nil {
		return 0, err
	}
	if len(transactionIDs) == 0 {
		return 0, nil
	}
	var transactions []paymentTransaction
	if err := s.db.WithContext(ctx).Where("user_id = ? AND id IN ? AND direction = ?", userID, transactionIDs, "income").Find(&transactions).Error; err != nil {
		return 0, err
	}
	var allocations []paymentAllocation
	if err := s.db.WithContext(ctx).Where("user_id = ? AND payment_transaction_id IN ?", userID, transactionIDs).Find(&allocations).Error; err != nil {
		return 0, err
	}
	allocatedByTransaction := make(map[uint64]int64, len(allocations))
	for _, allocation := range allocations {
		if !ledgerAllocationIsEffective(allocation) || allocation.AmountCents <= 0 {
			continue
		}
		allocatedByTransaction[allocation.PaymentTransactionID] += allocation.AmountCents
	}
	var total int64
	for _, transaction := range transactions {
		if remaining := transaction.AmountCents - allocatedByTransaction[transaction.ID]; remaining > 0 {
			total += remaining
		}
	}
	return total, nil
}

func (a *app) handleRoomDetail(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/rooms/")
	if path == "" || strings.Contains(path, "/") {
		http.Error(w, "room path is invalid", http.StatusBadRequest)
		return
	}
	roomID, err := strconv.ParseUint(path, 10, 64)
	if err != nil || roomID == 0 {
		http.Error(w, "room path is invalid", http.StatusBadRequest)
		return
	}
	period, err := parsePeriodMonth(strings.TrimSpace(r.URL.Query().Get("period")))
	if err != nil {
		http.Error(w, "period is invalid", http.StatusBadRequest)
		return
	}
	userID, ok := a.currentUserID(r)
	if !ok || a.db == nil {
		http.Error(w, "database session required", http.StatusServiceUnavailable)
		return
	}
	data, err := newRentWorkspaceService(a.db).loadRoomDetail(r.Context(), userID, roomID, period)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			http.Error(w, "room not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data.workspaceShell = a.fillWorkspaceShell(r, workspaceShell{ActivePage: "rooms", Username: a.displayUsername(r), Environment: a.cfg.Environment, FootNote: "房间详情", CompactTitle: data.PropertyName + " · 房间 " + data.RoomLabel, ShowNavCounts: true})
	data.Message, data.Error = r.URL.Query().Get("message"), roomMutationErrorMessage(r.URL.Query().Get("error"))
	data.Editing = r.URL.Query().Get("edit") == "1"
	data.ReturnURL = rentWorkspaceURL(data.Filters, 1)
	if r.URL.Query().Get("from") == "rooms" {
		data.FromList = true
		data.ReturnPropertyID, _ = parseOptionalUint(r.URL.Query().Get("return_property_id"))
		data.ReturnStatus = strings.TrimSpace(r.URL.Query().Get("return_status"))
		if data.ReturnStatus != "active" && data.ReturnStatus != "inactive" {
			data.ReturnStatus = "all"
		}
		data.ReturnSearch = strings.TrimSpace(r.URL.Query().Get("return_search"))
		data.ReturnCollection = strings.TrimSpace(r.URL.Query().Get("return_collection"))
		if data.ReturnCollection != "unpaid" && data.ReturnCollection != "paid" {
			data.ReturnCollection = "all"
		}
		data.ReturnURL = roomListURL(data.Period, data.ReturnPropertyID, data.ReturnStatus, data.ReturnSearch, data.ReturnCollection)
	}
	data.Form = roomPageForm{ID: data.RoomID, PropertyID: data.Summary.PropertyID, RoomLabel: data.RoomLabel, RoomType: data.RoomType, Capacity: data.Capacity, MonthlyRentValue: data.MonthlyRentValue, DueDay: data.DueDay, Notes: data.RoomNotes, ActiveFrom: data.RoomActiveFrom}
	if data.Editing {
		properties, listErr := newLandlordRentRepository(a.db).listProperties(r.Context(), userID, propertyQuery{})
		if listErr != nil {
			http.Error(w, listErr.Error(), http.StatusInternalServerError)
			return
		}
		data.Properties = make([]propertyPageRow, 0, len(properties))
		for _, propertyRow := range properties {
			data.Properties = append(data.Properties, propertyPageRow{ID: propertyRow.ID, Name: propertyRow.Name, Address: propertyAddress(propertyRow), Status: propertyRow.Status, StatusLabel: pageStatusLabel(propertyRow.Status)})
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := rentRoomDetailTemplate.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

var rentWorkspaceTemplateFuncs = template.FuncMap{
	"workspaceURL": func(filters rentWorkspaceFilters, page int) string {
		return rentWorkspaceURL(filters, page)
	},
	"workspaceViewURL": func(filters rentWorkspaceFilters, view string, propertyID, roomID uint64) string {
		filters.View = view
		filters.PropertyID = propertyID
		filters.RoomID = roomID
		filters.Page = 1
		return rentWorkspaceURL(filters, 1)
	},
	"workspacePeriodURL": func(filters rentWorkspaceFilters, period string) string {
		parsed, err := parsePeriodMonth(period)
		if err != nil {
			return rentWorkspaceURL(filters, 1)
		}
		filters.PeriodMonth = parsed
		filters.Page = 1
		return rentWorkspaceURL(filters, 1)
	},
	"roomDetailURL": func(roomID uint64, period string) string {
		return "/rooms/" + strconv.FormatUint(roomID, 10) + "?period=" + url.QueryEscape(period)
	},
	"propertyDetailURL": func(propertyID uint64, period string) string {
		return "/properties/" + strconv.FormatUint(propertyID, 10) + "?period=" + url.QueryEscape(period)
	},
	"workspacePropertyURL": func(filters rentWorkspaceFilters, propertyID uint64) string {
		filters.View = rentWorkspaceViewRooms
		filters.PropertyID = propertyID
		filters.RoomID = 0
		filters.Page = 1
		return rentWorkspaceURL(filters, 1)
	},
	"workspaceStatusClass": func(status string) string {
		return firstNonEmpty(status, "needs_review")
	},
	"workspacePreviousPage": func(page int) int {
		if page <= 1 {
			return 1
		}
		return page - 1
	},
	"workspaceNextPage": func(page, totalPages int) int {
		if totalPages <= 0 || page >= totalPages {
			return page
		}
		return page + 1
	},
}

func rentWorkspaceTransactionDetailURL(transactionID uint64, period time.Time) string {
	values := url.Values{}
	values.Set("detail", strconv.FormatUint(transactionID, 10))
	values.Set("period", monthStart(period).Format("2006-01"))
	return "/transactions?" + values.Encode()
}

var rentWorkspaceTemplate = newEmbeddedWorkspacePageTemplate(
	"rent-workspace",
	rentWorkspaceTemplateFuncs,
	"web/templates/pages/rent-workspace.html",
)

var rentRoomDetailTemplateFuncs = template.FuncMap{
	"workspaceURL": func(filters rentWorkspaceFilters) string {
		return rentWorkspaceURL(filters, 1)
	},
	"workspaceStatusClass": func(status string) string {
		return firstNonEmpty(status, "needs_review")
	},
}

var rentRoomDetailTemplate = newEmbeddedWorkspacePageTemplate(
	"rent-room-detail",
	rentRoomDetailTemplateFuncs,
	"web/templates/pages/room-detail.html",
)
