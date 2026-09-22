package main

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestRentWorkspaceFiltersPreserveTheSelectedViewAndMonth(t *testing.T) {
	filters, err := rentWorkspaceFiltersFromQuery(url.Values{
		"period": {"2026-09"}, "view": {rentWorkspaceViewTenants}, "status": {"outstanding"},
		"property_id": {"12"}, "room_id": {"34"}, "search": {" Aoife "}, "page": {"2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := url.Parse(rentWorkspaceURL(filters, 3))
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{"period": "2026-09", "view": "tenants", "status": "outstanding", "property_id": "12", "room_id": "34", "search": "Aoife", "page": "3"} {
		if value := got.Query().Get(key); value != want {
			t.Errorf("query %s=%q want %q", key, value, want)
		}
	}
}

func TestBuildRentWorkspaceKeepsInactiveAssetsAndTenantRoomRows(t *testing.T) {
	period := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)
	propertyRow := property{ID: 1, UserID: 7, Name: "Canal House", Status: "inactive"}
	roomOne := room{ID: 11, UserID: 7, PropertyID: propertyRow.ID, RoomLabel: "A-01", Status: "inactive"}
	roomTwo := room{ID: 12, UserID: 7, PropertyID: propertyRow.ID, RoomLabel: "A-02", Status: "active"}
	vacantRoom := room{ID: 13, UserID: 7, PropertyID: propertyRow.ID, RoomLabel: "A-03", Status: "active"}
	primary := tenant{ID: 101, UserID: 7, Name: "Aoife Murphy", DisplayAlias: "Aoife", Status: "active"}
	payer := tenant{ID: 102, UserID: 7, Name: "Brian Kelly", Status: "active"}
	planOne := roomRentPlan{ID: 301, UserID: 7, RoomID: roomOne.ID, EffectiveFromMonth: period.AddDate(0, -3, 0), MonthlyRentCents: 100000, Currency: "EUR", DueDay: 5}
	planTwo := roomRentPlan{ID: 302, UserID: 7, RoomID: roomTwo.ID, EffectiveFromMonth: period.AddDate(0, -3, 0), MonthlyRentCents: 50000, Currency: "EUR", DueDay: 5}
	memberOne := roomRentPlanMember{ID: 401, UserID: 7, RoomRentPlanID: planOne.ID, TenantID: primary.ID, ResponsibilityCents: 100000}
	memberTwo := roomRentPlanMember{ID: 402, UserID: 7, RoomRentPlanID: planTwo.ID, TenantID: payer.ID, ResponsibilityCents: 50000}
	chargeOne := rentCharge{ID: 501, UserID: 7, PropertyID: propertyRow.ID, RoomID: roomOne.ID, RoomRentPlanID: planOne.ID, PeriodMonth: period, DueDate: dueDateForMonth(period, 5), ExpectedAmountCents: 100000, Currency: "EUR", RecordStatus: obligationRecordActive, PropertyNameSnapshot: nullableString(propertyRow.Name), RoomLabelSnapshot: nullableString(roomOne.RoomLabel)}
	chargeTwo := rentCharge{ID: 502, UserID: 7, PropertyID: propertyRow.ID, RoomID: roomTwo.ID, RoomRentPlanID: planTwo.ID, PeriodMonth: period, DueDate: dueDateForMonth(period, 5), ExpectedAmountCents: 50000, Currency: "EUR", RecordStatus: obligationRecordActive, PropertyNameSnapshot: nullableString(propertyRow.Name), RoomLabelSnapshot: nullableString(roomTwo.RoomLabel)}
	obligationOne := rentObligation{ID: 601, UserID: 7, RentChargeID: chargeOne.ID, RoomRentPlanID: planOne.ID, RoomRentPlanMemberID: memberOne.ID, TenantID: primary.ID, PeriodMonth: period, DueDate: chargeOne.DueDate, ExpectedAmountCents: chargeOne.ExpectedAmountCents, Currency: "EUR", RecordStatus: obligationRecordActive}
	obligationTwo := rentObligation{ID: 602, UserID: 7, RentChargeID: chargeTwo.ID, RoomRentPlanID: planTwo.ID, RoomRentPlanMemberID: memberTwo.ID, TenantID: payer.ID, PeriodMonth: period, DueDate: chargeTwo.DueDate, ExpectedAmountCents: chargeTwo.ExpectedAmountCents, Currency: "EUR", RecordStatus: obligationRecordActive}
	transaction := paymentTransaction{ID: 701, UserID: 7, Direction: "income", AmountCents: 100000, Currency: "EUR", MatchedTenantID: ptrUint64(payer.ID), TransactionTime: ptrTime(now), Source: "bank"}
	allocation := paymentAllocation{ID: 801, UserID: 7, PaymentTransactionID: transaction.ID, RentObligationID: ptrUint64(obligationOne.ID), TenantID: ptrUint64(primary.ID), AmountCents: 100000, AllocationKind: allocationKindRent, Status: allocationStatusConfirmed, ConfirmationSource: "manual"}
	input := rentWorkspaceInput{
		UserID: 7, PeriodMonth: period, Now: now,
		Properties: []property{propertyRow}, Rooms: []room{roomOne, roomTwo, vacantRoom},
		Plans: []roomRentPlan{planOne, planTwo}, Parties: []roomRentPlanMember{memberOne, memberTwo},
		Charges: []rentCharge{chargeOne, chargeTwo}, Obligations: []rentObligation{obligationOne, obligationTwo},
		Tenants: []tenant{primary, payer}, Transactions: []paymentTransaction{transaction}, Allocations: []paymentAllocation{allocation},
	}
	propertyView, err := buildRentWorkspace(input, defaultRentWorkspaceFilters(period))
	if err != nil {
		t.Fatal(err)
	}
	if len(propertyView.RoomRows) != 3 {
		t.Fatalf("asset spine room rows=%+v", propertyView.RoomRows)
	}
	roomRowsByID := make(map[uint64]rentWorkspaceRoomRow, len(propertyView.RoomRows))
	for _, row := range propertyView.RoomRows {
		roomRowsByID[row.RoomID] = row
	}
	if got := roomRowsByID[roomOne.ID]; got.Status != "paid" || got.TenantCount != 1 {
		t.Fatalf("inactive room's historical responsibility was hidden: %+v", got)
	}
	if got := roomRowsByID[vacantRoom.ID]; got.Status != "vacant" || got.ExpectedAmount != "—" {
		t.Fatalf("empty room row=%+v", got)
	}

	filters := defaultRentWorkspaceFilters(period)
	filters.View = rentWorkspaceViewTenants
	filters.Status = "outstanding"
	tenantsView, err := buildRentWorkspace(input, filters)
	if err != nil {
		t.Fatal(err)
	}
	if len(tenantsView.TenantRows) != 1 || tenantsView.TenantRows[0].RoomID != roomTwo.ID || tenantsView.TenantRows[0].TenantID != payer.ID {
		t.Fatalf("tenant room-level outstanding rows=%+v", tenantsView.TenantRows)
	}
	if tenantsView.TenantRows[0].Status != "overdue" {
		t.Fatalf("late unpaid responsibility status=%q", tenantsView.TenantRows[0].Status)
	}
	filters.Status = "all"
	paidView, err := buildRentWorkspace(input, filters)
	if err != nil {
		t.Fatal(err)
	}
	var paidByOtherRow *rentWorkspaceTenantRow
	for index := range paidView.TenantRows {
		if paidView.TenantRows[index].RoomID == roomOne.ID {
			paidByOtherRow = &paidView.TenantRows[index]
			break
		}
	}
	if len(paidView.TenantRows) != 2 || paidByOtherRow == nil || !paidByOtherRow.PaidByOther {
		t.Fatalf("third-party allocation or distinct tenant room row lost: %+v", paidView.TenantRows)
	}
	seenTenantMonth := make(map[uint64]struct{}, len(paidView.TenantRows))
	for _, row := range paidView.TenantRows {
		if _, exists := seenTenantMonth[row.TenantID]; exists {
			t.Fatalf("valid tenant workspace data contains duplicate tenant/month rows: %+v", paidView.TenantRows)
		}
		seenTenantMonth[row.TenantID] = struct{}{}
	}
}

func TestWorkspaceOutstandingFilterOnlyIncludesValidUnpaidResponsibilities(t *testing.T) {
	for status, want := range map[string]bool{
		"open": true, "partial": true, "overdue": true,
		"needs_review": false, "paid": false, "forecast": false, "vacant": false,
	} {
		if got := workspaceStatusMatches(status, "outstanding"); got != want {
			t.Errorf("outstanding filter matches %q=%t, want %t", status, got, want)
		}
	}
	if _, err := rentWorkspaceFiltersFromQuery(url.Values{"period": {"2026-09"}, "status": {"unpaid"}}); err == nil {
		t.Fatal("obsolete unpaid filter should be rejected")
	}
}

func TestRentWorkspaceTenantFormShowsNoRentPlanInputs(t *testing.T) {
	page, err := executeTemplate(tenantTemplate, tenantPageData{ShowForm: true, Form: tenantRecord{Status: "active"}, ReturnURL: "/tenants", PostReturnURL: "/tenants?add=1"})
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"monthly_rent", "room_id", "due_day", "rent_effective_from_month", "arrangement_start_month", "room_plan"} {
		if strings.Contains(page, `name="`+field+`"`) {
			t.Errorf("tenant page still writes room rent field %q", field)
		}
	}
}
