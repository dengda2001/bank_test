package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

type transactionDetailPageData struct {
	workspaceShell
	Transaction      transactionPageRow
	Title            string
	Subtitle         string
	StatusClass      string
	BackURL          string
	BackRowURL       string
	ActionBase       string
	TransactionTime  string
	AllocatedAmount  string
	RemainingAmount  string
	AllocationCount  int
	SourceLabel      string
	ProviderID       string
	Reference        string
	ParsedPeriod     string
	ParsedPeriodNote string
	RawRecord        string
	HasRawRecord     bool
	Suggestion       transactionDetailSuggestion
	Allocations      []transactionDetailAllocationRow
	Events           []transactionDetailEvent
}

// formatTransactionTimestamp renders the bank timestamp the same way the
// prototype's 「交易时间」 fact does. A missing timestamp renders as an em dash
// rather than an empty row.
func formatTransactionTimestamp(value *time.Time) string {
	if value == nil || value.IsZero() {
		return "—"
	}
	return value.UTC().Format("2006-01-02 15:04")
}

type transactionDetailSuggestion struct {
	Available bool
	Title     string
	Subtitle  string
	Status    string
	Reason    string
}

type transactionDetailAllocationRow struct {
	TenantName     string
	TenantURL      string
	PropertyName   string
	RoomLabel      string
	RoomURL        string
	Period         string
	DueDate        string
	BillURL        string
	BillLabel      string
	KindLabel      string
	Amount         string
	ExpectedAmount string
	StatusLabel    string
	StatusClass    string
	Note           string
	CreatedAt      string
	Effective      bool
}

type transactionDetailEvent struct {
	Timestamp string
	Title     string
	Detail    string
	Status    string
}

func setTransactionDetailLinks(path string, query url.Values, rows []transactionPageRow) {
	pagePath := transactionListPath(path)
	for index := range rows {
		row := &rows[index]
		key := strings.TrimSpace(row.InternalID)
		if key == "" || key == "0" {
			key = fmt.Sprintf("demo-%d", index)
		}
		row.DetailKey = key
		values := cloneQueryValues(query)
		values.Set("detail", key)
		row.DetailURL = pagePath + "?" + values.Encode()
	}
}

func transactionListPath(path string) string {
	if path == "/billing" {
		return "/billing"
	}
	return "/transactions"
}

func transactionListURL(path string, query url.Values) string {
	values := cloneQueryValues(query)
	values.Del("detail")
	pagePath := transactionListPath(path)
	if len(values) == 0 {
		return pagePath
	}
	return pagePath + "?" + values.Encode()
}

func cloneQueryValues(query url.Values) url.Values {
	values := make(url.Values, len(query))
	for key, items := range query {
		values[key] = append([]string(nil), items...)
	}
	return values
}

func (a *app) renderTransactionDetail(w http.ResponseWriter, r *http.Request, key string) {
	filters := filtersFromQuery(r.URL.Query())
	if err := validateTransactionFilters(filters); err != nil {
		filters = transactionFilters{Page: 1, PageSize: 10}
	}

	var data transactionDetailPageData
	userID, hasUser := a.currentUserID(r)
	if a.db != nil && hasUser {
		transactionID, err := strconv.ParseUint(key, 10, 64)
		if err != nil || transactionID == 0 {
			http.NotFound(w, r)
			return
		}
		var source paymentTransaction
		if err := a.db.WithContext(r.Context()).Where("id = ? AND user_id = ?", transactionID, userID).First(&source).Error; err != nil {
			http.NotFound(w, r)
			return
		}
		detailData, err := a.transactionDetailPageData(r.Context(), r, userID, source, key, filters)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		data = detailData
	} else {
		index, err := demoTransactionIndex(key)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		result, loadErr := a.loadLatestDemoResult()
		if loadErr != nil {
			http.Error(w, loadErr.Error(), http.StatusInternalServerError)
			return
		}
		rows := fallbackTransactionPageRows(result, filters)
		if index < 0 || index >= len(rows) {
			http.NotFound(w, r)
			return
		}
		data = a.demoTransactionDetailPageData(r, rows[index], key, filters)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := transactionDetailPageTemplate.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func demoTransactionIndex(key string) (int, error) {
	if !strings.HasPrefix(key, "demo-") {
		return 0, fmt.Errorf("invalid demo transaction key")
	}
	index, err := strconv.Atoi(strings.TrimPrefix(key, "demo-"))
	if err != nil || index < 0 {
		return 0, fmt.Errorf("invalid demo transaction key")
	}
	return index, nil
}

func (a *app) transactionDetailPageData(ctx context.Context, r *http.Request, userID uint64, source paymentTransaction, key string, filters transactionFilters) (transactionDetailPageData, error) {
	repository := newLandlordRentRepository(a.db)
	allocations, err := repository.listPaymentAllocations(ctx, userID, paymentAllocationQuery{PaymentTransactionID: source.ID})
	if err != nil {
		return transactionDetailPageData{}, err
	}
	var tenants []tenant
	if err := a.db.WithContext(ctx).Where("user_id = ?", userID).Find(&tenants).Error; err != nil {
		return transactionDetailPageData{}, err
	}
	var obligations []rentObligation
	if err := a.db.WithContext(ctx).Where("user_id = ?", userID).Find(&obligations).Error; err != nil {
		return transactionDetailPageData{}, err
	}
	var payers []tenantPayer
	if err := a.db.WithContext(ctx).Where("user_id = ? AND removed_at IS NULL", userID).Find(&payers).Error; err != nil {
		return transactionDetailPageData{}, err
	}

	row := enrichTransactionPageRow(transactionPageRowFromModel(source), source, allocations, obligations)
	summary := summarizeTransactionAllocations(source, allocations)
	nameByTenant := make(map[uint64]string, len(tenants))
	tenantByID := make(map[uint64]tenant, len(tenants))
	for _, tenantRow := range tenants {
		nameByTenant[tenantRow.ID] = firstNonEmpty(tenantRow.DisplayAlias, tenantRow.Name)
		tenantByID[tenantRow.ID] = tenantRow
	}
	decorateTransactionPageRow(&row, source, allocations, obligations, tenants, tenantByID, nameByTenant, payers)
	row.Deferred, err = transactionDeferredState(ctx, a.db, userID, source.ID)
	if err != nil {
		return transactionDetailPageData{}, err
	}
	obligationByID := make(map[uint64]rentObligation, len(obligations))
	for _, obligation := range obligations {
		obligationByID[obligation.ID] = obligation
	}
	chargeIDs := make([]uint64, 0)
	seenChargeIDs := make(map[uint64]struct{})
	for _, obligation := range obligations {
		if obligation.RentChargeID == nil || *obligation.RentChargeID == 0 {
			continue
		}
		if _, exists := seenChargeIDs[*obligation.RentChargeID]; exists {
			continue
		}
		seenChargeIDs[*obligation.RentChargeID] = struct{}{}
		chargeIDs = append(chargeIDs, *obligation.RentChargeID)
	}
	charges := make(map[uint64]rentCharge)
	rooms := make(map[uint64]room)
	properties := make(map[uint64]property)
	if len(chargeIDs) > 0 {
		var chargeRows []rentCharge
		if err := a.db.WithContext(ctx).Where("user_id = ? AND id IN ?", userID, chargeIDs).Find(&chargeRows).Error; err != nil {
			return transactionDetailPageData{}, err
		}
		roomIDs := make([]uint64, 0, len(chargeRows))
		propertyIDs := make([]uint64, 0, len(chargeRows))
		for _, charge := range chargeRows {
			charges[charge.ID] = charge
			roomIDs = append(roomIDs, charge.RoomID)
			propertyIDs = append(propertyIDs, charge.PropertyID)
		}
		var roomRows []room
		if len(roomIDs) > 0 {
			if err := a.db.WithContext(ctx).Where("user_id = ? AND id IN ?", userID, roomIDs).Find(&roomRows).Error; err != nil {
				return transactionDetailPageData{}, err
			}
		}
		for _, roomRow := range roomRows {
			rooms[roomRow.ID] = roomRow
		}
		var propertyRows []property
		if len(propertyIDs) > 0 {
			if err := a.db.WithContext(ctx).Where("user_id = ? AND id IN ?", userID, propertyIDs).Find(&propertyRows).Error; err != nil {
				return transactionDetailPageData{}, err
			}
		}
		for _, propertyRow := range propertyRows {
			properties[propertyRow.ID] = propertyRow
		}
	}

	allocationRows := transactionDetailAllocationRows(allocations, source.Currency, obligationByID, charges, rooms, properties, nameByTenant)
	events, err := a.transactionDetailEvents(ctx, userID, source, allocations)
	if err != nil {
		return transactionDetailPageData{}, err
	}
	backURL := transactionListURL(r.URL.Path, r.URL.Query())
	pageKey := key
	if pageKey == "" {
		pageKey = strconv.FormatUint(source.ID, 10)
	}
	backRowURL := backURL + "#transaction-row-" + url.PathEscape(pageKey)
	data := transactionDetailPageData{
		workspaceShell:   a.transactionDetailShell(r, userID, filters),
		Transaction:      row,
		Title:            row.AmountDisplay + " · " + row.DirectionLabel,
		Subtitle:         row.AccountName + " · " + row.DateDisplay,
		StatusClass:      row.MatchStatus,
		BackURL:          backURL,
		BackRowURL:       backRowURL,
		ActionBase:       transactionListPath(r.URL.Path),
		TransactionTime:  formatTransactionTimestamp(source.TransactionTime),
		AllocatedAmount:  row.AllocatedAmountDisplay,
		RemainingAmount:  row.RemainingAmountDisplay,
		SourceLabel:      transactionSourceLabel(source.Source),
		ProviderID:       firstNonEmpty(stringValue(source.ProviderTransactionID), "—"),
		Reference:        firstNonEmpty(source.Reference, "—"),
		ParsedPeriod:     row.ParsedPeriodDisplay,
		ParsedPeriodNote: firstNonEmpty(source.ParsedPeriodNote, source.ParsedPeriodSource),
		RawRecord:        strings.TrimSpace(string(source.RawPayloadJSON)),
		Allocations:      allocationRows,
		AllocationCount:  effectiveAllocationCount(allocations),
		Events:           events,
	}
	data.HasRawRecord = data.RawRecord != ""
	if source.Direction == "income" && summary.AllocatedCents == 0 && source.MatchStatus != "ignored" {
		decision := decideStrictRentMatch(paymentTransactionInputFromModel(source), payers, tenants, obligations)
		if decision.TenantID != 0 {
			data.Suggestion.Available = decision.Status != "needs_review"
			data.Suggestion.Title = firstNonEmpty(nameByTenant[decision.TenantID], "已识别租客")
			if !decision.PeriodMonth.IsZero() {
				data.Suggestion.Subtitle = monthStart(decision.PeriodMonth).Format("2006年1月") + "租金责任"
			}
		}
		data.Suggestion.Status = firstNonEmpty(transactionMatchStatusLabel(decision.Status), "暂无匹配建议")
		data.Suggestion.Reason = firstNonEmpty(decision.Reason, source.MatchReason, "系统没有足够信息生成匹配建议。")
	}
	return data, nil
}

func (a *app) demoTransactionDetailPageData(r *http.Request, row transactionPageRow, key string, filters transactionFilters) transactionDetailPageData {
	row.DetailKey = key
	row.DetailURL = ""
	backURL := transactionListURL(r.URL.Path, r.URL.Query())
	return transactionDetailPageData{
		workspaceShell:  a.fillWorkspaceShell(r, workspaceShell{ActivePage: pageActiveTransactionKey(r), Username: a.displayUsername(r), Environment: a.cfg.Environment, FootNote: "银行流水与租金关联", CompactTitle: "流水详情", ShowNavCounts: true}),
		Transaction:     row,
		Title:           row.AmountDisplay + " · " + row.DirectionLabel,
		Subtitle:        row.AccountName + " · " + row.DateDisplay,
		StatusClass:     row.MatchStatus,
		BackURL:         backURL,
		BackRowURL:      backURL + "#transaction-row-" + url.PathEscape(key),
		ActionBase:      transactionListPath(r.URL.Path),
		TransactionTime: formatTransactionTimestamp(nil),
		AllocatedAmount: row.AllocatedAmountDisplay,
		RemainingAmount: row.RemainingAmountDisplay,
		SourceLabel:     transactionSourceLabel(row.Source),
		ProviderID:      row.ProviderTransactionID,
		Reference:       "—",
		ParsedPeriod:    row.ParsedPeriodDisplay,
	}
}

func pageActiveTransactionKey(r *http.Request) string {
	if r.URL.Path == "/billing" {
		return "billing"
	}
	return "transactions"
}

func (a *app) transactionDetailShell(r *http.Request, userID uint64, filters transactionFilters) workspaceShell {
	shell := canonicalPageShell(a, r, pageActiveTransactionKey(r), "银行流水详情")
	if a.db == nil || userID == 0 {
		return shell
	}
	_, total, err := newTransactionService(a.db).listTransactionsPage(r.Context(), userID, filters)
	if err == nil {
		shell.IncomeCount = int(total)
	}
	var count int64
	if err := a.db.WithContext(r.Context()).Model(&tenant{}).Where("user_id = ?", userID).Count(&count).Error; err == nil {
		shell.TenantCount = int(count)
	}
	if err := a.db.WithContext(r.Context()).Model(&manualExpense{}).Where("user_id = ?", userID).Count(&count).Error; err == nil {
		shell.ExpenseCount = int(count)
	}
	return shell
}

func transactionDetailAllocationRows(allocations []paymentAllocation, currency string, obligations map[uint64]rentObligation, charges map[uint64]rentCharge, rooms map[uint64]room, properties map[uint64]property, tenantNames map[uint64]string) []transactionDetailAllocationRow {
	rows := make([]transactionDetailAllocationRow, 0, len(allocations))
	for _, allocation := range allocations {
		row := transactionDetailAllocationRow{
			TenantName:  "未指定租客",
			KindLabel:   map[string]string{allocationKindRent: "房租", allocationKindDeposit: "押金", allocationKindOther: "其他收入"}[ledgerAllocationKind(allocation)],
			Amount:      formatMoney(centsToMoney(allocation.AmountCents), currency, 2),
			StatusLabel: "已撤销",
			StatusClass: "voided",
			Note:        allocation.Note,
			CreatedAt:   allocation.CreatedAt.UTC().Format("2006-01-02 15:04"),
			Effective:   ledgerAllocationIsEffective(allocation),
		}
		if row.KindLabel == "" {
			row.KindLabel = "其他分类"
		}
		if row.Effective {
			row.StatusLabel, row.StatusClass = "已确认", "paid"
		} else if allocation.Status != allocationStatusVoided {
			row.StatusLabel, row.StatusClass = "待处理", "needs_review"
		}
		if allocation.TenantID != nil {
			row.TenantName = firstNonEmpty(tenantNames[*allocation.TenantID], row.TenantName)
			row.TenantURL = fmt.Sprintf("/tenants/%d", *allocation.TenantID)
		}
		if allocation.RentObligationID != nil {
			if obligation, ok := obligations[*allocation.RentObligationID]; ok {
				row.TenantName = firstNonEmpty(tenantNames[obligation.TenantID], stringValue(obligation.TenantNameSnapshot), row.TenantName)
				row.TenantURL = fmt.Sprintf("/tenants/%d", obligation.TenantID)
				row.Period = monthStart(obligation.PeriodMonth).Format("2006年1月")
				if !obligation.DueDate.IsZero() {
					row.DueDate = obligation.DueDate.Format("2006-01-02")
				}
				row.ExpectedAmount = formatMoney(centsToMoney(obligation.ExpectedAmountCents), obligation.Currency, 2)
				row.BillLabel = fmt.Sprintf("责任 #%d", obligation.ID)
				row.BillURL = "/bills?period=" + monthStart(obligation.PeriodMonth).Format("2006-01")
				if obligation.RentChargeID != nil {
					if charge, ok := charges[*obligation.RentChargeID]; ok {
						row.PropertyName = firstNonEmpty(stringValue(charge.PropertyNameSnapshot), "")
						row.RoomLabel = firstNonEmpty(stringValue(charge.RoomLabelSnapshot), "")
						if roomRow, exists := rooms[charge.RoomID]; exists {
							row.RoomLabel = firstNonEmpty(roomRow.RoomLabel, row.RoomLabel)
							row.RoomURL = fmt.Sprintf("/rooms/%d", roomRow.ID)
						}
						if propertyRow, exists := properties[charge.PropertyID]; exists {
							row.PropertyName = firstNonEmpty(propertyRow.Name, row.PropertyName)
						}
					}
				}
			}
		}
		if row.BillLabel == "" {
			row.BillLabel = "无租金账单"
		}
		if row.CreatedAt == "" {
			row.CreatedAt = "—"
		}
		rows = append(rows, row)
	}
	return rows
}

func effectiveAllocationCount(allocations []paymentAllocation) int {
	count := 0
	for _, allocation := range allocations {
		if ledgerAllocationIsEffective(allocation) {
			count++
		}
	}
	return count
}

func (a *app) transactionDetailEvents(ctx context.Context, userID uint64, source paymentTransaction, allocations []paymentAllocation) ([]transactionDetailEvent, error) {
	events := make([]transactionDetailEvent, 0, len(allocations)*2+3)
	if source.TransactionTime != nil {
		events = append(events, transactionDetailEvent{Timestamp: source.TransactionTime.UTC().Format("2006-01-02 15:04"), Title: "银行流水发生", Detail: transactionSourceLabel(source.Source) + " · " + firstNonEmpty(stringValue(source.AccountName), "银行账户"), Status: "info"})
	}
	if !source.CreatedAt.IsZero() {
		events = append(events, transactionDetailEvent{Timestamp: source.CreatedAt.UTC().Format("2006-01-02 15:04"), Title: "流水记录已同步", Detail: firstNonEmpty(source.Description, source.Reference, transactionSourceLabel(source.Source)), Status: "info"})
	}
	var actions []paymentTransactionAction
	if err := a.db.WithContext(ctx).Where("user_id = ? AND payment_transaction_id = ?", userID, source.ID).Order("created_at ASC, id ASC").Find(&actions).Error; err != nil {
		return nil, err
	}
	for _, action := range actions {
		events = append(events, transactionDetailEvent{
			Timestamp: action.CreatedAt.UTC().Format("2006-01-02 15:04"),
			Title:     transactionActionLabel(action.ActionKind),
			Detail:    action.Reason,
			Status:    "warning",
		})
	}
	for _, allocation := range allocations {
		if allocation.Status == allocationStatusVoided && allocation.VoidedAt != nil {
			events = append(events, transactionDetailEvent{Timestamp: allocation.VoidedAt.UTC().Format("2006-01-02 15:04"), Title: "撤销分配", Detail: stringValue(allocation.VoidReason), Status: "warning"})
			continue
		}
		if allocation.Status == allocationStatusConfirmed {
			events = append(events, transactionDetailEvent{Timestamp: allocation.CreatedAt.UTC().Format("2006-01-02 15:04"), Title: "确认分配", Detail: fmt.Sprintf("%s · %s", formatMoney(centsToMoney(allocation.AmountCents), source.Currency, 2), allocationKindLabel(ledgerAllocationKind(allocation))), Status: "paid"})
		}
	}
	sort.SliceStable(events, func(i, j int) bool { return events[i].Timestamp < events[j].Timestamp })
	return events, nil
}

func transactionActionLabel(action string) string {
	return firstNonEmpty(map[string]string{
		transactionActionIgnore:            "标记为非租金流水",
		transactionActionRestore:           "恢复流水处理",
		transactionActionDefer:             "从首页待处理队列暂缓",
		transactionActionUndefer:           "重新加入首页待处理队列",
		transactionActionRevokeAllocations: "撤销原有分配",
	}[action], "记录流水操作")
}

func allocationKindLabel(kind string) string {
	return map[string]string{allocationKindRent: "房租", allocationKindDeposit: "押金", allocationKindOther: "其他收入"}[kind]
}

func transactionSourceLabel(source string) string {
	return firstNonEmpty(map[string]string{"truelayer": "TrueLayer 银行同步", "manual_expense": "费用记录", "manual_balance": "人工平账", "cash_receipt": "现金收款"}[source], source, "未知来源")
}

var transactionDetailPageTemplate = newEmbeddedWorkspacePageTemplate("transaction-detail-page", nil, "web/templates/pages/transaction-detail.html")
