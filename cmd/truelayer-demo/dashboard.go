package main

import (
	"fmt"
	"html/template"
	"net/http"
	"time"
)

func (a *app) handleRentDashboard(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	if userID, ok := a.currentUserID(r); ok && a.db != nil {
		var propertyCount int64
		if err := a.db.WithContext(r.Context()).Model(&property{}).Where("user_id = ? AND status = ?", userID, "active").Count(&propertyCount).Error; err == nil && propertyCount > 0 {
			a.renderRentWorkspaceDashboard(w, r)
			return
		}
	}
	a.renderRentDashboard(w, r, nil)
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
			ActivePage:    "rent-dashboard",
			Username:      a.displayUsername(r),
			Environment:   a.cfg.Environment,
			FootNote:      "月度收租工作台",
			ShowNavCounts: true,
			NavLabel:      "主导航",
		},
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
	if err := rentDashboardTemplate.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

var rentDashboardTemplate = newWorkspacePageTemplate("rent-dashboard", template.FuncMap{
	"rentDashboardURL": func(period, search, status, sortValue string, page, pageSize int) string {
		return rentDashboardURL(period, search, status, sortValue, page, pageSize)
	},
	"dashboardPreviousPage": func(page int) int {
		if page <= 1 {
			return 1
		}
		return page - 1
	},
	"dashboardNextPage": func(page, totalPages int) int {
		if page >= totalPages {
			return totalPages
		}
		return page + 1
	},
	"dunningDeliveryLabel": func(status string) string {
		switch status {
		case dunningDeliveryAccepted:
			return "排队中"
		case dunningDeliverySent:
			return "已发送"
		case dunningDeliveryFailed:
			return "发送失败"
		case dunningDeliverySkipped:
			return "已跳过"
		default:
			return "未发送"
		}
	},
}, `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>RentOps Dashboard</title>
  <style>`+workspacePageCSS+workspaceCalendarCSS+`
	    /* ---- 顶部工具条：只保留月份选择 ---- */
	    .dashboard-toolbar { display: flex; align-items: center; justify-content: flex-end; gap: 12px; margin-bottom: 20px; }
	    .dashboard-toolbar .period-picker { display: flex; align-items: center; gap: 10px; margin: 0; }
	    .dashboard-toolbar .period-label { margin: 0; }
	    .dashboard-toolbar .period-field { display: block; flex: 0 0 auto; width: 200px; margin: 0; }
	    .dashboard-toolbar input[type="month"] { min-height: 42px; border-radius: 8px; padding: 9px 12px; background: var(--surface-muted); }
	    .dashboard-toolbar input[type="month"]:hover { border-color: var(--border-strong); background: var(--surface); }
	    .dashboard-toolbar input[type="month"]:focus { border-color: var(--accent); background: var(--surface); }
	    .month-nav { width: 42px; height: 42px; border: 1px solid var(--border); border-radius: 8px; display: grid; place-items: center; color: var(--foreground); background: var(--surface); text-decoration: none; font-size: 28px; line-height: 1; }
	    .month-nav:hover { background: var(--surface-muted); }
	    /* The month picker now sits against the right edge of the workspace, so the
	       calendar popover has to unfold leftwards or body{overflow-x:hidden} clips
	       it. Narrow viewports keep the calendar's own fixed bottom sheet. */
	    @media (min-width: 641px) {
	      .dashboard-toolbar .calendar-popover { left: auto; right: 0; transform-origin: top right; }
	    }

	    /* ---- 区块一：收租汇总 ---- */
	    .dashboard-section { margin-bottom: 22px; }
	    .section-head { display: flex; align-items: flex-end; justify-content: space-between; flex-wrap: wrap; gap: 10px 16px; margin-bottom: 12px; }
	    .section-head h2 { margin: 0; }
	    .section-head .tiny { margin-top: 4px; }
	    .section-actions { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; margin-left: auto; }
	    .dashboard-summary { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 12px; }
	    .dashboard-counts { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; margin-top: 12px; padding: 12px 16px; }
	    .count-chip { display: inline-flex; align-items: center; min-height: 32px; padding: 0 12px; border: 1px solid var(--border); border-radius: 999px; color: var(--foreground-subtle); background: var(--surface-muted); font-size: 12px; font-weight: 700; text-decoration: none; white-space: nowrap; }
	    .count-chip.overdue, .count-chip.review { border-color: #fecaca; color: #991b1b; background: #fef2f2; }
	    .count-chip.partial { border-color: #fde68a; color: #92400e; background: #fef3c7; }
	    .count-chip.shown { margin-left: auto; color: var(--foreground-muted); background: var(--surface); }
	    a.count-chip:hover { border-color: #93c5fd; color: var(--accent-bright); background: var(--surface-accent); }
	    .sync-status { margin-bottom: 18px; }
	    .collection-panel { padding: 18px; margin-top: 12px; }

	    /* ---- 区块二：租客账单列表 ---- */
	    .list-filter { display: flex; flex-wrap: wrap; align-items: flex-end; gap: 12px; padding: 16px 20px; border-bottom: 1px solid var(--border); background: var(--surface-muted); }
	    .list-filter label { margin: 0; }
	    .list-filter .search-field { flex: 1 1 240px; min-width: 220px; }
	    .list-filter .select-field { flex: 0 0 auto; width: 180px; }
	    .list-filter input, .list-filter select { min-height: 40px; background: var(--surface); }
	    .list-filter .filter-actions { display: flex; gap: 8px; margin-left: auto; }

	    /* Sorting is now done through the headings, so they have to read as controls. */
	    th .sort-link { display: inline-flex; align-items: center; gap: 6px; color: inherit; font: inherit; letter-spacing: inherit; text-decoration: none; white-space: nowrap; }
	    th .sort-link:hover { color: var(--accent-bright); }
	    th .sort-link.active { color: var(--accent-bright); font-weight: 800; }
	    th .sort-link .sort-arrow { font-size: 10px; line-height: 1; }

	    /* The page size belongs with the pager, not with the filters. */
	    .dashboard-pagination { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 12px; padding: 16px 20px 20px; }
	    .dashboard-pagination .page-size-field { display: flex; align-items: center; gap: 8px; margin: 0; }
	    .dashboard-pagination .page-size-field select { min-height: 40px; background: var(--surface); }
	    .dashboard-pagination .pagination-nav { display: flex; align-items: center; gap: 12px; margin-left: auto; }
	    .dashboard-pagination .disabled { opacity: .42; pointer-events: none; }
    .collection-head { display: flex; justify-content: space-between; align-items: center; }
    .collection-head span { color: var(--positive); font: 700 18px var(--mono); }
    .progress-track { height: 10px; margin: 12px 0 8px; border-radius: 999px; overflow: hidden; background: #e5e7eb; }
    .progress-value { height: 100%; border-radius: inherit; background: var(--positive); transition: width 180ms ease; }
    .status.open, .status.overdue, .status.partial, .status.paid, .status.needs_review { border-radius: 999px; padding: 5px 9px; display: inline-block; font-size: 12px; }
    .status.open { color: #1e40af; background: #dbeafe; }
    .status.overdue, .status.needs_review { color: #991b1b; background: #fee2e2; }
    .status.partial { color: #92400e; background: #fef3c7; }
    .status.paid { color: #065f46; background: #d1fae5; }
    .status-actions { display: flex; align-items: center; justify-content: space-between; gap: 8px; min-width: max-content; }
    .manual-balance-form { margin: 0; }
    .manual-balance-form .btn { min-height: 32px; padding: 0 10px; font-size: 12px; white-space: nowrap; }
    .metric-link { display: block; color: inherit; text-decoration: none; }
    .metric-link:hover { border-color: #93c5fd; }
    .metric-link:focus-visible { outline: 2px solid var(--accent-bright); outline-offset: 3px; }
    .rent-row { cursor: pointer; }
    .rent-row:hover, .rent-row:focus { background: var(--surface-accent); outline: none; }
    .rent-row td:first-child::after { content: " +"; margin-left: 6px; color: var(--foreground-muted); font: 700 12px var(--mono); }
    .rent-row[aria-expanded="true"] td:first-child::after { content: " -"; }
    .rent-details td { padding: 0; background: var(--surface-muted); }
    .payment-list { padding: 14px 18px 16px 32px; border-top: 1px solid var(--border); }
    .payment-list h3 { margin: 0 0 10px; font-size: 12px; color: var(--foreground); }
    .payment-item { display: grid; grid-template-columns: 140px 170px minmax(180px, 1fr) minmax(160px, auto); gap: 12px; padding: 9px 0; border-bottom: 1px solid var(--border); color: var(--foreground-subtle); font-size: 12px; }
    .payment-item:last-child { border-bottom: 0; }
    .payment-item .amount { font-size: 13px; }
	    .tenant-link, .void-link { color: inherit; text-decoration: none; border-bottom: 1px dashed var(--border-strong); }
	    .tenant-link:hover, .void-link:hover { color: var(--accent-bright); border-color: currentColor; }
	    .dunning-launch { white-space: nowrap; }
	    .dunning-drawer { margin: 18px 0; padding: 20px; border-color: #93c5fd; background: var(--surface-accent); }
	    .dunning-drawer[hidden] { display: none; }
	    .dunning-drawer .panel-head { align-items: flex-start; }
	    .dunning-drawer h2 { margin: 0 0 5px; }
	    .dunning-grid { display: grid; grid-template-columns: minmax(220px, .75fr) minmax(0, 1.25fr); gap: 20px; }
	    .dunning-config, .dunning-selection { min-width: 0; }
	    .dunning-config { padding-right: 20px; border-right: 1px solid var(--border); }
	    .dunning-config form, .dunning-selection form { display: grid; gap: 10px; }
	    .dunning-config label { display: grid; gap: 6px; }
	    .dunning-config input { min-height: 40px; border-radius: 8px; padding: 8px 11px; background: var(--surface-muted); }
	    .dunning-selection { display: grid; gap: 12px; }
	    .dunning-candidates { display: grid; gap: 7px; }
	    .dunning-candidate { display: grid; grid-template-columns: auto minmax(0, 1fr) auto; align-items: center; gap: 10px; padding: 10px 12px; border: 1px solid var(--border); border-radius: 8px; background: var(--surface); }
	    .dunning-candidate:has(input:checked) { border-color: #93c5fd; background: var(--surface-accent); }
	    .dunning-candidate.is-disabled { opacity: .58; }
	    .dunning-candidate strong { display: block; }
	    .dunning-candidate .amount { white-space: nowrap; }
	    .dunning-actions { display: flex; align-items: center; flex-wrap: wrap; gap: 10px; }
	    .dunning-actions label { color: var(--foreground-subtle); font-size: 12px; }
	    .dunning-notice { margin: 0; }
	    .dunning-preview, .dunning-results { display: grid; gap: 8px; margin-top: 12px; }
	    .dunning-preview-row, .dunning-result-row { padding: 12px; border-left: 3px solid var(--accent); background: var(--surface); }
	    .dunning-preview-row pre { max-height: 170px; overflow: auto; margin: 8px 0 0; white-space: pre-wrap; color: var(--foreground-subtle); font: 12px/1.6 var(--mono); }
	    .dunning-result-row { display: flex; align-items: center; flex-wrap: wrap; gap: 8px 12px; border-left-color: var(--positive); }
	    .dunning-result-row.failed { border-left-color: var(--negative); }
	    .dunning-result-row.skipped { border-left-color: var(--foreground-muted); }
	    .dunning-result-row .result-error { color: #ffd0ce; }
	    .dunning-retry { margin-left: auto; }
	    @media (max-width: 760px) { .dunning-grid { grid-template-columns: 1fr; } .dunning-config { padding-right: 0; padding-bottom: 16px; border-right: 0; border-bottom: 1px solid var(--border); } .dunning-result-row { align-items: flex-start; } .dunning-retry { width: 100%; margin-left: 0; } }
	    @media (max-width: 760px) { .payment-item { grid-template-columns: 1fr 1fr; } .payment-item .payment-description { grid-column: 1 / -1; } }
    @media (max-width: 900px) { .dashboard-summary { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
    @media (max-width: 640px) {
	      .dashboard-summary { grid-template-columns: 1fr; }
      .dashboard-toolbar { justify-content: stretch; }
      .dashboard-toolbar .period-picker { display: grid; width: 100%; grid-template-columns: 44px minmax(0, 1fr) 44px; align-items: center; gap: 8px; }
      .dashboard-toolbar .period-label { display: none; }
      .dashboard-toolbar .period-field { width: auto; min-width: 0; }
      .list-filter { display: grid; grid-template-columns: 1fr; gap: 10px; }
      .list-filter .search-field, .list-filter .select-field { flex: none; width: 100%; min-width: 0; }
      .dashboard-pagination { flex-direction: column; align-items: stretch; }
      .dashboard-pagination .page-size-field { justify-content: space-between; }
      .dashboard-pagination .pagination-nav { margin-left: 0; justify-content: space-between; }
      .list-filter .filter-actions { margin-left: 0; display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 8px; }
      .list-filter .filter-actions .btn { width: 100%; min-width: 0; }
      .section-head { align-items: flex-start; flex-direction: column; }
      .section-actions { margin-left: 0; }
      .count-chip.shown { margin-left: 0; }
      /* 共享窄屏块里的 input/select 抬到 44px，但这几条页内规则带着 .list-filter
         之类的前缀、比裸元素选择器更具体，会把它盖回去（实测搜索框/状态筛选/每页
         条数下拉都是 40px）。就地覆盖；催缴面板默认收起，它里面的 input 同理。 */
      .list-filter input, .list-filter select { min-height: 44px; }
      .dashboard-pagination .page-size-field select { min-height: 44px; }
      .dunning-config input { min-height: 44px; }
      .dashboard-toolbar input[type="month"] { min-height: 44px; }
      /* 上/下月按钮实测 42x42，网格轨道一并抬到 44px，否则按钮会撑破轨道。 */
      .month-nav { width: 44px; height: 44px; }
      /* P1：可点的状态筛选 chip 实测 63x32 / 87x32 / 75x32，高度差 12px。
         只抬 <a>，非链接的「本月共 N 户」不是触控目标，保持原样。 */
      a.count-chip { min-height: 44px; }
      /* 分页按钮曾被 flex 压成「上一 / 页」；催缴面板的「收起」被挤成 48×47
         后文字竖排。两者都需要不被压缩，且标签不断行。 */
      .dashboard-pagination .pagination-nav .btn { white-space: nowrap; flex-shrink: 0; }
      .dunning-drawer .panel-head .btn { white-space: nowrap; flex-shrink: 0; }
      /* 共享窄屏块给 input 的 44px 被 input[type="checkbox"] 显式放行，而催缴面板
         默认收起、审计量的是可见元素，所以「确认同日重发」一直没被量到：整块可点
         区域只有 20px 高。label 包着 checkbox，抬 label 就抬了整个可点区域。 */
      .dunning-actions label { display: inline-flex; align-items: center; min-height: 44px; }
	  .manual-balance-form .btn { min-height: 44px; }
      /* 共享窄屏块把姓名链接抬到 44px 高，而它的唯一视觉提示是本页基础样式里的
         border-bottom——盒子一高，虚线就被推到文字下方约 22px，看着像一条孤立的
         分隔线，不像下划线（截图复核发现）。窄屏改用文字自身的下划线：虚线贴回
         文字，链接盒仍是 44px，命中区不变。同特异度，写在基础样式之后故生效。 */
      .tenant-link {
        border-bottom: 0;
        text-decoration: underline dashed var(--border-strong);
        text-underline-offset: 3px;
      }
      .tenant-link:hover { text-decoration-color: currentColor; }
    }
  </style>
  <script>`+workspaceCalendarScript+`</script>
</head>
<body>
  <div class="app">
    {{template "workspace-nav" .}}
    <main class="content">
      <header class="topbar">
        <div><div class="brand-title">本月收租</div><h1>{{.PeriodLabel}}</h1></div>
        <form method="post" action="/logout"><button class="btn danger" type="submit">退出登录</button></form>
      </header>
	  {{if eq .Error "invalid_period"}}<div class="notice error">选择的月份无效，请重新选择。</div>{{end}}
	  {{if eq .Error "invalid_dashboard_filter"}}<div class="notice error">筛选条件无效，请重新选择。</div>{{end}}
	  {{if eq .Error "invalid_manual_balance"}}<div class="notice error">平账请求无效，请返回列表后重试。</div>{{end}}
	  {{if eq .Error "manual_balance_failed"}}<div class="notice error">一键平账未完成，请刷新页面后重试。</div>{{end}}
	  {{if eq .Message "rent_confirmed"}}<div class="notice ok">租金已确认并计入对应月份。</div>{{end}}
	  {{if eq .Message "manual_balance_saved"}}<div class="notice ok">已创建“手动平账”收入，并补齐该月租金差额。</div>{{end}}
	  {{if eq .Message "manual_balance_not_needed"}}<div class="notice ok">该月租金已缴清，无需再平账。</div>{{end}}
	  {{if .SyncCoverage}}{{if or (eq .SyncStatus "partial") (eq .SyncStatus "failed")}}<div class="notice error sync-status">最近一次银行同步异常：{{.SyncCoverage}}{{if .LastSuccessfulSyncCoverage}}<br>最近一次成功同步：{{.LastSuccessfulSyncCoverage}}{{end}}</div>{{else}}<div class="notice sync-status">银行流水状态：{{.SyncCoverage}}</div>{{end}}{{else}}<div class="notice sync-status">尚未完成银行同步，待处理金额可能不完整。</div>{{end}}
	  <div class="dashboard-toolbar">
	    <form class="period-picker" method="get" action="/rent-dashboard">
	      <span class="label period-label">收租月份</span>
	      <a class="month-nav previous" href="{{rentDashboardURL .PreviousPeriod .SearchFilter .StatusFilter .SortFilter 1 .PageSize}}" aria-label="查看上个月">‹</a>
	      <label class="period-field" for="period"><input id="period" name="period" type="month" value="{{.Period}}" onchange="this.form.submit()"></label>
	      <a class="month-nav next" href="{{rentDashboardURL .NextPeriod .SearchFilter .StatusFilter .SortFilter 1 .PageSize}}" aria-label="查看下个月">›</a>
	      {{if .SearchFilter}}<input type="hidden" name="search" value="{{.SearchFilter}}">{{end}}
	      {{if and .StatusFilter (ne .StatusFilter "all")}}<input type="hidden" name="status" value="{{.StatusFilter}}">{{end}}
	      {{if and .SortFilter (ne .SortFilter "status")}}<input type="hidden" name="sort" value="{{.SortFilter}}">{{end}}
	      {{if and (gt .PageSize 0) (ne .PageSize 12)}}<input type="hidden" name="page_size" value="{{.PageSize}}">{{end}}
	    </form>
	  </div>
	  <section class="dashboard-section" aria-labelledby="dashboard-summary-title">
	    <div class="section-head">
	      <div><h2 id="dashboard-summary-title">收租汇总</h2><div class="tiny">{{.PeriodLabel}} · 各卡片金额仅统计 EUR</div></div>
	    </div>
	    <section class="dashboard-summary" aria-label="月度收租汇总">
	      <div class="panel metric metric-primary"><div class="label">本月应收</div><strong>{{.ExpectedTotal}}</strong><span>本月租金账单</span></div>
	      <a class="panel metric metric-success metric-link" href="{{rentDashboardURL .Period .SearchFilter "paid" .SortFilter 1 .PageSize}}"><div class="label">已收租金</div><strong>{{.PaidTotal}}</strong><span>{{.PaidCount}} 户已缴清 · 查看账单</span></a>
	      <a class="panel metric metric-warning metric-link" href="{{rentDashboardURL .Period .SearchFilter "unpaid" .SortFilter 1 .PageSize}}"><div class="label">剩余未收</div><strong>{{.BalanceTotal}}</strong><span>{{.UnpaidCount}} 户未缴或部分缴纳 · 查看账单</span></a>
	      <a class="panel metric metric-link" href="/billing?period={{.Period}}&amp;pending=1" aria-label="查看{{.PeriodLabel}}待处理流水"><div class="label">待处理</div><strong>{{.PendingCount}}</strong><span>笔流水需要关联或确认 · 查看对应流水</span></a>
	    </section>
	    <div class="panel dashboard-counts" aria-label="账单状态数量">
	      <span class="count-chip">本月共 {{.TotalRows}} 户</span>
	      <a class="count-chip overdue" href="{{rentDashboardURL .Period .SearchFilter "overdue" .SortFilter 1 .PageSize}}">逾期 {{.OverdueCount}}</a>
	      <a class="count-chip partial" href="{{rentDashboardURL .Period .SearchFilter "partial" .SortFilter 1 .PageSize}}">部分缴纳 {{.PartialCount}}</a>
	      <a class="count-chip review" href="{{rentDashboardURL .Period .SearchFilter "needs_review" .SortFilter 1 .PageSize}}">待确认 {{.ReviewCount}}</a>
	      <span class="count-chip shown">当前显示 {{.FilteredCount}} 户</span>
	    </div>
	    <section class="panel collection-panel" aria-label="收款进度"><div class="collection-head"><strong>收款进度</strong><span>{{.CollectionPercent}}%</span></div><div class="progress-track"><div class="progress-value" style="width: {{.CollectionPercent}}%"></div></div><div class="tiny">已收 {{.PaidTotal}}，本月还差 {{.BalanceTotal}}</div></section>
	  </section>
	  <section class="dashboard-section" aria-labelledby="rent-status-title">
	    <div class="section-head">
	      <div><h2 id="rent-status-title">租客缴费情况</h2><div class="tiny">共 {{.FilteredCount}} 户 · 点击任意一行展开收款明细</div></div>
	      {{if .Dunning.Enabled}}<div class="section-actions"><button class="btn dunning-launch" type="button" data-dunning-open aria-controls="dunning-drawer" aria-expanded="{{if .Dunning.Open}}true{{else}}false{{end}}">邮件催缴</button></div>{{end}}
	    </div>
	    <section class="panel surface" aria-label="租客账单列表">
	      <form class="list-filter" method="get" action="/rent-dashboard">
	        <input type="hidden" name="period" value="{{.Period}}">
	        <label class="search-field" for="dashboard-search">搜索租客<input id="dashboard-search" name="search" type="search" value="{{.SearchFilter}}" placeholder="姓名、别名、房间"></label>
	        <label class="select-field" for="dashboard-status">状态<select id="dashboard-status" name="status"><option value="all"{{if eq .StatusFilter "all"}} selected{{end}}>全部</option><option value="unpaid"{{if eq .StatusFilter "unpaid"}} selected{{end}}>未缴（含部分）</option><option value="overdue"{{if eq .StatusFilter "overdue"}} selected{{end}}>逾期</option><option value="needs_review"{{if eq .StatusFilter "needs_review"}} selected{{end}}>待确认</option><option value="partial"{{if eq .StatusFilter "partial"}} selected{{end}}>部分缴纳</option><option value="open"{{if eq .StatusFilter "open"}} selected{{end}}>未开始收款</option><option value="paid"{{if eq .StatusFilter "paid"}} selected{{end}}>已缴清</option></select></label>
	        <input type="hidden" name="sort" value="{{.SortFilter}}">
	        <div class="filter-actions"><button class="btn" type="submit">筛选</button><a class="btn subtle" href="/rent-dashboard?period={{.Period}}">清除筛选</a></div>
	      </form>
          {{if .Rows}}
          <div class="table-wrap"><table>
            <thead><tr><th><a class="sort-link{{if .TenantSort.Active}} active{{end}}" href="{{.TenantSort.URL}}">租客{{if .TenantSort.Arrow}}<span class="sort-arrow">{{.TenantSort.Arrow}}</span>{{end}}</a></th><th>房间</th><th><a class="sort-link{{if .DueSort.Active}} active{{end}}" href="{{.DueSort.URL}}">应缴日{{if .DueSort.Arrow}}<span class="sort-arrow">{{.DueSort.Arrow}}</span>{{end}}</a></th><th><a class="sort-link{{if .AmountSort.Active}} active{{end}}" href="{{.AmountSort.URL}}">应收{{if .AmountSort.Arrow}}<span class="sort-arrow">{{.AmountSort.Arrow}}</span>{{end}}</a></th><th>已收</th><th>未收</th><th><a class="sort-link{{if .StatusSort.Active}} active{{end}}" href="{{.StatusSort.URL}}">状态{{if .StatusSort.Arrow}}<span class="sort-arrow">{{.StatusSort.Arrow}}</span>{{end}}</a></th></tr></thead>
            <tbody>{{range .Rows}}
	            <tr class="rent-row" tabindex="0" role="button" aria-expanded="false" aria-controls="rent-details-{{.ObligationID}}" data-details-target="rent-details-{{.ObligationID}}"><td><a class="tenant-link" href="/tenants/{{.TenantID}}?from_month={{.Period}}&amp;to_month={{.Period}}"><strong>{{.TenantName}}</strong></a>{{if .TenantAlias}}<br><span class="tiny">别名：{{.TenantAlias}}</span>{{end}}</td><td>{{if .RoomLabel}}{{.RoomLabel}}<br>{{end}}{{.RoomAddress}}</td><td class="mono">{{.DueDate}}</td><td class="amount">{{.ExpectedAmount}}</td><td class="amount">{{.PaidAmount}}</td><td class="amount">{{.BalanceAmount}}</td><td><div class="status-actions"><span class="status {{.Status}}">{{.StatusLabel}}</span>{{if gt .ExpectedCents .PaidCents}}<form class="manual-balance-form" method="post" action="/rent-dashboard/settle" onclick="event.stopPropagation()" onkeydown="event.stopPropagation()" onsubmit="return confirm('确认一键平账吗？')"><input type="hidden" name="obligation_id" value="{{.ObligationID}}"><input type="hidden" name="period" value="{{$.Period}}"><input type="hidden" name="search" value="{{$.SearchFilter}}"><input type="hidden" name="status" value="{{$.StatusFilter}}"><input type="hidden" name="sort" value="{{$.SortFilter}}"><input type="hidden" name="page" value="{{$.Page}}"><input type="hidden" name="page_size" value="{{$.PageSize}}"><button class="btn subtle" type="submit">一键平账</button></form>{{end}}</div></td></tr>
              <tr id="rent-details-{{.ObligationID}}" class="rent-details" hidden><td colspan="7"><div class="payment-list"><h3>收款明细 · <a class="tenant-link" href="/cash-receipts/new?tenant_id={{.TenantID}}&amp;period={{.Period}}">补录现金</a></h3>{{if .Payments}}{{range .Payments}}<div class="payment-item"><span class="amount">{{.AmountDisplay}}</span><span class="mono">{{.DateDisplay}}</span><span class="payment-description">{{.Description}}</span><span class="mono">{{if eq .Source "现金"}}现金 · {{end}}{{.ConfirmationSource}}{{if eq .Source "现金"}} · <a class="void-link" href="/cash-receipts/void?receipt_id={{.PaymentID}}">撤销</a>{{end}}</span></div>{{end}}{{else}}<div class="tiny">本月暂无已确认收款。</div>{{end}}</div></td></tr>
            {{end}}</tbody>
  	    </table></div>
  	    {{else if gt .FilteredCount 0}}<div class="empty">当前页没有账单，请返回上一页。</div>
  	    {{else if gt .TotalRows 0}}<div class="empty">没有符合当前筛选条件的租金账单，请调整搜索词或状态。</div>
  	    {{else}}<div class="empty">本月还没有租金账单。请先添加租客并设置租金开始日期。</div>{{end}}
	    {{if .Rows}}<nav class="dashboard-pagination" aria-label="租客账单分页">
	      <form class="page-size-field" method="get" action="/rent-dashboard">
	        <input type="hidden" name="period" value="{{.Period}}">
	        {{if .SearchFilter}}<input type="hidden" name="search" value="{{.SearchFilter}}">{{end}}
	        {{if and .StatusFilter (ne .StatusFilter "all")}}<input type="hidden" name="status" value="{{.StatusFilter}}">{{end}}
	        {{if and .SortFilter (ne .SortFilter "status")}}<input type="hidden" name="sort" value="{{.SortFilter}}">{{end}}
	        <label for="dashboard-page-size">每页<select id="dashboard-page-size" name="page_size" onchange="this.form.submit()"><option value="12"{{if eq .PageSize 12}} selected{{end}}>12</option><option value="24"{{if eq .PageSize 24}} selected{{end}}>24</option><option value="50"{{if eq .PageSize 50}} selected{{end}}>50</option></select></label>
	      </form>
	      <div class="pagination-nav">
	        <a class="btn subtle{{if eq .Page 1}} disabled{{end}}" href="{{rentDashboardURL .Period .SearchFilter .StatusFilter .SortFilter (dashboardPreviousPage .Page) .PageSize}}">上一页</a>
	        <span class="tiny">第 {{.Page}} / {{.TotalPages}} 页 · 共 {{.FilteredCount}} 户</span>
	        <a class="btn subtle{{if eq .Page .TotalPages}} disabled{{end}}" href="{{rentDashboardURL .Period .SearchFilter .StatusFilter .SortFilter (dashboardNextPage .Page .TotalPages) .PageSize}}">下一页</a>
	      </div>
	    </nav>{{end}}
	    </section>
	  </section>
	  {{if .Dunning.Enabled}}
	  <section id="dunning-drawer" class="panel dunning-drawer" aria-labelledby="dunning-title"{{if not .Dunning.Open}} hidden{{end}}>
	    <div class="panel-head"><div><h2 id="dunning-title">邮件催缴</h2><div class="tiny">仅操作当前页未缴账单；每位租客单独发送，不共享收件人。</div></div><button class="btn subtle" type="button" data-dunning-close aria-controls="dunning-drawer">收起</button></div>
	    {{if .Dunning.Error}}<div class="notice error dunning-notice" role="alert">{{.Dunning.Error}}</div>{{end}}
	    {{if .Dunning.Notice}}<div class="notice ok dunning-notice" role="status">{{.Dunning.Notice}}</div>{{end}}
	    <div class="dunning-grid">
	      <section class="dunning-config" aria-labelledby="dunning-config-title">
	        <div class="label" id="dunning-config-title">房东发件配置</div>
	        <p class="tiny">邮件由 SMTP 服务地址发出，租客回复会回到这里。</p>
	        <form method="post" action="/dunning/config">
	          <input type="hidden" name="period" value="{{.Dunning.Period}}"><input type="hidden" name="search" value="{{.Dunning.SearchFilter}}"><input type="hidden" name="status" value="{{.Dunning.StatusFilter}}"><input type="hidden" name="sort" value="{{.Dunning.SortFilter}}"><input type="hidden" name="page" value="{{.Dunning.Page}}"><input type="hidden" name="page_size" value="{{.Dunning.PageSize}}">
	          <label for="dunning-display-name">显示名称<input id="dunning-display-name" name="display_name" value="{{.Dunning.Sender.DisplayName}}" autocomplete="organization" required></label>
	          <label for="dunning-reply-to">回复邮箱<input id="dunning-reply-to" name="reply_to_email" type="email" value="{{.Dunning.Sender.ReplyToEmail}}" autocomplete="email" required></label>
	          <button class="btn" type="submit">保存发件配置</button>
	        </form>
	      </section>
	      <section class="dunning-selection" aria-labelledby="dunning-selection-title">
	        <div><div class="label" id="dunning-selection-title">{{.Dunning.Period}} 当前页候选</div>{{if .Dunning.ConfigurationError}}<div class="tiny">{{.Dunning.ConfigurationError}}</div>{{end}}</div>
	        <form id="dunning-send-form" method="post" action="/dunning/preview">
	          <input type="hidden" name="period" value="{{.Dunning.Period}}"><input type="hidden" name="search" value="{{.Dunning.SearchFilter}}"><input type="hidden" name="status" value="{{.Dunning.StatusFilter}}"><input type="hidden" name="sort" value="{{.Dunning.SortFilter}}"><input type="hidden" name="page" value="{{.Dunning.Page}}"><input type="hidden" name="page_size" value="{{.Dunning.PageSize}}"><input type="hidden" name="request_key" value="{{.Dunning.RequestKey}}">
	          <div class="dunning-candidates">
	            {{if .Dunning.Candidates}}{{range .Dunning.Candidates}}<label class="dunning-candidate{{if not .Selectable}} is-disabled{{end}}"><input type="checkbox" name="obligation_id" value="{{.ObligationID}}"{{if .Selected}} checked{{end}}{{if not .Selectable}} disabled{{end}}><span><strong>{{.TenantName}}</strong><span class="tiny">{{.Email}}{{if not .EmailValid}} · {{.EmailError}}{{else if .SentToday}} · 今天已发送{{else if .LastDunningStatus}} · 最近：{{dunningDeliveryLabel .LastDunningStatus}}{{end}}</span></span><span class="amount">{{.BalanceAmount}}</span></label>{{end}}{{else}}<div class="empty">当前页没有可催缴账单。</div>{{end}}
	          </div>
	          <div class="dunning-actions"><button class="btn subtle" type="submit" formaction="/dunning/preview">预览邮件</button><label><input type="checkbox" name="confirm_resend" value="1">确认同日重发</label><button class="btn" type="submit" formaction="/dunning/send">发送已选</button></div>
	        </form>
	        {{if .Dunning.PreviewRows}}<div class="dunning-preview" aria-live="polite"><div class="label">发送前预览</div>{{range .Dunning.PreviewRows}}<article class="dunning-preview-row"><strong>{{.Candidate.TenantName}}</strong>{{if .Message}}<div>{{.Message.Subject}} · {{.Message.RecipientEmail}}</div><pre>{{.Message.Body}}</pre>{{else}}<div class="result-error">{{.Error}}</div>{{end}}</article>{{end}}</div>{{end}}
	        {{if .Dunning.Results}}<div class="dunning-results" aria-live="polite"><div class="label">发送结果</div>{{range .Dunning.Results}}<article class="dunning-result-row{{if .Attempt}}{{if eq .Attempt.DeliveryStatus "failed"}} failed{{else if eq .Attempt.DeliveryStatus "skipped"}} skipped{{end}}{{end}}"><strong>{{.Candidate.TenantName}}</strong>{{if .Attempt}}<span>{{dunningDeliveryLabel .Attempt.DeliveryStatus}}</span>{{end}}{{if .Error}}<span class="result-error">{{.Error}}</span>{{end}}{{if and .Attempt (eq .Attempt.DeliveryStatus "failed")}}<form class="dunning-retry" method="post" action="/dunning/send"><input type="hidden" name="period" value="{{$.Dunning.Period}}"><input type="hidden" name="search" value="{{$.Dunning.SearchFilter}}"><input type="hidden" name="status" value="{{$.Dunning.StatusFilter}}"><input type="hidden" name="sort" value="{{$.Dunning.SortFilter}}"><input type="hidden" name="page" value="{{$.Dunning.Page}}"><input type="hidden" name="page_size" value="{{$.Dunning.PageSize}}"><input type="hidden" name="request_key" value="{{.RetryRequestKey}}"><input type="hidden" name="obligation_id" value="{{.Candidate.ObligationID}}"><input type="hidden" name="retry_of_attempt_id" value="{{.Attempt.ID}}"><button class="btn subtle" type="submit">重试此人</button></form>{{end}}</article>{{end}}</div>{{end}}
	      </section>
	    </div>
	  </section>
	  {{end}}
	    </main>
  </div>
  <script>
	    for (const row of document.querySelectorAll("[data-details-target]")) {
      const details = document.getElementById(row.dataset.detailsTarget);
      const toggle = () => {
        const expanded = row.getAttribute("aria-expanded") === "true";
        row.setAttribute("aria-expanded", String(!expanded));
        details.hidden = expanded;
      };
      row.addEventListener("click", toggle);
      row.addEventListener("keydown", (event) => {
        if (event.key === "Enter" || event.key === " ") { event.preventDefault(); toggle(); }
	      });
	    }
	    const dunningDrawer = document.getElementById("dunning-drawer");
	    const dunningOpen = document.querySelector("[data-dunning-open]");
	    const dunningClose = document.querySelector("[data-dunning-close]");
	    const setDunningOpen = (open) => {
	      if (!dunningDrawer || !dunningOpen) return;
	      dunningDrawer.hidden = !open;
	      dunningOpen.setAttribute("aria-expanded", String(open));
	      if (open) dunningDrawer.scrollIntoView({behavior: "smooth", block: "start"});
	    };
	    dunningOpen?.addEventListener("click", () => setDunningOpen(true));
	    dunningClose?.addEventListener("click", () => { setDunningOpen(false); dunningOpen?.focus(); });
	  </script>
</body>
</html>
`)
