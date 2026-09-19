package main

import (
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"
)

func (a *app) handleRentDashboard(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	if _, ok := a.currentUserID(r); ok && a.db != nil {
		a.renderRentWorkspaceDashboard(w, r)
		return
	}
	// Without a database there is no dashboard read model left to fall back to:
	// the legacy JSON-backed template was removed. Fail loudly rather than hide a
	// missing database configuration behind a redirect. Mirrors handleDunningAction
	// (dunning_handlers.go) which guards the same configuration.
	http.Error(w, "rent dashboard requires database-backed user sessions", http.StatusServiceUnavailable)
}

func (a *app) renderRentDashboard(w http.ResponseWriter, r *http.Request, action *dunningDashboardAction) {
	period := r.URL.Query().Get("period")
	filters, filtersErr := rentDashboardFiltersFromQuery(r.URL.Query())
	if filtersErr != nil {
		filters = defaultRentDashboardFilters()
	}
	periodMonth, err := parsePeriodMonth(period)
	if err != nil {
		periodMonth = monthStart(time.Now().UTC())
	}
	if action != nil {
		periodMonth = monthStart(action.Period)
		filters = action.Filters
		filtersErr = nil
		period = periodMonth.Format("2006-01")
	}
	data := rentDashboardPageData{
		workspaceShell: workspaceShell{
			ActivePage: func() string {
				if r.URL.Path == "/bills" {
					return "bills"
				}
				if strings.HasPrefix(r.URL.Path, "/dunning") {
					return "dunning"
				}
				return "rent-dashboard"
			}(),
			Username:      a.displayUsername(r),
			Environment:   a.cfg.Environment,
			FootNote:      "月度收租工作台",
			CompactTitle:  "本月收租",
			ShowNavCounts: true,
			NavLabel:      "主导航",
		},
		PageKey: func() string {
			if r.URL.Path == "/bills" {
				return "bills"
			}
			if strings.HasPrefix(r.URL.Path, "/dunning") {
				return "dunning"
			}
			return "rent-dashboard"
		}(),
		CanonicalPath: func() string {
			if r.URL.Path == "/bills" {
				return "/bills"
			}
			if strings.HasPrefix(r.URL.Path, "/dunning") {
				return "/dunning"
			}
			return "/rent-dashboard"
		}(),
		Period:         periodMonth.Format("2006-01"),
		PeriodLabel:    fmt.Sprintf("%d年%d月", periodMonth.Year(), periodMonth.Month()),
		PreviousPeriod: periodMonth.AddDate(0, -1, 0).Format("2006-01"),
		NextPeriod:     periodMonth.AddDate(0, 1, 0).Format("2006-01"),
		SearchFilter:   filters.Search,
		StatusFilter:   filters.Status,
		SortFilter:     filters.Sort,
		Page:           filters.Page,
		PageSize:       filters.PageSize,
		Message:        r.URL.Query().Get("message"),
		Error:          r.URL.Query().Get("error"),
	}
	if filtersErr != nil {
		data.Error = "invalid_dashboard_filter"
	}
	if period != "" {
		if _, err := parsePeriodMonth(period); err != nil {
			data.Error = "invalid_period"
		}
	}

	// Sorting is driven by the list headings rather than a dropdown, so each
	// heading carries the link that applies its own sort. Columns with a single
	// meaningful order (房间, 已收, 未收) stay plain text.
	sortURL := func(sortValue string) string {
		return rentDashboardURL(data.Period, data.SearchFilter, data.StatusFilter, sortValue, 1, data.PageSize)
	}
	activeSort := normalisedSort(data.SortFilter, dashboardDefaultSort)
	data.TenantSort = sortLinkFor(sortURL, activeSort, "tenant_asc", "tenant_desc")
	data.DueSort = sortLinkFor(sortURL, activeSort, "due_asc", "due_desc")
	data.AmountSort = sortLinkFor(sortURL, activeSort, "amount_desc", "amount_asc")
	data.StatusSort = sortLinkFor(sortURL, activeSort, dashboardDefaultSort, "")

	if userID, ok := a.currentUserID(r); ok && a.db != nil {
		summary, err := newObligationService(a.db).summarizeRentDashboardWithFilters(r.Context(), userID, periodMonth, filters)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		data.Rows = summary.Rows
		data.FilteredCount = summary.FilteredCount
		data.TotalRows = summary.TotalRows
		data.TotalPages = summary.TotalPages
		data.Page = summary.Page
		data.PageSize = summary.PageSize
		currency := firstNonEmpty(summary.Currency, "EUR")
		data.ExpectedTotal = formatMoney(centsToMoney(summary.ExpectedCents), currency, 2)
		data.PaidTotal = formatMoney(centsToMoney(summary.PaidCents), currency, 2)
		data.BalanceTotal = formatMoney(centsToMoney(summary.BalanceCents), currency, 2)
		data.ExpenseTotal = formatMoney(centsToMoney(summary.ExpenseCents), currency, 2)
		data.OpenCount = summary.OpenCount
		data.OverdueCount = summary.OverdueCount
		data.UnpaidCount = summary.UnpaidCount
		data.PartialCount = summary.PartialCount
		data.PaidCount = summary.PaidCount
		data.ReviewCount = summary.ReviewCount
		data.TenantCount = summary.TenantCount
		data.IncomeCount = summary.IncomeCount
		data.ExpenseCount = summary.ExpenseCount
		data.PendingCount = summary.PendingCount
		data.PendingTotal = formatMoney(centsToMoney(summary.PendingCents), currency, 2)
		data.OtherIncomeTotal = formatMoney(centsToMoney(summary.OtherIncomeCents), currency, 2)
		data.OtherIncomeCount = summary.OtherIncomeCount
		data.SyncCoverage = summary.SyncCoverage
		data.SyncStatus = summary.SyncStatus
		data.LastSuccessfulSyncCoverage = summary.LastSuccessfulSyncCoverage
		if summary.ExpectedCents > 0 {
			data.CollectionPercent = int(summary.PaidCents * 100 / summary.ExpectedCents)
			if data.CollectionPercent > 100 {
				data.CollectionPercent = 100
			}
		}
		dunningView, err := a.dunningDrawerForDashboard(r.Context(), userID, periodMonth, filters, summary.Rows, action)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		data.Dunning = dunningView
	} else {
		tenants, _ := a.loadTenants()
		expenses, _ := a.loadExpenses()
		result, _ := a.loadLatestDemoResult()
		data.TenantCount = len(tenants)
		data.ExpenseCount = len(expenses)
		data.PendingCount = len(fallbackTransactionPageRows(result, transactionFilters{PeriodMonth: data.Period, PendingOnly: true}))
		data.PaidTotal = formatMoney(0, "EUR", 2)
		data.BalanceTotal = formatMoney(0, "EUR", 2)
		data.PendingTotal = formatMoney(0, "EUR", 2)
		data.OtherIncomeTotal = formatMoney(0, "EUR", 2)
		data.ExpenseTotal = formatMoney(sumExpenses(expenses), "EUR", 2)
		data.ExpectedTotal = formatMoney(sumTenantRent(tenants), "EUR", 2)
		data.PeriodLabel = fmt.Sprintf("%d年%d月", periodMonth.Year(), periodMonth.Month())
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// This function renders three live workspaces through one data contract, so the
	// template is chosen by request path. The legacy /rent-dashboard fallback
	// template is gone, so an unknown path is a programming error, not a request to
	// degrade: enumerate the live cases and fail loudly on anything else.
	var templateForPath *template.Template
	switch {
	case r.URL.Path == "/bills":
		templateForPath = billsPageTemplate
	case strings.HasPrefix(r.URL.Path, "/dunning"):
		templateForPath = dunningPageTemplate
	default:
		http.Error(w, "rent dashboard page is not available for this path", http.StatusInternalServerError)
		return
	}
	if err := templateForPath.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
