package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRoomRentPlanValidatesMonthlyDueDay(t *testing.T) {
	effectiveMonth := dublinCurrentMonth(time.Now())
	for _, test := range []struct {
		day   int
		valid bool
	}{
		{day: 1, valid: true},
		{day: 31, valid: true},
		{day: 0},
		{day: 32},
	} {
		_, err := validateRoomRentPlanCommand(SaveRoomRentPlanCommand{
			UserID: 1, RoomID: 1, EffectiveMonth: effectiveMonth,
			MonthlyRentCents: 100000, Currency: ledgerCurrencyEUR, DueDay: test.day,
			Members: []RoomRentPlanMemberInput{{TenantID: 1, ResponsibilityCents: 100000}},
		})
		if (err == nil) != test.valid {
			t.Errorf("due day %d validation error=%v, valid=%t", test.day, err, test.valid)
		}
	}
}

func TestRoomRentPlanRejectsPastEffectiveMonth(t *testing.T) {
	pastMonth := dublinCurrentMonth(time.Now()).AddDate(0, -1, 0)
	_, err := validateRoomRentPlanCommand(SaveRoomRentPlanCommand{
		UserID: 1, RoomID: 1, EffectiveMonth: pastMonth,
		MonthlyRentCents: 100000, Currency: ledgerCurrencyEUR, DueDay: 5,
		Members: []RoomRentPlanMemberInput{{TenantID: 1, ResponsibilityCents: 100000}},
	})
	if !errors.Is(err, ErrInvalidRentPlan) {
		t.Fatalf("past effective month error=%v; want ErrInvalidRentPlan", err)
	}
}

func TestRoomRentPlanAllowsAnEmptyRoomWithAStoredRentRule(t *testing.T) {
	month := dublinCurrentMonth(time.Now())
	members, err := validateRoomRentPlanCommand(SaveRoomRentPlanCommand{
		UserID: 1, RoomID: 1, EffectiveMonth: month,
		MonthlyRentCents: 100000, Currency: ledgerCurrencyEUR, DueDay: 1,
	})
	if err != nil {
		t.Fatalf("empty room rent rule error = %v", err)
	}
	if len(members) != 0 {
		t.Fatalf("empty room rent rule members = %#v, want none", members)
	}
}

func TestRoomRentPlanSplitsTheRemainderBetweenUnspecifiedMembers(t *testing.T) {
	month := dublinCurrentMonth(time.Now())
	members, err := validateRoomRentPlanCommand(SaveRoomRentPlanCommand{
		UserID: 1, RoomID: 1, EffectiveMonth: month,
		MonthlyRentCents: 100001, Currency: ledgerCurrencyEUR, DueDay: 1,
		Members: []RoomRentPlanMemberInput{
			{TenantID: 3},
			{TenantID: 2, ResponsibilityCents: 30000},
			{TenantID: 1},
		},
	})
	if err != nil {
		t.Fatalf("partial responsibility plan error = %v", err)
	}
	want := []RoomRentPlanMemberInput{
		{TenantID: 1, ResponsibilityCents: 35001},
		{TenantID: 2, ResponsibilityCents: 30000},
		{TenantID: 3, ResponsibilityCents: 35000},
	}
	if !reflect.DeepEqual(members, want) {
		t.Fatalf("partial responsibility plan = %#v, want %#v", members, want)
	}
}

func TestRoomRentPlanRejectsPartialResponsibilityWithoutAPositiveRemainder(t *testing.T) {
	month := dublinCurrentMonth(time.Now())
	_, err := validateRoomRentPlanCommand(SaveRoomRentPlanCommand{
		UserID: 1, RoomID: 1, EffectiveMonth: month,
		MonthlyRentCents: 100000, Currency: ledgerCurrencyEUR, DueDay: 1,
		Members: []RoomRentPlanMemberInput{
			{TenantID: 1, ResponsibilityCents: 100000},
			{TenantID: 2},
		},
	})
	if !errors.Is(err, ErrInvalidRentPlan) {
		t.Fatalf("partial responsibility without remainder error = %v, want ErrInvalidRentPlan", err)
	}
}

func TestRoomRentPlanTenantRoomConflictUsesStablePageMessage(t *testing.T) {
	if got := rentPlanErrorMessage("tenant_room_month_conflict"); got != "该租客从所选月份起已在其他房间入住，请先结束原房间的入住计划。" {
		t.Fatalf("tenant room conflict message=%q", got)
	}
}

func TestTenanciesRouteIsNotRegistered(t *testing.T) {
	mux := newAppMux(&app{})
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		t.Run(method, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(method, "/tenancies", strings.NewReader(""))
			mux.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusNotFound {
				t.Fatalf("/tenancies %s status = %d, want 404", method, recorder.Code)
			}
		})
	}
}

func TestAssetAndTenantFormsKeepRentPlanFieldsOutOfAssetProfiles(t *testing.T) {
	propertyPage, err := executeTemplate(propertyPageTemplate, propertyPageData{
		workspaceShell: workspaceShell{ActivePage: "properties"},
		ShowForm:       true,
	})
	if err != nil {
		t.Fatal(err)
	}
	roomPage, err := executeTemplate(roomPageTemplate, roomPageData{
		workspaceShell: workspaceShell{ActivePage: "rooms"},
		ShowForm:       true,
	})
	if err != nil {
		t.Fatal(err)
	}
	tenantPage, err := executeTemplate(tenantTemplate, tenantPageData{
		workspaceShell: workspaceShell{ActivePage: "tenants"},
		ShowForm:       true,
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name string
		page string
		bad  []string
	}{
		{name: "property", page: propertyPage, bad: []string{`name="inactive_from"`}},
		{name: "room", page: roomPage, bad: []string{`name="active_from"`, `name="inactive_from"`, `name="monthly_rent"`, `name="due_day"`}},
		{name: "tenant", page: tenantPage, bad: []string{`name="room_id"`, `name="monthly_rent"`, `name="due_day"`, `name="responsibility_`, `name="room_plan"`}},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, forbidden := range test.bad {
				if strings.Contains(test.page, forbidden) {
					t.Errorf("%s form exposes rent-plan/asset-validity field %q", test.name, forbidden)
				}
			}
		})
	}
}
