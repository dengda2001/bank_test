package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestCanonicalPageRoutesRequireAuthentication(t *testing.T) {
	a := testApp()
	handler := newAppMux(&a)
	paths := []string{"/bills", "/transactions", "/dunning", "/properties", "/properties/1", "/properties/not-an-id", "/rooms", "/rooms/1", "/tenancies", "/cash-receipts", "/bank"}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
			if rec.Code != http.StatusFound {
				t.Fatalf("status=%d want %d", rec.Code, http.StatusFound)
			}
			if location := rec.Header().Get("Location"); location != "/" {
				t.Fatalf("Location=%q want /", location)
			}
		})
	}
}

func TestCanonicalRoutesKeepLegacyRefreshTargetAndBankTarget(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/refresh", nil)
	if got := bankRefreshRedirect(request, "message=refreshed"); got != "/billing?message=refreshed" {
		t.Fatalf("legacy refresh target=%q", got)
	}
	request = httptest.NewRequest(http.MethodPost, "/bank/sync", nil)
	if got := bankRefreshRedirect(request, "message=refreshed"); got != "/bank?message=refreshed" {
		t.Fatalf("canonical refresh target=%q", got)
	}
}

func TestDashboardManualBalanceRequiresReasonField(t *testing.T) {
	var body strings.Builder
	if err := rentDashboardTemplate.Execute(&body, rentDashboardPageData{Period: "2026-09", PeriodLabel: "2026年9月", Rows: []rentDashboardRow{{ObligationID: 7, ExpectedCents: 100, PaidCents: 50, Status: "partial", StatusLabel: "部分缴纳"}}, Dunning: dunningDrawerData{Enabled: true}}); err != nil {
		t.Fatal(err)
	}
	page := body.String()
	for _, marker := range []string{`name="reason"`, `placeholder="填写平账原因"`, `required`, `onsubmit="return confirm('确认一键平账吗？')"`, `onclick="return confirm('确认发送催收邮件吗？')"`} {
		if !strings.Contains(page, marker) {
			t.Fatalf("dashboard missing manual-balance marker %q", marker)
		}
	}
}

func TestCanonicalPageTemplatesExposeSemanticEntityIDs(t *testing.T) {
	var propertyPage strings.Builder
	if err := propertyPageTemplate.Execute(&propertyPage, propertyPageData{Rows: []propertyPageRow{{ID: 1, Name: "House", Status: "active", StatusLabel: "有效"}}}); err != nil {
		t.Fatal(err)
	}
	var roomPage strings.Builder
	if err := roomPageTemplate.Execute(&roomPage, roomPageData{Rows: []roomPageRow{{ID: 2, RoomLabel: "A-01", Status: "active", StatusLabel: "有效"}}}); err != nil {
		t.Fatal(err)
	}
	var tenancyPage strings.Builder
	if err := tenancyPageTemplate.Execute(&tenancyPage, tenancyPageData{TableRows: []tenancyPageRow{{ID: 3, RoomID: 2, RoomLabel: "A-01", Status: "active", StatusLabel: "有效"}}}); err != nil {
		t.Fatal(err)
	}
	var bankPage strings.Builder
	if err := bankPageTemplate.Execute(&bankPage, bankPageData{Runs: []bankPageRun{{ID: 4, Status: "succeeded", StatusLabel: "同步成功"}}}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(propertyPage.String(), `data-property-id`) {
		t.Fatal("property page does not expose a stable property id")
	}
	if !strings.Contains(roomPage.String(), `data-room-id`) {
		t.Fatal("room page does not expose a stable room id")
	}
	if !strings.Contains(tenancyPage.String(), `data-tenancy-id`) {
		t.Fatal("tenancy page does not expose a stable tenancy id")
	}
	if !strings.Contains(bankPage.String(), `data-sync-run-id`) {
		t.Fatal("bank page does not expose a stable sync run id")
	}
}

func TestObjectListFiltersSearchAndStatus(t *testing.T) {
	properties := []propertyPageRow{
		{ID: 1, Name: "Canal House", Address: "Dublin 2", Status: "active"},
		{ID: 2, Name: "Park View", Address: "Cork", Status: "inactive"},
	}
	if got := filterPropertyPageRows(properties, "DUBLIN"); len(got) != 1 || got[0].ID != 1 {
		t.Fatalf("property search result=%+v", got)
	}
	rooms := []roomPageRow{
		{ID: 1, RoomLabel: "A-01", PropertyName: "Canal House", TenantNames: []string{"Aoife Murphy"}, Status: "active"},
		{ID: 2, RoomLabel: "B-02", PropertyName: "Park View", TenantNames: []string{"Chen Xi"}, Status: "inactive"},
	}
	if got := filterRoomPageRows(rooms, "AOIFE", "active"); len(got) != 1 || got[0].ID != 1 {
		t.Fatalf("tenant room search result=%+v", got)
	}
	if got := filterRoomPageRows(rooms, "", "inactive"); len(got) != 1 || got[0].ID != 2 {
		t.Fatalf("inactive room result=%+v", got)
	}
}

func TestObjectListCollectionStatesMatchPrototypeLabelsAndFilters(t *testing.T) {
	for _, test := range []struct {
		expected int64
		paid     int64
		status   string
		label    string
	}{
		{expected: 0, paid: 0, status: "vacant", label: "无应收"},
		{expected: 10000, paid: 0, status: "overdue", label: "未收"},
		{expected: 10000, paid: 4000, status: "partial", label: "部分未收"},
		{expected: 10000, paid: 10000, status: "paid", label: "已收齐"},
		{expected: 10000, paid: 12000, status: "paid", label: "已收齐"},
	} {
		status, label := collectionStatus(test.expected, test.paid)
		if status != test.status || label != test.label {
			t.Fatalf("collectionStatus(%d,%d)=(%q,%q), want (%q,%q)", test.expected, test.paid, status, label, test.status, test.label)
		}
	}
	rows := []propertyPageRow{
		{ID: 1, CollectionStatus: "overdue"},
		{ID: 2, CollectionStatus: "partial"},
		{ID: 3, CollectionStatus: "paid"},
		{ID: 4, CollectionStatus: "vacant"},
	}
	if got := filterPropertyCollectionRows(rows, "unpaid"); len(got) != 2 || got[0].ID != 1 || got[1].ID != 2 {
		t.Fatalf("unpaid collection rows=%+v, want overdue and partial", got)
	}
	if got := filterPropertyCollectionRows(rows, "paid"); len(got) != 1 || got[0].ID != 3 {
		t.Fatalf("paid collection rows=%+v, want paid rows", got)
	}
	roomRows := []roomPageRow{{ID: 5, CollectionStatus: "partial"}, {ID: 6, CollectionStatus: "paid"}}
	if got := filterRoomCollectionRows(roomRows, "unpaid"); len(got) != 1 || got[0].ID != 5 {
		t.Fatalf("unpaid room rows=%+v, want partial row", got)
	}
}

func TestObjectListMutationRedirectsPreservePeriodAndFilters(t *testing.T) {
	propertyRequest := httptest.NewRequest(http.MethodPost, "/properties", nil)
	propertyRequest.Form = url.Values{"period": {"2026-08"}, "status": {"all"}, "search": {"Canal & Park"}, "collection": {"unpaid"}}
	propertyRecorder := httptest.NewRecorder()
	redirectPropertyList(propertyRecorder, propertyRequest, "property_saved", "", false)
	propertyTarget, err := url.Parse(propertyRecorder.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if propertyTarget.Path != "/properties" || propertyTarget.Query().Get("period") != "2026-08" || propertyTarget.Query().Get("status") != "all" || propertyTarget.Query().Get("search") != "Canal & Park" || propertyTarget.Query().Get("collection") != "unpaid" {
		t.Fatalf("property redirect lost context: %s", propertyTarget)
	}

	roomRequest := httptest.NewRequest(http.MethodPost, "/rooms", nil)
	roomRequest.Form = url.Values{"period": {"2026-07"}, "filter_status": {"active"}, "filter_property_id": {"4"}, "search": {"A-01"}, "collection": {"paid"}}
	roomRecorder := httptest.NewRecorder()
	redirectRoomList(roomRecorder, roomRequest, "room_saved", "", false)
	roomTarget, err := url.Parse(roomRecorder.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if roomTarget.Path != "/rooms" || roomTarget.Query().Get("period") != "2026-07" || roomTarget.Query().Get("status") != "active" || roomTarget.Query().Get("property_id") != "4" || roomTarget.Query().Get("search") != "A-01" || roomTarget.Query().Get("collection") != "paid" {
		t.Fatalf("room redirect lost context: %s", roomTarget)
	}

	propertyDetailRequest := httptest.NewRequest(http.MethodPost, "/properties/3", nil)
	propertyDetailRequest.Form = url.Values{"period": {"2026-06"}, "list_status": {"all"}, "list_search": {"Canal House"}, "list_collection": {"paid"}}
	propertyDetailRecorder := httptest.NewRecorder()
	redirectPropertyMutation(propertyDetailRecorder, propertyDetailRequest, 3, "", true)
	propertyDetailTarget, err := url.Parse(propertyDetailRecorder.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if propertyDetailTarget.Path != "/properties/3" || propertyDetailTarget.Query().Get("period") != "2026-06" || propertyDetailTarget.Query().Get("list_status") != "all" || propertyDetailTarget.Query().Get("list_search") != "Canal House" || propertyDetailTarget.Query().Get("list_collection") != "paid" || propertyDetailTarget.Query().Get("edit") != "1" {
		t.Fatalf("property detail redirect lost list context: %s", propertyDetailTarget)
	}

	if got := roomListURL("2026-05", 7, "active", "A-01"); got != "/rooms?period=2026-05&property_id=7&search=A-01&status=active" {
		t.Fatalf("room list return URL=%q", got)
	}
	if got := roomListURL("2026-05", 7, "active", "A-01", "unpaid"); got != "/rooms?collection=unpaid&period=2026-05&property_id=7&search=A-01&status=active" {
		t.Fatalf("filtered room list return URL=%q", got)
	}
}

func TestRoomRentFormParsingUsesIntegerCentsAndValidBillDays(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  int64
	}{
		{value: "1250", want: 125000},
		{value: "1250.4", want: 125040},
		{value: ".05", want: 5},
		{value: "", want: 0},
	} {
		got, err := parseOptionalRoomRentCents(tc.value)
		if err != nil || got != tc.want {
			t.Fatalf("parse room rent %q = %d, %v; want %d cents", tc.value, got, err, tc.want)
		}
	}
	for _, invalid := range []string{"0", "0.00", "1.001", "-1", "1,200", "1."} {
		if _, err := parseOptionalRoomRentCents(invalid); err == nil {
			t.Fatalf("invalid room rent %q was accepted", invalid)
		}
	}

	if got, err := parseOptionalRoomDueDay("31"); err != nil || got != 31 {
		t.Fatalf("parse bill day 31 = %d, %v", got, err)
	}
	if got, err := parseOptionalRoomDueDay(""); err != nil || got != 0 {
		t.Fatalf("parse blank bill day = %d, %v; want inherited value", got, err)
	}
	for _, invalid := range []string{"0", "32", "1.5", "due"} {
		if _, err := parseOptionalRoomDueDay(invalid); err == nil {
			t.Fatalf("invalid bill day %q was accepted", invalid)
		}
	}
}

func TestRoomEditFailureRedirectPreservesPrototypeReturnContext(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/rooms/8", nil)
	request.Form = url.Values{
		"period":             {"2026-09"},
		"from":               {"rooms"},
		"return_property_id": {"4"},
		"return_status":      {"active"},
		"return_search":      {"room & tenant"},
	}
	recorder := httptest.NewRecorder()
	redirectRoomEdit(recorder, request, 8, "room_arrangement_locked")
	target, err := url.Parse(recorder.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if target.Path != "/rooms/8" || target.Query().Get("period") != "2026-09" || target.Query().Get("edit") != "1" || target.Query().Get("from") != "rooms" || target.Query().Get("return_property_id") != "4" || target.Query().Get("return_status") != "active" || target.Query().Get("return_search") != "room & tenant" || target.Query().Get("error") != "room_arrangement_locked" {
		t.Fatalf("room edit error redirect lost context: %s", target)
	}
}
