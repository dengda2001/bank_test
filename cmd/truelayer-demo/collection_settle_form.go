package main

import (
	"net/url"
	"strings"
	"time"
)

// The manual-balance ("平账") form is rendered from two carriers: the /bills
// table rows, and the rent workspace's tenant view (subtask ③) which reuses the
// same markup inline. The markup itself lives in the shared
// "collection-settle-form" partial
// (web/templates/partials/collection-settle-form.html) and reads nothing but the
// view below, so a page builds that view from its own row type and does not have
// to inherit /bills' page context.
const (
	settleDispositionMatchPayment = "match_payment"
	settleDispositionCashReceipt  = "cash_receipt"
	settleDispositionWaiver       = "waiver"
	settleDispositionCarryForward = "carry_forward"
)

// settleDispositionImplemented reports whether the server can actually apply a
// disposition to the ledger. Only "匹配现有收款" is wired up in this subtask:
// the other three render (so the form matches the prototype) but must come back
// with an explicit "not implemented" notice rather than silently changing money.
func settleDispositionImplemented(value string) bool {
	switch strings.TrimSpace(value) {
	case "", settleDispositionMatchPayment:
		return true
	default:
		return false
	}
}

type collectionSettleOption struct {
	Value    string
	Label    string
	Selected bool
}

type collectionSettleFormView struct {
	ObligationID      uint64
	DutyLabel         string
	OutstandingAmount string
	EffectiveDate     string
	Dispositions      []collectionSettleOption
	ReturnPeriod      string
	ReturnSearch      string
	ReturnStatus      string
	ReturnSort        string
	ReturnPage        int
	ReturnPageSize    int
}

func collectionSettleDispositions(selected string) []collectionSettleOption {
	selected = strings.TrimSpace(selected)
	if selected == "" {
		selected = settleDispositionMatchPayment
	}
	options := []collectionSettleOption{
		{Value: settleDispositionMatchPayment, Label: "匹配现有收款"},
		{Value: settleDispositionCashReceipt, Label: "登记现金收款"},
		{Value: settleDispositionWaiver, Label: "登记减免"},
		{Value: settleDispositionCarryForward, Label: "结转下月"},
	}
	for index := range options {
		options[index].Selected = options[index].Value == selected
	}
	return options
}

// SettleForm is the /bills adapter from one bill row to the shared view. Keeping
// the page context here (and not in the partial) is what lets subtask ③ render
// the identical form from a different row type.
func (data rentDashboardPageData) SettleForm(row rentDashboardRow) collectionSettleFormView {
	duty := strings.TrimSpace(firstNonEmpty(data.PeriodLabel, data.Period, row.Period))
	if duty == "" {
		duty = "本月"
	}
	duty += " 租金责任"
	if name := strings.TrimSpace(firstNonEmpty(row.TenantName, row.TenantAlias)); name != "" {
		duty = name + " · " + duty
	}
	amount := strings.TrimSpace(row.BalanceAmount)
	if amount == "" {
		amount = row.ExpectedAmount
	}
	return collectionSettleFormView{
		ObligationID:      row.ObligationID,
		DutyLabel:         duty,
		OutstandingAmount: amount,
		EffectiveDate:     time.Now().UTC().Format("2006-01-02"),
		Dispositions:      collectionSettleDispositions(settleDispositionMatchPayment),
		ReturnPeriod:      data.Period,
		ReturnSearch:      data.SearchFilter,
		ReturnStatus:      data.StatusFilter,
		ReturnSort:        data.SortFilter,
		ReturnPage:        data.Page,
		ReturnPageSize:    data.PageSize,
	}
}

// billsMessageText / billsErrorText translate the codes the bills handlers emit.
// Unrecognised codes fall through verbatim: the raw-code contract that
// TestBillsPageSurfacesInvalidFilterAndPeriodErrors pins (invalid_dashboard_filter,
// invalid_period) must keep rendering exactly as before.
func billsMessageText(code string) string {
	switch code {
	case "manual_balance_saved":
		return "平账已完成：已按剩余未付金额登记一笔收款并核销到该责任。"
	case "manual_balance_not_needed":
		return "该责任已无未付金额，无需平账。"
	case "bills_generated":
		return "本月账单已生成，重复触发不会产生重复账单。"
	default:
		return code
	}
}

func billsErrorText(code string) string {
	switch code {
	case "manual_balance_disposition_unimplemented":
		return "当前仅支持「匹配现有收款」；所选处理方式尚未实现，本次未产生任何账务变更。"
	case "manual_balance_reason_required":
		return "请填写平账原因。"
	case "manual_balance_failed":
		return "平账未完成，请核对责任状态后重试。"
	case "invalid_manual_balance":
		return "平账请求无效，请刷新页面后重试。"
	case "bills_generate_unavailable", "bills_generate_failed":
		return "生成账单未完成，请稍后重试。"
	default:
		return code
	}
}

// listManualBalanceRedirect retargets the dashboard redirect helper at the list
// the form was actually submitted from. The /rent-dashboard contract (and the
// test that pins it) is untouched; /bills swaps only the path, so the period,
// search, status, sort and pagination values keep round-tripping exactly as the
// detail-return contract in responsive-conventions.md §3.10 requires.
func listManualBalanceRedirect(listPath string, values url.Values, message, actionError string) string {
	target := dashboardManualBalanceRedirect(values, message, actionError)
	if strings.HasPrefix(target, "/rent-dashboard") {
		return listPath + strings.TrimPrefix(target, "/rent-dashboard")
	}
	return target
}
