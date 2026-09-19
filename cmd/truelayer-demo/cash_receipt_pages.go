package main

import (
	"context"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// cashReceiptPageTemplate's func map carries the shared cash-receipt error wording
// (defined in cash_receipt_handlers.go). The embedded drawer and the inline
// /cash-receipts/new page must render the identical sentence for each error code,
// so both call the same funcs instead of holding a copy each.
var cashReceiptPageTemplate = newEmbeddedWorkspacePageTemplate("cash-receipts-page", template.FuncMap{
	"cashOverbalanceNotice":   cashOverbalanceNotice,
	"cashReceiptFailedNotice": cashReceiptFailedNotice,
}, "web/templates/pages/cash-receipts.html")

func cashReceiptListURL(period, status, search string, add bool, tenantID string, errorCode, message string) string {
	query := url.Values{}
	query.Set("period", validatedPeriodValue(period))
	if status != "all" && status != cashReceiptStatusConfirmed && status != cashReceiptStatusVoided {
		status = "all"
	}
	query.Set("status", status)
	if search = strings.TrimSpace(search); search != "" {
		query.Set("search", search)
	}
	if add {
		query.Set("add", "1")
	}
	if tenantID = strings.TrimSpace(tenantID); tenantID != "" {
		query.Set("tenant_id", tenantID)
	}
	if errorCode != "" {
		query.Set("error", errorCode)
	}
	if message != "" {
		query.Set("message", message)
	}
	return "/cash-receipts?" + query.Encode()
}

func cashReceiptListReturnURL(values url.Values, errorCode, message string, add bool, tenantID string) string {
	return cashReceiptListURL(values.Get("list_period"), values.Get("list_status"), values.Get("list_search"), add, tenantID, errorCode, message)
}

func cashReceiptFormErrorURL(values url.Values, draft cashReceiptFormInput, errorCode string) string {
	if values.Get("return_to") != "cash-receipts" {
		return cashReceiptNewURL(draft, errorCode)
	}
	tenantID := values.Get("tenant_id")
	if draft.TenantID != 0 {
		tenantID = strconv.FormatUint(draft.TenantID, 10)
	}
	return cashReceiptListReturnURL(values, errorCode, "", true, tenantID)
}

// cashReceiptDrawerIsOpen reports whether /cash-receipts must render its cash
// receipt drawer. The drawer is now the only carrier of the cash receipt error
// messages: the page-level copy was removed because one render printed the same
// code twice, in two different sentences.
//
// How much of the duplicate a user actually read depended on the width, which is
// why it went unnoticed. The backdrop is a 22%-opaque scrim, not an opaque wall,
// so on a desktop the banner stayed legible behind it and both wordings were on
// screen at once; only at <=640, where the drawer becomes a bottom sheet covering
// the banner outright, did the second copy go unread. A one-code/two-sentence
// render is the defect either way.
//
// The removal is safe only because an error code opens the drawer on its own --
// if this ever stopped being true, the message would have no visible carrier at
// all.
func cashReceiptDrawerIsOpen(values url.Values) bool {
	return values.Get("add") == "1" || values.Get("error") != ""
}

func (a *app) loadCashReceiptListPage(ctx context.Context, r *http.Request, userID uint64, period time.Time, statusFilter, search string) (cashReceiptPageData, error) {
	var receipts []cashReceipt
	if err := a.db.WithContext(ctx).Where("user_id = ?", userID).Order("received_at DESC, id DESC").Find(&receipts).Error; err != nil {
		return cashReceiptPageData{}, err
	}
	var tenants []tenant
	if err := a.db.WithContext(ctx).Where("user_id = ?", userID).Find(&tenants).Error; err != nil {
		return cashReceiptPageData{}, err
	}
	tenantByID := make(map[uint64]tenant, len(tenants))
	for _, row := range tenants {
		tenantByID[row.ID] = row
	}
	periodValue := monthStart(period).Format("2006-01")
	needle := strings.ToLower(strings.TrimSpace(search))
	rows := make([]cashReceiptPageRow, 0, len(receipts))
	for _, receipt := range receipts {
		if statusFilter != "all" && receipt.Status != statusFilter {
			continue
		}
		tenantRow := tenantByID[receipt.TenantID]
		row := cashReceiptPageRow{
			ID: receipt.ID, ReceiptNumber: receipt.ReceiptNumber, TenantID: receipt.TenantID,
			TenantName:   firstNonEmpty(tenantRow.DisplayAlias, tenantRow.Name, "未绑定租客"),
			ObligationID: receipt.RentObligationID, AmountCents: receipt.AmountCents,
			Amount: pageCurrencyAmount(receipt.AmountCents, receipt.Currency), Currency: receipt.Currency,
			ReceivedAt: receipt.ReceivedAt.Format(dateLayout), Status: receipt.Status,
			StatusLabel: pageStatusLabel(receipt.Status), Note: receipt.Note,
			VoidReason: stringValue(receipt.VoidReason),
			VoidURL:    "/cash-receipts/void?receipt_id=" + strconv.FormatUint(receipt.ID, 10),
		}
		var obligation rentObligation
		if err := a.db.WithContext(ctx).Where("id = ? AND user_id = ?", receipt.RentObligationID, userID).First(&obligation).Error; err == nil {
			row.Period = obligation.PeriodMonth.Format("2006-01")
			if obligation.RentChargeID != nil {
				var charge rentCharge
				if err := a.db.WithContext(ctx).Where("id = ? AND user_id = ?", *obligation.RentChargeID, userID).First(&charge).Error; err == nil {
					row.RoomLabel = stringValue(charge.RoomLabelSnapshot)
				}
			}
		}
		if row.Period != periodValue {
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(row.ReceiptNumber+" "+row.TenantName+" "+row.Period+" "+row.Note+" "+row.StatusLabel), needle) {
			continue
		}
		rows = append(rows, row)
	}
	form, err := a.loadCashReceiptFormData(ctx, r, userID, 0, period)
	if err != nil {
		return cashReceiptPageData{}, err
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("tenant_id")); raw != "" {
		id, parseErr := parsePositiveUint(raw)
		if parseErr == nil {
			for _, candidate := range form.Tenants {
				if candidate.ID == id {
					form.Tenant, form.TenantID, form.TenantSelected = candidate, raw, true
					break
				}
			}
		}
	}
	return cashReceiptPageData{
		workspaceShell: canonicalPageShell(a, r, "cash-receipts", "现金补录"),
		Rows:           rows, Period: periodValue, StatusFilter: statusFilter, Search: search,
		ShowForm: cashReceiptDrawerIsOpen(r.URL.Query()),
		Form:     form, Message: r.URL.Query().Get("message"), Error: r.URL.Query().Get("error"),
	}, nil
}

func (a *app) handleCashReceiptList(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.scopedPageUser(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	period, err := parsePeriodMonth(strings.TrimSpace(r.URL.Query().Get("period")))
	if err != nil {
		http.Error(w, "period is invalid", http.StatusBadRequest)
		return
	}
	status := firstNonEmpty(strings.TrimSpace(r.URL.Query().Get("status")), "all")
	if status != "all" && status != cashReceiptStatusConfirmed && status != cashReceiptStatusVoided {
		http.Error(w, "cash receipt status is invalid", http.StatusBadRequest)
		return
	}
	data, err := a.loadCashReceiptListPage(r.Context(), r, userID, period, status, strings.TrimSpace(r.URL.Query().Get("search")))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := cashReceiptPageTemplate.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (a *app) renderCashReceiptDrawerPreview(w http.ResponseWriter, r *http.Request, userID uint64, form cashReceiptFormData) {
	period, err := parsePeriodMonth(firstNonEmpty(r.Form.Get("list_period"), form.Period))
	if err != nil {
		period = monthStart(time.Now().UTC())
	}
	status := firstNonEmpty(r.Form.Get("list_status"), "all")
	search := strings.TrimSpace(r.Form.Get("list_search"))
	data, err := a.loadCashReceiptListPage(r.Context(), r, userID, period, status, search)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data.ShowForm = true
	data.Form = form
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := cashReceiptPageTemplate.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
