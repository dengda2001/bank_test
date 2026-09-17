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
			ActivePage:    "rent-dashboard",
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
	Filters     rentWorkspaceFilters
	Period      string
	PeriodLabel string
	Room        room
	Property    property
	Summary     rentWorkspaceRoomRow
	Tenants     []rentWorkspaceTenantRow
	Expenses    []manualExpense
	Error       string
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
	return rentRoomDetailPageData{Filters: filters, Period: period.Format("2006-01"), PeriodLabel: formatMonthLabel(period), Room: roomRow, Property: propertyRow, Summary: summary, Tenants: data.TenantRows, Expenses: expenses}, nil
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

var rentWorkspaceTemplate = newWorkspacePageTemplate("rent-workspace", template.FuncMap{
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
}, `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
  <title>RentOps 月度工作台</title>
  <style>`+workspacePageCSS+workspaceCalendarCSS+`
    .workspace-toolbar { display: flex; justify-content: space-between; align-items: end; gap: 16px; flex-wrap: wrap; margin-bottom: 18px; }
    .workspace-period { display: flex; align-items: center; gap: 8px; }
    .workspace-period input { min-height: 42px; }
    .workspace-tabs { display: flex; flex-wrap: wrap; gap: 8px; margin-bottom: 16px; }
    .workspace-tabs a { text-decoration: none; }
    .workspace-tabs a.active { color: var(--accent-bright); border-color: var(--accent-bright); background: var(--surface-accent); }
    .workspace-summary { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 12px; margin-bottom: 16px; }
    .workspace-summary .metric { min-height: 102px; }
    .workspace-filters { display: flex; align-items: end; flex-wrap: wrap; gap: 10px; padding: 16px 20px; border-bottom: 1px solid var(--border); background: var(--surface-muted); }
    .workspace-filters label { margin: 0; }
    .workspace-filters .search { flex: 1 1 220px; min-width: 200px; }
    .workspace-filters .property { flex: 0 1 190px; }
    .workspace-filters select, .workspace-filters input { min-height: 40px; background: var(--surface); }
    .workspace-table { min-width: 920px; }
    .workspace-table .amount { white-space: nowrap; }
    .workspace-table .status { white-space: nowrap; }
    .workspace-table a { color: inherit; }
    .workspace-table a:hover { color: var(--accent-bright); }
    .workspace-pager { display: flex; justify-content: space-between; align-items: center; gap: 12px; flex-wrap: wrap; padding: 16px 20px 20px; }
    .workspace-pager form { margin: 0; }
    .workspace-pager .pager-actions { display: flex; align-items: center; gap: 10px; }
    .workspace-pager .disabled { opacity: .45; pointer-events: none; }
    .workspace-empty { padding: 36px 20px; text-align: center; color: var(--foreground-muted); }
    .workspace-status { display: inline-block; border-radius: 999px; padding: 5px 9px; font-size: 12px; }
    .workspace-status.needs_review, .workspace-status.overdue { color: #991b1b; background: #fee2e2; }
    .workspace-status.partial { color: #92400e; background: #fef3c7; }
    .workspace-status.open { color: #1e40af; background: #dbeafe; }
    .workspace-status.paid { color: #065f46; background: #d1fae5; }
    .workspace-status.vacant { color: var(--foreground-muted); background: #e5e7eb; }
    .workspace-muted { color: var(--foreground-muted); }
    @media (max-width: 900px) { .workspace-summary { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
    @media (max-width: 640px) { .workspace-summary { grid-template-columns: 1fr; } .workspace-period { width: 100%; } .workspace-period input { flex: 1; } .workspace-table { min-width: 760px; } }
  </style>
</head>
<body>
  {{template "workspace-nav" .}}
  <div class="workspace"><main class="main">
    <div class="workspace-toolbar">
      <div><div class="eyebrow">RENTOPS · MONTHLY WORKSPACE</div><h1>月度收租工作台</h1><div class="tiny">{{.PeriodLabel}} · 所有金额来自同一套月度账务事实</div></div>
      <form class="workspace-period" method="get" action="/rent-dashboard"><label for="workspace-period">月份</label><input id="workspace-period" name="period" type="month" value="{{.Period}}"><input type="hidden" name="view" value="{{.View}}"><button class="btn" type="submit">查看</button></form>
    </div>
    {{if eq .Error "invalid_workspace_filter"}}<div class="notice error">筛选条件无效，已恢复安全默认值。</div>{{end}}
    {{if .Message}}<div class="notice ok">{{.Message}}</div>{{end}}
    <nav class="workspace-tabs" aria-label="工作台视角">
      <a class="btn subtle{{if eq .View "properties"}} active{{end}}" href="{{workspaceViewURL .Filters "properties" 0 0}}">房产视角</a>
      <a class="btn subtle{{if eq .View "rooms"}} active{{end}}" href="{{workspaceViewURL .Filters "rooms" .Filters.PropertyID 0}}">房间视角</a>
      <a class="btn subtle{{if eq .View "tenants"}} active{{end}}" href="{{workspaceViewURL .Filters "tenants" .Filters.PropertyID .Filters.RoomID}}">租客视角</a>
    </nav>
    <section class="workspace-summary" aria-label="月度汇总">
      <div class="panel metric"><div class="label">本月应收</div><strong>{{.Summary.ExpectedAmount}}</strong><span>{{.Summary.TotalRooms}} 间有效房间 · 空置 {{.Summary.VacantRooms}}</span></div>
      <div class="panel metric metric-success"><div class="label">已收租金</div><strong>{{.Summary.PaidAmount}}</strong><span>收缴率 {{.Summary.CollectionPercent}}% · {{.Summary.PaidRooms}} 间已交满</span></div>
      <div class="panel metric metric-warning"><div class="label">剩余未收</div><strong>{{.Summary.BalanceAmount}}</strong><span>{{.Summary.UnpaidRooms}} 间未交满</span></div>
      <div class="panel metric"><div class="label">经营净额</div><strong>{{.Summary.NetAmount}}</strong><span>已收租金 − 有效支出</span></div>
    </section>
    <section class="panel surface" aria-label="工作台列表">
      <form class="workspace-filters" method="get" action="/rent-dashboard">
        <input type="hidden" name="period" value="{{.Period}}"><input type="hidden" name="view" value="{{.View}}">
        <label class="search" for="workspace-search">搜索<input id="workspace-search" name="search" type="search" value="{{.Filters.Search}}" placeholder="房产、房间、租客"></label>
        <label class="property" for="workspace-property">房产<select id="workspace-property" name="property_id"><option value="">全部房产</option>{{range .PropertyOptions}}<option value="{{.ID}}"{{if eq $.Filters.PropertyID .ID}} selected{{end}}>{{.Name}}</option>{{end}}</select></label>
        <label for="workspace-status">状态<select id="workspace-status" name="status"><option value="all"{{if eq .Filters.Status "all"}} selected{{end}}>全部</option><option value="unpaid"{{if eq .Filters.Status "unpaid"}} selected{{end}}>未交满</option><option value="needs_review"{{if eq .Filters.Status "needs_review"}} selected{{end}}>待处理</option><option value="overdue"{{if eq .Filters.Status "overdue"}} selected{{end}}>逾期</option><option value="partial"{{if eq .Filters.Status "partial"}} selected{{end}}>部分缴纳</option><option value="open"{{if eq .Filters.Status "open"}} selected{{end}}>未到期未缴</option><option value="paid"{{if eq .Filters.Status "paid"}} selected{{end}}>已交满</option><option value="vacant"{{if eq .Filters.Status "vacant"}} selected{{end}}>空置</option></select></label>
        <input type="hidden" name="room_id" value="{{if .Filters.RoomID}}{{.Filters.RoomID}}{{end}}"><input type="hidden" name="sort" value="{{.Filters.Sort}}"><input type="hidden" name="page_size" value="{{.PageSize}}"><button class="btn" type="submit">筛选</button><a class="btn subtle" href="{{workspaceViewURL .Filters .View .Filters.PropertyID .Filters.RoomID}}">清除筛选</a>
      </form>
      {{if eq .View "properties"}}
      {{if .PropertyRows}}<div class="table-wrap"><table class="workspace-table"><thead><tr><th>房产</th><th>房间</th><th>已交满</th><th>未交满</th><th>应收</th><th>已收</th><th>未收</th><th>支出</th><th>经营净额</th><th>状态</th></tr></thead><tbody>{{range .PropertyRows}}<tr><td><a href="{{workspacePropertyURL $.Filters .PropertyID}}"><strong>{{.Name}}</strong></a>{{if .Address}}<br><span class="tiny">{{.Address}}</span>{{end}}</td><td>{{.TotalRooms}}</td><td>{{.PaidRooms}}</td><td>{{.UnpaidRooms}}</td><td class="amount">{{.ExpectedAmount}}</td><td class="amount">{{.PaidAmount}}</td><td class="amount">{{.BalanceAmount}}</td><td class="amount">{{.ExpenseAmount}}</td><td class="amount">{{.NetAmount}}</td><td><span class="workspace-status {{workspaceStatusClass .Status}}">{{.StatusLabel}}</span></td></tr>{{end}}</tbody></table></div>{{else}}<div class="workspace-empty">暂无符合条件的房产。</div>{{end}}
      {{else if eq .View "rooms"}}
      {{if .RoomRows}}<div class="table-wrap"><table class="workspace-table"><thead><tr><th>房间</th><th>房产</th><th>入住人数</th><th>应收</th><th>已收</th><th>未收</th><th>收缴率</th><th>到期日</th><th>状态</th></tr></thead><tbody>{{range .RoomRows}}<tr><td><a href="{{roomDetailURL .RoomID $.Period}}"><strong>{{.RoomLabel}}</strong></a>{{if .Address}}<br><span class="tiny">{{.Address}}</span>{{end}}</td><td>{{.PropertyName}}</td><td>{{.TenantCount}}</td><td class="amount">{{.ExpectedAmount}}</td><td class="amount">{{.PaidAmount}}</td><td class="amount">{{.BalanceAmount}}</td><td>{{if eq .Status "vacant"}}—{{else}}{{.CollectionPercent}}%{{end}}</td><td class="mono">{{.DueDate}}</td><td><span class="workspace-status {{workspaceStatusClass .Status}}">{{.StatusLabel}}</span></td></tr>{{end}}</tbody></table></div>{{else}}<div class="workspace-empty">暂无符合条件的房间。</div>{{end}}
      {{else}}
      {{if .TenantRows}}<div class="table-wrap"><table class="workspace-table"><thead><tr><th>租客</th><th>房间</th><th>个人责任</th><th>已覆盖</th><th>未付</th><th>付款来源</th><th>状态</th></tr></thead><tbody>{{range .TenantRows}}<tr><td><strong>{{.TenantName}}</strong>{{if .TenantAlias}}<br><span class="tiny">别名：{{.TenantAlias}}</span>{{end}}{{if .PaidByOther}}<br><span class="tiny">他人代付</span>{{end}}</td><td>{{.PropertyName}} · {{.RoomLabel}}</td><td class="amount">{{.ExpectedAmount}}</td><td class="amount">{{.PaidAmount}}</td><td class="amount">{{.BalanceAmount}}</td><td>{{if .Payments}}{{range .Payments}}{{.Source}} · {{.ConfirmationSource}}{{break}}{{end}}{{else}}—{{end}}</td><td><span class="workspace-status {{workspaceStatusClass .Status}}">{{.StatusLabel}}</span></td></tr>{{end}}</tbody></table></div>{{else}}<div class="workspace-empty">暂无符合条件的租客责任。</div>{{end}}
      {{end}}
      {{if .TotalRows}}<div class="workspace-pager"><form method="get" action="/rent-dashboard"><input type="hidden" name="period" value="{{.Period}}"><input type="hidden" name="view" value="{{.View}}"><input type="hidden" name="property_id" value="{{if .Filters.PropertyID}}{{.Filters.PropertyID}}{{end}}"><input type="hidden" name="room_id" value="{{if .Filters.RoomID}}{{.Filters.RoomID}}{{end}}"><input type="hidden" name="search" value="{{.Filters.Search}}"><input type="hidden" name="status" value="{{.Filters.Status}}"><input type="hidden" name="sort" value="{{.Filters.Sort}}"><label>每页<select name="page_size" onchange="this.form.submit()"><option value="12"{{if eq .PageSize 12}} selected{{end}}>12</option><option value="24"{{if eq .PageSize 24}} selected{{end}}>24</option><option value="50"{{if eq .PageSize 50}} selected{{end}}>50</option></select></label></form><span class="tiny">当前显示 {{.FilteredCount}} / {{.TotalRows}} 条</span><div class="pager-actions"><a class="btn subtle{{if eq .Page 1}} disabled{{end}}" href="{{workspaceURL .Filters (workspacePreviousPage .Page)}}">上一页</a><span class="tiny">第 {{.Page}} / {{.TotalPages}} 页</span><a class="btn subtle{{if eq .Page .TotalPages}} disabled{{end}}" href="{{workspaceURL .Filters (workspaceNextPage .Page .TotalPages)}}">下一页</a></div></div>{{end}}
    </section>
  </main></div>
</body>
</html>
`)

var rentRoomDetailTemplate = newWorkspacePageTemplate("rent-room-detail", template.FuncMap{
	"workspaceURL":         func(filters rentWorkspaceFilters) string { return rentWorkspaceURL(filters, 1) },
	"workspaceStatusClass": func(status string) string { return firstNonEmpty(status, "needs_review") },
	"formatExpenseAmount": func(row manualExpense) string {
		return formatMoney(centsToMoney(row.AmountCents), firstNonEmpty(row.Currency, ledgerCurrencyEUR), 2)
	},
}, `<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>RentOps 房间详情</title><style>`+workspacePageCSS+`
  .detail-head { display:flex; justify-content:space-between; align-items:start; gap:16px; flex-wrap:wrap; margin-bottom:18px; }
  .detail-summary { display:grid; grid-template-columns:repeat(4,minmax(0,1fr)); gap:12px; margin-bottom:16px; }
  .detail-table { min-width:760px; }
  .detail-section { margin-bottom:16px; }
  .detail-section h2 { margin:0; }
  .detail-list { display:grid; gap:10px; padding:18px 20px; }
  .detail-item { display:grid; grid-template-columns:1fr auto; gap:12px; border-bottom:1px solid var(--border); padding-bottom:10px; }
  .detail-item:last-child { border-bottom:0; padding-bottom:0; }
  .detail-status { display:inline-block; border-radius:999px; padding:5px 9px; font-size:12px; }
  .detail-status.needs_review,.detail-status.overdue{color:#991b1b;background:#fee2e2}.detail-status.partial{color:#92400e;background:#fef3c7}.detail-status.open{color:#1e40af;background:#dbeafe}.detail-status.paid{color:#065f46;background:#d1fae5}.detail-status.vacant{color:var(--foreground-muted);background:#e5e7eb}
  @media(max-width:700px){.detail-summary{grid-template-columns:repeat(2,minmax(0,1fr))}.detail-table{min-width:680px}}
</style></head><body>{{template "workspace-nav" .}}<div class="workspace"><main class="main">
  <div class="detail-head"><div><div class="eyebrow">{{.PeriodLabel}} · 房间详情</div><h1>{{.Room.RoomLabel}}</h1><div class="tiny">{{.Property.Name}}{{if .Property.Address}} · {{.Property.Address}}{{end}}</div></div><a class="btn subtle" href="{{workspaceURL .Filters}}">返回工作台</a></div>
  <section class="detail-summary"><div class="panel metric"><div class="label">应收</div><strong>{{.Summary.ExpectedAmount}}</strong></div><div class="panel metric metric-success"><div class="label">已收</div><strong>{{.Summary.PaidAmount}}</strong></div><div class="panel metric metric-warning"><div class="label">未收</div><strong>{{.Summary.BalanceAmount}}</strong></div><div class="panel metric"><div class="label">状态</div><strong><span class="detail-status {{workspaceStatusClass .Summary.Status}}">{{.Summary.StatusLabel}}</span></strong></div></section>
  <section class="panel detail-section"><div class="panel-head"><div><h2>租客责任</h2><div class="tiny">{{.PeriodLabel}} · 每一行是一条个人责任</div></div></div>{{if .Tenants}}<div class="table-wrap"><table class="detail-table"><thead><tr><th>租客</th><th>个人责任</th><th>已覆盖</th><th>未付</th><th>付款来源</th><th>状态</th></tr></thead><tbody>{{range .Tenants}}<tr><td><strong>{{.TenantName}}</strong>{{if .PaidByOther}}<br><span class="tiny">他人代付</span>{{end}}</td><td class="amount">{{.ExpectedAmount}}</td><td class="amount">{{.PaidAmount}}</td><td class="amount">{{.BalanceAmount}}</td><td>{{if .Payments}}{{range .Payments}}{{.Source}} · {{.ConfirmationSource}}{{break}}{{end}}{{else}}—{{end}}</td><td><span class="detail-status {{workspaceStatusClass .Status}}">{{.StatusLabel}}</span></td></tr>{{end}}</tbody></table></div>{{else}}<div class="detail-list"><div class="workspace-empty">本月暂无个人责任，可能是空置房。</div></div>{{end}}</section>
  <section class="panel detail-section"><div class="panel-head"><div><h2>房间支出</h2><div class="tiny">仅显示明确关联该房间的有效支出</div></div></div>{{if .Expenses}}<div class="detail-list">{{range .Expenses}}<div class="detail-item"><div><strong>{{.Description}}</strong><div class="tiny">{{.ExpenseDate.Format "2006-01-02"}} · {{.Category}}</div></div><span class="amount">{{formatExpenseAmount .}}</span></div>{{end}}</div>{{else}}<div class="detail-list"><div class="workspace-empty">本月暂无房间支出。</div></div>{{end}}</section>
</main></div></body></html>`)
