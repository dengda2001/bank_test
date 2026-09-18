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
	Filters         rentWorkspaceFilters
	Period          string
	PeriodLabel     string
	PreviousPeriod  string
	NextPeriod      string
	View            string
	Summary         rentWorkspaceSummary
	PropertyOptions []rentWorkspacePropertyOption
	PropertyRows    []rentWorkspacePropertyRow
	RoomRows        []rentWorkspaceRoomRow
	TenantRows      []rentWorkspaceTenantRow
	TotalRows       int
	FilteredCount   int
	TotalPages      int
	Page            int
	PageSize        int
	Message         string
	Error           string
}

func rentWorkspacePageFromData(a *app, r *http.Request, data rentWorkspaceData, message, pageError string) rentWorkspacePageData {
	period := monthStart(data.Filters.PeriodMonth)
	return rentWorkspacePageData{
		workspaceShell: workspaceShell{
			ActivePage: func() string {
				if r.URL.Path == "/bills" {
					return "bills"
				}
				return "rent-dashboard"
			}(),
			Username:      a.displayUsername(r),
			Environment:   a.cfg.Environment,
			FootNote:      "月度收租工作台",
			ShowNavCounts: true,
			NavLabel:      "主导航",
			TenantCount:   len(data.TenantRows),
		},
		Filters:         data.Filters,
		Period:          period.Format("2006-01"),
		PeriodLabel:     formatMonthLabel(period),
		PreviousPeriod:  period.AddDate(0, -1, 0).Format("2006-01"),
		NextPeriod:      period.AddDate(0, 1, 0).Format("2006-01"),
		View:            data.Filters.View,
		Summary:         data.Summary,
		PropertyOptions: data.PropertyOptions,
		PropertyRows:    data.PropertyRows,
		RoomRows:        data.RoomRows,
		TenantRows:      data.TenantRows,
		TotalRows:       data.TotalRows,
		FilteredCount:   data.FilteredCount,
		TotalPages:      data.TotalPages,
		Page:            data.Page,
		PageSize:        data.PageSize,
		Message:         message,
		Error:           pageError,
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
	Filters         rentWorkspaceFilters
	Period          string
	PeriodLabel     string
	RoomID          uint64
	RoomLabel       string
	PropertyName    string
	PropertyAddress string
	Summary         rentWorkspaceRoomRow
	Tenants         []rentWorkspaceTenantRow
	Expenses        []rentWorkspaceExpenseView
	Error           string
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
	return rentRoomDetailPageData{
		Filters:         filters,
		Period:          period.Format("2006-01"),
		PeriodLabel:     formatMonthLabel(period),
		RoomID:          roomRow.ID,
		RoomLabel:       roomRow.RoomLabel,
		PropertyName:    propertyRow.Name,
		PropertyAddress: propertyAddress(propertyRow),
		Summary:         summary,
		Tenants:         data.TenantRows,
		Expenses:        expenseViews,
	}, nil
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
	data.workspaceShell = workspaceShell{ActivePage: "rent-dashboard", Username: a.displayUsername(r), Environment: a.cfg.Environment, FootNote: "房间详情", ShowNavCounts: true, NavLabel: "主导航"}
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
	"roomDetailURL": func(roomID uint64, period string) string {
		return "/rooms/" + strconv.FormatUint(roomID, 10) + "?period=" + url.QueryEscape(period)
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
