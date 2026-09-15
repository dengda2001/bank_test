package main

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	defaultTenantHistoryMonths = 12
	maxTenantHistoryMonths     = 120
	defaultTenantHistoryPage   = 12
)

func parseTenantHistoryRange(values formValues, now time.Time) (time.Time, time.Time, int, int, error) {
	current := monthStart(now)
	from := current.AddDate(0, -(defaultTenantHistoryMonths - 1), 0)
	to := current
	var err error
	if value := strings.TrimSpace(values.Get("from_month")); value != "" {
		from, err = parsePeriodMonth(value)
		if err != nil {
			return time.Time{}, time.Time{}, 0, 0, fmt.Errorf("invalid from_month: %w", err)
		}
	}
	if value := strings.TrimSpace(values.Get("to_month")); value != "" {
		to, err = parsePeriodMonth(value)
		if err != nil {
			return time.Time{}, time.Time{}, 0, 0, fmt.Errorf("invalid to_month: %w", err)
		}
	}
	if values.Get("from_month") != "" && values.Get("to_month") == "" {
		to = from.AddDate(0, defaultTenantHistoryMonths-1, 0)
	}
	if values.Get("from_month") == "" && values.Get("to_month") != "" {
		from = to.AddDate(0, -(defaultTenantHistoryMonths - 1), 0)
	}
	if from.After(to) {
		return time.Time{}, time.Time{}, 0, 0, errors.New("from_month cannot be after to_month")
	}
	monthCount := (to.Year()-from.Year())*12 + int(to.Month()-from.Month()) + 1
	if monthCount > maxTenantHistoryMonths {
		return time.Time{}, time.Time{}, 0, 0, fmt.Errorf("history range cannot exceed %d months", maxTenantHistoryMonths)
	}
	page, pageSize, err := parseTenantHistoryPagination(values)
	if err != nil {
		return time.Time{}, time.Time{}, 0, 0, err
	}
	return from, to, page, pageSize, nil
}

func parseTenantHistoryPagination(values formValues) (int, int, error) {
	page, pageSize := 1, defaultTenantHistoryPage
	var err error
	if raw := strings.TrimSpace(values.Get("page")); raw != "" {
		page, err = strconv.Atoi(raw)
		if err != nil || page < 1 {
			return 0, 0, errors.New("page must be positive")
		}
	}
	if raw := strings.TrimSpace(values.Get("page_size")); raw != "" {
		pageSize, err = strconv.Atoi(raw)
		if err != nil || pageSize < 1 || pageSize > 50 {
			return 0, 0, errors.New("page_size must be between 1 and 50")
		}
	}
	return page, pageSize, nil
}

func paginateTenantBillingMonths(rows []tenantBillingMonth, page, pageSize int) ([]tenantBillingMonth, int, error) {
	if page < 1 || pageSize < 1 {
		return nil, 0, errors.New("page and pageSize must be positive")
	}
	totalPages := (len(rows) + pageSize - 1) / pageSize
	if totalPages == 0 {
		return []tenantBillingMonth{}, 0, nil
	}
	start := (page - 1) * pageSize
	if start >= len(rows) {
		return []tenantBillingMonth{}, totalPages, nil
	}
	end := start + pageSize
	if end > len(rows) {
		end = len(rows)
	}
	return rows[start:end], totalPages, nil
}

type tenantBillingHistoryPage struct {
	Rows       []tenantBillingMonth
	TotalRows  int
	TotalPages int
	Page       int
	PageSize   int
	FromMonth  time.Time
	ToMonth    time.Time
	FromPeriod string
	ToPeriod   string
	HasPrev    bool
	HasNext    bool
	PrevPage   int
	NextPage   int
}

func (s *obligationService) listTenantBillingHistoryPage(ctx context.Context, userID, tenantID uint64, fromMonth, toMonth time.Time, page, pageSize int) (tenantBillingHistoryPage, error) {
	if userID == 0 || tenantID == 0 {
		return tenantBillingHistoryPage{}, errors.New("userID and tenantID are required")
	}
	fromMonth = monthStart(fromMonth)
	toMonth = monthStart(toMonth)
	if fromMonth.After(toMonth) {
		return tenantBillingHistoryPage{}, errors.New("fromMonth cannot be after toMonth")
	}
	var tenantRow tenant
	if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", tenantID, userID).First(&tenantRow).Error; err != nil {
		return tenantBillingHistoryPage{}, err
	}
	if err := s.generateMonthlyObligations(ctx, userID, fromMonth, toMonth); err != nil {
		return tenantBillingHistoryPage{}, err
	}
	var obligations []rentObligation
	if err := s.db.WithContext(ctx).
		Where("user_id = ? AND tenant_id = ? AND period_month >= ? AND period_month < ?", userID, tenantID, fromMonth, toMonth.AddDate(0, 1, 0)).
		Order("period_month DESC, id DESC").Find(&obligations).Error; err != nil {
		return tenantBillingHistoryPage{}, err
	}
	var paymentRows []tenantBillingPaymentRow
	if err := s.db.WithContext(ctx).Table("payment_allocations AS pa").
		Select("pa.tenant_id, pa.rent_obligation_id AS obligation_id, pa.amount_cents, pt.currency, pt.source, pt.transaction_time, pt.description, pt.reference, pa.confirmation_source").
		Joins("JOIN rent_obligations AS ro ON ro.id = pa.rent_obligation_id AND ro.user_id = pa.user_id").
		Joins("JOIN payment_transactions AS pt ON pt.id = pa.payment_transaction_id AND pt.user_id = pa.user_id").
		Where("pa.user_id = ? AND pa.tenant_id = ? AND pa.status = ? AND pa.allocation_kind = ? AND pt.direction = ? AND ro.period_month >= ? AND ro.period_month < ?", userID, tenantID, allocationStatusConfirmed, allocationKindRent, "income", fromMonth, toMonth.AddDate(0, 1, 0)).
		Order("ro.period_month DESC, pt.transaction_time ASC, pa.id ASC").Scan(&paymentRows).Error; err != nil {
		return tenantBillingHistoryPage{}, err
	}
	history := buildTenantBillingHistory([]tenant{tenantRow}, obligations, paymentRows, time.Now().UTC())[tenantID]
	rows, totalPages, err := paginateTenantBillingMonths(history, page, pageSize)
	if err != nil {
		return tenantBillingHistoryPage{}, err
	}
	return tenantBillingHistoryPage{
		Rows:       rows,
		TotalRows:  len(history),
		TotalPages: totalPages,
		Page:       page,
		PageSize:   pageSize,
		FromMonth:  fromMonth,
		ToMonth:    toMonth,
		FromPeriod: fromMonth.Format("2006-01"),
		ToPeriod:   toMonth.Format("2006-01"),
		HasPrev:    page > 1,
		HasNext:    page < totalPages,
		PrevPage:   page - 1,
		NextPage:   page + 1,
	}, nil
}

type tenantDetailPageData struct {
	Username    string
	Environment string
	Message     string
	Error       string
	Tenant      tenantRecord
	Payers      []tenantPayerRecord
	History     tenantBillingHistoryPage
}

var tenantDetailTemplate = template.Must(template.New("tenant-detail").Parse(`<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
  <title>RentOps Tenant Detail</title>
  <style>` + workspacePageCSS + `
    .detail-grid { display: grid; grid-template-columns: minmax(0, 1.1fr) minmax(320px, .9fr); gap: 16px; margin-bottom: 16px; }
    .profile-list { display: grid; grid-template-columns: 130px 1fr; gap: 10px 18px; margin: 0; }
    .profile-list dt { color: var(--foreground-muted); }
    .profile-list dd { margin: 0; overflow-wrap: anywhere; }
    .payer-list { display: grid; gap: 10px; }
    .payer-item { display: flex; justify-content: space-between; gap: 12px; align-items: start; padding: 10px 0; border-bottom: 1px solid var(--border); }
    .payer-item:last-child { border-bottom: 0; }
    .payer-meta { display: grid; gap: 4px; }
    .flag { color: #ffe0a7; font-size: 12px; }
    .status { display: inline-block; border-radius: 999px; padding: 5px 9px; font-size: 12px; white-space: nowrap; }
    .status.open { color: #d8dcff; background: rgba(104,114,217,.12); }
    .status.overdue, .status.needs_review { color: #ffd0ce; background: rgba(255,139,134,.10); }
    .status.partial { color: #ffe0a7; background: rgba(233,184,114,.10); }
    .status.paid { color: #cbffe1; background: rgba(125,211,168,.10); }
    .status.voided { color: var(--foreground-muted); background: rgba(255,255,255,.08); }
    .history-filter { display: flex; align-items: end; flex-wrap: wrap; gap: 10px; margin-bottom: 16px; }
    .history-filter label { margin: 0; }
    .history-filter input { min-height: 40px; }
    .history-table { min-width: 760px; }
    .payment-list { display: grid; gap: 8px; margin-top: 10px; padding: 12px; background: rgba(255,255,255,.025); border-radius: 12px; }
    .payment-item { display: grid; grid-template-columns: 100px 145px 1fr 150px; gap: 10px; font-size: 12px; padding-bottom: 8px; border-bottom: 1px solid var(--border); }
    .payment-item:last-child { border-bottom: 0; padding-bottom: 0; }
    .pagination { display: flex; gap: 8px; align-items: center; margin-top: 16px; }
    @media (max-width: 900px) { .detail-grid { grid-template-columns: 1fr; } }
    @media (max-width: 680px) { .payment-item { grid-template-columns: 1fr 1fr; } }
  </style>
</head>
<body><div class="app">
  <aside class="sidebar" aria-label="Main navigation">
    <div class="side-brand"><div class="mark">R</div><div><div class="brand-title">RentOps</div><div class="brand-meta">{{.Environment}} workspace</div></div></div>
    <nav class="nav"><a href="/rent-dashboard"><span class="glyph">总</span><span>月度总览</span></a><a href="/billing"><span class="glyph">流</span><span>银行流水</span></a><a href="/tenants" class="active"><span class="glyph">租</span><span>租客管理</span></a><a href="/expenses"><span class="glyph">支</span><span>支出记录</span></a></nav>
    <div class="side-foot">当前用户：{{.Username}}<br>租客缴费详情</div>
  </aside>
  <main class="content">
    <header class="topbar"><div><div class="brand-title">租客详情</div><h1>{{if .Tenant.DisplayAlias}}{{.Tenant.DisplayAlias}}{{else}}{{.Tenant.Name}}{{end}}</h1><div class="tiny">正式姓名：{{.Tenant.Name}}</div></div><div class="actions"><a class="btn" href="/tenants">返回租客列表</a><a class="btn primary" href="/tenants?edit={{.Tenant.ID}}">编辑资料</a></div></header>
    {{if eq .Message "payer_added"}}<div class="notice ok">付款人关系已保存。</div>{{end}}
    {{if eq .Message "payer_removed"}}<div class="notice ok">付款人关系已移除，历史记录未改变。</div>{{end}}
    {{if eq .Error "invalid_payer"}}<div class="notice error">付款人名称不能为空，且字段长度必须有效。</div>{{end}}
    <div class="detail-grid">
      <section class="panel surface" aria-labelledby="profile-title"><div class="panel-head"><h2 id="profile-title">档案信息</h2><span class="tiny">{{.Tenant.Status}}</span></div><dl class="profile-list"><dt>邮箱</dt><dd>{{if .Tenant.Email}}{{.Tenant.Email}}{{else}}未填写{{end}}</dd><dt>房间</dt><dd>{{if .Tenant.RoomLabel}}{{.Tenant.RoomLabel}} · {{end}}{{.Tenant.RoomAddress}}</dd><dt>月租</dt><dd>{{.Tenant.RentDisplay}}，每月 {{.Tenant.DueDay}} 日</dd><dt>租期</dt><dd>{{.Tenant.RentStartDate}}{{if .Tenant.RentEndDate}} 至 {{.Tenant.RentEndDate}}{{end}}</dd><dt>计费开始</dt><dd>{{.Tenant.BillingStartDate}}</dd></dl></section>
      <section class="panel surface" aria-labelledby="payer-title"><div class="panel-head"><h2 id="payer-title">付款人关系</h2><span class="tiny">名称可用，稳定 ID 可选</span></div><div class="payer-list">{{if .Payers}}{{range .Payers}}<div class="payer-item"><div class="payer-meta"><strong>{{.Name}}</strong><span class="mono">{{if .PayerID}}ID: {{.PayerID}}{{else}}仅名称，无稳定 ID{{end}}</span>{{if .Shared}}<span class="flag">共享／冲突候选，不能自动选租客</span>{{end}}{{if .RemovedAt}}<span class="tiny">已移除：{{.RemovedAt}}</span>{{end}}</div>{{if not .RemovedAt}}<form method="post" action="/tenants/{{$.Tenant.ID}}/payers/remove"><input type="hidden" name="payer_id" value="{{.ID}}"><button class="btn subtle" type="submit">移除</button></form>{{end}}</div>{{end}}{{else}}<div class="empty">暂无付款人关系。</div>{{end}}</div><form method="post" action="/tenants/{{.Tenant.ID}}/payers" class="form"><label for="payer_name">付款人名称</label><input id="payer_name" name="payer_name" placeholder="例如 Mike" required><label for="payer_id_detail">稳定付款人 ID（可选）</label><input id="payer_id_detail" name="payer_id" placeholder="银行提供时填写"><button class="btn primary" type="submit">添加付款人</button></form></section>
    </div>
    <section class="panel surface" aria-labelledby="history-title"><div class="panel-head"><h2 id="history-title">缴费历史</h2><span class="tiny">{{.History.TotalRows}} 个适用月份</span></div>
      <form class="history-filter" method="get" action="/tenants/{{.Tenant.ID}}"><label for="from_month">起始月份<input id="from_month" name="from_month" type="month" value="{{.History.FromPeriod}}" required></label><label for="to_month">结束月份<input id="to_month" name="to_month" type="month" value="{{.History.ToPeriod}}" required></label><input type="hidden" name="page_size" value="{{.History.PageSize}}"><button class="btn" type="submit">查询历史</button></form>
      {{if .History.Rows}}<div class="table-wrap"><table class="history-table"><thead><tr><th>月份</th><th>应缴日</th><th>应收</th><th>实收</th><th>未收</th><th>状态／来源</th></tr></thead><tbody>{{range .History.Rows}}<tr><td><strong>{{.PeriodLabel}}</strong></td><td class="mono">{{.DueDate}}</td><td class="amount">{{.ExpectedAmount}}</td><td class="amount">{{.PaidAmount}}</td><td class="amount">{{.BalanceAmount}}</td><td><span class="status {{.Status}}">{{.StatusLabel}}</span>{{if .Payments}}<div class="payment-list">{{range .Payments}}<div class="payment-item"><span class="amount">{{.AmountDisplay}}</span><span class="mono">{{.DateDisplay}}</span><span>{{.Source}}</span><span class="mono">{{.Reference}}</span></div>{{end}}</div>{{else}}<div class="tiny">暂无有效收款</div>{{end}}</td></tr>{{end}}</tbody></table></div>{{else}}<div class="empty">所选期间没有适用账单。</div>{{end}}
      {{if gt .History.TotalPages 1}}<div class="pagination">{{if .History.HasPrev}}<a class="btn subtle" href="/tenants/{{.Tenant.ID}}?from_month={{.History.FromPeriod}}&amp;to_month={{.History.ToPeriod}}&amp;page={{.History.PrevPage}}&amp;page_size={{.History.PageSize}}">上一页</a>{{end}}<span class="tiny">第 {{.History.Page}} / {{.History.TotalPages}} 页</span>{{if .History.HasNext}}<a class="btn subtle" href="/tenants/{{.Tenant.ID}}?from_month={{.History.FromPeriod}}&amp;to_month={{.History.ToPeriod}}&amp;page={{.History.NextPage}}&amp;page_size={{.History.PageSize}}">下一页</a>{{end}}</div>{{end}}
    </section>
  </main>
</div></body></html>`))

func (a *app) handleTenantDetail(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := a.currentUserID(r)
	if !ok || a.db == nil {
		http.Error(w, "database session required", http.StatusBadRequest)
		return
	}
	rawID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/tenants/"), "/")
	tenantID, err := strconv.ParseUint(strings.TrimSpace(rawID), 10, 64)
	if err != nil || tenantID == 0 {
		http.NotFound(w, r)
		return
	}
	fromMonth, toMonth, page, pageSize, err := parseTenantHistoryRange(r.URL.Query(), time.Now().UTC())
	if err != nil {
		http.Error(w, "invalid history range", http.StatusBadRequest)
		return
	}
	var tenantRow tenant
	if err := a.db.WithContext(r.Context()).Where("id = ? AND user_id = ?", tenantID, userID).First(&tenantRow).Error; err != nil {
		http.NotFound(w, r)
		return
	}
	payers, err := newTenantService(a.db).listTenantPayers(r.Context(), userID, tenantID, true)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var allPayers []tenantPayer
	if err := a.db.WithContext(r.Context()).Where("user_id = ?", userID).Find(&allPayers).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	history, err := newObligationService(a.db).listTenantBillingHistoryPage(r.Context(), userID, tenantID, fromMonth, toMonth, page, pageSize)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := tenantDetailPageData{
		Username:    a.cfg.AdminUsername,
		Environment: a.cfg.Environment,
		Message:     r.URL.Query().Get("message"),
		Error:       r.URL.Query().Get("error"),
		Tenant:      tenantRecordFromModel(tenantRow),
		Payers:      classifyTenantPayersWithAllRows(payers, allPayers),
		History:     history,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tenantDetailTemplate.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (a *app) handleTenantSubroute(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSuffix(r.URL.Path, "/")
	if strings.HasSuffix(path, "/payers") && r.Method == http.MethodPost {
		a.handleAddTenantPayer(w, r)
		return
	}
	if strings.HasSuffix(path, "/payers/remove") && r.Method == http.MethodPost {
		a.handleRemoveTenantPayer(w, r)
		return
	}
	a.handleTenantDetail(w, r)
}

func tenantIDFromPayerPath(path string) (uint64, bool) {
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(strings.TrimPrefix(path, "/tenants/"), "/")
	if len(parts) < 2 {
		return 0, false
	}
	id, err := strconv.ParseUint(parts[0], 10, 64)
	return id, err == nil && id > 0
}

func (a *app) handleAddTenantPayer(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	userID, ok := a.currentUserID(r)
	if !ok || a.db == nil {
		http.Error(w, "database session required", http.StatusBadRequest)
		return
	}
	tenantID, ok := tenantIDFromPayerPath(r.URL.Path)
	if !ok || !strings.HasSuffix(strings.TrimSuffix(r.URL.Path, "/"), "/payers") {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, fmt.Sprintf("/tenants/%d?error=invalid_payer", tenantID), http.StatusFound)
		return
	}
	_, err := newTenantService(a.db).addTenantPayer(r.Context(), userID, tenantID, tenantPayerInput{
		Name:    r.Form.Get("payer_name"),
		PayerID: r.Form.Get("payer_id"),
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			http.NotFound(w, r)
			return
		}
		if isValidationError(err) {
			http.Redirect(w, r, fmt.Sprintf("/tenants/%d?error=invalid_payer", tenantID), http.StatusFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/tenants/%d?message=payer_added", tenantID), http.StatusFound)
}

func (a *app) handleRemoveTenantPayer(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	userID, ok := a.currentUserID(r)
	if !ok || a.db == nil {
		http.Error(w, "database session required", http.StatusBadRequest)
		return
	}
	tenantID, ok := tenantIDFromPayerPath(r.URL.Path)
	if !ok || !strings.HasSuffix(strings.TrimSuffix(r.URL.Path, "/"), "/payers/remove") {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, fmt.Sprintf("/tenants/%d?error=invalid_payer", tenantID), http.StatusFound)
		return
	}
	payerID, err := strconv.ParseUint(strings.TrimSpace(r.Form.Get("payer_id")), 10, 64)
	if err != nil || payerID == 0 {
		http.Redirect(w, r, fmt.Sprintf("/tenants/%d?error=invalid_payer", tenantID), http.StatusFound)
		return
	}
	err = newTenantService(a.db).removeTenantPayer(r.Context(), userID, tenantID, payerID, userID, "")
	if errors.Is(err, gorm.ErrRecordNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/tenants/%d?message=payer_removed", tenantID), http.StatusFound)
}

func classifyTenantPayers(rows []tenantPayer) []tenantPayerRecord {
	return classifyTenantPayersWithAllRows(rows, rows)
}

func classifyTenantPayersWithAllRows(rows, allRows []tenantPayer) []tenantPayerRecord {
	classified := classifyTenantPayerSharing(allRows)
	if len(rows) == len(allRows) {
		return classified
	}
	byID := make(map[uint64]tenantPayerRecord, len(classified))
	for _, row := range classified {
		id, _ := strconv.ParseUint(row.ID, 10, 64)
		byID[id] = row
	}
	result := make([]tenantPayerRecord, 0, len(rows))
	for _, row := range rows {
		result = append(result, byID[row.ID])
	}
	return result
}
