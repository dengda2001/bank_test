package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestCanonicalPageRoutesRequireAuthentication(t *testing.T) {
	a := testApp()
	handler := newAppMux(&a)
	paths := []string{"/bills", "/billing", "/transactions", "/dunning", "/properties", "/properties/1", "/properties/not-an-id", "/rooms", "/rooms/1", "/cash-receipts", "/bank"}
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

func TestCanonicalRoutesKeepRefreshAndBankTargets(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/refresh", nil)
	if got := bankRefreshRedirect(request, "message=refreshed"); got != "/transactions?message=refreshed" {
		t.Fatalf("transactions refresh target=%q", got)
	}
	request = httptest.NewRequest(http.MethodPost, "/bank/sync", nil)
	if got := bankRefreshRedirect(request, "message=refreshed"); got != "/bank?message=refreshed" {
		t.Fatalf("canonical refresh target=%q", got)
	}
}

// /billing 是银行回调的落点，也是老书签，所以它必须一直在——但它只转发，
// 不再渲染第二张流水表。查询串原样带过去，老书签里的筛选条件才不丢。
func TestBillingAliasForwardsQueryToTheTransactionList(t *testing.T) {
	a := testApp()
	a.db = &gorm.DB{}
	for _, tc := range []struct{ path, want string }{
		{path: "/billing", want: "/transactions"},
		{path: "/billing?message=bank_connected", want: "/transactions?message=bank_connected"},
		{path: "/billing?period=2026-09&match_status=pending", want: "/transactions?period=2026-09&match_status=pending"},
		{path: "/billing?reconnect=1&error=no_saved_login", want: "/transactions?reconnect=1&error=no_saved_login"},
	} {
		rec := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, tc.path, nil)
		// 别名和 /bills 一样先过会话：未登录时它得先去登录，而不是把人送到列表页。
		request.AddCookie(userSessionCookie(a.cfg, 42, "ddrzh", time.Now().Add(sessionTTL)))
		a.handleBillingAlias(rec, request)
		if got := rec.Header().Get("Location"); got != tc.want {
			t.Fatalf("%s -> %q, want %q", tc.path, got, tc.want)
		}
		if rec.Code != http.StatusFound {
			t.Fatalf("%s status=%d, want %d", tc.path, rec.Code, http.StatusFound)
		}
	}
}

func TestBillingAliasRejectsNonGet(t *testing.T) {
	rec := httptest.NewRecorder()
	a := testApp()
	a.handleBillingAlias(rec, httptest.NewRequest(http.MethodPost, "/billing", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /billing status=%d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestLegacyBillsURLRedirectPreservesTenantFilters(t *testing.T) {
	target, err := url.Parse(legacyBillsWorkspaceURL(url.Values{
		"period": {"2026-08"}, "search": {"Aoife Murphy"}, "status": {"partial"},
		"sort": {"due_asc"}, "page": {"2"}, "page_size": {"24"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	query := target.Query()
	if target.Path != "/rent-dashboard" || query.Get("view") != "tenants" || query.Get("period") != "2026-08" || query.Get("search") != "Aoife Murphy" || query.Get("status") != "partial" || query.Get("sort") != "due_asc" || query.Get("page") != "2" || query.Get("page_size") != "24" {
		t.Fatalf("legacy bills target=%q", target.String())
	}
}

// The manual-balance form lives on the tenant responsibility workspace and
// carries a required reason plus the canonical return context.
func TestBillsManualBalanceRequiresReasonField(t *testing.T) {
	page, err := executeTemplate(billsPageTemplate, rentDashboardPageData{
		Period:      "2026-09",
		PeriodLabel: "2026年9月",
		Page:        1,
		PageSize:    12,
		TotalPages:  1,
		Rows:        []rentDashboardRow{{ObligationID: 7, TenantID: 7, ExpectedCents: 100, PaidCents: 50, Status: "partial", StatusLabel: "部分缴纳"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	form := markupBetween(t, page, `action="/rent-dashboard/settle"`, `</form>`)
	for _, marker := range []string{`name="reason"`, `required`, `placeholder="平账原因"`, `data-confirm="true"`, `name="return_to"`} {
		if !strings.Contains(form, marker) {
			t.Fatalf("bills settle form missing manual-balance marker %q: %s", marker, form)
		}
	}
	if !strings.Contains(page, "确认按剩余未付金额平账吗？") {
		t.Fatal("bills page lost its manual-balance confirmation prompt")
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
	propertyRequest.Form = url.Values{"period": {"2026-08"}, "status": {"all"}, "search": {"Canal & Park"}, "collection": {"unpaid"}, "sort": {"expected_desc"}}
	propertyRecorder := httptest.NewRecorder()
	redirectPropertyList(propertyRecorder, propertyRequest, "property_saved", "", false)
	propertyTarget, err := url.Parse(propertyRecorder.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if propertyTarget.Path != "/properties" || propertyTarget.Query().Get("period") != "2026-08" || propertyTarget.Query().Get("status") != "all" || propertyTarget.Query().Get("search") != "Canal & Park" || propertyTarget.Query().Get("collection") != "unpaid" || propertyTarget.Query().Get("sort") != "expected_desc" {
		t.Fatalf("property redirect lost context: %s", propertyTarget)
	}

	roomRequest := httptest.NewRequest(http.MethodPost, "/rooms", nil)
	roomRequest.Form = url.Values{"period": {"2026-07"}, "filter_status": {"active"}, "filter_property_id": {"4"}, "search": {"A-01"}, "collection": {"paid"}, "sort": {"count_desc"}}
	roomRecorder := httptest.NewRecorder()
	redirectRoomList(roomRecorder, roomRequest, "room_saved", "", false)
	roomTarget, err := url.Parse(roomRecorder.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if roomTarget.Path != "/rooms" || roomTarget.Query().Get("period") != "2026-07" || roomTarget.Query().Get("status") != "active" || roomTarget.Query().Get("property_id") != "4" || roomTarget.Query().Get("search") != "A-01" || roomTarget.Query().Get("collection") != "paid" || roomTarget.Query().Get("sort") != "count_desc" {
		t.Fatalf("room redirect lost context: %s", roomTarget)
	}

	propertyDetailRequest := httptest.NewRequest(http.MethodPost, "/properties/3", nil)
	propertyDetailRequest.Form = url.Values{"period": {"2026-06"}, "list_status": {"all"}, "list_search": {"Canal House"}, "list_collection": {"paid"}, "list_sort": {"paid_desc"}}
	propertyDetailRecorder := httptest.NewRecorder()
	redirectPropertyMutation(propertyDetailRecorder, propertyDetailRequest, 3, "", "", true)
	propertyDetailTarget, err := url.Parse(propertyDetailRecorder.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if propertyDetailTarget.Path != "/properties/3" || propertyDetailTarget.Query().Get("period") != "2026-06" || propertyDetailTarget.Query().Get("list_status") != "all" || propertyDetailTarget.Query().Get("list_search") != "Canal House" || propertyDetailTarget.Query().Get("list_collection") != "paid" || propertyDetailTarget.Query().Get("list_sort") != "paid_desc" || propertyDetailTarget.Query().Get("edit") != "1" {
		t.Fatalf("property detail redirect lost list context: %s", propertyDetailTarget)
	}

	if got := roomListURL("2026-05", 7, "active", "A-01"); got != "/rooms?period=2026-05&property_id=7&search=A-01&status=active" {
		t.Fatalf("room list return URL=%q", got)
	}
	if got := roomListURL("2026-05", 7, "active", "A-01", "unpaid"); got != "/rooms?collection=unpaid&period=2026-05&property_id=7&search=A-01&status=active" {
		t.Fatalf("filtered room list return URL=%q", got)
	}
	if got := roomListURL("2026-05", 7, "active", "A-01", "unpaid", "paid_desc"); got != "/rooms?collection=unpaid&period=2026-05&property_id=7&search=A-01&sort=paid_desc&status=active" {
		t.Fatalf("sorted room list return URL=%q", got)
	}
}

func TestRoomRentPlanFormParsesRentAsIntegerCents(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  int64
	}{
		{value: "1250", want: 125000},
		{value: "1250.4", want: 125040},
		{value: ".05", want: 5},
		{value: "", want: 0},
	} {
		got, err := parseOptionalRentPlanAmountCents(tc.value)
		if err != nil || got != tc.want {
			t.Fatalf("parse room rent %q = %d, %v; want %d cents", tc.value, got, err, tc.want)
		}
	}
	for _, invalid := range []string{"0", "0.00", "1.001", "-1", "1,200", "1."} {
		if _, err := parseOptionalRentPlanAmountCents(invalid); err == nil {
			t.Fatalf("invalid room rent %q was accepted", invalid)
		}
	}
}

func TestRoomEditKeepsCurrentInactivePropertySelectable(t *testing.T) {
	options := roomEditPropertyOptions([]property{
		{ID: 1, Name: "Active property", Status: "active"},
		{ID: 2, Name: "Current inactive property", Status: "inactive"},
		{ID: 3, Name: "Other inactive property", Status: "inactive"},
	}, 2)
	if len(options) != 2 || options[0].ID != 1 || options[1].ID != 2 {
		t.Fatalf("room edit property options=%+v; want active and current properties", options)
	}
	if options[0].StatusLabel != "在用" || options[1].StatusLabel != "已停用" {
		t.Fatalf("asset status labels=%q and %q", options[0].StatusLabel, options[1].StatusLabel)
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
	redirectRoomEdit(recorder, request, 8, "room_property_locked")
	target, err := url.Parse(recorder.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if target.Path != "/rooms/8" || target.Query().Get("period") != "2026-09" || target.Query().Get("edit") != "1" || target.Query().Get("from") != "rooms" || target.Query().Get("return_property_id") != "4" || target.Query().Get("return_status") != "active" || target.Query().Get("return_search") != "room & tenant" || target.Query().Get("error") != "room_property_locked" {
		t.Fatalf("room edit error redirect lost context: %s", target)
	}
}
