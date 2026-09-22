package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func TestTenantRoomPlanAssignmentValidatesBrowserMemberSnapshot(t *testing.T) {
	month := dublinCurrentMonth(time.Now())
	valid := tenantRoomPlanAssignmentInput{
		RoomID: 1, EffectiveMonth: month,
		ExistingMembers: []RoomRentPlanMemberInput{{TenantID: 2}, {TenantID: 3, ResponsibilityCents: 40000}},
	}
	if err := validateTenantRoomPlanAssignment(valid); err != nil {
		t.Fatalf("valid tenant room assignment error = %v", err)
	}
	if err := validateTenantRoomPlanAssignment(tenantRoomPlanAssignmentInput{
		RoomID: 1, EffectiveMonth: month,
		ExistingMembers: []RoomRentPlanMemberInput{{TenantID: 2}, {TenantID: 2}},
	}); !errors.Is(err, ErrInvalidRentPlan) {
		t.Fatalf("duplicate browser members error = %v, want ErrInvalidRentPlan", err)
	}
	if sameRoomPlanMemberIDs([]roomRentPlanMember{{TenantID: 2}, {TenantID: 3}}, []RoomRentPlanMemberInput{{TenantID: 3}, {TenantID: 2}}) != true {
		t.Fatal("matching room members should be accepted regardless of browser order")
	}
	if sameRoomPlanMemberIDs([]roomRentPlanMember{{TenantID: 2}}, []RoomRentPlanMemberInput{{TenantID: 3}}) {
		t.Fatal("changed room members should be rejected as stale")
	}
}

func TestTenantRoomPlanAssignmentInputFromForm(t *testing.T) {
	month := dublinCurrentMonth(time.Now()).Format("2006-01")
	assignment, present, err := tenantRoomPlanAssignmentInputFromForm(url.Values{
		"property_id":             {"2"},
		"room_id":                 {"3"},
		"arrangement_start_month": {month},
		"plan_version":            {"4"},
		"new_responsibility":      {"350.01"},
		"room_plan":               {`[{"tenant_id":8,"responsibility_cents":20000}]`},
	})
	if err != nil || !present {
		t.Fatalf("parse tenant room assignment present=%t err=%v", present, err)
	}
	if assignment.PropertyID != 2 || assignment.RoomID != 3 || assignment.ExpectedTimelineVersion != 4 || assignment.NewTenantResponsibilityCents != 35001 || len(assignment.ExistingMembers) != 1 || assignment.ExistingMembers[0].TenantID != 8 {
		t.Fatalf("parsed assignment = %#v", assignment)
	}
	if _, _, err := tenantRoomPlanAssignmentInputFromForm(url.Values{
		"property_id":             {"2"},
		"room_id":                 {"3"},
		"arrangement_start_month": {month},
		"plan_version":            {"4"},
		"room_plan":               {`[{"tenant_id":8,"unexpected":true}]`},
	}); !errors.Is(err, errInvalidTenantRoomPlan) {
		t.Fatalf("unexpected browser plan fields error=%v, want errInvalidTenantRoomPlan", err)
	}
	if _, present, err := tenantRoomPlanAssignmentInputFromForm(url.Values{}); err != nil || present {
		t.Fatalf("blank optional room assignment present=%t err=%v", present, err)
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

func TestRoomAndNewTenantFormsExposeTheCompositeRentPlanFields(t *testing.T) {
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

	for _, forbidden := range []string{`name="inactive_from"`} {
		if strings.Contains(propertyPage, forbidden) {
			t.Errorf("property form exposes asset-validity field %q", forbidden)
		}
	}
	for _, expected := range []string{`name="monthly_rent"`, `name="effective_month"`, `name="due_day"`} {
		if !strings.Contains(roomPage, expected) {
			t.Errorf("new room form is missing rent-plan field %q", expected)
		}
	}
	for _, forbidden := range []string{`name="active_from"`, `name="inactive_from"`} {
		if strings.Contains(roomPage, forbidden) {
			t.Errorf("room form exposes asset-validity field %q", forbidden)
		}
	}
	for _, expected := range []string{`name="property_id"`, `name="room_id"`, `name="arrangement_start_month"`, `name="plan_version"`, `name="room_plan"`, `name="new_responsibility"`} {
		if !strings.Contains(tenantPage, expected) {
			t.Errorf("new tenant form is missing room-plan field %q", expected)
		}
	}
	for _, expected := range []string{`id="tenant-existing-occupants"`, "留空则按照入住人数均分房间租金", "已固定 "} {
		if !strings.Contains(tenantPage, expected) {
			t.Errorf("new tenant form is missing grouped occupancy hint %q", expected)
		}
	}
	editingTenantPage, err := executeTemplate(tenantTemplate, tenantPageData{
		workspaceShell: workspaceShell{ActivePage: "tenants"},
		ShowForm:       true,
		Editing:        true,
		Form:           tenantRecord{ID: "1", Status: "active"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{`name="room_id"`, `name="room_plan"`, `name="new_responsibility"`} {
		if strings.Contains(editingTenantPage, forbidden) {
			t.Errorf("tenant edit form exposes create-only room-plan field %q", forbidden)
		}
	}
}
