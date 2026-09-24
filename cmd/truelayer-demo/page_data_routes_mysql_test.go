package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCanonicalPropertyPagesAreUserScopedOnMySQL(t *testing.T) {
	db, sqlDB := openLedgerMySQLTestDB(t)
	if err := runMigrations(sqlDB, "../../migrations"); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	ctx := context.Background()
	owner := user{Username: fmt.Sprintf("page-owner-%d", time.Now().UnixNano()), PasswordHash: "test"}
	other := user{Username: fmt.Sprintf("page-other-%d", time.Now().UnixNano()), PasswordHash: "test"}
	if err := db.WithContext(ctx).Create(&owner).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.WithContext(ctx).Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	repo := newLandlordRentRepository(db)
	ownerProperty, err := repo.createProperty(ctx, owner.ID, property{Name: "Owner only"})
	if err != nil {
		t.Fatal(err)
	}
	otherProperty, err := repo.createProperty(ctx, other.ID, property{Name: "Other only"})
	if err != nil {
		t.Fatal(err)
	}
	a := testApp()
	a.db = db
	a.cfg.Environment = "sandbox"
	handler := newAppMux(&a)
	cookie := userSessionCookie(a.cfg, owner.ID, owner.Username, time.Now().Add(sessionTTL))
	request := func(method, path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, nil)
		req.AddCookie(cookie)
		handler.ServeHTTP(rec, req)
		return rec
	}
	list := request(http.MethodGet, "/properties")
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), ownerProperty.Name) || strings.Contains(list.Body.String(), otherProperty.Name) {
		t.Fatalf("scoped property list status=%d body=%s", list.Code, list.Body.String())
	}
	otherDetail := request(http.MethodGet, fmt.Sprintf("/properties/%d", otherProperty.ID))
	if otherDetail.Code != http.StatusNotFound {
		t.Fatalf("cross-user property detail status=%d want %d", otherDetail.Code, http.StatusNotFound)
	}
}

func TestObjectSearchUsesSelectedMonthNamesAndOwnerScopeOnMySQL(t *testing.T) {
	f := newRoomRentPlanConflictFixture(t)
	month := dublinCurrentMonth(time.Now())
	if err := f.db.WithContext(f.ctx).Model(&tenant{}).Where("id = ? AND user_id = ?", f.tenant.ID, f.owner.ID).
		Updates(map[string]any{"name": "Aoife Murphy", "display_alias": "Fi"}).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err := newRoomRentPlanService(f.db).SaveRoomRentPlan(f.ctx, roomRentPlanCommandFor(f, f.roomOne.ID, month)); err != nil {
		t.Fatal(err)
	}

	other := user{Username: fmt.Sprintf("object-search-other-%d", time.Now().UnixNano()), PasswordHash: "test"}
	if err := f.db.WithContext(f.ctx).Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, table := range []string{"rent_obligations", "rent_charges", "room_rent_plan_members", "room_rent_plans", "rooms", "properties", "tenants"} {
			_ = f.db.WithContext(f.ctx).Exec("DELETE FROM "+table+" WHERE user_id = ?", other.ID).Error
		}
		_ = f.db.WithContext(f.ctx).Delete(&user{}, other.ID).Error
	})
	otherTenant := repositoryTestTenant(other.ID, "Foreign Tenant")
	otherTenant.DisplayAlias = "Foreign Alias"
	if err := f.db.WithContext(f.ctx).Create(&otherTenant).Error; err != nil {
		t.Fatal(err)
	}
	domain := newLandlordDomainService(f.db)
	otherProperty, err := domain.createProperty(f.ctx, other.ID, propertyInput{Name: "Foreign Property"})
	if err != nil {
		t.Fatal(err)
	}
	otherRoom, err := domain.createRoom(f.ctx, other.ID, roomInput{PropertyID: otherProperty.ID, RoomLabel: "Foreign Room"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := newRoomRentPlanService(f.db).SaveRoomRentPlan(f.ctx, SaveRoomRentPlanCommand{
		UserID: other.ID, RoomID: otherRoom.ID, EffectiveMonth: month,
		MonthlyRentCents: 100000, Currency: ledgerCurrencyEUR, DueDay: 5,
		Members: []RoomRentPlanMemberInput{{TenantID: otherTenant.ID, ResponsibilityCents: 100000}},
	}); err != nil {
		t.Fatal(err)
	}

	a := testApp()
	a.db = f.db
	propertyPage, err := a.loadPropertyPage(f.ctx, f.owner.ID, month, "all")
	if err != nil {
		t.Fatal(err)
	}
	roomRows, err := a.loadRoomRows(f.ctx, f.owner.ID, month, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, term := range []string{"Aoife", "Fi", f.roomOne.RoomLabel} {
		if got := filterPropertyPageRows(propertyPage.Rows, term); len(got) != 1 {
			t.Fatalf("property search %q returned %d rows, want 1", term, len(got))
		}
	}
	for _, term := range []string{"Aoife", "Fi", f.roomOne.RoomLabel} {
		if got := filterRoomPageRows(roomRows, term, "active"); len(got) != 1 || got[0].ID != f.roomOne.ID {
			t.Fatalf("room search %q returned %+v", term, got)
		}
	}
	if got := filterRoomPageRows(roomRows, "Foreign", "all"); len(got) != 0 {
		t.Fatalf("foreign room leaked: %+v", got)
	}
	if got := filterPropertyPageRows(propertyPage.Rows, "Foreign"); len(got) != 0 {
		t.Fatalf("foreign property leaked: %+v", got)
	}
	previous := month.AddDate(0, -1, 0)
	previousRows, err := a.loadRoomRows(f.ctx, f.owner.ID, previous, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := filterRoomPageRows(previousRows, "Aoife", "all"); len(got) != 0 {
		t.Fatalf("tenant matched outside selected month: %+v", got)
	}
}
