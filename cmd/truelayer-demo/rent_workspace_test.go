package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRentWorkspaceFiltersFromQueryDefaultsToPropertiesAndPreservesContext(t *testing.T) {
	filters, err := rentWorkspaceFiltersFromQuery(url.Values{
		"period":      []string{"2026-09"},
		"property_id": []string{"12"},
		"room_id":     []string{"34"},
		"search":      []string{"  Aoife  "},
		"status":      []string{"unpaid"},
		"page":        []string{"2"},
		"page_size":   []string{"24"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := rentWorkspaceFilters{
		PeriodMonth: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		View:        rentWorkspaceViewProperties,
		PropertyID:  12,
		RoomID:      34,
		Search:      "Aoife",
		Status:      "unpaid",
		Sort:        dashboardDefaultSort,
		Page:        2,
		PageSize:    24,
	}
	if !reflect.DeepEqual(filters, want) {
		t.Fatalf("filters=%+v want %+v", filters, want)
	}
	workspaceURL := rentWorkspaceURL(filters, 3)
	parsed, err := url.Parse(workspaceURL)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Path != "/rent-dashboard" || parsed.Query().Get("view") != "properties" || parsed.Query().Get("period") != "2026-09" || parsed.Query().Get("property_id") != "12" || parsed.Query().Get("room_id") != "34" || parsed.Query().Get("search") != "Aoife" || parsed.Query().Get("status") != "unpaid" || parsed.Query().Get("page") != "3" || parsed.Query().Get("page_size") != "24" {
		t.Fatalf("workspace URL=%q", workspaceURL)
	}
}

func TestRentWorkspaceFiltersRejectInvalidScopeAndView(t *testing.T) {
	for name, query := range map[string]url.Values{
		"view":     {"view": []string{"invalid"}},
		"property": {"property_id": []string{"nope"}},
		"room":     {"room_id": []string{"0"}},
		"status":   {"status": []string{"unknown"}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := rentWorkspaceFiltersFromQuery(query); err == nil {
				t.Fatal("expected invalid workspace filter error")
			}
		})
	}
}

func TestBuildRentWorkspaceSeparatesVacantMissingChargeAndPaidRooms(t *testing.T) {
	period := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	propertyOne := property{ID: 1, UserID: 7, Name: "Canal House", Status: "active"}
	propertyTwo := property{ID: 2, UserID: 7, Name: "Park House", Status: "active"}
	roomPaid := room{ID: 11, UserID: 7, PropertyID: 1, RoomLabel: "A-01", Status: "active", ActiveFrom: period.AddDate(0, -3, 0)}
	roomVacant := room{ID: 12, UserID: 7, PropertyID: 1, RoomLabel: "A-02", Status: "active", ActiveFrom: period.AddDate(0, -3, 0)}
	roomMissingCharge := room{ID: 21, UserID: 7, PropertyID: 2, RoomLabel: "P-01", Status: "active", ActiveFrom: period.AddDate(0, -3, 0)}
	tenantOne := tenant{ID: 101, UserID: 7, Name: "Aoife Murphy", Status: "active", RentStartDate: period.AddDate(0, -3, 0)}
	tenantTwo := tenant{ID: 102, UserID: 7, Name: "Zoe Byrne", Status: "active", RentStartDate: period.AddDate(0, -3, 0)}
	agreementPaid := tenancyAgreement{ID: 301, UserID: 7, RoomID: 11, Status: "active", StartDate: period.AddDate(0, -3, 0), MonthlyRentCents: 100000, Currency: "EUR", DueDay: 5}
	agreementMissing := tenancyAgreement{ID: 302, UserID: 7, RoomID: 21, Status: "active", StartDate: period.AddDate(0, -3, 0), MonthlyRentCents: 80000, Currency: "EUR", DueDay: 5}
	charge := rentCharge{ID: 401, UserID: 7, PropertyID: 1, RoomID: 11, TenancyAgreementID: agreementPaid.ID, PeriodMonth: period, DueDate: dueDateForMonth(period, 5), ExpectedAmountCents: 100000, Currency: "EUR", RecordStatus: obligationRecordActive}
	obligationOne := rentObligation{ID: 501, UserID: 7, RentChargeID: &charge.ID, TenantID: tenantOne.ID, TenantNameSnapshot: nullableString(tenantOne.Name), PeriodMonth: period, DueDate: charge.DueDate, ExpectedAmountCents: 60000, Currency: "EUR", RecordStatus: obligationRecordActive}
	obligationTwo := rentObligation{ID: 502, UserID: 7, RentChargeID: &charge.ID, TenantID: tenantTwo.ID, TenantNameSnapshot: nullableString(tenantTwo.Name), PeriodMonth: period, DueDate: charge.DueDate, ExpectedAmountCents: 40000, Currency: "EUR", RecordStatus: obligationRecordActive}
	transaction := paymentTransaction{ID: 601, UserID: 7, Direction: "income", AmountCents: 100000, Currency: "EUR", MatchedTenantID: &tenantOne.ID, PayerName: nullableString(tenantOne.Name), TransactionTime: timePtr(period.AddDate(0, 0, 4))}
	allocationOne := paymentAllocation{ID: 701, UserID: 7, PaymentTransactionID: transaction.ID, RentObligationID: &obligationOne.ID, TenantID: &tenantOne.ID, AmountCents: 60000, AllocationKind: allocationKindRent, Status: allocationStatusConfirmed, ConfirmationSource: "manual"}
	allocationTwo := paymentAllocation{ID: 702, UserID: 7, PaymentTransactionID: transaction.ID, RentObligationID: &obligationTwo.ID, TenantID: &tenantTwo.ID, AmountCents: 40000, AllocationKind: allocationKindRent, Status: allocationStatusConfirmed, ConfirmationSource: "manual"}
	expensePropertyID := propertyOne.ID
	expenseRoomID := roomPaid.ID
	expenses := []manualExpense{
		{ID: 801, UserID: 7, PropertyID: &expensePropertyID, AmountCents: 5000, Currency: "EUR", ExpenseDate: period.AddDate(0, 0, 3), RecordStatus: obligationRecordActive},
		{ID: 802, UserID: 7, PropertyID: &expensePropertyID, RoomID: &expenseRoomID, AmountCents: 10000, Currency: "EUR", ExpenseDate: period.AddDate(0, 0, 6), RecordStatus: obligationRecordActive},
	}

	data, err := buildRentWorkspace(rentWorkspaceInput{
		UserID:       7,
		PeriodMonth:  period,
		Properties:   []property{propertyOne, propertyTwo},
		Rooms:        []room{roomPaid, roomVacant, roomMissingCharge},
		Agreements:   []tenancyAgreement{agreementPaid, agreementMissing},
		Parties:      []agreementParty{{UserID: 7, AgreementID: agreementPaid.ID, TenantID: tenantOne.ID, ResponsibilityCents: 60000, Status: "active"}, {UserID: 7, AgreementID: agreementPaid.ID, TenantID: tenantTwo.ID, ResponsibilityCents: 40000, Status: "active"}, {UserID: 7, AgreementID: agreementMissing.ID, TenantID: tenantOne.ID, ResponsibilityCents: 80000, Status: "active"}},
		Charges:      []rentCharge{charge},
		Obligations:  []rentObligation{obligationOne, obligationTwo},
		Tenants:      []tenant{tenantOne, tenantTwo},
		Transactions: []paymentTransaction{transaction},
		Allocations:  []paymentAllocation{allocationOne, allocationTwo},
		Expenses:     expenses,
		Now:          time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC),
	}, defaultRentWorkspaceFilters(period))
	if err != nil {
		t.Fatal(err)
	}
	if len(data.RoomRows) != 3 || data.RoomRows[0].Status != "needs_review" || data.RoomRows[0].TenantCount != 1 || data.RoomRows[1].Status != "paid" || data.RoomRows[2].Status != "vacant" {
		t.Fatalf("room rows=%+v", data.RoomRows)
	}
	if data.Summary.ExpectedCents != 100000 || data.Summary.PaidCents != 100000 || data.Summary.BalanceCents != 0 || data.Summary.ExpenseCents != 15000 {
		t.Fatalf("summary=%+v", data.Summary)
	}
	if len(data.PropertyRows) != 2 || data.PropertyRows[0].Status != "needs_review" || data.PropertyRows[1].Status != "paid" {
		t.Fatalf("property rows=%+v", data.PropertyRows)
	}
	if data.PropertyRows[1].TotalRooms != 2 || data.PropertyRows[1].PaidRooms != 1 || data.PropertyRows[1].UnpaidRooms != 0 || data.PropertyRows[1].ExpenseCents != 15000 || data.PropertyRows[1].NetCents != 85000 {
		t.Fatalf("paid property row=%+v", data.PropertyRows[1])
	}
	if len(data.TenantRows) != 2 || !data.TenantRows[1].PaidByOther || data.TenantRows[0].PaidByOther {
		t.Fatalf("tenant rows=%+v", data.TenantRows)
	}
}

func TestBuildRentWorkspaceStatusFilterAndSearchDoNotChangeSummary(t *testing.T) {
	period := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	propertyRow := property{ID: 1, UserID: 7, Name: "Canal House", Status: "active"}
	roomRow := room{ID: 11, UserID: 7, PropertyID: propertyRow.ID, RoomLabel: "A-01", Status: "active", ActiveFrom: period.AddDate(0, -1, 0)}
	tenantRow := tenant{ID: 101, UserID: 7, Name: "Aoife Murphy", DisplayAlias: "Aoife", Status: "active", RentStartDate: period.AddDate(0, -1, 0)}
	agreement := tenancyAgreement{ID: 301, UserID: 7, RoomID: roomRow.ID, Status: "active", StartDate: period.AddDate(0, -1, 0), MonthlyRentCents: 100000, Currency: "EUR", DueDay: 5}
	charge := rentCharge{ID: 401, UserID: 7, PropertyID: propertyRow.ID, RoomID: roomRow.ID, TenancyAgreementID: agreement.ID, PeriodMonth: period, DueDate: dueDateForMonth(period, 5), ExpectedAmountCents: 100000, Currency: "EUR", RecordStatus: obligationRecordActive}
	obligation := rentObligation{ID: 501, UserID: 7, RentChargeID: &charge.ID, TenantID: tenantRow.ID, PeriodMonth: period, DueDate: charge.DueDate, ExpectedAmountCents: 100000, Currency: "EUR", RecordStatus: obligationRecordActive}
	allData := rentWorkspaceInput{UserID: 7, PeriodMonth: period, Properties: []property{propertyRow}, Rooms: []room{roomRow}, Agreements: []tenancyAgreement{agreement}, Parties: []agreementParty{{UserID: 7, AgreementID: agreement.ID, TenantID: tenantRow.ID, ResponsibilityCents: 100000, Status: "active"}}, Charges: []rentCharge{charge}, Obligations: []rentObligation{obligation}, Tenants: []tenant{tenantRow}, Now: time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)}
	all, err := buildRentWorkspace(allData, defaultRentWorkspaceFilters(period))
	if err != nil {
		t.Fatal(err)
	}
	filtered := defaultRentWorkspaceFilters(period)
	filtered.Search = "missing"
	filtered.Status = "unpaid"
	filtered.PageSize = 1
	filtered.Page = 1
	filteredData, err := buildRentWorkspace(allData, filtered)
	if err != nil {
		t.Fatal(err)
	}
	if all.Summary != filteredData.Summary {
		t.Fatalf("summary changed with list filters: all=%+v filtered=%+v", all.Summary, filteredData.Summary)
	}
	if filteredData.TotalRows != 1 || filteredData.FilteredCount != 0 || len(filteredData.RoomRows) != 0 {
		t.Fatalf("filtered page=%+v total=%d filtered=%d", filteredData.RoomRows, filteredData.TotalRows, filteredData.FilteredCount)
	}
	if !strings.Contains(rentWorkspaceURL(filtered, 1), "status=unpaid") {
		t.Fatal("workspace URL dropped status context")
	}
}

func TestRentWorkspaceTemplateRendersThreeViewsAndRoomDrilldownWithoutReferenceNumber(t *testing.T) {
	var body strings.Builder
	filters := defaultRentWorkspaceFilters(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))
	data := rentWorkspacePageData{
		Period:          "2026-09",
		PeriodLabel:     "2026年9月",
		View:            rentWorkspaceViewProperties,
		Filters:         filters,
		Summary:         rentWorkspaceSummary{ExpectedAmount: "EUR 1,000.00", PaidAmount: "EUR 1,000.00", BalanceAmount: "EUR 0.00", NetAmount: "EUR 850.00", TotalRooms: 2, PaidRooms: 1, VacantRooms: 1},
		PropertyOptions: []rentWorkspacePropertyOption{{ID: 1, Name: "Canal House"}},
		PropertyRows:    []rentWorkspacePropertyRow{{PropertyID: 1, Name: "Canal House", TotalRooms: 2, PaidRooms: 1, ExpectedAmount: "EUR 1,000.00", PaidAmount: "EUR 1,000.00", BalanceAmount: "EUR 0.00", ExpenseAmount: "EUR 150.00", NetAmount: "EUR 850.00", Status: "paid", StatusLabel: "已缴清"}},
		TotalRows:       1,
		FilteredCount:   1,
		TotalPages:      1,
		Page:            1,
		PageSize:        dashboardDefaultPageSize,
	}
	if err := rentWorkspaceTemplate.Execute(&body, data); err != nil {
		t.Fatal(err)
	}
	page := body.String()
	for _, expected := range []string{"房产视角", "房间视角", "租客视角", "Canal House", "经营净额", "workspace-property"} {
		if !strings.Contains(page, expected) {
			t.Fatalf("workspace missing %q: %s", expected, page)
		}
	}
	if strings.Contains(page, "参考号") || strings.Contains(page, "rent-2026-09") {
		t.Fatalf("workspace must not render payment reference number: %s", page)
	}
}

func TestRoomDetailHandlerRejectsInvalidPathBeforeDatabaseAccess(t *testing.T) {
	a := testApp()
	req := httptest.NewRequest(http.MethodGet, "/rooms/not-a-number?period=2026-09", nil)
	req.AddCookie(sessionCookie(a.cfg, time.Now().Add(sessionTTL)))
	rec := httptest.NewRecorder()
	a.handleRoomDetail(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "room path is invalid") {
		t.Fatalf("room detail response=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestRentWorkspaceServiceAggregatesPropertiesRoomsAndDetailOnMySQL(t *testing.T) {
	db, sqlDB := openLedgerMySQLTestDB(t)
	if err := runMigrations(sqlDB, "../../migrations"); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	ctx := context.Background()
	owner := user{Username: "workspace-owner-" + time.Now().UTC().Format("20060102150405.000000000"), PasswordHash: "test"}
	if err := db.WithContext(ctx).Create(&owner).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.WithContext(ctx).Delete(&user{}, owner.ID).Error })
	primaryTenant := repositoryTestTenant(owner.ID, "Primary tenant")
	if err := db.WithContext(ctx).Create(&primaryTenant).Error; err != nil {
		t.Fatal(err)
	}
	repo := newLandlordRentRepository(db)
	propertyOne, err := repo.createProperty(ctx, owner.ID, property{Name: "Workspace House"})
	if err != nil {
		t.Fatal(err)
	}
	propertyTwo, err := repo.createProperty(ctx, owner.ID, property{Name: "Empty House"})
	if err != nil {
		t.Fatal(err)
	}
	period := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	roomOne, err := repo.createRoom(ctx, owner.ID, room{PropertyID: propertyOne.ID, RoomLabel: "A-01", ActiveFrom: period.AddDate(0, -2, 0)})
	if err != nil {
		t.Fatal(err)
	}
	roomTwo, err := repo.createRoom(ctx, owner.ID, room{PropertyID: propertyTwo.ID, RoomLabel: "E-01", ActiveFrom: period.AddDate(0, -2, 0)})
	if err != nil {
		t.Fatal(err)
	}
	agreement, err := repo.createTenancyAgreement(ctx, owner.ID, tenancyAgreement{RoomID: roomOne.ID, StartDate: period.AddDate(0, -2, 0), MonthlyRentCents: 100000, Currency: "EUR", DueDay: 5})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.createAgreementParty(ctx, owner.ID, agreementParty{AgreementID: agreement.ID, TenantID: primaryTenant.ID, ResponsibilityCents: 100000}); err != nil {
		t.Fatal(err)
	}
	ledger := newRentLedgerService(db)
	charge, err := ledger.ensureRentCharge(ctx, owner.ID, propertyOne.ID, roomOne.ID, period)
	if err != nil {
		t.Fatal(err)
	}
	transaction := paymentTransaction{UserID: owner.ID, Source: "test", StableTransactionKey: "workspace-payment-" + time.Now().UTC().Format("20060102150405.000000000"), Direction: "income", AmountCents: 100000, Currency: "EUR", MatchedTenantID: &primaryTenant.ID, MatchStatus: "unmatched"}
	if err := db.WithContext(ctx).Create(&transaction).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := newTransactionService(db).allocateTransaction(ctx, owner.ID, transaction.ID, []transactionAllocationDraft{{TenantID: primaryTenant.ID, RentObligationID: charge.Obligations[0].ID, AmountCents: 100000, Kind: allocationKindRent}}, "workspace-allocation", "manual"); err != nil {
		t.Fatal(err)
	}
	expensePropertyID := propertyOne.ID
	if err := db.WithContext(ctx).Create(&manualExpense{UserID: owner.ID, PropertyID: &expensePropertyID, Description: "Repairs", Category: "Maintenance", AmountCents: 10000, Currency: "EUR", ExpenseDate: period.AddDate(0, 0, 4), PaymentMethod: "Manual", RecordStatus: obligationRecordActive}).Error; err != nil {
		t.Fatal(err)
	}
	service := newRentWorkspaceService(db)
	data, err := service.load(ctx, owner.ID, defaultRentWorkspaceFilters(period))
	if err != nil {
		t.Fatal(err)
	}
	if len(data.PropertyRows) != 2 || data.Summary.ExpectedCents != 100000 || data.Summary.PaidCents != 100000 || data.Summary.ExpenseCents != 10000 || data.PropertyRows[0].Status != "paid" {
		t.Fatalf("workspace data=%+v", data)
	}
	roomFilters := defaultRentWorkspaceFilters(period)
	roomFilters.View = rentWorkspaceViewRooms
	roomFilters.PropertyID = propertyOne.ID
	roomData, err := service.load(ctx, owner.ID, roomFilters)
	if err != nil {
		t.Fatal(err)
	}
	if len(roomData.RoomRows) != 1 || roomData.RoomRows[0].RoomID != roomOne.ID || roomData.RoomRows[0].Status != "paid" {
		t.Fatalf("room data=%+v", roomData.RoomRows)
	}
	detail, err := service.loadRoomDetail(ctx, owner.ID, roomOne.ID, period)
	if err != nil || len(detail.Tenants) != 1 || len(detail.Expenses) != 0 {
		t.Fatalf("room detail=%+v err=%v", detail, err)
	}
	_ = roomTwo
}

func timePtr(value time.Time) *time.Time {
	return &value
}
