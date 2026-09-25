package main

import (
	"testing"
	"time"
)

func TestRoomSuccessorKeepsPriorTenantFactsOnMySQL(t *testing.T) {
	f := newRoomRentPlanConflictFixture(t)
	oldStart := dublinCurrentMonth(time.Now()).AddDate(0, -2, 0)
	august := oldStart.AddDate(0, 1, 0)
	september := oldStart.AddDate(0, 2, 0)
	oldPlan := roomRentPlan{UserID: f.owner.ID, RoomID: f.roomOne.ID, EffectiveFromMonth: oldStart, MonthlyRentCents: 100000, Currency: ledgerCurrencyEUR, DueDay: 5}
	if err := f.db.WithContext(f.ctx).Create(&oldPlan).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.WithContext(f.ctx).Create(&roomRentPlanMember{UserID: f.owner.ID, RoomRentPlanID: oldPlan.ID, TenantID: f.tenant.ID, ResponsibilityCents: 100000}).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.WithContext(f.ctx).Model(&room{}).Where("id = ? AND user_id = ?", f.roomOne.ID, f.owner.ID).Update("rent_plan_version", 1).Error; err != nil {
		t.Fatal(err)
	}
	for _, month := range []time.Time{oldStart, august} {
		if err := newMonthlyRentFactsService(f.db).ensureMonthlyRentFacts(f.ctx, f.owner.ID, month, rentFactsIntentRead); err != nil {
			t.Fatal(err)
		}
	}
	var augustBefore rentObligation
	if err := f.db.WithContext(f.ctx).Where("user_id = ? AND tenant_id = ? AND room_id = ? AND period_month = ?", f.owner.ID, f.tenant.ID, f.roomOne.ID, august).First(&augustBefore).Error; err != nil {
		t.Fatal(err)
	}
	successor := repositoryTestTenant(f.owner.ID, "September successor")
	if err := f.db.WithContext(f.ctx).Create(&successor).Error; err != nil {
		t.Fatal(err)
	}
	_, version, err := newRoomRentPlanService(f.db).SaveRoomRentPlan(f.ctx, SaveRoomRentPlanCommand{UserID: f.owner.ID, RoomID: f.roomOne.ID, EffectiveMonth: september, MonthlyRentCents: 100000, Currency: ledgerCurrencyEUR, DueDay: 5, Members: []RoomRentPlanMemberInput{{TenantID: successor.ID, ResponsibilityCents: 100000}}, ExpectedTimelineVersion: 1})
	if err != nil || version != 2 {
		t.Fatalf("successor save version=%d err=%v", version, err)
	}
	var prior roomRentPlan
	if err := f.db.WithContext(f.ctx).Where("id = ?", oldPlan.ID).First(&prior).Error; err != nil || prior.EffectiveToMonth == nil || !monthStart(*prior.EffectiveToMonth).Equal(august) {
		t.Fatalf("prior plan=%+v err=%v", prior, err)
	}
	var augustAfter rentObligation
	if err := f.db.WithContext(f.ctx).Where("id = ?", augustBefore.ID).First(&augustAfter).Error; err != nil || augustAfter.TenantID != f.tenant.ID || augustAfter.ExpectedAmountCents != augustBefore.ExpectedAmountCents {
		t.Fatalf("August changed: %+v, %v", augustAfter, err)
	}
	if err := newMonthlyRentFactsService(f.db).ensureMonthlyRentFacts(f.ctx, f.owner.ID, september, rentFactsIntentRead); err != nil {
		t.Fatal(err)
	}
	var septemberFact rentObligation
	if err := f.db.WithContext(f.ctx).Where("user_id = ? AND room_id = ? AND period_month = ? AND record_status = ?", f.owner.ID, f.roomOne.ID, september, obligationRecordActive).First(&septemberFact).Error; err != nil || septemberFact.TenantID != successor.ID {
		t.Fatalf("September fact=%+v err=%v", septemberFact, err)
	}
}

func TestSoftDeletedPropertyHidesRoomsButKeepsHistoryOnMySQL(t *testing.T) {
	f := newRoomRentPlanConflictFixture(t)
	propertyID := f.roomOne.PropertyID
	expense := manualExpense{UserID: f.owner.ID, PropertyID: &propertyID, RoomID: &f.roomOne.ID, Description: "Historical repair", Category: "维修", AmountCents: 2000, Currency: ledgerCurrencyEUR, ExpenseDate: dublinCurrentMonth(time.Now()), PaymentMethod: "Manual", RecordStatus: obligationRecordActive}
	if err := f.db.WithContext(f.ctx).Create(&expense).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = f.db.WithContext(f.ctx).Where("id = ? AND user_id = ?", expense.ID, f.owner.ID).Delete(&manualExpense{}).Error
	})
	if err := newLandlordDomainService(f.db).deleteProperty(f.ctx, f.owner.ID, propertyID); err != nil {
		t.Fatal(err)
	}
	var propertyRow property
	if err := f.db.WithContext(f.ctx).Where("id = ?", propertyID).First(&propertyRow).Error; err != nil || propertyRow.DeletedAt == nil {
		t.Fatalf("property=%+v err=%v", propertyRow, err)
	}
	var roomRow room
	if err := f.db.WithContext(f.ctx).Where("id = ?", f.roomOne.ID).First(&roomRow).Error; err != nil || roomRow.DeletedAt != nil {
		t.Fatalf("historical room=%+v err=%v", roomRow, err)
	}
	visible, err := newLandlordRentRepository(f.db).listRooms(f.ctx, f.owner.ID, roomQuery{})
	if err != nil || len(visible) != 0 {
		t.Fatalf("daily room list=%+v err=%v", visible, err)
	}
	a := &app{db: f.db}
	period := dublinCurrentMonth(time.Now())
	historicalRooms, err := a.loadRoomRowsWithAssetHistory(f.ctx, f.owner.ID, period, propertyID, true)
	if err != nil || len(historicalRooms) != 1 || historicalRooms[0].ID != f.roomOne.ID {
		t.Fatalf("historical room rows=%+v err=%v", historicalRooms, err)
	}
	historicalProperties, err := a.loadPropertyPageWithAssetHistory(f.ctx, f.owner.ID, period, "all", true)
	if err != nil || len(historicalProperties.Rows) != 1 || historicalProperties.Rows[0].ID != propertyID || historicalProperties.Rows[0].RoomCount != 1 || historicalProperties.Rows[0].ExpenseCents != 2000 {
		t.Fatalf("historical property rows=%+v err=%v", historicalProperties.Rows, err)
	}
	roomDetail, err := newRentWorkspaceService(f.db).loadRoomDetail(f.ctx, f.owner.ID, f.roomOne.ID, period)
	if err != nil || roomDetail.Summary.RoomID != f.roomOne.ID {
		t.Fatalf("historical room detail=%+v err=%v", roomDetail.Summary, err)
	}
	var retainedExpense manualExpense
	if err := f.db.WithContext(f.ctx).Where("id = ? AND user_id = ?", expense.ID, f.owner.ID).First(&retainedExpense).Error; err != nil || retainedExpense.RoomID == nil || *retainedExpense.RoomID != f.roomOne.ID {
		t.Fatalf("historical expense=%+v err=%v", retainedExpense, err)
	}
}
