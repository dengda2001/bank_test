package main

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

type cashReceiptFormInput struct {
	TenantID       uint64
	Period         time.Time
	AmountCents    int64
	Currency       string
	ReceivedAt     time.Time
	Note           string
	IdempotencyKey string
}

type cashReceiptFormData struct {
	Username          string
	Environment       string
	Tenants           []tenant
	Tenant            tenant
	TenantSelected    bool
	TenantID          string
	Period            string
	ObligationID      uint64
	Amount            string
	Currency          string
	ReceivedAt        string
	Note              string
	IdempotencyKey    string
	ExpectedAmount    string
	CurrentPaidAmount string
	CurrentRemaining  string
	AfterPaidAmount   string
	AfterRemaining    string
	Preview           bool
	Error             string
}

type cashReceiptVoidPageData struct {
	Username      string
	Environment   string
	Receipt       cashReceipt
	Tenant        tenant
	AmountDisplay string
	DateDisplay   string
	Error         string
	AlreadyVoided bool
}

func parseCashReceiptForm(values formValues, userID uint64) (cashReceiptFormInput, error) {
	if userID == 0 {
		return cashReceiptFormInput{}, fmt.Errorf("userID is required")
	}
	tenantID, err := parsePositiveUint(values.Get("tenant_id"))
	if err != nil {
		return cashReceiptFormInput{}, fmt.Errorf("tenant_id: %w", err)
	}
	periodValue := strings.TrimSpace(values.Get("period"))
	if periodValue == "" {
		return cashReceiptFormInput{}, fmt.Errorf("period is required")
	}
	period, err := parsePeriodMonth(periodValue)
	if err != nil {
		return cashReceiptFormInput{}, fmt.Errorf("period: %w", err)
	}
	amount, err := parsePositiveAmount(values.Get("amount"))
	if err != nil {
		return cashReceiptFormInput{}, err
	}
	amountCents := moneyToCents(amount)
	if amountCents <= 0 {
		return cashReceiptFormInput{}, fmt.Errorf("amount must be at least one cent")
	}
	currency, err := normalizeLedgerCurrency(firstNonEmpty(strings.ToUpper(strings.TrimSpace(values.Get("currency"))), ledgerCurrencyEUR))
	if err != nil {
		return cashReceiptFormInput{}, err
	}
	receivedAt, err := parseDate(values.Get("received_at"))
	if err != nil {
		return cashReceiptFormInput{}, fmt.Errorf("received_at: %w", err)
	}
	idempotencyKey := strings.TrimSpace(values.Get("idempotency_key"))
	if idempotencyKey == "" {
		return cashReceiptFormInput{}, fmt.Errorf("idempotency key is required")
	}
	return cashReceiptFormInput{
		TenantID:       tenantID,
		Period:         period,
		AmountCents:    amountCents,
		Currency:       currency,
		ReceivedAt:     dateOnly(receivedAt),
		Note:           strings.TrimSpace(values.Get("note")),
		IdempotencyKey: idempotencyKey,
	}, nil
}

func (a *app) cashReceiptInputForPeriod(ctx context.Context, userID uint64, values formValues) (cashReceiptInput, cashReceiptFormInput, error) {
	draft, err := parseCashReceiptForm(values, userID)
	if err != nil {
		return cashReceiptInput{}, cashReceiptFormInput{}, err
	}
	var obligation rentObligation
	if err := a.db.WithContext(ctx).Where("user_id = ? AND tenant_id = ? AND period_month = ?", userID, draft.TenantID, draft.Period).First(&obligation).Error; err != nil {
		return cashReceiptInput{}, draft, err
	}
	return cashReceiptInput{
		UserID:           userID,
		TenantID:         draft.TenantID,
		RentObligationID: obligation.ID,
		AmountCents:      draft.AmountCents,
		Currency:         draft.Currency,
		ReceivedAt:       draft.ReceivedAt,
		Note:             draft.Note,
		IdempotencyKey:   draft.IdempotencyKey,
	}, draft, nil
}

func cashReceiptNewURL(draft cashReceiptFormInput, errorCode string) string {
	values := url.Values{}
	if draft.TenantID != 0 {
		values.Set("tenant_id", strconv.FormatUint(draft.TenantID, 10))
	}
	if !draft.Period.IsZero() {
		values.Set("period", draft.Period.Format("2006-01"))
	}
	if errorCode != "" {
		values.Set("error", errorCode)
	}
	return "/cash-receipts/new?" + values.Encode()
}

func cashReceiptErrorCode(err error) string {
	if strings.Contains(strings.ToLower(err.Error()), "balance") || strings.Contains(strings.ToLower(err.Error()), "projection") {
		return "cash_overbalance"
	}
	return "cash_receipt_failed"
}

func (a *app) loadCashReceiptFormData(ctx context.Context, userID, tenantID uint64, period time.Time) (cashReceiptFormData, error) {
	period = monthStart(period)
	data := cashReceiptFormData{
		Username:       a.cfg.AdminUsername,
		Environment:    a.cfg.Environment,
		TenantID:       strconv.FormatUint(tenantID, 10),
		Period:         period.Format("2006-01"),
		Currency:       ledgerCurrencyEUR,
		ReceivedAt:     time.Now().UTC().Format(dateLayout),
		IdempotencyKey: recordID("cash-request", time.Now().UTC()),
	}
	if err := a.db.WithContext(ctx).Where("user_id = ?", userID).Order("name ASC, id ASC").Find(&data.Tenants).Error; err != nil {
		return cashReceiptFormData{}, err
	}
	if tenantID == 0 {
		return data, nil
	}
	if err := a.db.WithContext(ctx).Where("id = ? AND user_id = ?", tenantID, userID).First(&data.Tenant).Error; err != nil {
		return cashReceiptFormData{}, err
	}
	data.TenantSelected = true
	var obligation rentObligation
	if err := a.db.WithContext(ctx).Where("user_id = ? AND tenant_id = ? AND period_month = ?", userID, tenantID, period).First(&obligation).Error; err != nil {
		return cashReceiptFormData{}, err
	}
	data.ObligationID = obligation.ID
	data.ExpectedAmount = formatMoney(centsToMoney(obligation.ExpectedAmountCents), obligation.Currency, 2)
	_, bankAllocations, cashReceipts, err := newCashReceiptService(a.db).loadRentObligationSources(ctx, a.db, userID, obligation.ID)
	if err != nil {
		return cashReceiptFormData{}, err
	}
	projected := projectRentObligation(obligation, bankAllocations, cashReceipts, time.Now().UTC())
	data.CurrentPaidAmount = formatMoney(centsToMoney(projected.PaidAmountCents), obligation.Currency, 2)
	data.CurrentRemaining = formatMoney(centsToMoney(maxInt64(obligation.ExpectedAmountCents-projected.PaidAmountCents, 0)), obligation.Currency, 2)
	return data, nil
}

func (a *app) cashReceiptFormDataFromPreview(ctx context.Context, preview cashReceiptPreview) (cashReceiptFormData, error) {
	data, err := a.loadCashReceiptFormData(ctx, preview.Input.UserID, preview.Input.TenantID, preview.Obligation.PeriodMonth)
	if err != nil {
		return cashReceiptFormData{}, err
	}
	data.Preview = true
	data.TenantID = strconv.FormatUint(preview.Input.TenantID, 10)
	data.Period = preview.Obligation.PeriodMonth.Format("2006-01")
	data.Amount = fmt.Sprintf("%.2f", centsToMoney(preview.Input.AmountCents))
	data.Currency = preview.Input.Currency
	data.ReceivedAt = preview.Input.ReceivedAt.Format(dateLayout)
	data.Note = preview.Input.Note
	data.IdempotencyKey = preview.Input.IdempotencyKey
	data.ObligationID = preview.Obligation.ID
	data.ExpectedAmount = formatMoney(centsToMoney(preview.Obligation.ExpectedAmountCents), preview.Obligation.Currency, 2)
	data.CurrentPaidAmount = formatMoney(centsToMoney(preview.CurrentPaidCents), preview.Obligation.Currency, 2)
	data.CurrentRemaining = formatMoney(centsToMoney(preview.CurrentRemainingCents), preview.Obligation.Currency, 2)
	data.AfterPaidAmount = formatMoney(centsToMoney(preview.AfterPaidCents), preview.Obligation.Currency, 2)
	data.AfterRemaining = formatMoney(centsToMoney(preview.AfterRemainingCents), preview.Obligation.Currency, 2)
	return data, nil
}

var cashReceiptTemplate = template.Must(template.New("cash-receipt").Funcs(template.FuncMap{"cashReceiptNewURL": func(tenantID, period string) string {
	return "/cash-receipts/new?tenant_id=" + url.QueryEscape(tenantID) + "&period=" + url.QueryEscape(period)
}}).Parse(`<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>现金租金补录</title><style>` + workspacePageCSS + `
.cash-shell{max-width:980px}.cash-form{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:16px}.cash-form .wide{grid-column:1/-1}.cash-form label{display:grid;gap:7px}.cash-form input,.cash-form select,.cash-form textarea{width:100%;box-sizing:border-box}.cash-form textarea{min-height:96px;resize:vertical}.summary-grid{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:10px;margin:18px 0}.summary-card{padding:14px;border:1px solid var(--border);border-radius:14px;background:rgba(255,255,255,.03)}.summary-card strong{display:block;margin-top:6px;font:700 18px var(--mono)}.cash-actions{display:flex;gap:10px;align-items:center;margin-top:18px}.cash-actions .btn{cursor:pointer}.muted{color:var(--foreground-muted)}.danger-note{color:#ffd0ce}.void-link{color:#ffd0ce;text-decoration:none;border-bottom:1px dashed currentColor}@media(max-width:700px){.cash-form,.summary-grid{grid-template-columns:1fr}.cash-form .wide{grid-column:auto}}
</style></head><body><div class="app"><aside class="sidebar" aria-label="Main navigation"><div class="side-brand"><div class="mark">R</div><div><div class="brand-title">RentOps</div><div class="brand-meta">{{.Environment}} workspace</div></div></div><nav class="nav"><a href="/rent-dashboard"><span class="glyph">总</span><span>月度总览</span></a><a href="/billing"><span class="glyph">流</span><span>银行流水</span></a><a href="/tenants" class="active"><span class="glyph">租</span><span>租客管理</span></a><a href="/expenses"><span class="glyph">支</span><span>支出记录</span></a></nav><div class="side-foot">当前用户：{{.Username}}<br>现金租金补录</div></aside><main class="content cash-shell"><header class="topbar"><div><div class="brand-title">手工收款</div><h1>现金租金补录</h1><div class="tiny">现金记录只计入选择的租金月份，不会创建银行流水。</div></div><a class="btn" href="/rent-dashboard">返回总览</a></header>{{if eq .Error "cash_receipt_failed"}}<div class="notice error">现金补录失败，请检查租客、月份、金额和币种。</div>{{end}}{{if eq .Error "cash_overbalance"}}<div class="notice error">这笔现金会超过该月份的未收余额，请重新核对银行与现金收款。</div>{{end}}{{if not .Preview}}<section class="panel surface"><div class="panel-head"><h2>填写收款信息</h2><span class="tiny">仅支持 EUR</span></div><form class="cash-form" method="post" action="/cash-receipts/preview"><label>租客<select name="tenant_id" required><option value="">请选择租客</option>{{range .Tenants}}<option value="{{.ID}}"{{if eq .ID $.Tenant.ID}} selected{{end}}>{{.Name}}{{if .RoomLabel}} · {{.RoomLabel}}{{end}}</option>{{end}}</select></label><label>租金月份<input name="period" type="month" value="{{.Period}}" required></label><label>现金金额<input name="amount" inputmode="decimal" placeholder="例如 400.00" value="{{.Amount}}" required></label><label>币种<input name="currency" value="{{.Currency}}" readonly></label><label>实际收款日期<input name="received_at" type="date" value="{{.ReceivedAt}}" required></label><label>幂等请求号<input name="idempotency_key" value="{{.IdempotencyKey}}" maxlength="191" required><span class="tiny">重复提交同一请求号不会重复记账。</span></label><label class="wide">备注（可选）<textarea name="note" maxlength="512" placeholder="例如 现金交付，已核对收据">{{.Note}}</textarea></label><div class="cash-actions wide"><button class="btn primary" type="submit">预览入账</button><a class="btn subtle" href="/tenants">返回租客</a></div></form></section>{{else}}<section class="panel surface"><div class="panel-head"><h2>确认现金入账</h2><span class="tiny">提交时会再次锁定并核对余额</span></div><div class="summary-grid"><div class="summary-card"><span class="tiny">本月应收</span><strong>{{.ExpectedAmount}}</strong></div><div class="summary-card"><span class="tiny">当前已收</span><strong>{{.CurrentPaidAmount}}</strong></div><div class="summary-card"><span class="tiny">本次现金</span><strong>{{.Amount}} {{.Currency}}</strong></div><div class="summary-card"><span class="tiny">入账后未收</span><strong>{{.AfterRemaining}}</strong></div></div><dl><dt class="muted">租客／月份</dt><dd>{{.Tenant.Name}} · {{.Period}}</dd><dt class="muted">实际收款日期</dt><dd>{{.ReceivedAt}}</dd><dt class="muted">备注</dt><dd>{{if .Note}}{{.Note}}{{else}}无{{end}}</dd><dt class="muted">请求号</dt><dd class="mono">{{.IdempotencyKey}}</dd></dl><form class="cash-actions" method="post" action="/cash-receipts"><input type="hidden" name="tenant_id" value="{{.TenantID}}"><input type="hidden" name="period" value="{{.Period}}"><input type="hidden" name="amount" value="{{.Amount}}"><input type="hidden" name="currency" value="{{.Currency}}"><input type="hidden" name="received_at" value="{{.ReceivedAt}}"><input type="hidden" name="note" value="{{.Note}}"><input type="hidden" name="idempotency_key" value="{{.IdempotencyKey}}"><button class="btn primary" type="submit">确认入账</button><a class="btn subtle" href="{{cashReceiptNewURL .TenantID .Period}}">修改</a></form></section>{{end}}</main></div></body></html>`))

var cashReceiptVoidTemplate = template.Must(template.New("cash-receipt-void").Parse(`<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>作废现金收款</title><style>` + workspacePageCSS + `
.void-shell{max-width:760px}.facts{display:grid;grid-template-columns:150px 1fr;gap:10px 18px}.facts dt{color:var(--foreground-muted)}.facts dd{margin:0;overflow-wrap:anywhere}.void-actions{display:flex;gap:10px;margin-top:20px}.void-actions textarea{width:100%;min-height:90px;box-sizing:border-box}.void-actions form{display:grid;gap:8px;width:100%}
</style></head><body><div class="app"><aside class="sidebar" aria-label="Main navigation"><div class="side-brand"><div class="mark">R</div><div><div class="brand-title">RentOps</div><div class="brand-meta">{{.Environment}} workspace</div></div></div><nav class="nav"><a href="/rent-dashboard"><span class="glyph">总</span><span>月度总览</span></a><a href="/billing"><span class="glyph">流</span><span>银行流水</span></a><a href="/tenants" class="active"><span class="glyph">租</span><span>租客管理</span></a><a href="/expenses"><span class="glyph">支</span><span>支出记录</span></a></nav><div class="side-foot">当前用户：{{.Username}}<br>现金收款纠正</div></aside><main class="content void-shell"><header class="topbar"><div><div class="brand-title">收款纠正</div><h1>作废现金收款</h1></div><a class="btn" href="/tenants/{{.Tenant.ID}}">返回租客详情</a></header>{{if eq .Error "cash_void_failed"}}<div class="notice error">作废失败，请检查作废原因后重试。</div>{{end}}<section class="panel surface"><p class="muted">作废只会停止这笔现金收款对租金余额的贡献，原始收据与作废原因会保留。需要更正时，请作废后重新补录正确金额。</p><dl class="facts"><dt>租客</dt><dd>{{.Tenant.Name}}</dd><dt>金额</dt><dd>{{.AmountDisplay}}</dd><dt>收款日期</dt><dd>{{.DateDisplay}}</dd><dt>收据号</dt><dd class="mono">{{.Receipt.ReceiptNumber}}</dd><dt>备注</dt><dd>{{if .Receipt.Note}}{{.Receipt.Note}}{{else}}无{{end}}</dd><dt>状态</dt><dd>{{.Receipt.Status}}</dd></dl>{{if .AlreadyVoided}}<div class="notice ok">这笔现金收款已经作废，不会重复扣减。</div>{{else}}<div class="void-actions"><form method="post" action="/cash-receipts/void"><input type="hidden" name="receipt_id" value="{{.Receipt.ID}}"><label for="void_reason">作废原因（必填）</label><textarea id="void_reason" name="reason" maxlength="512" required placeholder="例如 实收金额录入错误"></textarea><button class="btn danger" type="submit">确认作废</button></form></div>{{end}}</section></main></div></body></html>`))

func (a *app) handleCashReceiptNew(w http.ResponseWriter, r *http.Request) {
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
	tenantID, err := parseOptionalUint(r.URL.Query().Get("tenant_id"))
	if err != nil {
		http.Redirect(w, r, "/cash-receipts/new?error=cash_receipt_failed", http.StatusFound)
		return
	}
	period, err := parsePeriodMonth(r.URL.Query().Get("period"))
	if err != nil {
		http.Redirect(w, r, "/cash-receipts/new?error=cash_receipt_failed", http.StatusFound)
		return
	}
	data, err := a.loadCashReceiptFormData(r.Context(), userID, tenantID, period)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data.Error = r.URL.Query().Get("error")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := cashReceiptTemplate.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (a *app) handleCashReceiptPreview(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := a.currentUserID(r)
	if !ok || a.db == nil {
		http.Error(w, "database session required", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/cash-receipts/new?error=cash_receipt_failed", http.StatusFound)
		return
	}
	input, draft, err := a.cashReceiptInputForPeriod(r.Context(), userID, r.Form)
	if err != nil {
		http.Redirect(w, r, cashReceiptNewURL(draft, "cash_receipt_failed"), http.StatusFound)
		return
	}
	preview, err := newCashReceiptService(a.db).previewCashReceipt(r.Context(), input)
	if err != nil {
		http.Redirect(w, r, cashReceiptNewURL(draft, cashReceiptErrorCode(err)), http.StatusFound)
		return
	}
	data, err := a.cashReceiptFormDataFromPreview(r.Context(), preview)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := cashReceiptTemplate.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (a *app) handleCashReceiptCreate(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := a.currentUserID(r)
	if !ok || a.db == nil {
		http.Error(w, "database session required", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/cash-receipts/new?error=cash_receipt_failed", http.StatusFound)
		return
	}
	input, draft, err := a.cashReceiptInputForPeriod(r.Context(), userID, r.Form)
	if err != nil {
		http.Redirect(w, r, cashReceiptNewURL(draft, "cash_receipt_failed"), http.StatusFound)
		return
	}
	if _, err := newCashReceiptService(a.db).recordCashReceipt(r.Context(), input); err != nil {
		http.Redirect(w, r, cashReceiptNewURL(draft, cashReceiptErrorCode(err)), http.StatusFound)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/tenants/%d?message=cash_receipt_saved&from_month=%s&to_month=%s", draft.TenantID, draft.Period.Format("2006-01"), draft.Period.Format("2006-01")), http.StatusFound)
}

func (a *app) handleCashReceiptVoid(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	userID, ok := a.currentUserID(r)
	if !ok || a.db == nil {
		http.Error(w, "database session required", http.StatusBadRequest)
		return
	}
	if r.Method == http.MethodGet {
		receiptID, err := parsePositiveUint(r.URL.Query().Get("receipt_id"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		var receipt cashReceipt
		if err := a.db.WithContext(r.Context()).Where("id = ? AND user_id = ?", receiptID, userID).First(&receipt).Error; err != nil {
			http.NotFound(w, r)
			return
		}
		var tenantRow tenant
		if err := a.db.WithContext(r.Context()).Where("id = ? AND user_id = ?", receipt.TenantID, userID).First(&tenantRow).Error; err != nil {
			http.NotFound(w, r)
			return
		}
		data := cashReceiptVoidPageData{Username: a.cfg.AdminUsername, Environment: a.cfg.Environment, Receipt: receipt, Tenant: tenantRow, AmountDisplay: formatMoney(centsToMoney(receipt.AmountCents), receipt.Currency, 2), DateDisplay: receipt.ReceivedAt.Format(dateLayout), Error: r.URL.Query().Get("error"), AlreadyVoided: receipt.Status == cashReceiptStatusVoided}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := cashReceiptVoidTemplate.Execute(w, data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/cash-receipts/void?error=cash_void_failed", http.StatusFound)
		return
	}
	receiptID, err := parsePositiveUint(r.Form.Get("receipt_id"))
	if err != nil {
		http.Redirect(w, r, "/cash-receipts/void?error=cash_void_failed", http.StatusFound)
		return
	}
	receipt, err := newCashReceiptService(a.db).voidCashReceipt(r.Context(), userID, receiptID, r.Form.Get("reason"))
	if err != nil {
		http.Redirect(w, r, fmt.Sprintf("/cash-receipts/void?receipt_id=%d&error=cash_void_failed", receiptID), http.StatusFound)
		return
	}
	var obligation rentObligation
	if err := a.db.WithContext(r.Context()).Where("id = ? AND user_id = ?", receipt.RentObligationID, userID).First(&obligation).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/tenants/%d?message=cash_receipt_voided&from_month=%s&to_month=%s", receipt.TenantID, obligation.PeriodMonth.Format("2006-01"), obligation.PeriodMonth.Format("2006-01")), http.StatusFound)
}
