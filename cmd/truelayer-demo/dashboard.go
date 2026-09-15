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
	period := r.URL.Query().Get("period")
	filters, filtersErr := rentDashboardFiltersFromQuery(r.URL.Query())
	if filtersErr != nil {
		filters = defaultRentDashboardFilters()
	}
	periodMonth, err := parsePeriodMonth(period)
	if err != nil {
		periodMonth = monthStart(time.Now().UTC())
	}
	data := rentDashboardPageData{
		Username:       a.cfg.AdminUsername,
		Environment:    a.cfg.Environment,
		ActivePage:     "rent-dashboard",
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

	if userID, ok := a.currentUserID(r); ok && a.db != nil {
		service := newTransactionService(a.db)
		if err := service.reconcileTransactions(r.Context(), userID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
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

var rentDashboardTemplate = template.Must(template.New("rent-dashboard").Funcs(template.FuncMap{
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
}).Parse(`<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>RentOps Dashboard</title>
  <style>` + workspacePageCSS + workspaceCalendarCSS + `
	    .dashboard-summary { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 12px; }
	    .dashboard-toolbar { display: flex; align-items: end; justify-content: space-between; gap: 16px; margin-bottom: 18px; }
	    .dashboard-toolbar form { display: flex; align-items: end; gap: 10px; }
	    .dashboard-toolbar label { margin: 0; min-width: 150px; }
	    .dashboard-filter { flex-wrap: wrap; justify-content: flex-end; }
	    .dashboard-filter label { min-width: 140px; }
	    .dashboard-filter input, .dashboard-filter select { min-height: 42px; border-radius: 14px; padding: 9px 12px; background: rgba(255,255,255,0.07); box-shadow: inset 0 1px 0 rgba(255,255,255,0.10), 0 0 0 1px rgba(255,255,255,0.03); }
	    .dashboard-filter input:focus, .dashboard-filter select:focus { border-color: rgba(104,114,217,0.85); background: rgba(255,255,255,0.08); }
	    .dashboard-counts { display: flex; flex-wrap: wrap; gap: 8px 16px; margin: -6px 0 18px; color: var(--foreground-subtle); }
	    .dashboard-counts a { color: inherit; text-decoration: none; border-bottom: 1px dashed rgba(255,255,255,.28); }
	    .dashboard-counts a:hover { color: #fff; }
	    .dashboard-secondary { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; margin: 0 0 18px; }
	    .dashboard-secondary .metric { min-height: 118px; }
	    .sync-status { margin-bottom: 18px; }
	    .dashboard-pagination { display: flex; align-items: center; justify-content: center; gap: 12px; margin-top: 16px; }
	    .dashboard-pagination .disabled { opacity: .42; pointer-events: none; }
    .dashboard-toolbar input[type="month"] { min-height: 42px; border-radius: 14px; padding: 9px 12px; background: rgba(255,255,255,0.07); box-shadow: inset 0 1px 0 rgba(255,255,255,0.10), 0 0 0 1px rgba(255,255,255,0.03); }
    .dashboard-toolbar input[type="month"]:hover { border-color: rgba(255,255,255,0.22); background: rgba(255,255,255,0.09); }
    .dashboard-toolbar input[type="month"]:focus { border-color: rgba(104,114,217,0.85); background: rgba(255,255,255,0.08); }
    .month-nav { width: 42px; height: 42px; border-radius: 14px; display: grid; place-items: center; color: var(--foreground); background: rgba(255,255,255,0.055); box-shadow: inset 0 1px 0 rgba(255,255,255,0.10), 0 0 0 1px rgba(255,255,255,0.08); text-decoration: none; font-size: 28px; line-height: 1; }
    .month-nav:hover { background: rgba(255,255,255,0.10); }
    .collection-panel { padding: 18px; margin: 0 0 18px; }
    .collection-head { display: flex; justify-content: space-between; align-items: center; }
    .collection-head span { color: var(--positive); font: 700 18px var(--mono); }
    .progress-track { height: 10px; margin: 12px 0 8px; border-radius: 999px; overflow: hidden; background: rgba(255,255,255,0.08); }
    .progress-value { height: 100%; border-radius: inherit; background: var(--positive); transition: width 180ms ease; }
    .status.open, .status.overdue, .status.partial, .status.paid, .status.needs_review { border-radius: 999px; padding: 5px 9px; display: inline-block; font-size: 12px; }
    .status.open { color: #d8dcff; background: rgba(104,114,217,0.12); }
    .status.overdue, .status.needs_review { color: #ffd0ce; background: rgba(255,139,134,0.10); }
    .review-link { color: #ffd0ce; text-decoration: none; border-bottom: 1px dashed currentColor; }
    .review-link:hover { color: #fff; }
    .status.partial { color: #ffe0a7; background: rgba(233,184,114,0.10); }
    .status.paid { color: #cbffe1; background: rgba(125,211,168,0.10); }
    .metric-link { display: block; color: inherit; text-decoration: none; }
    .metric-link:hover { border-color: rgba(104,114,217,0.48); }
    .metric-link:focus-visible { outline: 2px solid rgba(104,114,217,0.9); outline-offset: 3px; }
    .rent-row { cursor: pointer; }
    .rent-row:hover, .rent-row:focus { background: rgba(255,255,255,0.035); outline: none; }
    .rent-row td:first-child::after { content: " +"; margin-left: 6px; color: var(--foreground-muted); font: 700 12px var(--mono); }
    .rent-row[aria-expanded="true"] td:first-child::after { content: " -"; }
    .rent-details td { padding: 0; background: rgba(255,255,255,0.025); }
    .payment-list { padding: 14px 18px 16px 32px; border-top: 1px solid rgba(255,255,255,0.045); }
    .payment-list h3 { margin: 0 0 10px; font-size: 12px; color: var(--foreground); }
    .payment-item { display: grid; grid-template-columns: 140px 170px minmax(180px, 1fr) minmax(160px, 1fr) 100px; gap: 12px; padding: 9px 0; border-bottom: 1px solid rgba(255,255,255,0.04); color: var(--foreground-subtle); font-size: 12px; }
    .payment-item:last-child { border-bottom: 0; }
    .payment-item .amount { font-size: 13px; }
    .tenant-link, .void-link { color: inherit; text-decoration: none; border-bottom: 1px dashed rgba(255,255,255,.35); }
    .tenant-link:hover, .void-link:hover { color: #fff; border-color: currentColor; }
    @media (max-width: 760px) { .payment-item { grid-template-columns: 1fr 1fr; } .payment-item .payment-description { grid-column: 1 / -1; } }
    @media (max-width: 900px) { .dashboard-summary { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
	    @media (max-width: 640px) { .dashboard-summary, .dashboard-secondary { grid-template-columns: 1fr; } .dashboard-toolbar { align-items: stretch; flex-direction: column; min-width: 0; } .dashboard-toolbar form { display: flex; width: 100%; max-width: 100%; min-width: 0; flex-wrap: wrap; align-items: stretch; gap: 8px; } .dashboard-toolbar form label { width: 100%; flex: 1 1 100%; min-width: 0; } .dashboard-toolbar form input, .dashboard-toolbar form select { min-width: 0; width: 100%; } .month-nav { flex: 0 0 42px; } .dashboard-filter .month-nav { flex: 0 0 42px; } .dashboard-filter .btn { flex: 1 1 auto; } }
  </style>
  <script>` + workspaceCalendarScript + `</script>
</head>
<body>
  <div class="app">
    <aside class="sidebar" aria-label="Main navigation">
      <div class="side-brand"><div class="mark">R</div><div><div class="brand-title">RentOps</div><div class="brand-meta">{{.Environment}} workspace</div></div></div>
      <nav class="nav" aria-label="主导航">
        <a href="/rent-dashboard" class="active"><span class="glyph">总</span><span>月度总览</span><span class="nav-count">{{.TenantCount}}</span></a>
        <a href="/billing"><span class="glyph">流</span><span>银行流水</span><span class="nav-count">{{.IncomeCount}}</span></a>
        <a href="/tenants"><span class="glyph">租</span><span>租客管理</span><span class="nav-count">{{.TenantCount}}</span></a>
        <a href="/expenses"><span class="glyph">支</span><span>支出记录</span><span class="nav-count">{{.ExpenseCount}}</span></a>
      </nav>
      <div class="side-foot">当前用户：{{.Username}}<br>月度收租工作台</div>
    </aside>
    <main class="content">
      <header class="topbar">
        <div><div class="brand-title">本月收租</div><h1>{{.PeriodLabel}}</h1></div>
        <form method="post" action="/logout"><button class="btn danger" type="submit">退出登录</button></form>
      </header>
	  {{if eq .Error "invalid_period"}}<div class="notice error">选择的月份无效，请重新选择。</div>{{end}}
	  {{if eq .Error "invalid_dashboard_filter"}}<div class="notice error">筛选条件无效，请重新选择。</div>{{end}}
	  {{if eq .Message "rent_confirmed"}}<div class="notice ok">租金已确认并计入对应月份。</div>{{end}}
	  {{if .SyncCoverage}}{{if or (eq .SyncStatus "partial") (eq .SyncStatus "failed")}}<div class="notice error sync-status">最近一次银行同步异常：{{.SyncCoverage}}{{if .LastSuccessfulSyncCoverage}}<br>最近一次成功同步：{{.LastSuccessfulSyncCoverage}}{{end}}</div>{{else}}<div class="notice sync-status">银行流水状态：{{.SyncCoverage}}</div>{{end}}{{else}}<div class="notice sync-status">尚未完成银行同步，待处理金额可能不完整。</div>{{end}}
	  <div class="dashboard-toolbar">
	    <div><div class="label">月度收租情况</div><div class="tiny">查看本月应收、已收、未收和需要处理的流水。</div></div>
	    <form class="dashboard-filter" method="get" action="/rent-dashboard">
	      <a class="month-nav previous" href="{{rentDashboardURL .PreviousPeriod .SearchFilter .StatusFilter .SortFilter 1 .PageSize}}" aria-label="查看上个月">‹</a>
	      <label for="period">选择月份<input id="period" name="period" type="month" value="{{.Period}}" onchange="this.form.submit()"></label>
	      <a class="month-nav next" href="{{rentDashboardURL .NextPeriod .SearchFilter .StatusFilter .SortFilter 1 .PageSize}}" aria-label="查看下个月">›</a>
	      <label for="dashboard-search">搜索租客<input id="dashboard-search" name="search" type="search" value="{{.SearchFilter}}" placeholder="姓名、别名、房间"></label>
	      <label for="dashboard-status">状态<select id="dashboard-status" name="status"><option value="all"{{if eq .StatusFilter "all"}} selected{{end}}>全部</option><option value="unpaid"{{if eq .StatusFilter "unpaid"}} selected{{end}}>未缴（含部分）</option><option value="overdue"{{if eq .StatusFilter "overdue"}} selected{{end}}>逾期</option><option value="needs_review"{{if eq .StatusFilter "needs_review"}} selected{{end}}>待确认</option><option value="partial"{{if eq .StatusFilter "partial"}} selected{{end}}>部分缴纳</option><option value="open"{{if eq .StatusFilter "open"}} selected{{end}}>未开始收款</option><option value="paid"{{if eq .StatusFilter "paid"}} selected{{end}}>已缴清</option></select></label>
	      <label for="dashboard-sort">排序<select id="dashboard-sort" name="sort"><option value="status"{{if eq .SortFilter "status"}} selected{{end}}>优先显示待处理</option><option value="tenant_asc"{{if eq .SortFilter "tenant_asc"}} selected{{end}}>租客姓名</option><option value="amount_desc"{{if eq .SortFilter "amount_desc"}} selected{{end}}>应收金额从高到低</option><option value="amount_asc"{{if eq .SortFilter "amount_asc"}} selected{{end}}>应收金额从低到高</option><option value="due_asc"{{if eq .SortFilter "due_asc"}} selected{{end}}>应缴日从早到晚</option><option value="due_desc"{{if eq .SortFilter "due_desc"}} selected{{end}}>应缴日从晚到早</option></select></label>
	      <label for="dashboard-page-size">每页<select id="dashboard-page-size" name="page_size"><option value="12"{{if eq .PageSize 12}} selected{{end}}>12</option><option value="24"{{if eq .PageSize 24}} selected{{end}}>24</option><option value="50"{{if eq .PageSize 50}} selected{{end}}>50</option></select></label>
	      <button class="btn" type="submit">筛选</button>
	      <a class="btn subtle" href="/rent-dashboard?period={{.Period}}">清除筛选</a>
	    </form>
	  </div>
	  <section class="dashboard-summary" aria-label="月度收租汇总">
	    <div class="panel metric metric-primary"><div class="label">本月应收</div><strong>{{.ExpectedTotal}}</strong><span>本月租金账单</span></div>
	    <a class="panel metric metric-success metric-link" href="{{rentDashboardURL .Period .SearchFilter "paid" .SortFilter 1 .PageSize}}"><div class="label">已收租金</div><strong>{{.PaidTotal}}</strong><span>{{.PaidCount}} 户已缴清 · 查看账单</span></a>
	    <a class="panel metric metric-warning metric-link" href="{{rentDashboardURL .Period .SearchFilter "unpaid" .SortFilter 1 .PageSize}}"><div class="label">剩余未收</div><strong>{{.BalanceTotal}}</strong><span>{{.UnpaidCount}} 户未缴或部分缴纳 · 查看账单</span></a>
	    <a class="panel metric metric-link" href="/billing?period={{.Period}}&amp;pending=1" aria-label="查看{{.PeriodLabel}}待处理流水"><div class="label">待处理</div><strong>{{.PendingCount}}</strong><span>笔流水需要关联或确认 · 查看对应流水</span></a>
	  </section>
	  <div class="dashboard-counts" aria-label="账单状态数量"><span>本月共 {{.TotalRows}} 户</span><a href="{{rentDashboardURL .Period .SearchFilter "overdue" .SortFilter 1 .PageSize}}">逾期 {{.OverdueCount}}</a><a href="{{rentDashboardURL .Period .SearchFilter "partial" .SortFilter 1 .PageSize}}">部分缴纳 {{.PartialCount}}</a><a href="{{rentDashboardURL .Period .SearchFilter "needs_review" .SortFilter 1 .PageSize}}">待确认 {{.ReviewCount}}</a><span>当前显示 {{.FilteredCount}} 户</span></div>
	  <section class="dashboard-secondary" aria-label="银行流水汇总">
	    <a class="panel metric metric-link" href="/billing?period={{.Period}}&amp;direction=income&amp;pending=1"><div class="label">待分配金额</div><strong>{{.PendingTotal}}</strong><span>{{.PendingCount}} 笔收入仍有未分配余额，金额仅统计 EUR</span></a>
	    <a class="panel metric metric-link" href="/billing?period={{.Period}}&amp;allocation=other_income"><div class="label">其他收入</div><strong>{{.OtherIncomeTotal}}</strong><span>{{.OtherIncomeCount}} 笔已确认的非租金收入</span></a>
	  </section>
	  <section class="panel collection-panel" aria-label="收款进度"><div class="collection-head"><strong>收款进度</strong><span>{{.CollectionPercent}}%</span></div><div class="progress-track"><div class="progress-value" style="width: {{.CollectionPercent}}%"></div></div><div class="tiny">已收 {{.PaidTotal}}，本月还差 {{.BalanceTotal}}</div></section>
      <section class="panel surface" aria-labelledby="rent-status-title">
        <div class="panel-head"><h2 id="rent-status-title">租客缴费情况</h2><a class="tiny review-link" href="/billing?period={{.Period}}&amp;match_status=needs_review">{{.ReviewCount}} 笔待确认</a></div>
        {{if .Rows}}
        <div class="table-wrap"><table>
          <thead><tr><th>租客</th><th>房间</th><th>应缴日</th><th>应收</th><th>已收</th><th>未收</th><th>状态</th></tr></thead>
          <tbody>{{range .Rows}}
	            <tr class="rent-row" tabindex="0" role="button" aria-expanded="false" aria-controls="rent-details-{{.ObligationID}}" data-details-target="rent-details-{{.ObligationID}}"><td><a class="tenant-link" href="/tenants/{{.TenantID}}?from_month={{.Period}}&amp;to_month={{.Period}}"><strong>{{.TenantName}}</strong></a>{{if .TenantAlias}}<br><span class="tiny">别名：{{.TenantAlias}}</span>{{end}}</td><td>{{if .RoomLabel}}{{.RoomLabel}}<br>{{end}}{{.RoomAddress}}</td><td class="mono">{{.DueDate}}</td><td class="amount">{{.ExpectedAmount}}</td><td class="amount">{{.PaidAmount}}</td><td class="amount">{{.BalanceAmount}}</td><td><span class="status {{.Status}}">{{.StatusLabel}}</span></td></tr>
            <tr id="rent-details-{{.ObligationID}}" class="rent-details" hidden><td colspan="7"><div class="payment-list"><h3>收款明细 · <a class="tenant-link" href="/cash-receipts/new?tenant_id={{.TenantID}}&amp;period={{.Period}}">补录现金</a></h3>{{if .Payments}}{{range .Payments}}<div class="payment-item"><span class="amount">{{.AmountDisplay}}</span><span class="mono">{{.DateDisplay}}</span><span class="payment-description">{{.Description}}</span><span class="mono">{{if eq .Source "现金"}}现金 · {{end}}参考号：{{.Reference}}</span><span class="mono">{{.ConfirmationSource}}{{if eq .Source "现金"}} · <a class="void-link" href="/cash-receipts/void?receipt_id={{.PaymentID}}">作废</a>{{end}}</span></div>{{end}}{{else}}<div class="tiny">本月暂无已确认收款。</div>{{end}}</div></td></tr>
          {{end}}</tbody>
	    </table></div>
	    {{else if gt .FilteredCount 0}}<div class="empty">当前页没有账单，请返回上一页。</div>
	    {{else if gt .TotalRows 0}}<div class="empty">没有符合当前筛选条件的租金账单，请调整搜索词或状态。</div>
	    {{else}}<div class="empty">本月还没有租金账单。请先添加租客并设置租金开始日期。</div>{{end}}
	    {{if gt .TotalPages 1}}<nav class="dashboard-pagination" aria-label="租客账单分页"><a class="btn subtle{{if eq .Page 1}} disabled{{end}}" href="{{rentDashboardURL .Period .SearchFilter .StatusFilter .SortFilter (dashboardPreviousPage .Page) .PageSize}}">上一页</a><span class="tiny">第 {{.Page}} / {{.TotalPages}} 页 · 共 {{.FilteredCount}} 户</span><a class="btn subtle{{if eq .Page .TotalPages}} disabled{{end}}" href="{{rentDashboardURL .Period .SearchFilter .StatusFilter .SortFilter (dashboardNextPage .Page .TotalPages) .PageSize}}">下一页</a></nav>{{end}}
	  </section>
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
  </script>
</body>
</html>
`))
