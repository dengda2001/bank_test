package main

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFirstRoomTenantConflictMatchesPlanWriteWindow(t *testing.T) {
	intervals := []roomTenantOccupancy{
		{RoomID: 4, RoomLabel: "Old room", From: "2026-01", To: "2026-06"},
		{RoomID: 5, RoomLabel: "Future room", From: "2026-11"},
	}
	for _, test := range []struct {
		name       string
		month      string
		targetRoom uint64
		wantRoom   uint64
	}{
		{"ended before target", "2026-07", 5, 0},
		{"inclusive end", "2026-06", 5, 4},
		{"future plan blocks earlier assignment", "2026-07", 9, 5},
		{"same room is allowed", "2026-12", 5, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			conflict, found := firstRoomTenantConflict(intervals, test.targetRoom, test.month)
			if found != (test.wantRoom != 0) || conflict.RoomID != test.wantRoom {
				t.Fatalf("conflict = %+v, found = %t; want room %d", conflict, found, test.wantRoom)
			}
		})
	}
}

func TestTenantCreatedRoomReturnURLIsBoundToAssignedRoom(t *testing.T) {
	form := url.Values{"room_id": {"7"}, "arrangement_start_month": {"2026-09"}}
	got, ok := tenantCreatedRoomReturnURL("/tenants?add=1&return_room_id=7", form)
	if !ok || got != "/rooms/7?period=2026-09&rent=1&message=tenant_added" {
		t.Fatalf("return URL = %q, valid = %t", got, ok)
	}
	for _, target := range []string{
		"https://evil.example/tenants?return_room_id=7",
		"//evil.example/tenants?return_room_id=7",
		"/tenants?return_room_id=8",
		"/tenants?return_room_id=invalid",
	} {
		if got, ok := tenantCreatedRoomReturnURL(target, form); ok {
			t.Errorf("accepted %q as %q", target, got)
		}
	}
}

func TestRoomTenantDrawersExposeCreateAndUnbindPaths(t *testing.T) {
	occupied := roomRentPlanTenantOption{ID: 17, Name: "Occupied tenant", Status: "active", OccupanciesJSON: `[{"room_id":8,"room_label":"Other room","from":"2026-01"}]`, ConflictRoomID: 8, ConflictRoomName: "Other room", UnbindURL: "/rooms/8?period=2026-09&rent=1"}
	roomPage, err := executeTemplate(roomPageTemplate, roomPageData{
		workspaceShell: workspaceShell{ActivePage: "rooms"}, ShowForm: true,
		Drawer: &roomCreateDrawerData{Period: "2026-09", Form: roomPageForm{EffectiveMonth: "2026-09", DueDay: 1}, Properties: []propertyPageRow{{ID: 2, Name: "Property"}}, Tenants: []roomRentPlanTenantOption{occupied}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{`name="tenant_choice" value="existing"`, `name="tenant_choice" value="new"`, `name="tenant_id"`, `data-tenant-conflict="true" disabled`, `href="/rooms/8?period=2026-09&amp;rent=1"`, `room-tenant-availability.js`} {
		if !strings.Contains(roomPage, marker) {
			t.Errorf("room creation lacks %q", marker)
		}
	}
	roomDetail, err := executeTemplate(rentRoomDetailTemplate, rentRoomDetailPageData{
		workspaceShell: workspaceShell{ActivePage: "rooms"}, RoomID: 7, Period: "2026-09", PlanEditor: true, PlanExists: true,
		Summary: rentWorkspaceRoomRow{PropertyID: 2}, PlanMembers: []roomRentPlanMemberForm{{}}, PlanTenants: []roomRentPlanTenantOption{occupied},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{`return_room_id=7`, `新增租客`, `data-tenant-conflict="true" disabled`, `去解绑`, `room-tenant-availability.js`} {
		if !strings.Contains(roomDetail, marker) {
			t.Errorf("room occupancy editor lacks %q", marker)
		}
	}
	tenantPage, err := executeTemplate(tenantTemplate, tenantPageData{
		workspaceShell: workspaceShell{ActivePage: "tenants"}, ShowForm: true, TenantRoomLocked: true,
		TenantAssignmentPropertyID: 2, TenantAssignmentRoomID: 7, TenantArrangementStartMonth: "2026-09",
		TenantProperties: []tenantPropertyAssignmentOption{{ID: 2, Name: "Property"}},
		TenantRooms:      []tenantRoomAssignmentOption{{ID: 7, PropertyID: 2, RoomLabel: "New room", RentPlanVersion: 1, PlansJSON: `[{"effective_from_month":"2026-09","monthly_rent_cents":120000,"currency":"EUR","due_day":3,"members":[]}]`}},
		ReturnURL:        "/rooms/7?period=2026-09&rent=1", PostReturnURL: "/tenants?add=1&property_id=2&room_id=7&arrangement_start_month=2026-09&return_room_id=7",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{`name="property_id" value="2"`, `name="room_id" value="7"`, `id="tenant-property" name="property_id" disabled`, `id="tenant-room" name="room_id" disabled`, `保存后返回该房间`} {
		if !strings.Contains(tenantPage, marker) {
			t.Errorf("room-bound tenant form lacks %q", marker)
		}
	}
	if os.Getenv("RENTOPS_WRITE_ROOM_TENANT_PREVIEW") == "1" {
		root := "/private/tmp/rentops-room-tenant-preview"
		if err := os.MkdirAll(root, 0o700); err != nil {
			t.Fatal(err)
		}
		for name, page := range map[string]string{"room-create.html": roomPage, "room-detail.html": roomDetail, "tenant-create.html": tenantPage} {
			if err := os.WriteFile(filepath.Join(root, name), []byte(page), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
}
