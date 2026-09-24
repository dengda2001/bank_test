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
	PayerTenantID  uint64
	PayerName      string
	Period         time.Time
	AmountCents    int64
	Currency       string
	ReceivedAt     time.Time
	Note           string
	IdempotencyKey string
}

type cashReceiptFormData struct {
	workspaceShell
	Tenants           []tenant
	Tenant            tenant
	TenantSelected    bool
	TenantID          string
	PayerTenantID     string
	PayerName         string
	PayerDisplay      string
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
	workspaceShell
	Receipt       cashReceipt
	Tenant        tenant
	PayerName     string
	AmountDisplay string
	DateDisplay   string
	Error         string
	AlreadyVoided bool
	ReturnURL     string
	PostURL       string
}

func parseCashReceiptForm(values formValues, userID uint64) (cashReceiptFormInput, error) {
	if userID == 0 {
		return cashReceiptFormInput{}, fmt.Errorf("userID is required")
	}
	tenantID, err := parsePositiveUint(values.Get("tenant_id"))
	if err != nil {
		return cashReceiptFormInput{}, fmt.Errorf("tenant_id: %w", err)
	}
	payerTenantID, err := parseOptionalUint(values.Get("payer_tenant_id"))
	if err != nil {
		return cashReceiptFormInput{}, fmt.Errorf("payer_tenant_id: %w", err)
	}
	payerName := strings.TrimSpace(values.Get("payer_name"))
	if payerTenantID != 0 && payerName != "" {
		return cashReceiptFormInput{}, fmt.Errorf("choose a tenant payer or enter a payer name, not both")
	}
	if len([]rune(payerName)) > 191 {
		return cashReceiptFormInput{}, fmt.Errorf("cash payer name is too long")
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
		PayerTenantID:  payerTenantID,
		PayerName:      payerName,
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
	var tenantRow tenant
	if err := a.db.WithContext(ctx).Where("id = ? AND user_id = ?", draft.TenantID, userID).First(&tenantRow).Error; err != nil {
		return cashReceiptInput{}, draft, err
	}
	if err := newMonthlyRentFactsService(a.db).ensureMonthlyRentFacts(ctx, userID, draft.Period, rentFactsIntentExplicitPayment); err != nil {
		return cashReceiptInput{}, draft, err
	}
	var obligation rentObligation
	if err := a.db.WithContext(ctx).Where("user_id = ? AND tenant_id = ? AND period_month = ?", userID, draft.TenantID, draft.Period).First(&obligation).Error; err != nil {
		return cashReceiptInput{}, draft, err
	}
	var payerTenantID *uint64
	if draft.PayerTenantID != 0 {
		payerTenantID = &draft.PayerTenantID
	}
	return cashReceiptInput{
		UserID:           userID,
		TenantID:         draft.TenantID,
		PayerTenantID:    payerTenantID,
		RentObligationID: obligation.ID,
		AmountCents:      draft.AmountCents,
		Currency:         draft.Currency,
		ReceivedAt:       draft.ReceivedAt,
		Note:             draft.Note,
		PayerName:        draft.PayerName,
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
	if draft.PayerTenantID != 0 {
		values.Set("payer_tenant_id", strconv.FormatUint(draft.PayerTenantID, 10))
	}
	if payerName := strings.TrimSpace(draft.PayerName); payerName != "" {
		values.Set("payer_name", payerName)
	}
	if errorCode != "" {
		values.Set("error", errorCode)
	}
	return "/cash-receipts/new?" + values.Encode()
}

func cashReceiptErrorCode(err error) string {
	if errors.Is(err, errCashReceiptOverbalance) {
		return "cash_overbalance"
	}
	if errors.Is(err, ErrRentFactsConflict) {
		return "rent_facts_conflict"
	}
	return "cash_receipt_failed"
}

// cashOverbalanceText is the one wording for the overbalance error code returned by
// cashReceiptErrorCode. That code is reachable from two parallel server-rendered
// forms -- the embedded /cash-receipts drawer and the inline /cash-receipts/new page
// (cashReceiptFormErrorURL sends it to the latter whenever the posted form carries no
// return_to) -- and they are never rendered together, so each form has to show it
// while the repository keeps a single copy of the sentence. Both call the
// cashOverbalanceNotice template func below rather than holding their own text, so
// the wording cannot drift.
const cashOverbalanceText = "这笔现金会超过该月份未收余额，请核对已有收款。"

// cashOverbalanceNotice renders cashOverbalanceText into the two cash receipt
// templates. A func rather than a data field keeps the handlers from threading a
// constant through their view models.
var cashOverbalanceNotice = func() string { return cashOverbalanceText }

const cashRentFactsConflictText = "所选租金月份的计划刚刚更新，请刷新后重新补录。"

var cashRentFactsConflictNotice = func() string { return cashRentFactsConflictText }

// cashReceiptFailedText is the one wording for the catch-all cash receipt error
// code. It has exactly the same shape as cashOverbalanceText and for the same
// reason: the two parallel server-rendered forms (the /cash-receipts drawer and the
// inline /cash-receipts/new page) can each be the one that shows it, and one render
// of /cash-receipts used to print it twice -- a page-level banner and the drawer --
// in two different sentences.
//
// The named fields are the ones parseCashReceiptForm actually rejects (tenant_id,
// period, amount, received_at) and each is editable in both forms. The inline page
// used to add 币种 there, which is a dead end for the user: the currency input is
// `readonly` in both forms (EUR is the only ledger currency), so a user can never
// mistype it and must not be sent to inspect it. The message also cannot name the
// remaining cause -- a tenant with no rent obligation for that month -- which is
// what 月份 covers loosely.
const cashReceiptFailedText = "现金补录失败，请检查租客、月份、金额和日期。"

// cashReceiptFailedNotice renders cashReceiptFailedText into the two cash receipt
// templates, so the sentence cannot drift between them.
var cashReceiptFailedNotice = func() string { return cashReceiptFailedText }

func cashReceiptErrorMessage(code string) string {
	switch code {
	case "cash_receipt_failed":
		return cashReceiptFailedText
	case "cash_overbalance":
		return cashOverbalanceText
	case "rent_facts_conflict":
		return cashRentFactsConflictText
	default:
		return ""
	}
}

func (a *app) loadCashReceiptFormData(ctx context.Context, r *http.Request, userID, tenantID uint64, period time.Time) (cashReceiptFormData, error) {
	period = monthStart(period)
	if err := newMonthlyRentFactsService(a.db).ensureMonthlyRentFacts(ctx, userID, period, rentFactsIntentRead); err != nil {
		return cashReceiptFormData{}, err
	}
	data := cashReceiptFormData{
		workspaceShell: a.fillWorkspaceShell(r, workspaceShell{
			ActivePage:  "tenants",
			Username:    a.displayUsername(r),
			Environment: a.cfg.Environment,
			FootNote:    "现金租金补录",
		}),
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

func (a *app) cashReceiptFormDataFromPreview(ctx context.Context, r *http.Request, preview cashReceiptPreview) (cashReceiptFormData, error) {
	data, err := a.loadCashReceiptFormData(ctx, r, preview.Input.UserID, preview.Input.TenantID, preview.Obligation.PeriodMonth)
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
	data.PayerName = preview.Input.PayerName
	if preview.Input.PayerTenantID != nil {
		data.PayerTenantID = strconv.FormatUint(*preview.Input.PayerTenantID, 10)
		for _, candidate := range data.Tenants {
			if candidate.ID == *preview.Input.PayerTenantID {
				data.PayerDisplay = firstNonEmpty(candidate.DisplayAlias, candidate.Name)
				break
			}
		}
	} else if preview.Input.PayerName != "" {
		data.PayerDisplay = preview.Input.PayerName
	} else {
		data.PayerDisplay = firstNonEmpty(data.Tenant.DisplayAlias, data.Tenant.Name)
	}
	data.IdempotencyKey = preview.Input.IdempotencyKey
	data.ObligationID = preview.Obligation.ID
	data.ExpectedAmount = formatMoney(centsToMoney(preview.Obligation.ExpectedAmountCents), preview.Obligation.Currency, 2)
	data.CurrentPaidAmount = formatMoney(centsToMoney(preview.CurrentPaidCents), preview.Obligation.Currency, 2)
	data.CurrentRemaining = formatMoney(centsToMoney(preview.CurrentRemainingCents), preview.Obligation.Currency, 2)
	data.AfterPaidAmount = formatMoney(centsToMoney(preview.AfterPaidCents), preview.Obligation.Currency, 2)
	data.AfterRemaining = formatMoney(centsToMoney(preview.AfterRemainingCents), preview.Obligation.Currency, 2)
	return data, nil
}

var cashReceiptTemplate = newWorkspacePageTemplate("cash-receipt", template.FuncMap{"cashReceiptNewURL": func(tenantID, period, payerTenantID, payerName string) string {
	values := url.Values{"tenant_id": {tenantID}, "period": {period}}
	if payerTenantID != "" {
		values.Set("payer_tenant_id", payerTenantID)
	}
	if payerName = strings.TrimSpace(payerName); payerName != "" {
		values.Set("payer_name", payerName)
	}
	return "/cash-receipts/new?" + values.Encode()
}, "cashOverbalanceNotice": cashOverbalanceNotice, "cashRentFactsConflictNotice": cashRentFactsConflictNotice, "cashReceiptFailedNotice": cashReceiptFailedNotice}, `<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>现金租金补录</title><style>`+workspacePageCSS+`
.cash-shell{max-width:980px}.cash-form{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:16px}.cash-form .wide{grid-column:1/-1}.cash-form label{display:grid;gap:7px}.cash-form input,.cash-form select,.cash-form textarea{width:100%;box-sizing:border-box}.cash-form textarea{min-height:96px;resize:vertical}.summary-grid{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:10px;margin:18px 0}.summary-card{padding:14px;border:1px solid var(--border);border-radius:8px;background:var(--surface-muted)}.summary-card strong{display:block;margin-top:6px;font:700 18px var(--mono)}.cash-actions{display:flex;gap:10px;align-items:center;margin-top:18px}.cash-actions .btn{cursor:pointer}.muted{color:var(--foreground-muted)}.danger-note{color:var(--danger)}.void-link{color:var(--danger);text-decoration:none;border-bottom:1px dashed currentColor}@media(max-width:700px){.cash-form,.summary-grid{grid-template-columns:1fr}.cash-form .wide{grid-column:auto}.cash-actions{align-items:stretch;flex-wrap:wrap}.cash-actions .btn{flex:1 1 180px}}
</style></head><body><div class="app">{{template "workspace-nav" .}}<main class="content cash-shell"><header class="topbar"><div><div class="brand-title">手工收款</div><h1>现金租金补录</h1><div class="tiny">现金记录只计入选择的租金月份，不会创建银行流水。</div></div><a class="btn" href="/rent-dashboard">返回总览</a></header>{{if eq .Error "cash_receipt_failed"}}<div class="notice error">{{cashReceiptFailedNotice}}</div>{{end}}{{if eq .Error "cash_overbalance"}}<div class="notice error">{{cashOverbalanceNotice}}</div>{{end}}{{if eq .Error "rent_facts_conflict"}}<div class="notice error">{{cashRentFactsConflictNotice}}</div>{{end}}{{if not .Preview}}<section class="panel surface"><div class="panel-head"><h2>填写收款信息</h2><span class="tiny">仅支持 EUR</span></div><form class="cash-form" method="post" action="/cash-receipts/preview"><label>入账租客<select name="tenant_id" data-searchable required><option value="">请选择租客</option>{{range .Tenants}}<option value="{{.ID}}"{{if eq .ID $.Tenant.ID}} selected{{end}}>{{.Name}}</option>{{end}}</select></label><label>租金月份<input name="period" type="month" value="{{.Period}}" required></label><label>现金金额<input name="amount" inputmode="decimal" placeholder="例如 400.00" value="{{.Amount}}" required></label><label>实际付款人<select name="payer_tenant_id" data-searchable><option value="">与入账租客相同</option>{{range .Tenants}}<option value="{{.ID}}"{{if eq $.PayerTenantID (printf "%d" .ID)}} selected{{end}}>{{.Name}}</option>{{end}}</select></label><label>其他付款人姓名（可选）<input name="payer_name" maxlength="191" value="{{.PayerName}}" placeholder="非租客代付时填写"></label><label>币种<input name="currency" value="{{.Currency}}" readonly></label><label>实际收款日期<input name="received_at" type="date" value="{{.ReceivedAt}}" required></label><label>幂等请求号<input name="idempotency_key" value="{{.IdempotencyKey}}" maxlength="191" required><span class="tiny">重复提交同一请求号不会重复记账。</span></label><label class="wide">备注（可选）<textarea name="note" maxlength="512" placeholder="例如 现金交付，已核对收据">{{.Note}}</textarea></label><div class="cash-actions wide"><button class="btn primary" type="submit">预览入账</button><a class="btn subtle" href="/tenants">返回租客</a></div></form></section>{{else}}<section class="panel surface"><div class="panel-head"><h2>确认现金入账</h2><span class="tiny">提交时会再次锁定并核对余额</span></div><div class="summary-grid"><div class="summary-card"><span class="tiny">本月应收</span><strong>{{.ExpectedAmount}}</strong></div><div class="summary-card"><span class="tiny">当前已收</span><strong>{{.CurrentPaidAmount}}</strong></div><div class="summary-card"><span class="tiny">本次现金</span><strong>{{.Amount}} {{.Currency}}</strong></div><div class="summary-card"><span class="tiny">入账后未收</span><strong>{{.AfterRemaining}}</strong></div></div><dl><dt class="muted">入账租客／月份</dt><dd>{{.Tenant.Name}} · {{.Period}}</dd><dt class="muted">实际付款人</dt><dd>{{if .PayerDisplay}}{{.PayerDisplay}}{{else}}入账租客本人{{end}}</dd><dt class="muted">实际收款日期</dt><dd>{{.ReceivedAt}}</dd><dt class="muted">备注</dt><dd>{{if .Note}}{{.Note}}{{else}}无{{end}}</dd><dt class="muted">请求号</dt><dd class="mono">{{.IdempotencyKey}}</dd></dl><form class="cash-actions" method="post" action="/cash-receipts"><input type="hidden" name="tenant_id" value="{{.TenantID}}"><input type="hidden" name="payer_tenant_id" value="{{.PayerTenantID}}"><input type="hidden" name="payer_name" value="{{.PayerName}}"><input type="hidden" name="period" value="{{.Period}}"><input type="hidden" name="amount" value="{{.Amount}}"><input type="hidden" name="currency" value="{{.Currency}}"><input type="hidden" name="received_at" value="{{.ReceivedAt}}"><input type="hidden" name="note" value="{{.Note}}"><input type="hidden" name="idempotency_key" value="{{.IdempotencyKey}}"><button class="btn primary" type="submit">确认入账</button><a class="btn subtle" href="{{cashReceiptNewURL .TenantID .Period .PayerTenantID .PayerName}}">修改</a></form></section>{{end}}</main></div></body></html>`)

var cashReceiptVoidTemplate = newWorkspacePageTemplate("cash-receipt-void", nil, `<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>撤销现金收款</title><style>`+workspacePageCSS+`
.void-shell{max-width:760px}.facts{display:grid;grid-template-columns:150px 1fr;gap:10px 18px;padding:18px 20px}.facts dt{color:var(--foreground-muted)}.facts dd{margin:0;overflow-wrap:anywhere}.void-actions{display:flex;gap:10px;margin-top:20px}.void-actions textarea{width:100%;min-height:90px;box-sizing:border-box}.void-actions form{display:grid;gap:8px;width:100%}
/* 这个面板没有 .panel-head，.facts 自带 18px 20px 内边距，而它的兄弟节点
   （说明段、notice、操作区）原本零内边距，顶着面板边框。补齐同样的 20px 内缩，
   vertical 方向交给 .facts 自己的上下内边距，避免叠成 38px 的空档。 */
.void-shell .panel.surface>.muted{margin:18px 20px 0}
.void-shell .panel.surface>.notice{margin:0 20px 18px}
.void-shell .void-actions{margin:0 20px 18px}
@media(max-width:640px){
  /* 与 /tenants 详情的 .profile-list 同一处理：标签列固定 150px，值列被压到
     131px，收据号这类长串只能逐字断行。窄屏改单列。 */
  .facts{grid-template-columns:1fr;gap:4px 0}
  .facts dt{font-size:12px}
}
</style></head><body><div class="app">{{template "workspace-nav" .}}<main class="content void-shell"><header class="topbar"><div><div class="brand-title">收款纠正</div><h1>撤销现金收款</h1></div><a class="btn" href="{{if .ReturnURL}}{{.ReturnURL}}{{else}}/tenants/{{.Tenant.ID}}{{end}}">返回{{if .ReturnURL}}现金收款{{else}}租客详情{{end}}</a></header>{{if eq .Error "cash_void_failed"}}<div class="notice error">撤销失败，请检查撤销原因后重试。</div>{{end}}<section class="panel surface"><p class="muted">撤销只会停止这笔现金收款对租金余额的贡献，原始收据与撤销原因会保留。需要更正时，请撤销后重新补录正确金额。</p><dl class="facts"><dt>入账租客</dt><dd>{{.Tenant.Name}}</dd><dt>实际付款人</dt><dd>{{.PayerName}}</dd><dt>金额</dt><dd>{{.AmountDisplay}}</dd><dt>收款日期</dt><dd>{{.DateDisplay}}</dd><dt>收据号</dt><dd class="mono">{{.Receipt.ReceiptNumber}}</dd><dt>备注</dt><dd>{{if .Receipt.Note}}{{.Receipt.Note}}{{else}}无{{end}}</dd><dt>状态</dt><dd>{{.Receipt.Status}}</dd></dl>{{if .AlreadyVoided}}<div class="notice ok">这笔现金收款已经撤销，不会重复扣减。</div>{{else}}<div class="void-actions"><form method="post" action="{{if .PostURL}}{{.PostURL}}{{else}}/cash-receipts/void{{end}}"><input type="hidden" name="receipt_id" value="{{.Receipt.ID}}"><label for="void_reason">撤销原因（必填）</label><textarea id="void_reason" name="reason" maxlength="512" required placeholder="例如 实收金额录入错误"></textarea><button class="btn danger" type="submit">确认撤销</button></form></div>{{end}}</section></main></div></body></html>`)

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
	data, err := a.loadCashReceiptFormData(r.Context(), r, userID, tenantID, period)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data.PayerName = strings.TrimSpace(r.URL.Query().Get("payer_name"))
	data.PayerTenantID = strings.TrimSpace(r.URL.Query().Get("payer_tenant_id"))
	data.Error = r.URL.Query().Get("error")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := cashReceiptTemplate.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (a *app) renderCashReceiptHostError(w http.ResponseWriter, r *http.Request, userID uint64, values url.Values, errorCode string) bool {
	returnTo := values.Get("return_to")
	if _, _, _, valid := cashReceiptHostReturnURL(returnTo); !valid {
		return false
	}
	form, err := a.cashReceiptDraftFormData(r.Context(), r, userID, values, errorCode)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return true
	}
	return a.renderCashReceiptHostPage(w, r, userID, returnTo, form)
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
		if r.URL.Query().Get("return_to") == "cash-receipts" {
			http.Redirect(w, r, "/cash-receipts?error=cash_receipt_failed&add=1", http.StatusFound)
		} else {
			http.Redirect(w, r, "/cash-receipts/new?error=cash_receipt_failed", http.StatusFound)
		}
		return
	}
	if r.Form.Get("edit") == "1" {
		if _, _, _, valid := cashReceiptHostReturnURL(r.Form.Get("return_to")); valid {
			form, formErr := a.cashReceiptDraftFormData(r.Context(), r, userID, r.Form, "")
			if formErr != nil {
				http.Error(w, formErr.Error(), http.StatusInternalServerError)
				return
			}
			if a.renderCashReceiptHostPage(w, r, userID, r.Form.Get("return_to"), form) {
				return
			}
		}
	}
	if _, _, hostTenantID, valid := cashReceiptHostReturnURL(r.Form.Get("return_to")); valid && hostTenantID > 0 && strings.TrimSpace(r.Form.Get("tenant_id")) != strconv.FormatUint(hostTenantID, 10) {
		if a.renderCashReceiptHostError(w, r, userID, r.Form, "cash_receipt_failed") {
			return
		}
	}
	input, draft, err := a.cashReceiptInputForPeriod(r.Context(), userID, r.Form)
	if err != nil {
		if _, _, _, valid := cashReceiptHostReturnURL(r.Form.Get("return_to")); valid {
			form, formErr := a.cashReceiptDraftFormData(r.Context(), r, userID, r.Form, "cash_receipt_failed")
			if formErr != nil {
				http.Error(w, formErr.Error(), http.StatusInternalServerError)
				return
			}
			if a.renderCashReceiptHostPage(w, r, userID, r.Form.Get("return_to"), form) {
				return
			}
		}
		http.Redirect(w, r, cashReceiptFormErrorURL(r.Form, draft, "cash_receipt_failed"), http.StatusFound)
		return
	}
	preview, err := newCashReceiptService(a.db).previewCashReceipt(r.Context(), input)
	if err != nil {
		if _, _, _, valid := cashReceiptHostReturnURL(r.Form.Get("return_to")); valid {
			form, formErr := a.cashReceiptDraftFormData(r.Context(), r, userID, r.Form, cashReceiptErrorCode(err))
			if formErr != nil {
				http.Error(w, formErr.Error(), http.StatusInternalServerError)
				return
			}
			if a.renderCashReceiptHostPage(w, r, userID, r.Form.Get("return_to"), form) {
				return
			}
		}
		http.Redirect(w, r, cashReceiptFormErrorURL(r.Form, draft, cashReceiptErrorCode(err)), http.StatusFound)
		return
	}
	data, err := a.cashReceiptFormDataFromPreview(r.Context(), r, preview)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if r.Form.Get("return_to") == "cash-receipts" {
		a.renderCashReceiptDrawerPreview(w, r, userID, data)
		return
	}
	if _, _, _, valid := cashReceiptHostReturnURL(r.Form.Get("return_to")); valid {
		if a.renderCashReceiptHostPage(w, r, userID, r.Form.Get("return_to"), data) {
			return
		}
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
	if _, _, hostTenantID, valid := cashReceiptHostReturnURL(r.Form.Get("return_to")); valid && hostTenantID > 0 && strings.TrimSpace(r.Form.Get("tenant_id")) != strconv.FormatUint(hostTenantID, 10) {
		if a.renderCashReceiptHostError(w, r, userID, r.Form, "cash_receipt_failed") {
			return
		}
	}
	input, draft, err := a.cashReceiptInputForPeriod(r.Context(), userID, r.Form)
	if err != nil {
		if _, _, _, valid := cashReceiptHostReturnURL(r.Form.Get("return_to")); valid {
			form, formErr := a.cashReceiptDraftFormData(r.Context(), r, userID, r.Form, "cash_receipt_failed")
			if formErr != nil {
				http.Error(w, formErr.Error(), http.StatusInternalServerError)
				return
			}
			if a.renderCashReceiptHostPage(w, r, userID, r.Form.Get("return_to"), form) {
				return
			}
		}
		http.Redirect(w, r, cashReceiptFormErrorURL(r.Form, draft, "cash_receipt_failed"), http.StatusFound)
		return
	}
	if _, err := newCashReceiptService(a.db).recordCashReceipt(r.Context(), input); err != nil {
		if _, _, _, valid := cashReceiptHostReturnURL(r.Form.Get("return_to")); valid {
			form, formErr := a.cashReceiptDraftFormData(r.Context(), r, userID, r.Form, cashReceiptErrorCode(err))
			if formErr != nil {
				http.Error(w, formErr.Error(), http.StatusInternalServerError)
				return
			}
			if a.renderCashReceiptHostPage(w, r, userID, r.Form.Get("return_to"), form) {
				return
			}
		}
		http.Redirect(w, r, cashReceiptFormErrorURL(r.Form, draft, cashReceiptErrorCode(err)), http.StatusFound)
		return
	}
	if r.Form.Get("return_to") == "cash-receipts" {
		http.Redirect(w, r, cashReceiptListReturnURL(r.Form, "", "cash_receipt_saved", false, ""), http.StatusFound)
		return
	}
	if returnURL, _, _, valid := cashReceiptHostReturnURL(r.Form.Get("return_to")); valid {
		http.Redirect(w, r, cashReceiptHostMessageURL(returnURL, "cash_receipt_saved"), http.StatusFound)
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
		var obligation rentObligation
		if err := a.db.WithContext(r.Context()).Where("id = ? AND user_id = ?", receipt.RentObligationID, userID).First(&obligation).Error; err != nil {
			http.NotFound(w, r)
			return
		}
		var tenantRow tenant
		if err := a.db.WithContext(r.Context()).Where("id = ? AND user_id = ?", obligation.TenantID, userID).First(&tenantRow).Error; err != nil {
			http.NotFound(w, r)
			return
		}
		payerName := stringValue(receipt.PayerNameSnapshot)
		if payerName == "" && receipt.PayerTenantID != nil {
			var payer tenant
			if err := a.db.WithContext(r.Context()).Where("id = ? AND user_id = ?", *receipt.PayerTenantID, userID).First(&payer).Error; err != nil {
				http.NotFound(w, r)
				return
			}
			payerName = firstNonEmpty(payer.DisplayAlias, payer.Name)
		}
		if payerName == "" {
			payerName = firstNonEmpty(tenantRow.DisplayAlias, tenantRow.Name)
		}
		returnURL, returnToList := cashReceiptVoidListReturnURL(r.URL.Query(), "", "")
		postURL := "/cash-receipts/void"
		if returnToList {
			postURL = cashReceiptVoidURL(receipt.ID, r.URL.Query().Get("list_period"), r.URL.Query().Get("list_status"), r.URL.Query().Get("list_search"), r.URL.Query().Get("list_sort"))
		}
		data := cashReceiptVoidPageData{workspaceShell: a.fillWorkspaceShell(r, workspaceShell{ActivePage: "tenants", Username: a.displayUsername(r), Environment: a.cfg.Environment, FootNote: "现金收款纠正"}), Receipt: receipt, Tenant: tenantRow, PayerName: payerName, AmountDisplay: formatMoney(centsToMoney(receipt.AmountCents), receipt.Currency, 2), DateDisplay: receipt.ReceivedAt.Format(dateLayout), Error: r.URL.Query().Get("error"), AlreadyVoided: receipt.Status == cashReceiptStatusVoided, ReturnURL: returnURL, PostURL: postURL}
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
		if returnURL, ok := cashReceiptVoidListReturnURL(r.URL.Query(), "cash_void_failed", ""); ok {
			http.Redirect(w, r, returnURL, http.StatusFound)
			return
		}
		http.Redirect(w, r, "/cash-receipts/void?error=cash_void_failed", http.StatusFound)
		return
	}
	receiptID, err := parsePositiveUint(r.Form.Get("receipt_id"))
	if err != nil {
		if returnURL, ok := cashReceiptVoidListReturnURL(r.Form, "cash_void_failed", ""); ok {
			http.Redirect(w, r, returnURL, http.StatusFound)
			return
		}
		http.Redirect(w, r, "/cash-receipts/void?error=cash_void_failed", http.StatusFound)
		return
	}
	receipt, err := newCashReceiptService(a.db).voidCashReceipt(r.Context(), userID, receiptID, r.Form.Get("reason"))
	if err != nil {
		if returnURL, ok := cashReceiptVoidListReturnURL(r.Form, "cash_void_failed", ""); ok {
			http.Redirect(w, r, returnURL, http.StatusFound)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/cash-receipts/void?receipt_id=%d&error=cash_void_failed", receiptID), http.StatusFound)
		return
	}
	var obligation rentObligation
	if err := a.db.WithContext(r.Context()).Where("id = ? AND user_id = ?", receipt.RentObligationID, userID).First(&obligation).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if returnURL, ok := cashReceiptVoidListReturnURL(r.Form, "", "cash_receipt_voided"); ok {
		http.Redirect(w, r, returnURL, http.StatusFound)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/tenants/%d?message=cash_receipt_voided&from_month=%s&to_month=%s", obligation.TenantID, obligation.PeriodMonth.Format("2006-01"), obligation.PeriodMonth.Format("2006-01")), http.StatusFound)
}
