package main

import (
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestTransactionReviewRoommateAnchors(t *testing.T) {
	tenants := []tenant{{ID: 1, Name: "Alex Lee"}, {ID: 2, Name: "Alex Lee"}, {ID: 3, Name: "Taylor Wong"}}
	for _, test := range []struct {
		name       string
		selectedID uint64
		payer      string
		wantIDs    []uint64
	}{
		{name: "selected tenant wins", selectedID: 3, payer: "Alex Lee", wantIDs: []uint64{3}},
		{name: "exact payer name", payer: "  ALEX LEE  ", wantIDs: []uint64{1, 2}},
		{name: "unrelated payer", payer: "Jordan Lee"},
		{name: "missing selected tenant", selectedID: 9, payer: "Alex Lee"},
	} {
		t.Run(test.name, func(t *testing.T) {
			anchors := transactionReviewRoommateAnchors(test.selectedID, test.payer, tenants)
			if len(anchors) != len(test.wantIDs) {
				t.Fatalf("got %d anchors, want %d", len(anchors), len(test.wantIDs))
			}
			for index, row := range anchors {
				if row.ID != test.wantIDs[index] {
					t.Fatalf("anchor %d = %d, want %d", index, row.ID, test.wantIDs[index])
				}
			}
		})
	}
}

func TestTransactionReviewRoommatesUseActiveRoomPlanOnMySQL(t *testing.T) {
	f := newRoomRentPlanConflictFixture(t)
	roommate := repositoryTestTenant(f.owner.ID, "Roommate")
	if err := f.db.WithContext(f.ctx).Create(&roommate).Error; err != nil {
		t.Fatal(err)
	}
	month := dublinCurrentMonth(time.Now())
	if _, _, err := newRoomRentPlanService(f.db).SaveRoomRentPlan(f.ctx, SaveRoomRentPlanCommand{
		UserID: f.owner.ID, RoomID: f.roomOne.ID, EffectiveMonth: month,
		MonthlyRentCents: 120000, Currency: ledgerCurrencyEUR, DueDay: 1,
		Members: []RoomRentPlanMemberInput{
			{TenantID: f.tenant.ID, ResponsibilityCents: 60000},
			{TenantID: roommate.ID, ResponsibilityCents: 60000},
		},
	}); err != nil {
		t.Fatal(err)
	}
	tenants := []tenant{f.tenant, roommate}
	groups, err := loadTransactionReviewRoommates(f.ctx, f.db, f.owner.ID, []tenant{f.tenant}, tenants, month)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || len(groups[0].Roommates) != 1 || groups[0].Roommates[0].ID != roommate.ID {
		t.Fatalf("roommate groups = %+v", groups)
	}
	if groups[0].Period != month.Format("2006-01") || groups[0].RoomLabel != f.roomOne.RoomLabel {
		t.Fatalf("room details = %+v", groups[0])
	}
	before := month.AddDate(0, -1, 0)
	groups, err = loadTransactionReviewRoommates(f.ctx, f.db, f.owner.ID, []tenant{f.tenant}, tenants, before)
	if err != nil || len(groups) != 0 {
		t.Fatalf("roommate groups before plan = %+v, %v", groups, err)
	}
	payerName := f.tenant.Name
	source := paymentTransaction{
		UserID: f.owner.ID, Source: "test", StableTransactionKey: fmt.Sprintf("roommate-review-%d", time.Now().UnixNano()),
		Direction: "income", AmountCents: 120000, Currency: ledgerCurrencyEUR,
		PayerName: &payerName, PayerNameKind: "confirmed", MatchStatus: "unmatched",
	}
	if err := f.db.WithContext(f.ctx).Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.db.WithContext(f.ctx).Delete(&source).Error })
	review, err := newTransactionService(f.db).transactionMatchReview(f.ctx, f.owner.ID, source.ID, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if review.SelectedTenantID != f.tenant.ID || !review.IntelligentSuggestion || len(review.RoommateGroups) != 1 {
		t.Fatalf("exact-name review = %+v", review)
	}
	review.setURLs(url.Values{"match": {fmt.Sprint(source.ID)}})
	if !strings.Contains(review.RoommateGroups[0].RoomURL, fmt.Sprintf("/rooms/%d?period=%s", f.roomOne.ID, month.Format("2006-01"))) || !strings.Contains(review.RoommateGroups[0].Roommates[0].URL, "match_origin=roommate") {
		t.Fatalf("room and roommate links = %+v", review.RoommateGroups[0])
	}
	contextReview, err := newTransactionService(f.db).transactionMatchReviewForMonthWithOrigin(f.ctx, f.owner.ID, source.ID, roommate.ID, 1, month.Format("2006-01"), false)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range contextReview.Months {
		if candidate.Period == month.Format("2006-01") && candidate.Viewed {
			t.Fatal("roommate navigation incorrectly marked month as manually viewed")
		}
	}
	explicitReview, err := newTransactionService(f.db).transactionMatchReviewForMonthWithOrigin(f.ctx, f.owner.ID, source.ID, roommate.ID, 1, month.Format("2006-01"), true)
	if err != nil {
		t.Fatal(err)
	}
	viewed := false
	for _, candidate := range explicitReview.Months {
		viewed = viewed || candidate.Period == month.Format("2006-01") && candidate.Viewed
	}
	if !viewed {
		t.Fatal("explicit month lookup was not marked viewed")
	}
	review, err = newTransactionService(f.db).transactionMatchReview(f.ctx, f.owner.ID, source.ID, roommate.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if review.IntelligentSuggestion || len(review.SuggestedTenants) != 0 {
		t.Fatalf("manual tenant switch kept suggestion: %+v", review)
	}
}
