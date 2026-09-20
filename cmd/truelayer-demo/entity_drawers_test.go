package main

import (
	"strings"
	"testing"
	"time"
)

func TestPropertyAndRoomFormsRenderAsMobileObjectDrawers(t *testing.T) {
	tests := []struct {
		name   string
		render func() (string, error)
		want   []string
		absent []string
	}{
		{
			name: "property create",
			render: func() (string, error) {
				return executeTemplate(propertyPageTemplate, propertyPageData{workspaceShell: workspaceShell{ActivePage: "properties"}, Period: "2026-09", PeriodLabel: "2026年9月", StatusFilter: "active", ShowForm: true})
			},
			want: []string{`class="object-tabs"`, `class="entity-drawer-backdrop"`, `aria-modal="true"`, "新建房产"},
		},
		{
			name: "room create",
			render: func() (string, error) {
				return executeTemplate(roomPageTemplate, roomPageData{workspaceShell: workspaceShell{ActivePage: "rooms"}, Period: "2026-09", PeriodLabel: "2026年9月", ShowForm: true, Form: roomPageForm{ActiveFrom: "2026-09"}, Properties: []propertyPageRow{{ID: 1, Name: "Canal House"}}})
			},
			want:   []string{`class="object-tabs"`, `class="entity-drawer-backdrop"`, `name="property_id"`, "新建房间"},
			absent: []string{`name="room_type"`, `name="capacity"`, "房间类型", "可住人数"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			page, err := test.render()
			if err != nil {
				t.Fatal(err)
			}
			for _, expected := range test.want {
				if !strings.Contains(page, expected) {
					t.Fatalf("rendered form is missing %q", expected)
				}
			}
			for _, unexpected := range test.absent {
				if strings.Contains(page, unexpected) {
					t.Fatalf("rendered form unexpectedly contains %q", unexpected)
				}
			}
		})
	}
}

func TestTenantFormUsesSharedObjectTabsAndDrawer(t *testing.T) {
	page, err := executeTemplate(tenantTemplate, tenantPageData{
		workspaceShell: workspaceShell{ActivePage: "tenants"},
		ShowForm:       true,
		Form:           tenantRecord{Name: "Aoife Murphy", Status: "active", Currency: "EUR", DueDay: 1, RentStartDate: "2026-09-01"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`class="object-tabs"`, `class="entity-drawer-backdrop"`, `aria-modal="true"`, "新增租客"} {
		if !strings.Contains(page, expected) {
			t.Fatalf("tenant form is missing %q", expected)
		}
	}
}

func TestRoomDetailEditStateRendersTheDetailsAndDrawerTogether(t *testing.T) {
	filters := defaultRentWorkspaceFilters(time.Now())
	page, err := executeTemplate(rentRoomDetailTemplate, rentRoomDetailPageData{
		workspaceShell: workspaceShell{ActivePage: "rooms"},
		Filters:        filters,
		Period:         "2026-09",
		PeriodLabel:    "2026年9月",
		RoomID:         2,
		RoomLabel:      "A-01",
		RoomActiveFrom: "2026-01",
		PropertyName:   "Canal House",
		Editing:        true,
		Form:           roomPageForm{ID: 2, PropertyID: 1, RoomLabel: "A-01", ActiveFrom: "2026-01"},
		Properties:     []propertyPageRow{{ID: 1, Name: "Canal House"}},
		Summary:        rentWorkspaceRoomRow{PropertyID: 1, Status: "vacant", StatusLabel: "空置", ExpectedAmount: "—", PaidAmount: "—", BalanceAmount: "—", DueDate: "—"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`class="room-detail-grid"`, `class="entity-drawer-backdrop"`, "租客责任", "房间与租约"} {
		if !strings.Contains(page, expected) {
			t.Fatalf("room detail edit state is missing %q", expected)
		}
	}
	if strings.Contains(page, `name="active_from"`) {
		t.Fatal("room edit drawer should not expose the active-from field omitted by the prototype")
	}
}
