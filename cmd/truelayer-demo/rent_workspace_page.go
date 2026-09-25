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
	IsFuturePeriod   bool
	Period           string
	PeriodLabel      string
	PreviousPeriod   string
	NextPeriod       string
	View             string
	Summary          rentWorkspaceSummary
	DimensionSummary rentWorkspaceDimensionSummary
	PendingCount     int
	PendingItems     []rentWorkspacePendingItem
	MatchReview      *transactionMatchReviewData
	ReceiptFinder    *tenantReceiptFinderData
	PropertyOptions  []rentWorkspacePropertyOption
	PropertyRows     []rentWorkspacePropertyRow
	PropertyTreeRows []rentWorkspacePropertyTreeRow
	RoomRows         []rentWorkspaceRoomRow
	RoomTreeRows     []rentWorkspaceRoomTreeRow
	TenantRows       []rentWorkspaceTenantRow
	SortLinks        map[string]tableSortLink
	TotalRows        int
	FilteredCount    int
	TotalPages       int
	Pagination       []paginationLink
	Page             int
	PageSize         int
	Error            string
	Message          string
}

type rentWorkspacePendingItem struct {
	// Index is the 01/02/03 badge the prototype puts in front of every queued
	// row. It is numbered at the source because Go templates cannot add, and a
	// template func for one badge would be a poor trade.
	Index    int
	ID       uint64
	Title    string
	Subtitle string
	Amount   string
	MatchURL string
}

// rentWorkspacePendingItems maps the queued transactions to the panel's rows and
// numbers them 01/02/03 for the prototype's index badge. It stays a plain
// function (no *app, no request) so the numbering is unit-testable without a
// database session.
func rentWorkspacePendingItems(rows []paymentTransaction, periodMonth time.Time) []rentWorkspacePendingItem {
	items := make([]rentWorkspacePendingItem, 0, len(rows))
	for i, transaction := range rows {
		payer := stringValue(transaction.PayerName)
		if strings.TrimSpace(payer) == "" {
			payer = "付款人待识别"
		}
		subtitle := firstNonEmpty(strings.TrimSpace(transaction.Description), strings.TrimSpace(transaction.Reference), "银行收款需要核对")
		if transaction.TransactionTime != nil {
			subtitle = transaction.TransactionTime.In(time.UTC).Format("01-02") + " · " + subtitle
		}
		items = append(items, rentWorkspacePendingItem{
			Index:    i + 1,
			ID:       transaction.ID,
			Title:    payer,
			Subtitle: subtitle,
			Amount:   formatMoney(centsToMoney(transaction.AmountCents), firstNonEmpty(transaction.Currency, ledgerCurrencyEUR), 2),
			MatchURL: rentWorkspaceURL(defaultRentWorkspaceFilters(periodMonth), 1) + "&match=" + strconv.FormatUint(transaction.ID, 10),
		})
	}
	return items
}

func rentWorkspacePendingTransactions(db *gorm.DB, ctx context.Context, userID uint64, period time.Time) *gorm.DB {
	start := monthStart(period)
	return rentWorkspacePendingIncomeQuery(db, ctx, userID).
		Where("payment_transactions.transaction_time >= ? AND payment_transactions.transaction_time < ?", start, start.AddDate(0, 1, 0))
}

func rentWorkspacePendingIncomeQuery(db *gorm.DB, ctx context.Context, userID uint64) *gorm.DB {
	return db.WithContext(ctx).Model(&paymentTransaction{}).
		Where("payment_transactions.user_id = ? AND payment_transactions.direction = ? AND payment_transactions.match_status IN ?", userID, "income", pendingMatchStatuses).
		Where(`NOT EXISTS (
			SELECT 1 FROM payment_transaction_actions AS defer_action
			WHERE defer_action.user_id = payment_transactions.user_id
			  AND defer_action.payment_transaction_id = payment_transactions.id
			  AND defer_action.action_kind = ?
			  AND defer_action.id > COALESCE((
				SELECT MAX(undefer_action.id) FROM payment_transaction_actions AS undefer_action
				WHERE undefer_action.user_id = payment_transactions.user_id
				  AND undefer_action.payment_transaction_id = payment_transactions.id
				  AND undefer_action.action_kind = ?
			  ), 0)
		)`, transactionActionDefer, transactionActionUndefer)
}

func rentWorkspacePageFromData(a *app, r *http.Request, data rentWorkspaceData, pageError string) rentWorkspacePageData {
	period := monthStart(data.Filters.PeriodMonth)
	pageURL := func(page int) string {
		if r.URL.Path == "/bills" {
			return billsPageURL(period.Format("2006-01"), data.Filters.Search, data.Filters.Status, data.Filters.Sort, page, data.Filters.PageSize)
		}
		return rentWorkspaceURL(data.Filters, page)
	}
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
			CompactTitle:  "所选月份收租",
			ShowNavCounts: true,
			TenantCount:   len(data.TenantRows),
		}),
		Filters:          data.Filters,
		IsFuturePeriod:   data.IsFuturePeriod,
		Period:           period.Format("2006-01"),
		PeriodLabel:      formatMonthLabel(period),
		PreviousPeriod:   period.AddDate(0, -1, 0).Format("2006-01"),
		NextPeriod:       period.AddDate(0, 1, 0).Format("2006-01"),
		View:             data.Filters.View,
		Summary:          data.Summary,
		DimensionSummary: data.DimensionSummary,
		PropertyOptions:  data.PropertyOptions,
		PropertyRows:     data.PropertyRows,
		PropertyTreeRows: data.PropertyTreeRows,
		RoomRows:         data.RoomRows,
		RoomTreeRows:     data.RoomTreeRows,
		TenantRows:       data.TenantRows,
		SortLinks:        rentWorkspaceSortLinks(data.Filters),
		TotalRows:        data.TotalRows,
		FilteredCount:    data.FilteredCount,
		TotalPages:       data.TotalPages,
		Pagination:       paginationLinks(data.Page, data.TotalPages, pageURL),
		Page:             data.Page,
		PageSize:         data.PageSize,
		Error:            pageError,
	}
}

func rentWorkspaceSortLinks(filters rentWorkspaceFilters) map[string]tableSortLink {
	activeSort := normalisedSort(filters.Sort, dashboardDefaultSort)
	sortURL := func(sortValue string) string {
		next := filters
		next.Sort = sortValue
		return rentWorkspaceURL(next, 1)
	}
	return map[string]tableSortLink{
		"property":     sortLinkFor(sortURL, activeSort, "name_asc", "name_desc"),
		"rooms":        sortLinkFor(sortURL, activeSort, "rooms_asc", "rooms_desc"),
		"paid_rooms":   sortLinkFor(sortURL, activeSort, "paid_rooms_asc", "paid_rooms_desc"),
		"unpaid_rooms": sortLinkFor(sortURL, activeSort, "unpaid_rooms_asc", "unpaid_rooms_desc"),
		"tenant_count": sortLinkFor(sortURL, activeSort, "tenant_count_asc", "tenant_count_desc"),
		"tenant":       sortLinkFor(sortURL, activeSort, "name_asc", "name_desc"),
		"room":         sortLinkFor(sortURL, activeSort, "room_asc", "room_desc"),
		"expected":     sortLinkFor(sortURL, activeSort, "expected_desc", "expected_asc"),
		"paid":         sortLinkFor(sortURL, activeSort, "paid_desc", "paid_asc"),
		"balance":      sortLinkFor(sortURL, activeSort, "balance_desc", "balance_asc"),
		"expense":      sortLinkFor(sortURL, activeSort, "expense_desc", "expense_asc"),
		"net":          sortLinkFor(sortURL, activeSort, "net_desc", "net_asc"),
		"rate":         sortLinkFor(sortURL, activeSort, "rate_desc", "rate_asc"),
		"due":          sortLinkFor(sortURL, activeSort, "due_asc", "due_desc"),
		"source":       sortLinkFor(sortURL, activeSort, "source_asc", "source_desc"),
		"status":       sortLinkFor(sortURL, activeSort, "status_asc", "status_desc"),
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
	page := rentWorkspacePageFromData(a, r, data, "")
	page.Message = r.URL.Query().Get("message")
	page.Error = r.URL.Query().Get("error")
	var pendingTransactions int64
	if err := rentWorkspacePendingTransactions(a.db, r.Context(), userID, filters.PeriodMonth).Count(&pendingTransactions).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	page.PendingCount = int(pendingTransactions)
	var pendingRows []paymentTransaction
	if err := rentWorkspacePendingTransactions(a.db, r.Context(), userID, filters.PeriodMonth).
		Order("payment_transactions.transaction_time ASC, payment_transactions.id ASC").Limit(3).Find(&pendingRows).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	page.PendingItems = rentWorkspacePendingItems(pendingRows, filters.PeriodMonth)
	for index, transaction := range pendingRows {
		page.PendingItems[index].MatchURL = rentWorkspaceURL(filters, filters.Page) + "&match=" + strconv.FormatUint(transaction.ID, 10)
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("find_tenant")); raw != "" {
		findTenantID, parseErr := parsePositiveUint(raw)
		if parseErr != nil {
			http.NotFound(w, r)
			return
		}
		finder, finderErr := loadTenantReceiptFinder(r.Context(), a.db, userID, findTenantID, filters, r.URL.Query(), time.Now())
		if errors.Is(finderErr, gorm.ErrRecordNotFound) || errors.Is(finderErr, errInvalidTenantReceiptFinder) {
			http.NotFound(w, r)
			return
		}
		if finderErr != nil {
			http.Error(w, finderErr.Error(), http.StatusInternalServerError)
			return
		}
		page.ReceiptFinder = &finder
	}
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
		origin := r.URL.Query().Get("match_origin")
		review, reviewErr := newTransactionService(a.db).transactionMatchReviewForMonthWithOrigin(r.Context(), userID, matchID, tenantID, historyPage, strings.TrimSpace(r.URL.Query().Get("match_month")), origin != "roommate" && origin != "lookup")
		if errors.Is(reviewErr, gorm.ErrRecordNotFound) {
			http.NotFound(w, r)
			return
		}
		if reviewErr != nil {
			http.Error(w, reviewErr.Error(), http.StatusInternalServerError)
			return
		}
		review.setWorkspaceURLs(filters, r.URL.Query())
		if page.ReceiptFinder != nil {
			review.FinderMode = true
			review.FinderBackURL = page.ReceiptFinder.SearchURL
			review.FinderCloseURL = page.ReceiptFinder.CloseURL
			review.FinderTenantName = page.ReceiptFinder.TenantName
			review.FinderPeriod = page.Period
			for _, month := range review.Months {
				if month.Period == page.Period && month.Selectable && review.SourceRemainingCents > 0 {
					review.FinderCanPrefill = true
					break
				}
			}
			review.ReturnURL = page.ReceiptFinder.SearchURL
			review.CloseURL = page.ReceiptFinder.SearchURL
		}
		page.MatchReview = &review
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
	ReturnSort        string
	Period            string
	PeriodLabel       string
	RoomID            uint64
	IsDeleted         bool
	RoomLabel         string
	RoomType          string
	Capacity          int
	RoomNotes         string
	MonthlyRent       string
	MonthlyRentValue  string
	DueDay            int
	PlanEditor        bool
	PlanExists        bool
	PlanVersion       uint64
	PlanEffectiveFrom string
	PlanEffectiveTo   string
	PlanRentValue     string
	PlanDueDay        int
	PlanMembers       []roomRentPlanMemberForm
	PlanTenants       []roomRentPlanTenantOption
	PlanSplitEvenly   bool
	VacantFromMonth   string
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
	ExpenseDrawer     *expenseDrawerData
	TenantDrawer      *tenantPageData
	Message           string
	Error             string
	PlanError         string
}

type roomRentPlanTenantOption struct {
	ID               uint64
	Name             string
	Status           string
	OccupanciesJSON  string
	ConflictRoomID   uint64
	ConflictRoomName string
	UnbindURL        string
}

type roomRentPlanMemberForm struct {
	TenantID            uint64
	TenantName          string
	ResponsibilityValue string
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
	var activePlan roomRentPlan
	planQuery := s.db.WithContext(ctx).Where("user_id = ? AND room_id = ? AND effective_from_month <= ? AND (effective_to_month IS NULL OR effective_to_month >= ?)", userID, roomID, period, period)
	planErr := planQuery.Order("effective_from_month DESC, id DESC").First(&activePlan).Error
	if planErr != nil && !errors.Is(planErr, gorm.ErrRecordNotFound) {
		return rentRoomDetailPageData{}, planErr
	}
	monthlyRent, monthlyRentValue, dueDay := "—", "", 1
	planMembers := []roomRentPlanMemberForm{}
	planExists := planErr == nil
	planEffectiveFrom, planEffectiveTo, planRentValue := "", "", ""
	planDueDay := 1
	if planErr == nil {
		monthlyRent = pageCurrencyAmount(activePlan.MonthlyRentCents, activePlan.Currency)
		monthlyRentValue = strconv.FormatFloat(float64(activePlan.MonthlyRentCents)/100, 'f', 2, 64)
		dueDay = activePlan.DueDay
		planEffectiveFrom = activePlan.EffectiveFromMonth.Format("2006-01")
		if activePlan.EffectiveToMonth != nil {
			planEffectiveTo = activePlan.EffectiveToMonth.Format("2006-01")
		}
		planRentValue = monthlyRentValue
		planDueDay = activePlan.DueDay
		members, membersErr := newLandlordRentRepository(s.db).listRoomRentPlanMembers(ctx, userID, roomRentPlanMemberQuery{RoomRentPlanID: activePlan.ID})
		if membersErr != nil {
			return rentRoomDetailPageData{}, membersErr
		}
		planMembers = make([]roomRentPlanMemberForm, 0, len(members))
		for _, member := range members {
			planMembers = append(planMembers, roomRentPlanMemberForm{
				TenantID:            member.TenantID,
				ResponsibilityValue: strconv.FormatFloat(float64(member.ResponsibilityCents)/100, 'f', 2, 64),
			})
		}
	}
	tenantOptions, err := loadRoomTenantOptions(ctx, s.db, userID, roomID, period)
	if err != nil {
		return rentRoomDetailPageData{}, err
	}
	tenantNameByID := make(map[uint64]string, len(tenantOptions))
	for _, option := range tenantOptions {
		tenantNameByID[option.ID] = option.Name
	}
	for i := range planMembers {
		planMembers[i].TenantName = tenantNameByID[planMembers[i].TenantID]
	}
	filters := defaultRentWorkspaceFilters(period)
	filters.View = rentWorkspaceViewRooms
	filters.PropertyID = propertyRow.ID
	filters.RoomID = roomRow.ID
	data, err := s.loadWithAssetHistory(ctx, userID, filters, roomRow.DeletedAt != nil || propertyRow.DeletedAt != nil)
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
		IsDeleted:         roomRow.DeletedAt != nil || propertyRow.DeletedAt != nil,
		RoomLabel:         roomRow.RoomLabel,
		RoomType:          firstNonEmpty(roomRow.RoomType, "未设置"),
		Capacity:          roomRow.Capacity,
		RoomNotes:         stringValue(roomRow.Notes),
		MonthlyRent:       monthlyRent,
		MonthlyRentValue:  monthlyRentValue,
		DueDay:            dueDay,
		PlanExists:        planExists,
		PlanVersion:       roomRow.RentPlanVersion,
		PlanEffectiveFrom: planEffectiveFrom,
		PlanEffectiveTo:   planEffectiveTo,
		PlanRentValue:     planRentValue,
		PlanDueDay:        planDueDay,
		PlanMembers:       planMembers,
		PlanTenants:       tenantOptions,
		PlanSplitEvenly:   !planExists,
		VacantFromMonth:   period.Format("2006-01"),
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
	tenantAdd := r.URL.Query().Get("tenant_add") == "1"
	data.Editing = !data.IsDeleted && !tenantAdd && r.URL.Query().Get("edit") == "1"
	data.PlanEditor = !data.IsDeleted && !tenantAdd && r.URL.Query().Get("rent") == "1"
	data.PlanError = rentPlanErrorMessage(r.URL.Query().Get("rent_error"))
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
		data.ReturnSort = strings.TrimSpace(r.URL.Query().Get("return_sort"))
		if !validRoomPageSort(data.ReturnSort) {
			data.ReturnSort = ""
		}
		data.ReturnURL = roomListURL(data.Period, data.ReturnPropertyID, data.ReturnStatus, data.ReturnSearch, data.ReturnCollection, data.ReturnSort)
	}
	data.Form = roomPageForm{ID: data.RoomID, PropertyID: data.Summary.PropertyID, RoomLabel: data.RoomLabel, RoomType: data.RoomType, Capacity: data.Capacity, Notes: data.RoomNotes}
	if tenantAdd {
		properties, rooms, loadErr := a.loadTenantRoomAssignmentOptions(r.Context(), userID)
		if loadErr != nil {
			http.Error(w, loadErr.Error(), http.StatusInternalServerError)
			return
		}
		found := false
		for _, room := range rooms {
			if room.ID == roomID && room.PropertyID == data.Summary.PropertyID {
				found = true
				break
			}
		}
		if !found {
			http.NotFound(w, r)
			return
		}
		roomURL := "/rooms/" + strconv.FormatUint(roomID, 10) + "?period=" + url.QueryEscape(data.Period)
		data.TenantDrawer = &tenantPageData{
			Form: tenantRecord{Status: "active"}, ReturnURL: roomURL + "&rent=1",
			PostReturnURL: roomURL + "&tenant_add=1", Error: r.URL.Query().Get("error"),
			ErrorMessage:     tenantFormErrorMessage(r.URL.Query().Get("error")),
			TenantProperties: properties, TenantRooms: rooms,
			TenantAssignmentPropertyID: data.Summary.PropertyID, TenantAssignmentRoomID: roomID,
			TenantArrangementStartMonth: data.Period, TenantRoomLocked: true,
		}
		data.Error = ""
	}
	if !data.IsDeleted && !tenantAdd && (r.URL.Query().Get("expense") == "1" || isExpenseFormError(r.URL.Query().Get("error"))) {
		returnURL := expenseFormReturnURL(r)
		expenseDrawer, drawerErr := a.loadExpenseDrawerData(r.Context(), userID, data.Period, data.Summary.PropertyID, data.RoomID, returnURL, r.URL.Query().Get("error"))
		if drawerErr != nil {
			http.Error(w, drawerErr.Error(), http.StatusInternalServerError)
			return
		}
		data.ExpenseDrawer = expenseDrawer
	}
	if data.Editing {
		properties, listErr := newLandlordRentRepository(a.db).listProperties(r.Context(), userID, propertyQuery{})
		if listErr != nil {
			http.Error(w, listErr.Error(), http.StatusInternalServerError)
			return
		}
		data.Properties = make([]propertyPageRow, 0, len(properties))
		for _, propertyRow := range properties {
			data.Properties = append(data.Properties, propertyPageRow{ID: propertyRow.ID, Name: propertyRow.Name, Address: propertyAddress(propertyRow), Status: propertyRow.Status, StatusLabel: assetStatusLabel(propertyRow.Status)})
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := rentRoomDetailTemplate.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

var rentWorkspaceTemplateFuncs = template.FuncMap{
	"tenantFinderURL": func(filters rentWorkspaceFilters, tenantID uint64) string {
		return tenantReceiptFinderURL(rentWorkspaceURL(filters, filters.Page), tenantID, tenantReceiptFinderFilters{Scope: "two", Mode: "all", Field: "all", Page: 1}, 0)
	},
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
	"tenantDetailURL": func(tenantID uint64, period string) string {
		return "/tenants/" + strconv.FormatUint(tenantID, 10) + "?from_month=" + url.QueryEscape(period) + "&to_month=" + url.QueryEscape(period)
	},
	// The section note renders the ledger currency only. The prototype put
	// "演示数据 · EUR" there; the 演示数据 half is a prototype-only label
	// (DESIGN-HANDOFF.md:9) and is deliberately dropped (prd.md 已确认的决策).
	"workspaceCurrency": func() string {
		return ledgerCurrencyEUR
	},
	// The 收缴率 progress bar needs a real CSS width. Returning template.CSS from
	// Go keeps html/template's CSS sanitiser out of it and clamps the value so a
	// stray percent can never escape the track.
	"workspaceRateStyle": func(percent int) template.CSS {
		if percent < 0 {
			percent = 0
		}
		if percent > 100 {
			percent = 100
		}
		return template.CSS("width:" + strconv.Itoa(percent) + "%")
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
