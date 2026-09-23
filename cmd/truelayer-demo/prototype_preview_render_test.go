package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWritePrototypePreviewHTML(t *testing.T) {
	root := "/private/tmp/rentops-prototype-preview"
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	write := func(name string, render func() error) {
		t.Helper()
		if err := render(); err != nil {
			t.Fatalf("render %s: %v", name, err)
		}
	}
	writeFile := func(name string, execute func(*strings.Builder) error) error {
		var body strings.Builder
		if err := execute(&body); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(root, name), []byte(body.String()), 0o600)
	}
	write("expenses.html", func() error {
		return writeFile("expenses.html", func(body *strings.Builder) error {
			return expenseTemplate.Execute(body, expensePageData{
				workspaceShell: workspaceShell{ActivePage: "expenses", Username: "audit", Environment: "sandbox", FootNote: "费用支出", ShowNavCounts: true, ExpenseCount: 2},
				Period:         "2026-09", StatusFilter: "all", ShowForm: false, FilteredCount: 2, Today: "2026-09-19",
				Properties: []expensePropertyOption{{ID: 7, Name: "78 Old County Road"}}, Rooms: []expenseRoomOption{{ID: 3, PropertyID: 7, Label: "03"}},
				Rows: []expenseRecord{{ID: "1", PeriodDisplay: "2026-09", Description: "Repair shared kitchen tap", PropertyName: "78 Old County Road", RoomLabel: "03", Category: "维修", AmountDisplay: "€400.00", InvoiceLinked: true, InvoiceNumber: "INV-2609-018", InvoiceActionURL: "/expenses?invoice=1"}, {ID: "2", PeriodDisplay: "2026-09", Description: "Monthly maintenance", PropertyName: "72 Walkinstown Rd", Category: "房产支出", AmountDisplay: "€180.00", InvoiceActionURL: "/expenses?invoice=2"}},
			})
		})
	})
	write("expense-invoice.html", func() error {
		return writeFile("expense-invoice.html", func(body *strings.Builder) error {
			return expenseTemplate.Execute(body, expensePageData{
				workspaceShell: workspaceShell{ActivePage: "expenses", Username: "audit", Environment: "sandbox", FootNote: "费用支出", ShowNavCounts: true, ExpenseCount: 2},
				Period:         "2026-09", StatusFilter: "all", FilteredCount: 2, Today: "2026-09-19",
				InvoiceForm: &expenseInvoiceFormView{ExpenseID: "1", Description: "Repair shared kitchen tap", ExpenseAmount: "400.00", InvoiceNumber: "INV-2609-018", Vendor: "North Dublin Maintenance", InvoiceDate: "2026-09-01", InvoiceAmount: "400.00", Current: &expenseInvoiceView{ID: 8, InvoiceNumber: "INV-2609-018", Vendor: "North Dublin Maintenance", DateDisplay: "2026-09-01", AmountDisplay: "€400.00", DownloadURL: "/expenses/invoices/8"}, History: []expenseInvoiceView{{ID: 7, InvoiceNumber: "INV-2608-017", Vendor: "North Dublin Maintenance", CreatedAtDisplay: "2026-08-15", DownloadURL: "/expenses/invoices/7"}}, Period: "2026-09", StatusFilter: "all", ReturnURL: "/expenses?period=2026-09", PostURL: "/expenses/invoices?invoice=1&period=2026-09&status=all"},
				Properties:  []expensePropertyOption{{ID: 7, Name: "78 Old County Road"}}, Rooms: []expenseRoomOption{{ID: 3, PropertyID: 7, Label: "03"}},
				Rows: []expenseRecord{{ID: "1", PeriodDisplay: "2026-09", Description: "Repair shared kitchen tap", PropertyName: "78 Old County Road", RoomLabel: "03", Category: "维修", AmountDisplay: "€400.00", InvoiceLinked: true, InvoiceNumber: "INV-2609-018", InvoiceActionURL: "/expenses?invoice=1"}, {ID: "2", PeriodDisplay: "2026-09", Description: "Monthly maintenance", PropertyName: "72 Walkinstown Rd", Category: "房产支出", AmountDisplay: "€180.00", InvoiceActionURL: "/expenses?invoice=2"}},
			})
		})
	})
	write("bills.html", func() error {
		return writeFile("bills.html", func(body *strings.Builder) error {
			return billsPageTemplate.Execute(body, rentDashboardPageData{
				workspaceShell: workspaceShell{ActivePage: "bills", Username: "audit", Environment: "sandbox", FootNote: "应收账单", ShowNavCounts: true, CompactTitle: "应收账单", TenantCount: 3, IncomeCount: 4},
				PageKey:        "bills", CanonicalPath: "/bills", Period: "2026-09", PeriodLabel: "2026年9月", SearchFilter: "", StatusFilter: "unpaid", SortFilter: "due_asc", Page: 1, PageSize: 12, FilteredCount: 3, TotalRows: 3, TotalPages: 1,
				ExpectedTotal: "€2,980", PaidTotal: "€400", BalanceTotal: "€2,580", PendingCount: 3, UnpaidCount: 2, PaidCount: 1,
				Rows: []rentDashboardRow{
					{TenantID: 11, TenantName: "QIANG LI", RoomAddress: "169 Windmill Park", RoomLabel: "06", Period: "2026-09", DueDate: "2026-09-01", ExpectedAmount: "€1,200", PaidAmount: "€0", BalanceAmount: "€1,200", Status: "overdue", StatusLabel: "逾期", ObligationID: 301, ExpectedCents: 120000, PaidCents: 0},
					{TenantID: 12, TenantName: "CHENXI LIU", RoomAddress: "116 Kimmage Rd W", RoomLabel: "04", Period: "2026-09", DueDate: "2026-09-01", ExpectedAmount: "€880", PaidAmount: "€0", BalanceAmount: "€880", Status: "open", StatusLabel: "未结清", ObligationID: 302, ExpectedCents: 88000, PaidCents: 0},
					{TenantID: 13, TenantName: "BALA ANUSH CHOUDHARY", RoomAddress: "116 Kimmage Rd W", RoomLabel: "05", Period: "2026-09", DueDate: "2026-09-01", ExpectedAmount: "€900", PaidAmount: "€400", BalanceAmount: "€500", Status: "partial", StatusLabel: "部分缴纳", ObligationID: 303, ExpectedCents: 90000, PaidCents: 40000},
				},
			})
		})
	})
	write("dunning.html", func() error {
		return writeFile("dunning.html", func(body *strings.Builder) error {
			return dunningPageTemplate.Execute(body, rentDashboardPageData{
				workspaceShell: workspaceShell{ActivePage: "dunning", Username: "audit", Environment: "sandbox", FootNote: "催收任务", ShowNavCounts: true, CompactTitle: "催收任务", TenantCount: 3, IncomeCount: 4},
				PageKey:        "dunning", CanonicalPath: "/dunning", Period: "2026-09", PeriodLabel: "2026年9月", ExpectedTotal: "€2,980", PaidTotal: "€400", BalanceTotal: "€2,580", PendingCount: 3, UnpaidCount: 2, OverdueCount: 1,
				Dunning: dunningDrawerData{Period: "2026-09", SearchFilter: "", StatusFilter: "unpaid", SortFilter: "amount_desc", Page: 1, PageSize: 12, SenderConfigured: true, Sender: dunningSenderConfig{DisplayName: "RentOps Dublin", ReplyToEmail: "rent@example.ie"},
					Candidates: []dunningCandidate{
						{TenantID: 11, ObligationID: 301, TenantName: "QIANG LI", RoomAddress: "169 Windmill Park", RoomLabel: "06", Period: "2026-09", DueDate: "2026-09-01", BalanceAmount: "€1,200", BalanceCents: 120000, Status: "overdue", StatusLabel: "逾期", Email: "qiang@example.ie", EmailValid: true, Selectable: true, DefaultSelected: true, Selected: true},
						{TenantID: 12, ObligationID: 302, TenantName: "CHENXI LIU", RoomAddress: "116 Kimmage Rd W", RoomLabel: "04", Period: "2026-09", DueDate: "2026-09-01", BalanceAmount: "€880", BalanceCents: 88000, Status: "open", StatusLabel: "未结清", Email: "chenxi@example.ie", EmailValid: true, Selectable: true},
					}, RequestKey: "audit-dunning-preview"},
			})
		})
	})
	write("cash.html", func() error {
		return writeFile("cash.html", func(body *strings.Builder) error {
			return cashReceiptPageTemplate.Execute(body, cashReceiptPageData{
				workspaceShell: workspaceShell{ActivePage: "cash-receipts", Username: "audit", Environment: "sandbox", FootNote: "现金补录", ShowNavCounts: true},
				Period:         "2026-09", StatusFilter: "all", ShowForm: true,
				Form: cashReceiptFormData{Period: "2026-09", Currency: "EUR", ReceivedAt: "2026-09-19", IdempotencyKey: "cash-audit-preview", Tenants: []tenant{{ID: 11, Name: "Tenant A"}, {ID: 12, Name: "Tenant B"}}},
				Rows: []cashReceiptPageRow{{ID: 8, ReceiptNumber: "CASH-0008", TenantID: 11, TenantName: "Tenant A", RoomLabel: "78 Old County · 03", Amount: "€400.00", Period: "2026-09", ReceivedAt: "2026-09-19", Status: cashReceiptStatusConfirmed, StatusLabel: "已确认"}},
			})
		})
	})
	write("bank.html", func() error {
		return writeFile("bank.html", func(body *strings.Builder) error {
			return bankPageTemplate.Execute(body, bankPageData{
				workspaceShell: workspaceShell{ActivePage: "bank", Username: "audit", Environment: "sandbox", FootNote: "银行设置", ShowNavCounts: true, IncomeCount: 4},
				Connected:      true, Provider: "truelayer", Environment: "sandbox", SavedAt: "2026-09-19 09:40", LastSyncAt: "2026-09-19 09:42", SyncStatus: "succeeded", SyncStatusLabel: "成功", SyncCoverage: "2026-06-21 至 2026-09-19", LastSuccessfulCoverage: "2026-06-21 至 2026-09-19",
				Runs: []bankPageRun{{ID: 21, Mode: "manual", Status: "succeeded", StatusLabel: "成功", RequestedFrom: "2026-06-21", RequestedTo: "2026-09-19", StartedAt: "2026-09-19 09:42", FinishedAt: "2026-09-19 09:42", Accounts: []bankPageAccount{{AccountID: "acct-2841", AccountName: "AIB Current Account · 2841", Status: "succeeded", StatusLabel: "成功", CoveredFrom: "2026-06-21", CoveredTo: "2026-09-19", TransactionCount: 8}}}},
			})
		})
	})
	write("more.html", func() error {
		return writeFile("more.html", func(body *strings.Builder) error {
			return morePageTemplate.Execute(body, morePageData{workspaceShell: workspaceShell{ActivePage: "more", Username: "audit", Environment: "sandbox", FootNote: "全部功能", ShowNavCounts: true}})
		})
	})
	write("transactions.html", func() error {
		return writeFile("transactions.html", func(body *strings.Builder) error {
			return transactionListTemplate.Execute(body, transactionListPageData{
				workspaceShell: workspaceShell{ActivePage: "transactions", Username: "audit", Environment: "sandbox", FootNote: "流水匹配", CompactTitle: "流水处理"},
				CanonicalPath:  "/transactions", Connected: true, TransactionScope: "pending", PendingCount: 3,
				PeriodFilter: "2026-09", MatchStatusSelection: "pending", TotalTransactions: 3, Page: 1, PageSize: 50, TotalPages: 1,
				TransactionRows: []transactionPageRow{
					{ID: "10", InternalID: "10", DetailKey: "10", DetailURL: "/transactions?detail=10&match_status=pending&period=2026-09", Direction: "income", DirectionLabel: "收入", PayerName: "WAHAJULLAH KHAN", DateShort: "09-01", ObjectLabel: "78 Old County Road · 03", AmountDisplay: "€1,250.00", AllocatedAmountDisplay: "€0.00", RemainingAmountDisplay: "€1,250.00", RemainingAmountInput: "1250.00", AllocationUseDisplay: "同住代付", DateDisplay: "01 Sep 2026 09:12", Description: "RENT SEPT 03", AccountName: "AIB", MatchStatus: "candidate", MatchStatusLabel: "待确认", CandidateTenantName: "WAHAJULLAH KHAN", CandidateRentObligationID: 301, CandidatePeriod: "2026-09", CanConfirm: true, MatchReason: "建议拆为两条责任，各 €625；确认前可调整。", ParsedPeriodDisplay: "2026-09", ManualMatchTenantOptions: []billingTenantOption{{ID: 7, Name: "WAHAJULLAH KHAN"}, {ID: 11, Name: "Bríd Ní Bhraonáin"}}, ManualMatchOptions: []billingRentMatchOption{{TenantID: 7, TenantName: "WAHAJULLAH KHAN", Period: "2026-09", PeriodLabel: "2026 年 9 月", Expected: "€625.00", Remaining: "€625.00"}}, ReturnURL: "/transactions?match_status=pending&period=2026-09"},
					{ID: "11", InternalID: "11", DetailKey: "11", DetailURL: "/transactions?detail=11&match_status=pending&period=2026-09", Direction: "income", DirectionLabel: "收入", PayerName: "付款人名称待识别", DateShort: "09-04", ObjectLabel: "116 Kimmage Rd W · 08", AmountDisplay: "€800.00", AllocatedAmountDisplay: "€0.00", RemainingAmountDisplay: "€800.00", RemainingAmountInput: "800.00", DateDisplay: "04 Sep 2026 14:26", Description: "FASTER PAYMENT 88213", AccountName: "AIB", MatchStatus: "needs_review", MatchStatusLabel: "需处理", CandidateTenantName: "AJAY PANDIKASALA", CandidatePeriod: "2026-09", NeedsMonthChoice: true, TenantID: 8, MonthOptions: []billingMonthOption{{Period: "2026-09", Label: "2026 年 9 月", Expected: "€800", Paid: "€0", Remaining: "€800"}}, MatchReason: "金额吻合", ReturnURL: "/transactions?match_status=pending&period=2026-09"},
					// 已关联的行：这一行是"点错了想改"的现场。它必须同时给出改法和撤销，
					// 否则详情页那句"请先在流水列表撤销匹配"指的就是个死胡同。
					{ID: "12", InternalID: "12", DetailKey: "12", DetailURL: "/transactions?detail=12&match_status=pending&period=2026-09", Direction: "income", DirectionLabel: "收入", PayerName: "BRID NI BHRAONAIN", DateShort: "09-02", ObjectLabel: "72 Walkinstown Rd · 01", AmountDisplay: "€950.00", AllocatedAmountDisplay: "€950.00", RemainingAmountDisplay: "€0.00", RemainingAmountInput: "0.00", AllocationUseDisplay: "房租", DateDisplay: "02 Sep 2026 11:40", Description: "RENT SEPT", AccountName: "AIB", MatchStatus: "matched", MatchStatusLabel: "已关联", MatchedTenantName: "Bríd Ní Bhraonáin", CandidateTenantName: "Bríd Ní Bhraonáin", CandidatePeriod: "2026-09", MatchReason: "付款人与租客一致", ParsedPeriodDisplay: "2026-09", CanRematch: true, CanEditRentMatch: true, RematchTenantOptions: []billingTenantOption{{ID: 11, Name: "Bríd Ní Bhraonáin"}, {ID: 7, Name: "WAHAJULLAH KHAN"}}, RematchMonthOptions: []billingMonthOption{{Period: "2026-09", Label: "2026 年 9 月", Remaining: "€0.00"}, {Period: "2026-10", Label: "2026 年 10 月", Remaining: "€950.00"}}, ReturnURL: "/transactions?match_status=pending&period=2026-09"},
				},
			})
		})
	})
	write("transaction-match-review.html", func() error {
		return writeFile("transaction-match-review.html", func(body *strings.Builder) error {
			review := transactionMatchReviewData{
				Source:    transactionPageRow{ID: "11", InternalID: "11", DetailKey: "11", MatchURL: "/transactions?match=11&match_status=pending", DateDisplay: "12 Aug 2026 09:12", PayerName: "WAHAJULLAH KHAN", AmountDisplay: "€1,250.00", RemainingAmountDisplay: "€1,250.00", Description: "AUGUST RENT WAHAJULLAH KHAN VERY LONG BANK DESCRIPTION", MatchStatus: "needs_review", MatchStatusLabel: "需处理", ParsedPeriodDisplay: "2026-08", MatchReason: "租客已识别，但8月似乎已经交过。"},
				Reference: "IE0023260847", SelectedTenantID: 7, SelectedTenantName: "WAHAJULLAH KHAN", IdentifiedTenant: true, IdentityNote: "根据已保存的付款人关系识别；请与银行原文核对。", CloseURL: "/transactions?match_status=pending", ReturnURL: "/transactions?match_status=pending", FormAction: "/transactions", CanMatch: true, SourceAmountCents: 125000, SourceRemainingCents: 125000, RequestKey: "preview-review-key",
				TenantOptions: []transactionReviewTenant{{ID: 7, Name: "WAHAJULLAH KHAN", Selected: true}, {ID: 11, Name: "Bríd Ní Bhraonáin"}},
				Months:        []transactionReviewMonth{{Period: "2026-08", Label: "2026年8月", Expected: "€1,250.00", Paid: "€1,250.00", Remaining: "€0.00", Highlighted: true, Note: "本月已交清，请先核对下方原匹配流水", Evidence: []transactionReviewEvidence{{Date: "2026-08-01", PayerName: "WAHAJULLAH KHAN", Description: "AUGUST RENT RECEIVED ON 1ST", Amount: "€1,250.00", DetailURL: "/transactions?detail=10"}}}, {Period: "2026-09", Label: "2026年9月", Expected: "€625.00", Paid: "€0.00", Remaining: "€625.00", RemainingCents: 62500, Selectable: true, Coverage: "€625.00", SourceRemainder: "€625.00"}, {Period: "2026-10", Label: "2026年10月", Expected: "€625.00", Paid: "€0.00", Remaining: "€625.00", RemainingCents: 62500, Selectable: true, Coverage: "€625.00", SourceRemainder: "€0.00"}},
				History:       []transactionReviewHistory{{Date: "2026-08-01", PayerName: "WAHAJULLAH KHAN", Description: "AUGUST RENT RECEIVED ON 1ST", Amount: "€1,250.00", ParsedPeriod: "2026-08", MatchedPeriod: "2026-08", Status: "已关联", DetailURL: "/transactions?detail=10"}}, HistoryPage: 1, HistoryPages: 2, NextHistoryURL: "/transactions?match=11&match_tenant=7&match_history_page=2",
			}
			return transactionListTemplate.Execute(body, transactionListPageData{workspaceShell: workspaceShell{ActivePage: "transactions", Username: "audit", Environment: "sandbox", FootNote: "银行流水与租金关联", CompactTitle: "流水"}, CanonicalPath: "/transactions", TransactionScope: "pending", MatchReview: &review, TransactionRows: []transactionPageRow{review.Source}})
		})
	})
	write("transaction-match-list.html", func() error {
		return writeFile("transaction-match-list.html", func(body *strings.Builder) error {
			return transactionListTemplate.Execute(body, transactionListPageData{CanonicalPath: "/transactions", TransactionRows: []transactionPageRow{{ID: "11", InternalID: "11", DetailKey: "11", PayerName: "WAHAJULLAH KHAN", MatchURL: "/transactions?match=11&match_status=pending", MatchStatus: "needs_review", MatchStatusLabel: "需处理"}}})
		})
	})
	write("properties.html", func() error {
		return writeFile("properties.html", func(body *strings.Builder) error {
			return propertyPageTemplate.Execute(body, propertyPageData{
				workspaceShell: workspaceShell{ActivePage: "properties", Username: "audit", Environment: "sandbox", FootNote: "房产管理", CompactTitle: "对象管理"},
				Period:         "2026-09", PeriodLabel: "2026 年 9 月", PeriodOptions: pagePeriodOptions(parseTestPeriod(t, "2026-09")), StatusFilter: "all", CollectionFilter: "all", Search: "",
				Rows: []propertyPageRow{{ID: 7, Mark: "78", Name: "78 Old County Road", CityRegion: "Dublin 12", Address: "78 Old County Road, Dublin 12", Timezone: "Europe/Dublin", Status: "active", StatusLabel: "在用", CollectionStatus: "paid", CollectionStatusLabel: "已收齐", RoomCount: 6, ResponsibilityCount: 9, ExpectedCents: 553000, PaidCents: 553000, BalanceCents: 0, ExpectedAmount: "€5,530", PaidAmount: "€5,530", BalanceAmount: "€0"}},
			})
		})
	})
	write("property-detail-edit.html", func() error {
		return writeFile("property-detail-edit.html", func(body *strings.Builder) error {
			return propertyDetailPageTemplate.Execute(body, propertyDetailPageData{
				workspaceShell: workspaceShell{ActivePage: "properties", Username: "audit", Environment: "sandbox", FootNote: "房产详情", CompactTitle: "78 Old County Road"},
				Period:         "2026-09", PeriodLabel: "2026 年 9 月", ListStatus: "all",
				Property: propertyPageRow{ID: 7, Mark: "78", Name: "78 Old County Road", CityRegion: "Dublin 12", Address: "78 Old County Road, Dublin 12", Timezone: "Europe/Dublin", Notes: "房产备注", Status: "active", StatusLabel: "在用", CollectionStatus: "paid", CollectionStatusLabel: "已收齐", RoomCount: 6, ActiveRoomCount: 6, ResponsibilityCount: 9, ExpectedCents: 553000, PaidCents: 553000, BalanceCents: 0, ExpectedAmount: "€5,530", PaidAmount: "€5,530", BalanceAmount: "€0", ExpenseAmount: "€400", NetAmount: "€5,130", CollectionPercent: 100},
				Rooms:    []roomPageRow{{ID: 3, PropertyID: 7, PropertyName: "78 Old County Road", RoomLabel: "03", TenantNames: []string{"WAHAJULLAH KHAN", "Tenant B"}, MonthlyRent: "€1,250", ExpectedAmount: "€1,250", PaidAmount: "€1,250", BalanceAmount: "€0", Status: "paid", StatusLabel: "已收齐"}},
				Editing:  true,
			})
		})
	})
	write("rooms.html", func() error {
		return writeFile("rooms.html", func(body *strings.Builder) error {
			return roomPageTemplate.Execute(body, roomPageData{
				workspaceShell: workspaceShell{ActivePage: "rooms", Username: "audit", Environment: "sandbox", FootNote: "房间管理", CompactTitle: "对象管理"},
				Period:         "2026-09", PeriodLabel: "2026 年 9 月", PeriodOptions: pagePeriodOptions(parseTestPeriod(t, "2026-09")), StatusFilter: "all", CollectionFilter: "all", ShowForm: false, Form: roomPageForm{PropertyID: 7, Capacity: 2},
				Properties: []propertyPageRow{{ID: 7, Name: "78 Old County Road", CityRegion: "Dublin 12", Address: "78 Old County Road, Dublin 12"}},
				Rows:       []roomPageRow{{ID: 3, PropertyID: 7, PropertyName: "78 Old County Road", RoomLabel: "03", RoomType: "双人间", Capacity: 2, Status: "active", StatusLabel: "在用", CollectionStatus: "paid", CollectionStatusLabel: "已收齐", TenantNames: []string{"WAHAJULLAH KHAN", "Tenant B"}, ExpectedAmount: "€1,250", PaidAmount: "€1,250", BalanceAmount: "€0"}},
			})
		})
	})
	write("room-detail-edit.html", func() error {
		return writeFile("room-detail-edit.html", func(body *strings.Builder) error {
			return rentRoomDetailTemplate.Execute(body, rentRoomDetailPageData{
				workspaceShell: workspaceShell{ActivePage: "rooms", Username: "audit", Environment: "sandbox", FootNote: "房间详情", CompactTitle: "78 Old County Road · 房间 03"},
				Period:         "2026-09", PeriodLabel: "2026 年 9 月", RoomID: 3, RoomLabel: "03", RoomType: "双人间", Capacity: 2, MonthlyRent: "€1,250", DueDay: 1, PropertyName: "78 Old County Road",
				Summary: rentWorkspaceRoomRow{RoomID: 3, PropertyID: 7, Status: "paid", StatusLabel: "已收齐", TenantCount: 2, ExpectedAmount: "€1,250", PaidAmount: "€1,250", BalanceAmount: "€0", CollectionPercent: 100, DueDate: "2026-09-01"}, PaymentCount: 1,
				Form: roomPageForm{ID: 3, PropertyID: 7, RoomLabel: "03", RoomType: "双人间", Capacity: 2, Notes: "两人同住；两条个人责任各 €625。"}, Properties: []propertyPageRow{{ID: 7, Name: "78 Old County Road"}}, Editing: true,
			})
		})
	})
	write("property-detail.html", func() error {
		return writeFile("property-detail.html", func(body *strings.Builder) error {
			return propertyDetailPageTemplate.Execute(body, propertyDetailPageData{
				workspaceShell: workspaceShell{ActivePage: "properties", Username: "audit", Environment: "sandbox", FootNote: "房产详情", CompactTitle: "78 Old County Road"},
				Period:         "2026-09", PeriodLabel: "2026 年 9 月", ListStatus: "all", ListCollection: "all",
				Property: propertyPageRow{ID: 7, Mark: "78", Name: "78 Old County Road", CityRegion: "Dublin 12", Address: "78 Old County Road, Dublin 12", Timezone: "Europe/Dublin", Notes: "房产备注", Status: "active", StatusLabel: "在用", CollectionStatus: "paid", CollectionStatusLabel: "已收齐", RoomCount: 6, ActiveRoomCount: 6, ResponsibilityCount: 9, ExpectedCents: 553000, PaidCents: 553000, BalanceCents: 0, ExpectedAmount: "€5,530", PaidAmount: "€5,530", BalanceAmount: "€0", ExpenseAmount: "€400", NetAmount: "€5,130", CollectionPercent: 100},
				Rooms: []roomPageRow{
					{ID: 3, PropertyID: 7, PropertyName: "78 Old County Road", RoomLabel: "03", TenantNames: []string{"WAHAJULLAH KHAN", "Tenant B"}, MonthlyRent: "€1,250", ExpectedAmount: "€1,250", PaidAmount: "€1,250", BalanceAmount: "€0", Status: "paid", StatusLabel: "已收齐"},
					{ID: 5, PropertyID: 7, PropertyName: "78 Old County Road", RoomLabel: "05", TenantNames: []string{"SATVIK TALAWAR", "ANANYA M"}, MonthlyRent: "€1,200", ExpectedAmount: "€1,200", PaidAmount: "€1,200", BalanceAmount: "€0", Status: "paid", StatusLabel: "已收齐"},
				},
				Expenses: []expenseRecord{{DateDisplay: "2026-09-12", Description: "Property maintenance", Category: "维修", AmountDisplay: "€400.00"}},
			})
		})
	})
	write("property-detail-mobile.html", func() error {
		return writeFile("property-detail-mobile.html", func(body *strings.Builder) error {
			return propertyDetailPageTemplate.Execute(body, propertyDetailPageData{
				workspaceShell: workspaceShell{ActivePage: "properties", Username: "audit", Environment: "sandbox", FootNote: "房产详情", CompactTitle: "72 Walkinstown Rd"},
				Period:         "2026-09", PeriodLabel: "2026 年 9 月", ListStatus: "all", ListCollection: "all",
				Property: propertyPageRow{ID: 72, Mark: "72", Name: "72 Walkinstown Rd", CityRegion: "Dublin 12", Address: "72 Walkinstown Rd, Dublin 12", Timezone: "Europe/Dublin", Status: "partial", StatusLabel: "部分未收", CollectionStatus: "partial", CollectionStatusLabel: "部分未收", RoomCount: 8, ActiveRoomCount: 8, ResponsibilityCount: 12, ExpectedCents: 753000, PaidCents: 673000, BalanceCents: 80000, ExpectedAmount: "€7,530", PaidAmount: "€6,730", BalanceAmount: "€800", ExpenseAmount: "€400", NetAmount: "€6,330", CollectionPercent: 89},
				Rooms: []roomPageRow{
					{ID: 8, PropertyID: 72, PropertyName: "72 Walkinstown Rd", RoomLabel: "08", TenantNames: []string{"AJAY"}, MonthlyRent: "€800", ExpectedAmount: "€800", PaidAmount: "€0", BalanceAmount: "€800", Status: "open", StatusLabel: "未缴"},
					{ID: 2, PropertyID: 72, PropertyName: "72 Walkinstown Rd", RoomLabel: "02", TenantNames: []string{"两位租客"}, MonthlyRent: "€1,160", ExpectedAmount: "€1,160", PaidAmount: "€1,160", BalanceAmount: "€0", Status: "paid", StatusLabel: "已缴满"},
					{ID: 3, PropertyID: 72, PropertyName: "72 Walkinstown Rd", RoomLabel: "03", TenantNames: []string{"两位租客"}, MonthlyRent: "€1,200", ExpectedAmount: "€1,200", PaidAmount: "€1,200", BalanceAmount: "€0", Status: "paid", StatusLabel: "已缴满"},
				}, Expenses: []expenseRecord{{DateDisplay: "2026-09-12", Description: "房产支出", Category: "维修", AmountDisplay: "€400.00"}},
			})
		})
	})
	write("room-detail.html", func() error {
		return writeFile("room-detail.html", func(body *strings.Builder) error {
			return rentRoomDetailTemplate.Execute(body, rentRoomDetailPageData{
				workspaceShell: workspaceShell{ActivePage: "rooms", Username: "audit", Environment: "sandbox", FootNote: "房间详情", CompactTitle: "78 Old County Road · 房间 03"},
				Period:         "2026-09", PeriodLabel: "2026 年 9 月", RoomID: 3, RoomLabel: "03", RoomType: "双人间", Capacity: 2, MonthlyRent: "€1,250", MonthlyRentValue: "1250.00", DueDay: 1, PropertyName: "78 Old County Road", ReturnURL: "/rooms?period=2026-09", FromList: true,
				Summary: rentWorkspaceRoomRow{RoomID: 3, PropertyID: 7, Status: "paid", StatusLabel: "已收齐", TenantCount: 2, ExpectedCents: 125000, PaidCents: 125000, BalanceCents: 0, ExpectedAmount: "€1,250", PaidAmount: "€1,250", BalanceAmount: "€0", CollectionPercent: 100, DueDate: "2026-09-01"}, PaymentCount: 1,
				Tenants: []rentWorkspaceTenantRow{
					{TenantID: 11, TenantName: "WAHAJULLAH KHAN", ExpectedCents: 62500, PaidCents: 62500, BalanceCents: 0, ExpectedAmount: "€625", PaidAmount: "€625", BalanceAmount: "€0", Status: "paid", StatusLabel: "已缴清"},
					{TenantID: 12, TenantName: "同住人", ExpectedCents: 62500, PaidCents: 62500, BalanceCents: 0, ExpectedAmount: "€625", PaidAmount: "€625", BalanceAmount: "€0", Status: "paid", StatusLabel: "已缴清", PaidByOther: true},
				},
				Expenses: []rentWorkspaceExpenseView{{Description: "Kitchen repair", Category: "维修", ExpenseDate: "2026-09-12", Amount: "€50"}},
			})
		})
	})
	write("room-detail-overdue.html", func() error {
		return writeFile("room-detail-overdue.html", func(body *strings.Builder) error {
			return rentRoomDetailTemplate.Execute(body, rentRoomDetailPageData{
				workspaceShell: workspaceShell{ActivePage: "rooms", Username: "audit", Environment: "sandbox", FootNote: "房间详情", CompactTitle: "72 Walkinstown Rd · 房间 08"},
				Period:         "2026-09", PeriodLabel: "2026 年 9 月", RoomID: 8, RoomLabel: "08", RoomType: "单人间", Capacity: 1, MonthlyRent: "€800", MonthlyRentValue: "800.00", DueDay: 1, PropertyName: "72 Walkinstown Rd", ReturnURL: "/rooms?period=2026-09", FromList: true,
				Summary: rentWorkspaceRoomRow{RoomID: 8, PropertyID: 72, Status: "overdue", StatusLabel: "已逾期", TenantCount: 1, ExpectedCents: 80000, PaidCents: 0, BalanceCents: 80000, ExpectedAmount: "€800", PaidAmount: "€0", BalanceAmount: "€800", CollectionPercent: 0, DueDate: "2026-09-01"},
				Tenants: []rentWorkspaceTenantRow{
					{TenantID: 21, TenantName: "AJAY", ExpectedCents: 80000, PaidCents: 0, BalanceCents: 80000, ExpectedAmount: "€800", PaidAmount: "€0", BalanceAmount: "€800", Status: "overdue", StatusLabel: "已逾期"},
				},
			})
		})
	})
	// 流水详情是这一版改动最大的页面，之前没有预览夹具，改完只能靠读模板确认。
	// 给一份"待确认"状态的流水：右栏的匹配表单、左栏的原始流水都在这一个状态下
	// 才同时有内容。
	write("transaction-detail.html", func() error {
		return writeFile("transaction-detail.html", func(body *strings.Builder) error {
			return transactionDetailPageTemplate.Execute(body, transactionDetailPageData{
				workspaceShell:   workspaceShell{ActivePage: "transactions", Username: "audit", Environment: "sandbox", FootNote: "流水详情", CompactTitle: "EUR 640.00 · 收入"},
				Title:            "EUR 640.00 · 收入",
				Subtitle:         "AIB Current Account · 2026-09-12 09:18",
				StatusClass:      "candidate",
				ActionBase:       "/transactions",
				BackURL:          "/transactions?period=2026-09&match_status=pending",
				AllocatedAmount:  "EUR 0.00",
				RemainingAmount:  "EUR 640.00",
				SourceLabel:      "TrueLayer 银行同步",
				ProviderID:       "tl-8f21c4",
				Reference:        "RENT SEPT 2026",
				RawRecord:        "{\n  \"amount\": 640.00,\n  \"currency\": \"GBP\",\n  \"timestamp\": \"2026-09-12T09:18:00Z\",\n  \"description\": \"CREDIT TRANSFER\\nC. CHEN\\nRENT SEPT 2026\"\n}",
				HasRawRecord:     true,
				TransactionTime:  "2026-09-12 09:18",
				ParsedPeriod:     "2026-09",
				ParsedPeriodNote: "由摘要中的 SEPT 识别",
				Transaction: transactionPageRow{
					ID:                        "42",
					Direction:                 "income",
					MatchStatus:               "candidate",
					MatchStatusLabel:          "待确认",
					PayerName:                 "C. CHEN",
					PayerID:                   "payer-9",
					AccountName:               "AIB Current Account",
					AccountID:                 "acc-2",
					Description:               "CREDIT TRANSFER\nC. CHEN\nRENT SEPT 2026",
					CanConfirm:                true,
					CandidateTenantName:       "C. CHEN",
					CandidatePeriod:           "2026-09",
					CandidateRentObligationID: 77,
					ManualMatchTenantOptions:  []billingTenantOption{{ID: 9, Name: "C. CHEN"}, {ID: 11, Name: "Aoife Murphy"}},
					ManualMatchOptions: []billingRentMatchOption{
						{TenantID: 9, TenantName: "C. CHEN", Period: "2026-09", PeriodLabel: "2026年9月", Expected: "€640.00", Remaining: "€640.00"},
						{TenantID: 11, TenantName: "Aoife Murphy", Period: "2026-09", PeriodLabel: "2026年9月", Expected: "€1,100.00", Remaining: "€400.00"},
					},
				},
				// 「归类／拆分」的租客下拉在页面级数据上，不在行上（老表格用的就是
				// 页面级的 $.TenantOptions）。漏了这一条，预览里的下拉只剩「不指定租客」，
				// 看着像坏了，其实是夹具没给。
				TenantOptions: []billingTenantOption{{ID: 9, Name: "C. CHEN"}, {ID: 11, Name: "Aoife Murphy"}},
			})
		})
	})
	write("rent-workspace-review.html", func() error {
		return writeFile("rent-workspace-review.html", func(body *strings.Builder) error {
			period := parseTestPeriod(t, "2026-09")
			filters := defaultRentWorkspaceFilters(period)
			review := transactionMatchReviewData{
				Source:       transactionPageRow{ID: "10", PayerName: "WAHAJULLAH KHAN", AmountDisplay: "€1,250.00", RemainingAmountDisplay: "€1,250.00", DateDisplay: "2026-09-01", ParsedPeriodDisplay: "2026-09", Description: "RENT SEPT 03", MatchStatus: "unmatched", MatchStatusLabel: "未关联", DetailURL: "/transactions?detail=10"},
				HistoryTotal: 2, HistoryPage: 1, HistoryPages: 1,
				History: []transactionReviewHistory{
					{Date: "2026-08-01", PayerName: "WAHAJULLAH KHAN", Amount: "€1,250.00", Description: "RENT AUG 03", ParsedPeriod: "2026-08", MatchedPeriod: "2026-08", Status: "已关联", DetailURL: "/transactions?detail=9"},
					{Date: "2026-07-01", PayerName: "WAHAJULLAH KHAN", Amount: "€1,250.00", Description: "RENT JUL 03", ParsedPeriod: "2026-07", MatchedPeriod: "—", Status: "未关联", DetailURL: "/transactions?detail=8"},
				},
			}
			review.setWorkspaceURLs(filters, nil)
			return rentWorkspaceTemplate.Execute(body, rentWorkspacePageData{
				workspaceShell: workspaceShell{ActivePage: "rent-dashboard", Username: "audit", Environment: "sandbox", CompactTitle: "本月收租"},
				Filters:        filters, Period: "2026-09", PeriodLabel: "2026 年 9 月", View: rentWorkspaceViewProperties,
				PendingCount: 1, PendingItems: []rentWorkspacePendingItem{{Index: 1, ID: 10, Title: "WAHAJULLAH KHAN", Subtitle: "09-01 · RENT SEPT 03", Amount: "€1,250.00", MatchURL: "/rent-dashboard?period=2026-09&match=10"}},
				MatchReview: &review,
			})
		})
	})
	write("rent-workspace.html", func() error {
		return writeFile("rent-workspace.html", func(body *strings.Builder) error {
			period := parseTestPeriod(t, "2026-09")
			tenantRows := []rentWorkspaceTenantRow{
				{TenantID: 11, PropertyID: 7, PropertyName: "78 Old County Road", RoomID: 3, RoomLabel: "03", TenantName: "WAHAJULLAH KHAN", ObligationID: 301, Period: "2026-09", ExpectedCents: 62500, PaidCents: 62500, ExpectedAmount: "€625", PaidAmount: "€625", BalanceAmount: "€0", Status: "paid", StatusLabel: "已缴清"},
				{TenantID: 12, PropertyID: 7, PropertyName: "78 Old County Road", RoomID: 3, RoomLabel: "03", TenantName: "同住人", ObligationID: 302, Period: "2026-09", ExpectedCents: 62500, PaidCents: 0, BalanceCents: 62500, ExpectedAmount: "€625", PaidAmount: "€0", BalanceAmount: "€625", Status: "partial", StatusLabel: "部分缴纳", PaidByOther: true},
			}
			room := rentWorkspaceRoomRow{RoomID: 3, PropertyID: 7, PropertyName: "78 Old County Road", RoomLabel: "03", Address: "78 Old County Road, Dublin 12", Status: "partial", StatusLabel: "部分缴纳", TenantCount: 2, ExpectedAmount: "€1,250", PaidAmount: "€625", BalanceAmount: "€625", DueDate: "2026-09-01", CollectionPercent: 50}
			property := rentWorkspacePropertyRow{PropertyID: 7, Name: "78 Old County Road", Address: "78 Old County Road, Dublin 12", Status: "partial", StatusLabel: "部分缴纳", TotalRooms: 6, PaidRooms: 2, UnpaidRooms: 3, ExpectedAmount: "€5,530", PaidAmount: "€5,130", BalanceAmount: "€400", ExpenseAmount: "€400", NetAmount: "€4,730"}
			return rentWorkspaceTemplate.Execute(body, rentWorkspacePageData{
				workspaceShell: workspaceShell{ActivePage: "rent-dashboard", Username: "audit", Environment: "sandbox", FootNote: "月度收租工作台", CompactTitle: "本月收租", ShowNavCounts: true},
				Filters:        rentWorkspaceFilters{PeriodMonth: period, View: rentWorkspaceViewProperties, Status: "all", Page: 1, PageSize: 12}, Period: "2026-09", PeriodLabel: "2026 年 9 月", PreviousPeriod: "2026-08", NextPeriod: "2026-10", View: rentWorkspaceViewProperties,
				Summary:      rentWorkspaceSummary{ExpectedCents: 2621000, PaidCents: 2279000, BalanceCents: 342000, ExpectedAmount: "€26,210", PaidAmount: "€22,790", BalanceAmount: "€3,420", ExpenseAmount: "€1,600", NetAmount: "€21,190", CollectionPercent: 86, TotalRooms: 25, PaidRooms: 15, UnpaidRooms: 7, VacantRooms: 3, ResponsibilityCount: 40, FollowupCount: 4},
				PendingCount: 3, PendingItems: []rentWorkspacePendingItem{{
					Index: 1, ID: 10, Title: "WAHAJULLAH KHAN", Subtitle: "09-01 · 同住代付待确认", Amount: "€1,250", MatchURL: "/rent-dashboard?period=2026-09&match=10",
				}, {Index: 2, ID: 11, Title: "付款人待识别", Subtitle: "09-04 · AIB · RENT SEPT", Amount: "€800", MatchURL: "/rent-dashboard?period=2026-09&match=11"}}, PropertyOptions: []rentWorkspacePropertyOption{{ID: 7, Name: "78 Old County Road"}},
				PropertyRows: []rentWorkspacePropertyRow{property}, PropertyTreeRows: []rentWorkspacePropertyTreeRow{{Property: property, Rooms: []rentWorkspaceRoomTreeRow{{Room: room, Tenants: tenantRows}}}},
				RoomRows: []rentWorkspaceRoomRow{room}, RoomTreeRows: []rentWorkspaceRoomTreeRow{{Room: room, Tenants: tenantRows}}, TenantRows: tenantRows, TotalRows: 1, FilteredCount: 1, TotalPages: 1, Page: 1, PageSize: 12,
			})
		})
	})
}
