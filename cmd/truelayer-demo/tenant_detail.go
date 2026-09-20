package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	defaultTenantHistoryMonths = 12
	maxTenantHistoryMonths     = 120
	defaultTenantHistoryPage   = 10
)

func parseTenantHistoryRange(values formValues, now time.Time) (time.Time, time.Time, int, int, error) {
	current := monthStart(now)
	from := current.AddDate(0, -(defaultTenantHistoryMonths - 1), 0)
	to := current
	page, pageSize, err := parseTenantHistoryPagination(values)
	if err != nil {
		return time.Time{}, time.Time{}, 0, 0, err
	}
	// The prototype offers two presets, "近 12 个月" and "近 24 个月". A preset
	// wins over explicit months so the select and the URL never disagree.
	if raw := strings.TrimSpace(values.Get("range")); raw != "" {
		months, convErr := strconv.Atoi(raw)
		if convErr != nil || (months != defaultTenantHistoryMonths && months != 24) {
			return time.Time{}, time.Time{}, 0, 0, errors.New("range must be 12 or 24 months")
		}
		return current.AddDate(0, -(months - 1), 0), current, page, pageSize, nil
	}
	hasFrom := strings.TrimSpace(values.Get("from_month")) != ""
	hasTo := strings.TrimSpace(values.Get("to_month")) != ""
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
	if hasFrom && !hasTo {
		to = from.AddDate(0, defaultTenantHistoryMonths-1, 0)
	}
	if !hasFrom && hasTo {
		from = to.AddDate(0, -(defaultTenantHistoryMonths - 1), 0)
	}
	if from.After(to) {
		return time.Time{}, time.Time{}, 0, 0, errors.New("from_month cannot be after to_month")
	}
	monthCount := (to.Year()-from.Year())*12 + int(to.Month()-from.Month()) + 1
	if monthCount > maxTenantHistoryMonths {
		return time.Time{}, time.Time{}, 0, 0, fmt.Errorf("history range cannot exceed %d months", maxTenantHistoryMonths)
	}
	return from, to, page, pageSize, nil
}

// tenantHistoryRangeValue maps a resolved range back to the preset the select
// shows. The page only offers 12 and 24 months, so 24 is returned only when the
// range actually spans 24 months; anything else reads as 近 12 个月.
func tenantHistoryRangeValue(from, to time.Time) string {
	from = monthStart(from)
	to = monthStart(to)
	months := (to.Year()-from.Year())*12 + int(to.Month()-from.Month()) + 1
	if months == 24 {
		return "24"
	}
	return "12"
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
	if page > totalPages {
		return []tenantBillingMonth{}, totalPages, nil
	}
	start := (page - 1) * pageSize
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
	Pagination []paginationLink
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
	Reference  string
}

// tenantPaidByOtherRow is one allocation where the bank payer and the
// responsibility owner are different tenants. It backs the tenant detail page's
// "代付与被代付" table. It is derived from real allocations, not a placeholder.
type tenantPaidByOtherRow struct {
	PeriodLabel string
	PayerName   string
	OwnerName   string
	Amount      string
	Result      string
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
	cashRows, err := s.listCashBillingPaymentRows(ctx, userID, tenantID, fromMonth, toMonth)
	if err != nil {
		return tenantBillingHistoryPage{}, err
	}
	paymentRows = append(paymentRows, cashRows...)
	sortTenantBillingPaymentRows(paymentRows)
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
		Reference:  latestTenantBillingReference(history),
	}, nil
}

// latestTenantBillingReference returns the reference of the most recent payment
// already loaded for this tenant. history is ordered newest period first, and
// within an obligation payments are ordered oldest first, so the last reference
// of the first month that has one is the most recent.
func latestTenantBillingReference(history []tenantBillingMonth) string {
	for _, month := range history {
		var reference string
		for _, payment := range month.Payments {
			if value := strings.TrimSpace(payment.Reference); value != "" && value != "No reference" {
				reference = value
			}
		}
		if reference != "" {
			return reference
		}
	}
	return ""
}

type tenantDetailPageData struct {
	workspaceShell
	CurrentPeriod      string
	HistoryRange       string
	Message            string
	Error              string
	ReturnSearch       string
	EditURL            string
	ShowForm           bool
	Editing            bool
	Form               tenantRecord
	Rooms              []tenantRoomOption
	ReturnURL          string
	PostReturnURL      string
	Tenant             tenantRecord
	Payers             []tenantPayerRecord
	History            tenantBillingHistoryPage
	HasCurrentBilling  bool
	CurrentBilling     tenantBillingMonth
	PaidByOtherRows    []tenantPaidByOtherRow
	CashReceiptDrawer  *cashReceiptDrawerData
	CashReceiptOpenURL string
}

// listTenantPaidByOtherRows returns every confirmed rent allocation inside the
// range where the bank payer (the transaction's matched tenant) and the
// responsibility owner (the allocation's tenant) are different, and this tenant
// is one of the two. Rows where the payer is unknown are excluded rather than
// guessed: a missing matched tenant does not prove that someone else paid.
func (s *obligationService) listTenantPaidByOtherRows(ctx context.Context, userID, tenantID uint64, fromMonth, toMonth time.Time) ([]tenantPaidByOtherRow, error) {
	if userID == 0 || tenantID == 0 {
		return nil, errors.New("userID and tenantID are required")
	}
	fromMonth = monthStart(fromMonth)
	toMonth = monthStart(toMonth)
	if fromMonth.After(toMonth) {
		return nil, errors.New("fromMonth cannot be after toMonth")
	}
	type paidByOtherRaw struct {
		PeriodMonth time.Time `gorm:"column:period_month"`
		OwnerID     uint64    `gorm:"column:owner_id"`
		PayerID     uint64    `gorm:"column:payer_id"`
		PayerName   string    `gorm:"column:payer_name"`
		AmountCents int64     `gorm:"column:amount_cents"`
		Currency    string    `gorm:"column:currency"`
	}
	var raw []paidByOtherRaw
	if err := s.db.WithContext(ctx).Table("payment_allocations AS pa").
		Select("ro.period_month, pa.tenant_id AS owner_id, pt.matched_tenant_id AS payer_id, pt.payer_name, pa.amount_cents, pt.currency").
		Joins("JOIN rent_obligations AS ro ON ro.id = pa.rent_obligation_id AND ro.user_id = pa.user_id").
		Joins("JOIN payment_transactions AS pt ON pt.id = pa.payment_transaction_id AND pt.user_id = pa.user_id").
		Where("pa.user_id = ? AND pa.status = ? AND pa.allocation_kind = ? AND pt.direction = ?", userID, allocationStatusConfirmed, allocationKindRent, "income").
		Where("ro.period_month >= ? AND ro.period_month < ?", fromMonth, toMonth.AddDate(0, 1, 0)).
		Where("pt.matched_tenant_id IS NOT NULL AND pt.matched_tenant_id <> pa.tenant_id").
		Where("(pa.tenant_id = ? OR pt.matched_tenant_id = ?)", tenantID, tenantID).
		Order("ro.period_month DESC, pa.id ASC").Scan(&raw).Error; err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, nil
	}
	var tenants []tenant
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).Find(&tenants).Error; err != nil {
		return nil, err
	}
	nameByID := make(map[uint64]string, len(tenants))
	for _, row := range tenants {
		nameByID[row.ID] = firstNonEmpty(row.DisplayAlias, row.Name)
	}
	rows := make([]tenantPaidByOtherRow, 0, len(raw))
	for _, item := range raw {
		result := "代付他人"
		if item.OwnerID == tenantID {
			result = "本人被代付"
		}
		rows = append(rows, tenantPaidByOtherRow{
			PeriodLabel: formatMonthLabel(monthStart(item.PeriodMonth)),
			PayerName:   firstNonEmpty(nameByID[item.PayerID], strings.TrimSpace(item.PayerName), "未知付款人"),
			OwnerName:   firstNonEmpty(nameByID[item.OwnerID], "未知责任人"),
			Amount:      formatMoney(centsToMoney(item.AmountCents), firstNonEmpty(item.Currency, ledgerCurrencyEUR), 2),
			Result:      result,
		})
	}
	return rows, nil
}

var tenantDetailTemplate = newWorkspacePageTemplate("tenant-detail", nil, `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
  <title>RentOps Tenant Detail</title>
  <style>`+workspacePageCSS+`
    .tenant-detail-page { display: grid; gap: 16px; padding-bottom: 28px; }
    .tenant-detail-head { display: grid; gap: 14px; }
    .tenant-detail-back { color: var(--foreground-muted); font-size: 13px; font-weight: 700; text-decoration: none; }
    .tenant-detail-back:hover { color: var(--accent); }
    .tenant-identity-row { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 16px; }
    .tenant-identity { display: flex; align-items: center; gap: 14px; min-width: 0; }
    .tenant-identity .identity-mark { display: grid; place-items: center; width: 48px; height: 48px; flex: 0 0 auto; border-radius: 10px; color: var(--surface); background: var(--sidebar); font: 700 16px var(--mono); }
    .tenant-identity h1 { margin: 2px 0 5px; overflow-wrap: anywhere; }
    .tenant-identity-meta { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; color: var(--foreground-muted); font-size: 12px; }
    .tenant-identity-meta .detail-status { display: inline-flex; align-items: center; border: 1px solid var(--border); border-radius: 999px; padding: 4px 8px; background: var(--surface); color: var(--foreground-subtle); white-space: nowrap; }
    .tenant-identity-row > .actions { justify-content: flex-end; }
    .tenant-metrics { display: grid; grid-template-columns: repeat(4,minmax(0,1fr)); gap: 12px; }
    .tenant-metrics .metric { min-height: 104px; }
    .tenant-metrics .metric strong { white-space: nowrap; }
    .tenant-metric-state { font-size: 18px; }
    .detail-grid { display: grid; grid-template-columns: minmax(0, 1.8fr) minmax(320px, .9fr); grid-template-areas: "history side" "paidfor side" "responsibility side"; align-items: start; gap: 16px; margin-bottom: 16px; }
    .tenant-history-panel { grid-area: history; min-width: 0; overflow: hidden; }
    .tenant-responsibility-panel { grid-area: responsibility; min-width: 0; overflow: hidden; }
    .tenant-detail-side { grid-area: side; display: grid; gap: 16px; min-width: 0; }
    .tenant-detail-side > .panel { min-width: 0; overflow: hidden; }
    .tenant-detail-side .risk-note { padding: 14px 16px; border: 1px solid color-mix(in oklch, var(--warning) 30%, var(--border)); border-left: 3px solid var(--warning); border-radius: 8px; background: color-mix(in oklch, var(--warning) 6%, var(--surface)); color: var(--foreground-subtle); font-size: 12px; line-height: 1.6; }
    .tenant-responsibility-list { display: grid; padding: 6px 20px 10px; }
    .tenant-responsibility-row { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 13px 0; border-bottom: 1px solid var(--border); }
    .tenant-responsibility-row:last-child { border-bottom: 0; }
    .tenant-responsibility-row > span { display: grid; gap: 4px; }
    .tenant-responsibility-row small { color: var(--foreground-muted); }
    .tenant-responsibility-row strong { white-space: nowrap; }
    .profile-list { display: grid; grid-template-columns: 130px 1fr; gap: 10px 18px; margin: 0; padding: 18px 20px; }
    .profile-list dt { color: var(--foreground-muted); }
    .profile-list dd { margin: 0; overflow-wrap: anywhere; }
    .payer-list { display: grid; gap: 10px; padding: 18px 20px; }
    .payer-item { display: flex; justify-content: space-between; gap: 12px; align-items: start; padding: 10px 0; border-bottom: 1px solid var(--border); }
    .payer-item:last-child { border-bottom: 0; }
    .payer-meta { display: grid; gap: 4px; }
    /* 付款识别：参考码行与付款人列表同一张卡内，左右分别对齐卡内边距。 */
    .reference-row { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin: 0 20px; padding: 13px 0; border-bottom: 1px solid var(--border); }
    .reference-row > div { display: grid; gap: 4px; min-width: 0; }
    .reference-row strong { font: 700 13px var(--mono); overflow-wrap: anywhere; }
    .payer-badge { display: inline-block; border-radius: 999px; padding: 2px 7px; font-size: 11px; background: var(--surface-muted); color: var(--foreground-muted); }
    .tenant-paid-for-panel { grid-area: paidfor; min-width: 0; overflow: hidden; }
    .paid-for-table { min-width: 640px; }
    .flag { color: var(--warning); font-size: 12px; }
    .status { display: inline-block; border-radius: 999px; padding: 5px 9px; font-size: 12px; white-space: nowrap; }
    .status.open { color: #1e40af; background: #dbeafe; }
    .status.overdue, .status.needs_review { color: #991b1b; background: #fee2e2; }
    .status.partial { color: #92400e; background: #fef3c7; }
    .status.paid { color: #065f46; background: #d1fae5; }
    .status.voided { color: var(--foreground-muted); background: #e5e7eb; }
    .history-filter { display: flex; align-items: end; flex-wrap: wrap; gap: 10px; margin-bottom: 16px; padding: 16px 20px 0; }
    .history-filter label { margin: 0; }
    .history-filter input, .history-filter select { min-height: 40px; }
    .history-table { min-width: 760px; }
    .payment-list { display: grid; gap: 8px; margin-top: 10px; padding: 12px; background: var(--surface-muted); border-radius: 8px; }
    .payment-item { display: grid; grid-template-columns: 100px 145px 1fr; gap: 10px; font-size: 12px; padding-bottom: 8px; border-bottom: 1px solid var(--border); }
    .payment-item:last-child { border-bottom: 0; padding-bottom: 0; }
    .pagination { display: flex; gap: 8px; align-items: center; margin-top: 16px; padding: 0 20px 18px; }
    .tenant-detail-mobile-actions { display: none; }
    @media (max-width: 980px) { .detail-grid { grid-template-columns: 1fr; grid-template-areas: "history" "paidfor" "responsibility" "side"; } .tenant-detail-side { grid-template-columns: repeat(2,minmax(0,1fr)); align-items: start; } }
    @media (max-width: 680px) { .payment-item { grid-template-columns: 1fr 1fr; } }
    @media (max-width: 640px) {
      .tenant-detail-page { gap: 13px; padding-bottom: 116px; }
      .tenant-identity-row > .actions { display: none; }
      .tenant-identity-row { padding: 14px; border: 1px solid color-mix(in oklch, var(--sidebar) 18%, var(--border)); border-radius: 10px; background: var(--sidebar); color: var(--surface); }
      .tenant-identity-row .identity-mark { color: var(--sidebar); background: var(--surface); }
      .tenant-identity-row .brand-title, .tenant-identity-row h1 { color: var(--surface); }
      .tenant-identity-row .tenant-identity-meta { color: color-mix(in oklch, var(--surface) 74%, var(--sidebar)); }
      .tenant-identity-meta .detail-status { border-color: color-mix(in oklch, var(--surface) 30%, transparent); background: color-mix(in oklch, var(--surface) 10%, transparent); color: var(--surface); }
      .tenant-metrics { grid-template-columns: repeat(3,minmax(0,1fr)); gap: 8px; }
      .tenant-metrics .metric { min-height: 82px; padding: 10px; }
      .tenant-metrics .metric:nth-child(4) { display: none; }
      .tenant-metrics .metric .label { font-size: 10px; white-space: nowrap; }
      .tenant-metrics .metric strong { font-size: 13px; }
      .detail-grid { grid-template-areas: "responsibility" "paidfor" "side" "history"; }
      .tenant-detail-side { grid-template-columns: 1fr; }
      .tenant-detail-side { order: 1; }
      .tenant-detail-main { order: 2; }
      .tenant-responsibility-list { padding-left: 16px; padding-right: 16px; }
      .tenant-responsibility-row { padding: 9px 0; }
      .tenant-detail-mobile-actions { position: fixed; left: 0; right: 0; bottom: calc(68px + env(safe-area-inset-bottom)); z-index: 35; display: grid; grid-template-columns: 1fr 1fr; gap: 8px; padding: 10px 16px; background: color-mix(in oklch, var(--surface) 96%, transparent); border-top: 1px solid var(--border); backdrop-filter: blur(12px); }
      .tenant-detail-mobile-actions .btn { min-height: 44px; }
      /* 「档案信息」的标签列实测文字宽只有 32px，却固定占 130px，把值列压到
         139px：邮箱从域名中间裂开、房间地址折 4 行。窄屏改单列，值列拿到全部
         宽度；宽屏的 130px 1fr 保持不变。 */
      .profile-list { grid-template-columns: 1fr; gap: 4px 0; }
      .profile-list dt { font-size: 12px; }
      /* 移除按钮实测 62×40。 */
      .payer-item .btn { min-height: 44px; }
      /* 「付款识别」的复制按钮与参考码同在卡内，需满足 44px 可点击高度。 */
      .reference-row .btn { min-height: 44px; }
      /* 页内的 .history-filter input / select 比共享表的裸控件更具体，它的 40px
         会盖过共享窄屏块里的 44px（范围下拉实测 221×40）。 */
      .history-filter input, .history-filter select { min-height: 44px; }
    }
  </style>
</head>
<body><div class="app">
  {{template "workspace-nav" .}}
  <main class="content">
	<div class="tenant-detail-page">
<header class="tenant-detail-head"><a class="tenant-detail-back" href="/tenants{{if .ReturnSearch}}?search={{urlquery .ReturnSearch}}{{end}}">← 返回租客管理</a><div class="tenant-identity-row"><div class="tenant-identity"><div class="identity-mark">TN</div><div><div class="brand-title">租客详情</div><h1>{{if .Tenant.DisplayAlias}}{{.Tenant.DisplayAlias}}{{else}}{{.Tenant.Name}}{{end}}</h1><div class="tenant-identity-meta"><span>正式姓名：{{.Tenant.Name}}</span><span>{{if .Tenant.RoomLabel}}{{.Tenant.RoomLabel}}{{else}}{{.Tenant.RoomAddress}}{{end}}</span><span class="detail-status {{.Tenant.Status}}">{{if eq .Tenant.Status "active"}}有效租约{{else}}已停用{{end}}</span></div></div></div><div class="actions"><a class="btn" href="{{.EditURL}}">编辑资料</a><a class="btn primary" href="{{.CashReceiptOpenURL}}">录入现金收款</a></div></div></header>
	{{if eq .Message "cash_receipt_saved"}}<div class="notice ok" data-toast>现金收款已入账，并计入对应租金月份。</div>{{end}}
	{{if eq .Message "cash_receipt_voided"}}<div class="notice ok" data-toast>现金收款已撤销，原始记录与撤销原因已保留。</div>{{end}}
    {{if eq .Message "payer_added"}}<div class="notice ok" data-toast>付款人关系已保存。</div>{{end}}
    {{if eq .Message "payer_removed"}}<div class="notice ok" data-toast>付款人关系已移除，历史记录未改变。</div>{{end}}
    {{if eq .Error "invalid_payer"}}<div class="notice error">付款人名称不能为空，且字段长度必须有效。</div>{{end}}
	{{if eq .Message "tenant_updated"}}<div class="notice ok" data-toast>租客资料已更新。</div>{{end}}
	{{if .ShowForm}}{{template "tenant-form-drawer" .}}{{end}}
	{{if .CashReceiptDrawer}}{{template "cash-receipt-drawer" .CashReceiptDrawer}}{{end}}
	<div class="tenant-metrics" aria-label="本月租金摘要"><div class="panel metric"><div class="label">本月个人责任</div><strong>{{.Tenant.RentDisplay}}</strong><span>{{if .Tenant.RoomLabel}}{{.Tenant.RoomLabel}}{{else}}月度租金责任{{end}}</span></div><div class="panel metric"><div class="label">个人责任已覆盖</div><strong>{{if .HasCurrentBilling}}{{.CurrentBilling.PaidAmount}}{{else}}—{{end}}</strong><span>银行收款与现金收款</span></div><div class="panel metric"><div class="label">本月未收</div><strong>{{if .HasCurrentBilling}}{{.CurrentBilling.BalanceAmount}}{{else}}—{{end}}</strong><span>按个人租金责任计算</span></div><div class="panel metric"><div class="label">本月状态</div><strong class="tenant-metric-state">{{if .HasCurrentBilling}}{{.CurrentBilling.StatusLabel}}{{else}}暂无账单{{end}}</strong><span>{{.CurrentPeriod}}</span></div></div>
	<div class="detail-grid"><section class="panel surface tenant-history-panel" aria-labelledby="history-title"><div class="panel-head"><h2 id="history-title">缴费历史</h2><span class="tiny">{{.History.TotalRows}} 个适用月份</span></div>
	  <form class="history-filter" method="get" action="/tenants/{{.Tenant.ID}}"><label for="range">历史范围<select id="range" name="range"><option value="12"{{if eq .HistoryRange "12"}} selected{{end}}>近 12 个月</option><option value="24"{{if eq .HistoryRange "24"}} selected{{end}}>近 24 个月</option></select></label><input type="hidden" name="page_size" value="{{.History.PageSize}}"><button class="btn" type="submit">搜索</button></form>
	  {{if .History.Rows}}<div class="table-wrap"><table class="history-table"><thead><tr><th>月份</th><th>应缴日</th><th>应收</th><th>实收</th><th>未收</th><th>状态／来源</th></tr></thead><tbody>{{range .History.Rows}}<tr><td><strong>{{.PeriodLabel}}</strong></td><td class="mono">{{.DueDate}}</td><td class="amount">{{.ExpectedAmount}}</td><td class="amount">{{.PaidAmount}}</td><td class="amount">{{.BalanceAmount}}</td><td><span class="status {{.Status}}">{{.StatusLabel}}</span>{{if .Payments}}<div class="payment-list">{{range .Payments}}<div class="payment-item"><span class="amount">{{.AmountDisplay}}</span><span class="mono">{{.DateDisplay}}</span><span>{{.Source}}</span>{{if eq .Source "现金"}}<a class="void-link" href="/cash-receipts/void?receipt_id={{.PaymentID}}">撤销</a>{{end}}</div>{{end}}</div>{{else}}<div class="tiny">暂无有效收款</div>{{end}}</td></tr>{{end}}</tbody></table></div>{{else}}<div class="empty">所选期间没有适用账单。</div>{{end}}
	  {{if gt .History.TotalPages 1}}<div class="pagination">{{if .History.HasPrev}}<a class="btn subtle" href="/tenants/{{.Tenant.ID}}?from_month={{.History.FromPeriod}}&amp;to_month={{.History.ToPeriod}}&amp;page={{.History.PrevPage}}&amp;page_size={{.History.PageSize}}{{if $.ReturnSearch}}&amp;search={{urlquery $.ReturnSearch}}{{end}}">上一页</a>{{end}}<span class="pagination-numbers" aria-label="选择页码">{{range .History.Pagination}}{{if .Ellipsis}}<span class="pagination-ellipsis" aria-hidden="true">…</span>{{else if .Current}}<span class="pagination-number is-current" aria-current="page">{{.Number}}</span>{{else}}<a class="pagination-number" href="{{.URL}}">{{.Number}}</a>{{end}}{{end}}</span><span class="tiny">第 {{.History.Page}} / {{.History.TotalPages}} 页</span>{{if .History.HasNext}}<a class="btn subtle" href="/tenants/{{.Tenant.ID}}?from_month={{.History.FromPeriod}}&amp;to_month={{.History.ToPeriod}}&amp;page={{.History.NextPage}}&amp;page_size={{.History.PageSize}}{{if $.ReturnSearch}}&amp;search={{urlquery $.ReturnSearch}}{{end}}">下一页</a>{{end}}</div>{{end}}
	</section><section class="panel surface tenant-paid-for-panel" aria-labelledby="paid-for-title"><div class="panel-head"><div><h2 id="paid-for-title">代付与被代付</h2><p class="tiny">只影响责任覆盖，不改变每位住客的责任归属。</p></div></div>{{if .PaidByOtherRows}}<div class="table-wrap"><table class="paid-for-table"><thead><tr><th>责任月份</th><th>实际付款人</th><th>责任所有者</th><th class="amount">分配金额</th><th>结果</th></tr></thead><tbody>{{range .PaidByOtherRows}}<tr><td class="mono">{{.PeriodLabel}}</td><td>{{.PayerName}}{{if eq .Result "本人被代付"}} <span class="payer-badge">代付</span>{{end}}</td><td>{{.OwnerName}}</td><td class="amount">{{.Amount}}</td><td><span class="status paid">{{.Result}}</span></td></tr>{{end}}</tbody></table></div>{{else}}<div class="empty">所选期间没有代付或代收记录。</div>{{end}}</section><section class="panel surface tenant-responsibility-panel" aria-labelledby="responsibility-title"><div class="panel-head"><h2 id="responsibility-title">责任与代付</h2><span class="tiny">{{.CurrentPeriod}}</span></div><div class="tenant-responsibility-list"><div class="tenant-responsibility-row"><span><strong>本月个人责任</strong><small>按租约金额计算</small></span><strong>{{.Tenant.RentDisplay}}</strong></div><div class="tenant-responsibility-row"><span><strong>已确认收款</strong><small>{{if .HasCurrentBilling}}{{if .CurrentBilling.Payments}}{{range .CurrentBilling.Payments}}{{.Source}} {{.AmountDisplay}} · {{.DateDisplay}} {{end}}{{else}}暂无有效收款{{end}}{{else}}当月暂无账单{{end}}</small></span><strong>{{if .HasCurrentBilling}}{{.CurrentBilling.PaidAmount}}{{else}}—{{end}}</strong></div></div></section><aside class="tenant-detail-side"><section class="panel surface" aria-labelledby="profile-title"><div class="panel-head"><h2 id="profile-title">租客档案</h2><span class="tiny">档案信息</span></div><dl class="profile-list"><dt>邮箱</dt><dd>{{if .Tenant.Email}}{{.Tenant.Email}}{{else}}未填写{{end}}</dd><dt>房间</dt><dd>{{if .Tenant.RoomLabel}}{{.Tenant.RoomLabel}} · {{end}}{{.Tenant.RoomAddress}}</dd><dt>月租</dt><dd>{{.Tenant.RentDisplay}}，每月 {{.Tenant.DueDay}} 日</dd><dt>租期</dt><dd>{{.Tenant.RentStartDate}}{{if .Tenant.RentEndDate}} 至 {{.Tenant.RentEndDate}}{{end}}</dd><dt>计费开始</dt><dd>{{.Tenant.BillingStartDate}}</dd></dl></section><section class="panel surface" aria-labelledby="payer-title"><div class="panel-head"><h2 id="payer-title">付款识别</h2><span class="tiny">付款人名称与参考码</span></div><div class="reference-row"><div><strong>{{if .History.Reference}}{{.History.Reference}}{{else}}暂无付款参考码{{end}}</strong><span class="tiny">付款参考码</span></div>{{if .History.Reference}}<button class="btn subtle copy-reference" type="button" data-copy-value="{{.History.Reference}}">复制</button>{{end}}</div><div class="payer-list">{{if .Payers}}{{range .Payers}}<div class="payer-item"><div class="payer-meta"><strong>{{.Name}}</strong><span class="mono">{{if .PayerID}}ID: {{.PayerID}}{{else}}仅名称，无稳定 ID{{end}}</span>{{if .Shared}}<span class="flag">共享／冲突候选，不能自动选租客</span>{{end}}{{if .RemovedAt}}<span class="tiny">已移除：{{.RemovedAt}}</span>{{end}}</div>{{if not .RemovedAt}}<form method="post" action="/tenants/{{$.Tenant.ID}}/payers/remove"><input type="hidden" name="payer_id" value="{{.ID}}"><button class="btn subtle" type="submit">移除</button></form>{{end}}</div>{{end}}{{else}}<div class="empty">暂无付款人关系。</div>{{end}}</div><form method="post" action="/tenants/{{.Tenant.ID}}/payers" class="form"><label for="payer_name">付款人名称</label><input id="payer_name" name="payer_name" placeholder="例如 Mike" required><label for="payer_id_detail">稳定付款人 ID（可选）</label><input id="payer_id_detail" name="payer_id" placeholder="银行提供时填写"><button class="btn primary" type="submit">添加付款人</button></form></section><div class="risk-note">是否欠租只看个人责任的未覆盖金额。即使由他人代付，完整覆盖后也不会进入催收。</div></aside></div><div class="tenant-detail-mobile-actions"><a class="btn" href="{{.EditURL}}">编辑资料</a><a class="btn primary" href="{{.CashReceiptOpenURL}}">现金补录</a></div></div>
  </main>
</div><script>(function(){var buttons=document.querySelectorAll('.copy-reference');for(var i=0;i<buttons.length;i++){buttons[i].addEventListener('click',function(){var button=this;var value=button.getAttribute('data-copy-value')||'';if(!value){return;}var flash=function(){var original=button.textContent;button.textContent='已复制';window.setTimeout(function(){button.textContent=original;},1500);};if(navigator.clipboard&&navigator.clipboard.writeText){navigator.clipboard.writeText(value).then(flash,function(){});return;}var field=document.createElement('textarea');field.value=value;document.body.appendChild(field);field.select();try{document.execCommand('copy');flash();}catch(error){}document.body.removeChild(field);});}})();</script></body></html>`)

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
	showForm := r.URL.Query().Get("edit") == "1" || isTenantFormError(r.URL.Query().Get("error"))
	roomOptions := []tenantRoomOption(nil)
	formRecord := tenantRecord{}
	returnURL := tenantDetailReturnURL(r)
	postReturnURL := returnURL
	if showForm {
		roomOptions, err = a.listTenantRoomOptions(r.Context(), r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		formRecord = tenantEditFormRecordWithRoomDefaults(tenantRecordFromModel(tenantRow), roomOptions)
		formRows := []tenantRecord{formRecord}
		prepareTenants(formRows)
		formRecord = formRows[0]
		postQuery := url.Values{}
		if parsed, parseErr := url.ParseRequestURI(returnURL); parseErr == nil {
			postQuery = parsed.Query()
		}
		postQuery.Set("edit", "1")
		postReturnURL = r.URL.Path + "?" + postQuery.Encode()
	}
	history, err := newObligationService(a.db).listTenantBillingHistoryPage(r.Context(), userID, tenantID, fromMonth, toMonth, page, pageSize)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	history.Pagination = paginationLinks(history.Page, history.TotalPages, func(target int) string {
		query := url.Values{
			"from_month": {history.FromPeriod},
			"to_month":   {history.ToPeriod},
			"page":       {strconv.Itoa(target)},
			"page_size":  {strconv.Itoa(history.PageSize)},
		}
		if search := strings.TrimSpace(r.URL.Query().Get("search")); search != "" {
			query.Set("search", search)
		}
		return fmt.Sprintf("/tenants/%d?%s", tenantID, query.Encode())
	})
	paidByOtherRows, err := newObligationService(a.db).listTenantPaidByOtherRows(r.Context(), userID, tenantID, fromMonth, toMonth)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := tenantDetailPageData{
		workspaceShell: a.fillWorkspaceShell(r, workspaceShell{
			ActivePage:   "tenants",
			Username:     a.displayUsername(r),
			Environment:  a.cfg.Environment,
			FootNote:     "租客缴费详情",
			CompactTitle: firstNonEmpty(tenantRow.DisplayAlias, tenantRow.Name),
		}),
		CurrentPeriod:   monthStart(time.Now().UTC()).Format("2006-01"),
		HistoryRange:    tenantHistoryRangeValue(fromMonth, toMonth),
		Message:         r.URL.Query().Get("message"),
		Error:           r.URL.Query().Get("error"),
		ReturnSearch:    r.URL.Query().Get("search"),
		EditURL:         tenantDetailEditURL(r, tenantID, history),
		ShowForm:        showForm || isTenantFormError(r.URL.Query().Get("error")),
		Editing:         showForm || isTenantFormError(r.URL.Query().Get("error")),
		Form:            formRecord,
		Rooms:           roomOptions,
		ReturnURL:       returnURL,
		PostReturnURL:   postReturnURL,
		Tenant:          tenantRecordFromModel(tenantRow),
		Payers:          classifyTenantPayersWithAllRows(payers, allPayers),
		History:         history,
		PaidByOtherRows: paidByOtherRows,
	}
	data.CashReceiptOpenURL = cashReceiptHostOpenURL(returnURL)
	if drawer := cashReceiptDrawerFromRequest(r); drawer != nil {
		data.CashReceiptDrawer = drawer
	} else if r.URL.Query().Get("cash") == "1" {
		drawer, drawerErr := a.loadCashReceiptHostDrawer(r.Context(), r, userID, tenantID, monthStart(time.Now().UTC()), returnURL, nil)
		if drawerErr != nil {
			http.Error(w, drawerErr.Error(), http.StatusInternalServerError)
			return
		}
		data.CashReceiptDrawer = drawer
	}
	for _, billing := range history.Rows {
		if billing.Period == data.CurrentPeriod {
			data.HasCurrentBilling = true
			data.CurrentBilling = billing
			break
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tenantDetailTemplate.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func tenantDetailReturnURL(r *http.Request) string {
	query := r.URL.Query()
	query.Del("edit")
	query.Del("add")
	query.Del("cash")
	query.Del("message")
	query.Del("error")
	returnURL := r.URL.Path
	if encoded := query.Encode(); encoded != "" {
		returnURL += "?" + encoded
	}
	return returnURL
}

func tenantDetailEditURL(r *http.Request, tenantID uint64, history tenantBillingHistoryPage) string {
	query := r.URL.Query()
	query.Del("range")
	query.Del("message")
	query.Del("error")
	query.Set("from_month", history.FromPeriod)
	query.Set("to_month", history.ToPeriod)
	query.Set("page", strconv.Itoa(history.Page))
	query.Set("page_size", strconv.Itoa(history.PageSize))
	query.Set("edit", "1")
	return fmt.Sprintf("/tenants/%d?%s", tenantID, query.Encode())
}

func isTenantFormError(code string) bool {
	switch code {
	case "invalid_form", "tenant_has_payments", "tenant_room_locked", "invalid_tenant":
		return true
	default:
		return false
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

func classifyTenantPayersWithAllRows(rows, allRows []tenantPayer) []tenantPayerRecord {
	classified := classifyTenantPayerSharing(allRows)
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
