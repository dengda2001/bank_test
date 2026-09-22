package main

import (
	"context"
	"errors"
	"html/template"
	"strconv"
	"time"
)

type transactionRevokePreviewData struct {
	TransactionID                 string
	Description                   string
	AmountDisplay                 string
	AllocatedAmountDisplay        string
	CurrentRemainingAmountDisplay string
	RemainingAmountDisplay        string
	// ReturnTo 是确认后回哪去。列表页那一侧的撤销表单把它带过来，确认页再原样
	// 交给 POST，否则从 /transactions 点撤销会落到兜底的 /billing（老页面）。
	ReturnTo    string
	Allocations []transactionRevokePreviewAllocation
}

type transactionRevokePreviewAllocation struct {
	Kind          string
	AmountDisplay string
	TenantName    string
	PeriodDisplay string
	Note          string
}

type transactionRevokePreview struct {
	Source      paymentTransaction
	Allocations []paymentAllocation
	Obligations map[uint64]rentObligation
	TenantNames map[uint64]string
}

var revokePreviewTemplate = template.Must(template.New("revoke-preview").Parse(`<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>撤销流水匹配</title><style>
:root{--canvas:#f3f4f6;--surface:#fff;--ink:#111827;--muted:#4b5563;--border:#d1d5db;--accent:#2563eb;--accent-dark:#1d4ed8;--sans:system-ui,-apple-system,"Segoe UI",sans-serif}*{box-sizing:border-box}body{margin:0;padding:32px;background:var(--canvas);color:var(--ink);font-family:var(--sans)}.card{max-width:760px;margin:auto;padding:24px;border:1px solid var(--border);border-radius:12px;background:var(--surface)}h1{margin-top:0}dt{color:var(--muted);margin-top:14px}dd{margin:4px 0 0;overflow-wrap:anywhere}.allocation{padding:12px 0;border-top:1px solid var(--border)}label{display:block;margin-top:18px;color:var(--muted);font-weight:700}input{width:100%;box-sizing:border-box;padding:10px;border:1px solid var(--border);border-radius:8px;color:var(--ink);background:#f9fafb}input:focus-visible{outline:2px solid #93c5fd;outline-offset:2px}button,a{display:inline-block;margin-top:16px;padding:10px 14px;border:1px solid var(--accent);border-radius:8px;color:#fff;background:var(--accent);text-decoration:none;cursor:pointer;font:inherit;font-weight:700}button:hover,a:hover{background:var(--accent-dark)}.back{margin-left:8px;color:var(--ink);border-color:var(--border);background:var(--surface)}.muted{color:var(--muted);font-size:13px}@media(max-width:640px){body{padding:16px}.card{padding:18px}button,a,input{min-height:44px}}
</style></head><body><main class="card"><h1>撤销整笔匹配</h1><p class="muted">请核对原流水及所有有效分配。提交后只会作废这笔来源的当前有效分配，不删除银行原文。</p><dl><dt>原流水</dt><dd>{{.Description}}</dd><dt>来源金额</dt><dd>{{.AmountDisplay}}</dd><dt>当前已分配</dt><dd>{{.AllocatedAmountDisplay}}</dd><dt>当前余款</dt><dd>{{.CurrentRemainingAmountDisplay}}</dd><dt>撤销后余款</dt><dd>{{.RemainingAmountDisplay}}</dd></dl><h2>有效分配</h2>{{range .Allocations}}<div class="allocation"><strong>{{.Kind}}</strong> · {{.AmountDisplay}}{{if .TenantName}} · 租客：{{.TenantName}}{{end}}{{if .PeriodDisplay}} · 租金月：{{.PeriodDisplay}}{{end}}{{if .Note}}<div class="muted">备注：{{.Note}}</div>{{end}}</div>{{end}}<form method="post" action="/billing/revoke"><input type="hidden" name="transaction_id" value="{{.TransactionID}}"><input type="hidden" name="return_to" value="{{.ReturnTo}}"><label for="reason">撤销原因（必填）</label><input id="reason" name="reason" required maxlength="512"><button type="submit">确认撤销</button><a class="back" href="{{.ReturnTo}}">返回流水</a></form></main></body></html>`))

func (s *transactionService) previewTransactionRevoke(ctx context.Context, userID, transactionID uint64) (transactionRevokePreview, error) {
	if userID == 0 || transactionID == 0 {
		return transactionRevokePreview{}, errors.New("userID and transactionID are required")
	}
	var preview transactionRevokePreview
	if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", transactionID, userID).First(&preview.Source).Error; err != nil {
		return transactionRevokePreview{}, err
	}
	if err := s.db.WithContext(ctx).Where("payment_transaction_id = ? AND user_id = ?", transactionID, userID).Order("id ASC").Find(&preview.Allocations).Error; err != nil {
		return transactionRevokePreview{}, err
	}
	if summarizeTransactionAllocations(preview.Source, preview.Allocations).AllocatedCents == 0 {
		return transactionRevokePreview{}, errors.New("transaction has no effective allocations to revoke")
	}
	preview.Obligations = make(map[uint64]rentObligation)
	preview.TenantNames = make(map[uint64]string)
	var tenants []tenant
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).Find(&tenants).Error; err != nil {
		return transactionRevokePreview{}, err
	}
	for _, tenant := range tenants {
		preview.TenantNames[tenant.ID] = tenant.Name
	}
	for _, allocation := range preview.Allocations {
		if !ledgerAllocationIsEffective(allocation) || ledgerAllocationKind(allocation) != allocationKindRent || allocation.RentObligationID == nil {
			continue
		}
		var obligation rentObligation
		if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", *allocation.RentObligationID, userID).First(&obligation).Error; err != nil {
			return transactionRevokePreview{}, err
		}
		preview.Obligations[obligation.ID] = obligation
	}
	return preview, nil
}

func transactionRevokePreviewDataFromModel(preview transactionRevokePreview, returnTo string) transactionRevokePreviewData {
	summary := summarizeTransactionAllocations(preview.Source, preview.Allocations)
	rows := make([]transactionRevokePreviewAllocation, 0)
	for _, allocation := range preview.Allocations {
		if !ledgerAllocationIsEffective(allocation) {
			continue
		}
		row := transactionRevokePreviewAllocation{
			Kind:          map[string]string{allocationKindRent: "房租", allocationKindDeposit: "押金", allocationKindOther: "其他收入"}[ledgerAllocationKind(allocation)],
			AmountDisplay: formatMoney(centsToMoney(allocation.AmountCents), preview.Source.Currency, 2),
			Note:          allocation.Note,
		}
		if allocation.TenantID != nil {
			row.TenantName = preview.TenantNames[*allocation.TenantID]
			if row.TenantName == "" {
				row.TenantName = "租客 #" + strconv.FormatUint(*allocation.TenantID, 10)
			}
		}
		if allocation.RentObligationID != nil {
			if obligation, ok := preview.Obligations[*allocation.RentObligationID]; ok {
				row.PeriodDisplay = monthStart(obligation.PeriodMonth).Format("2006-01")
			}
		}
		rows = append(rows, row)
	}
	return transactionRevokePreviewData{
		TransactionID:                 strconv.FormatUint(preview.Source.ID, 10),
		Description:                   firstNonEmpty(preview.Source.Description, "无描述"),
		AmountDisplay:                 formatMoney(centsToMoney(preview.Source.AmountCents), preview.Source.Currency, 2),
		AllocatedAmountDisplay:        formatMoney(centsToMoney(summary.AllocatedCents), preview.Source.Currency, 2),
		CurrentRemainingAmountDisplay: formatMoney(centsToMoney(summary.RemainingCents), preview.Source.Currency, 2),
		RemainingAmountDisplay:        formatMoney(centsToMoney(preview.Source.AmountCents), preview.Source.Currency, 2),
		ReturnTo:                      firstNonEmpty(returnTo, "/billing"),
		Allocations:                   rows,
	}
}

type payerPreviewPageData struct {
	Rows []payerPreviewRow
}

type payerPreviewRow struct {
	TransactionID      string
	PayerName          string
	PayerID            string
	AmountDisplay      string
	ProposedTenantName string
	ProposedPeriod     string
	StatusLabel        string
	Reason             string
	TenantID           uint64
	CanConfirm         bool
}

var payerPreviewTemplate = template.Must(template.New("payer-preview").Parse(`<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>付款人历史预览</title><style>
:root{--canvas:#f3f4f6;--surface:#fff;--ink:#111827;--muted:#4b5563;--border:#d1d5db;--accent:#2563eb;--accent-dark:#1d4ed8;--sans:system-ui,-apple-system,"Segoe UI",sans-serif}*{box-sizing:border-box}body{margin:0;padding:32px;background:var(--canvas);color:var(--ink);font-family:var(--sans)}.card{max-width:1100px;margin:auto;padding:24px;border:1px solid var(--border);border-radius:12px;background:var(--surface)}table{width:100%;border-collapse:collapse}th,td{padding:12px 10px;border-bottom:1px solid var(--border);text-align:left;vertical-align:top}th{color:var(--muted);font-size:12px}input{padding:8px;border:1px solid var(--border);border-radius:8px;color:var(--ink);background:#f9fafb}input[type=checkbox]{accent-color:var(--accent)}input:focus-visible{outline:2px solid #93c5fd;outline-offset:2px}button,a{display:inline-block;padding:8px 12px;border:1px solid var(--accent);border-radius:8px;color:#fff;background:var(--accent);text-decoration:none;cursor:pointer;font:inherit;font-weight:700}button:hover,a:hover{background:var(--accent-dark)}.muted{color:var(--muted);font-size:13px}.action{display:grid;gap:6px;min-width:180px}.back{margin-top:16px;color:var(--ink);border-color:var(--border);background:var(--surface)}@media(max-width:760px){body{padding:16px}.card{padding:18px}.tp-scroll{overflow-x:auto}table{min-width:720px}button,a,input:not([type=checkbox]){min-height:44px}label.muted{display:inline-flex;align-items:center;gap:6px;min-height:44px}}
</style></head><body><main class="card"><h1>历史待处理付款人预览</h1><p class="muted">以下结果是逐笔预览。共享付款人、缺少月份或有冲突的流水不会被批量自动确认；请逐笔核对后提交。</p>{{if .Rows}}<div class="tp-scroll"><table><thead><tr><th>流水</th><th>付款人</th><th>金额</th><th>建议</th><th>状态／原因</th><th>操作</th></tr></thead><tbody>{{range .Rows}}<tr><td>#{{.TransactionID}}</td><td>{{.PayerName}}<br><span class="muted">{{.PayerID}}</span></td><td>{{.AmountDisplay}}</td><td>{{if .ProposedTenantName}}{{.ProposedTenantName}}{{else}}未确定{{end}}<br>{{if .ProposedPeriod}}{{.ProposedPeriod}}{{else}}月份待选{{end}}</td><td>{{.StatusLabel}}{{if .Reason}}<br><span class="muted">{{.Reason}}</span>{{end}}</td><td>{{if .CanConfirm}}<form class="action" method="post" action="/billing/payer/confirm"><input type="hidden" name="transaction_id" value="{{.TransactionID}}"><input type="hidden" name="tenant_id" value="{{.TenantID}}"><input name="period" value="{{.ProposedPeriod}}" placeholder="YYYY-MM" required><label class="muted"><input type="checkbox" name="remember_payer" value="1" checked> 记住付款人</label><button type="submit">确认此笔</button></form>{{else}}<span class="muted">需人工处理</span>{{end}}</td></tr>{{end}}</tbody></table></div>{{else}}<p class="muted">没有可预览的历史待处理流水。</p>{{end}}<a class="back" href="/billing">返回流水</a></main></body></html>`))

func (s *transactionService) previewHistoricalPayerMatches(ctx context.Context, userID uint64) ([]payerPreviewRow, error) {
	if userID == 0 {
		return nil, errors.New("userID is required")
	}
	var transactions []paymentTransaction
	if err := s.db.WithContext(ctx).Where("user_id = ? AND direction = ? AND match_status IN ?", userID, "income", pendingMatchStatuses).Order("transaction_time ASC, id ASC").Find(&transactions).Error; err != nil {
		return nil, err
	}
	var tenants []tenant
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).Find(&tenants).Error; err != nil {
		return nil, err
	}
	var payers []tenantPayer
	if err := s.db.WithContext(ctx).Where("user_id = ? AND removed_at IS NULL", userID).Find(&payers).Error; err != nil {
		return nil, err
	}
	var obligations []rentObligation
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).Find(&obligations).Error; err != nil {
		return nil, err
	}
	nameByID := make(map[uint64]string, len(tenants))
	for _, tenant := range tenants {
		nameByID[tenant.ID] = tenant.Name
	}
	rows := make([]payerPreviewRow, 0, len(transactions))
	for _, transaction := range transactions {
		decision := decideStrictRentMatch(paymentTransactionInputFromModel(transaction), payers, tenants, obligations)
		row := payerPreviewRow{
			TransactionID: strconv.FormatUint(transaction.ID, 10),
			PayerName:     firstNonEmpty(stringValue(transaction.PayerName), "未知付款人"),
			PayerID:       firstNonEmpty(stringValue(transaction.PayerID), "无付款人编号"),
			AmountDisplay: formatMoney(centsToMoney(transaction.AmountCents), transaction.Currency, 2),
			StatusLabel:   transactionMatchStatusLabel(decision.Status),
			Reason:        decision.Reason,
			TenantID:      decision.TenantID,
			CanConfirm:    decision.TenantID != 0 && decision.Status != "needs_review" && decision.Status != "unmatched",
		}
		if decision.TenantID != 0 {
			row.ProposedTenantName = nameByID[decision.TenantID]
		}
		if decision.RentObligationID != 0 {
			row.ProposedPeriod = decision.PeriodMonth.Format("2006-01")
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func transactionMatchStatusLabel(status string) string {
	return map[string]string{
		"matched":      "已匹配候选",
		"partial":      "部分匹配候选",
		"candidate":    "待确认",
		"needs_review": "需处理",
		"unmatched":    "未找到候选",
	}[status]
}

func (s *transactionService) confirmHistoricalPayerMatch(ctx context.Context, userID, transactionID, tenantID uint64, period time.Time, rememberPayer bool) error {
	if period.IsZero() {
		return errors.New("period is required")
	}
	return s.confirmRentMatch(ctx, userID, transactionID, tenantID, &period, rememberPayer)
}
