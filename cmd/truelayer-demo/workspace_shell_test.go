package main

import (
	"io/fs"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func renderWorkspaceNav(t *testing.T, shell workspaceShell) string {
	t.Helper()
	nav := workspaceBase.Lookup("workspace-nav")
	if nav == nil {
		t.Fatal("workspace-nav is not defined in the shared chrome template")
	}
	page, err := executeTemplate(nav, shell)
	if err != nil {
		t.Fatalf("render workspace nav: %v", err)
	}
	return page
}

// The topbar search box may only appear where submitting it actually filters the
// list (prd.md A1: a page that cannot honour the box must not show one). The set
// is the routes whose handler reads ?search=; the notable absences are
// /transactions, whose list filter is ?payer=, and /tenants, which has no list
// search at all.
func TestTopbarSearchFillsOnlyRoutesThatParseSearch(t *testing.T) {
	app := &app{}
	wantSearch := []string{
		"/rent-dashboard", "/bills", "/dunning", "/properties",
		"/rooms", "/tenancies", "/cash-receipts", "/expenses",
	}
	for _, path := range wantSearch {
		r := httptest.NewRequest("GET", path, nil)
		if got := app.fillWorkspaceShell(r, workspaceShell{}).TopSearch.Action; got != path {
			t.Fatalf("%s: topbar search action = %q, want %q", path, got, path)
		}
	}
	wantNone := []string{
		"/transactions", "/tenants", "/bank", "/more",
		"/properties/7", "/rooms/7", "/tenants/7", "/properties/7/rooms/3",
	}
	for _, path := range wantNone {
		r := httptest.NewRequest("GET", path, nil)
		if got := app.fillWorkspaceShell(r, workspaceShell{}).TopSearch.Action; got != "" {
			t.Fatalf("%s: topbar search rendered with action %q, want none", path, got)
		}
	}
}

// Submitting the topbar box must keep the list state the user already chose and
// drop the parts that are positional or one-shot: a new search starts at page 1,
// and it must not reopen a drawer or re-show a message that was already read.
func TestTopbarSearchKeepsListStateAndDropsTransientParams(t *testing.T) {
	app := &app{}
	query := url.Values{
		"period": {"2026-09"}, "status": {"unpaid"}, "sort": {"amount_desc"},
		"page_size": {"24"}, "page": {"3"}, "search": {"old term"},
		"message": {"cash_receipt_saved"}, "error": {"invalid_filter"},
		"add": {"1"}, "detail": {"42"},
	}
	r := httptest.NewRequest("GET", "/expenses?"+query.Encode(), nil)
	shell := app.fillWorkspaceShell(r, workspaceShell{})

	if shell.TopSearch.Action != "/expenses" {
		t.Fatalf("topbar search action = %q, want /expenses", shell.TopSearch.Action)
	}
	if shell.TopSearch.Value != "old term" {
		t.Fatalf("topbar search value = %q, want the current term", shell.TopSearch.Value)
	}
	for _, kept := range []string{"period", "status", "sort", "page_size"} {
		if shell.TopSearch.Params.Get(kept) != query.Get(kept) {
			t.Fatalf("topbar search dropped list state %q (got %q)", kept, shell.TopSearch.Params.Get(kept))
		}
	}
	for _, dropped := range []string{"search", "message", "error", "add", "detail", "page"} {
		if shell.TopSearch.Params.Has(dropped) {
			t.Fatalf("topbar search replayed the transient parameter %q", dropped)
		}
	}
}

func TestWorkspaceNavRendersTopbarSearchOnlyWhenFilled(t *testing.T) {
	without := renderWorkspaceNav(t, workspaceShell{ActivePage: "tenants"})
	if strings.Contains(without, `class="workspace-topsearch"`) {
		t.Fatal("a page with no list search still rendered the topbar search box")
	}

	with := renderWorkspaceNav(t, workspaceShell{
		ActivePage: "expenses",
		TopSearch: topbarSearch{
			Action: "/expenses",
			Value:  "electricity",
			Params: url.Values{"period": {"2026-09"}, "sort": {"amount_desc"}},
		},
	})
	for _, marker := range []string{
		`<form class="workspace-topsearch" method="get" action="/expenses"`,
		`name="search" value="electricity"`,
	} {
		if !strings.Contains(with, marker) {
			t.Fatalf("topbar search box missing %q", marker)
		}
	}
	// The hidden fields are what keep the period and the sort the user had chosen;
	// without them the search would silently reset the list.
	for _, kept := range []string{`name="period" value="2026-09"`, `name="sort" value="amount_desc"`} {
		if !strings.Contains(with, kept) {
			t.Fatalf("topbar search did not carry list state %q", kept)
		}
	}
}

// The count button is the topbar's view of the dashboard's "待人工处理流水"
// panel. It renders only when the count was actually read (the URL is set with
// it), so a page that could not reach the data shows no button rather than a
// fabricated 0.
func TestWorkspaceNavRendersCountButtonOnlyWithAReadCount(t *testing.T) {
	without := renderWorkspaceNav(t, workspaceShell{ActivePage: "bills"})
	if strings.Contains(without, `class="workspace-topbar-count"`) {
		t.Fatal("the count button rendered without a readable pending count")
	}

	with := renderWorkspaceNav(t, workspaceShell{
		ActivePage:         "bills",
		PendingReviewCount: 4,
		PendingReviewURL:   "/rent-dashboard?period=2026-09#pending-review",
	})
	for _, marker := range []string{
		`class="workspace-topbar-count" href="/rent-dashboard?period=2026-09#pending-review"`,
		`>4</a>`,
	} {
		if !strings.Contains(with, marker) {
			t.Fatalf("count button missing %q", marker)
		}
	}
}

// The status card is real data or nothing. The prototype's copy
// ("测试数据已载入 / Rosewood 收租明细 / 更新于 2026-09-16") describes demo data
// and DESIGN-HANDOFF.md:37 forbids substituting placeholder copy for product
// text, so an unread status must render no card at all -- and the demo strings
// must never appear even when a status was read.
func TestSidebarStatusCardIsRealOrAbsent(t *testing.T) {
	absent := renderWorkspaceNav(t, workspaceShell{ActivePage: "expenses", Username: "owner"})
	if strings.Contains(absent, `class="side-status"`) {
		t.Fatal("the sidebar rendered a status card without a readable status")
	}
	if !strings.Contains(absent, `class="side-user"`) {
		t.Fatal("the sidebar footer user block must render on every page")
	}

	present := renderWorkspaceNav(t, workspaceShell{
		ActivePage:      "expenses",
		Username:        "owner",
		StatusTitle:     "银行账户已连接",
		StatusUpdatedAt: "2026-09-16 08:30",
	})
	for _, marker := range []string{`class="side-status"`, "银行账户已连接", "更新于 2026-09-16 08:30"} {
		if !strings.Contains(present, marker) {
			t.Fatalf("status card missing %q", marker)
		}
	}
	for _, forbidden := range []string{"测试数据已载入", "Rosewood 收租明细", "所有者视角"} {
		if strings.Contains(present, forbidden) {
			t.Fatalf("sidebar rendered prototype demo copy %q", forbidden)
		}
	}
}

// no-script fallback: the notice must stay visible when the .js root class was
// never set, and the marked notice must be hidden while scripting is available so
// the same message is not on screen twice (prd.md).
func TestToastHidesTheMarkedNoticeOnlyWhenScriptingIsAvailable(t *testing.T) {
	if !strings.Contains(workspacePageCSS, `.js .notice[data-toast] { display: none; }`) {
		t.Fatal("the marked notice must be hidden only under the scripted root class")
	}
	// Every rule whose selector is a marked notice must be scoped to the scripted
	// root. The selector is matched with a whitespace-tolerant pattern on purpose:
	// keying the scan on the exact spelling ".notice[data-toast] {" let a second
	// rule written ".notice[data-toast]{display:none;}" pass unseen, and that rule
	// hides the message outright — the one failure this test exists to prevent
	// (without scripting the block is the only copy left).
	markedSelector := regexp.MustCompile(`\.notice\[data-toast\][ \t\n]*\{`)
	scoped := markedSelector.FindAllStringIndex(workspacePageCSS, -1)
	if len(scoped) == 0 {
		t.Fatal("no rule targets the marked notice at all; the scripted-root scoping is not being checked")
	}
	for _, rule := range scoped {
		if !strings.HasSuffix(strings.TrimRight(workspacePageCSS[:rule[0]], " \t\n"), ".js") {
			t.Fatal("a rule hides the marked notice outside the scripted root; without scripting no message would be visible")
		}
	}
	if !strings.Contains(workspaceNav, `document.documentElement.classList.add("js")`) {
		t.Fatal("the shared chrome must mark the root element as scripted before the toast script runs")
	}
	if !strings.Contains(workspaceNav, `querySelectorAll(".notice[data-toast]")`) {
		t.Fatal("the shared script does not promote the marked notice into the toast")
	}
	// ...and it must look them up inside show(), not at parse time. The chrome is
	// rendered before the page body, so a top-level lookup always finds nothing:
	// the CSS part of the contract still hides the notice and no toast ever
	// appears, which is a silent, total loss of the message.
	lookup := strings.Index(workspaceNav, `querySelectorAll(".notice[data-toast]")`)
	showFn := strings.Index(workspaceNav, "const show = () => {")
	if showFn < 0 || lookup < showFn {
		t.Fatal("the marked notices are queried before show() runs; the chrome precedes the page body, so the lookup would always be empty")
	}
}

// The marker is an opt-in for a success flash and never for an error: an error
// banner has to stay on screen until the user has read and acted on it, and the
// toast dismisses itself after 2.2s. This scans the sources instead of one rendered
// page, so a page added later cannot quietly mark its error with data-toast.
func TestOnlySuccessFlashesAreMarkedForTheToast(t *testing.T) {
	marker := regexp.MustCompile(`class="(notice [^"]*)"\s+data-toast`)
	scanned := 0
	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return fs.SkipDir
			}
			return nil
		}
		name := entry.Name()
		isTemplate := strings.HasSuffix(name, ".html")
		isSource := strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go")
		if !isTemplate && !isSource {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, match := range marker.FindAllSubmatch(body, -1) {
			scanned++
			classes := string(match[1])
			if !strings.Contains(classes, "ok") || strings.Contains(classes, "error") {
				t.Errorf("%s marks a %q notice with data-toast; only a success flash may become the toast", path, classes)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if scanned < 20 {
		t.Fatalf("found only %d data-toast markers; the scan is not reading the real sources", scanned)
	}
}
