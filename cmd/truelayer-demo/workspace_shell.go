package main

import (
	"html/template"
	"net/http"
	"net/url"
	"strings"
)

// workspaceShell is the page chrome every workspace page shares: the sidebar
// navigation and, on narrow screens, the compact bar that opens it as a drawer.
//
// Every page data struct embeds it, so a template keeps reading {{.Username}} /
// {{.ActivePage}} exactly as before while the markup behind them lives in one
// place. Before this the <aside class="sidebar"> block had seven live copies and
// they had drifted apart: the two cash-receipt pages rendered no nav-count
// badges, and the footer line carried three different captions.
type workspaceShell struct {
	// ActivePage selects the highlighted nav item. Canonical pages use
	// "rent-dashboard", "bills", "transactions", "dunning", "properties",
	// "rooms", "tenants", "tenancies", "cash-receipts", "expenses", "bank"
	// or "more";
	// "billing" remains the legacy transaction alias.
	ActivePage  string
	Username    string
	Environment string
	// FootNote is the page caption: the breadcrumb's second half on desktop and
	// the compact bar's subtitle on mobile.
	FootNote string
	// CompactTitle is the object/page context shown beside RentOps on mobile.
	// It is separate from FootNote because a detail view shows the object name,
	// while the desktop sidebar keeps its stable page caption.
	CompactTitle string
	// ShowNavCounts is false on the pages that never rendered the count badges
	// (cash receipt entry, cash receipt void, tenant detail). Rendering them
	// there would change those pages' desktop appearance.
	ShowNavCounts bool
	TenantCount   int
	IncomeCount   int
	ExpenseCount  int

	// TopSearch is the topbar's list search. It stays zero on a page that has no
	// ?search= list, and the template then renders no box at all: prd.md rejects
	// a search input that cannot filter anything (A1), so a page that cannot
	// honour the box must not show one.
	TopSearch topbarSearch

	// PendingReviewCount and PendingReviewURL drive the topbar's count button:
	// the number of transactions waiting for manual review and the dashboard
	// panel they live in. The template renders no button while URL is empty, so a
	// page whose count could not be read shows nothing rather than a fabricated 0.
	PendingReviewCount int
	PendingReviewURL   string

	// StatusTitle / StatusUpdatedAt are the sidebar's data-status card. The
	// prototype shows one there (figma/rentops-desktop-suite.html:49) reading
	// "测试数据已载入 / Rosewood 收租明细 / 更新于 2026-09-16", but that describes
	// demo data: figma/DESIGN-HANDOFF.md:37 forbids substituting placeholder copy
	// for real product text and prd.md forbids writing that copy into the shell.
	// Both lines are therefore real and optional — an empty field renders no line,
	// and a workspace with no readable status renders no card.
	StatusTitle     string
	StatusUpdatedAt string
}

// topbarSearch is the topbar's list-search form. Action is the page's own path,
// so the box submits to the list it sits above rather than to a global search
// that does not exist yet (prd.md defers cross-entity search to a later task).
type topbarSearch struct {
	Action string
	Value  string
	// Params carries the page's other query values as hidden fields, so
	// submitting the topbar search keeps the period and status the user had
	// already chosen instead of silently resetting the list.
	Params url.Values
}

// topbarSearchRoutes are the routes whose handler parses ?search= and renders a
// list. The topbar box is rendered on exactly these. A detail route such as
// /properties/7 is deliberately absent even though its ActivePage is a list
// page, and /transactions is absent because its list filter is ?payer=, not
// ?search=, so the box there would not filter what the page shows.
var topbarSearchRoutes = map[string]bool{
	"/rent-dashboard": true,
	"/bills":          true,
	"/dunning":        true,
	"/properties":     true,
	"/rooms":          true,
	"/tenancies":      true,
	"/cash-receipts":  true,
	"/expenses":       true,
}

// topbarSearchTransient are query keys the search form must not replay. Most
// describe a transient overlay or a one-shot notice — replaying them would
// reopen a drawer the user had closed or re-show a message already read. "page"
// is different: it is real list state, but a new search restarts the list at
// page 1, which is also what the pages' own search boxes do (their forms omit
// the parameter), so the topbar box must not silently keep the old offset.
var topbarSearchTransient = map[string]bool{
	"search": true, "message": true, "error": true, "add": true,
	"edit": true, "detail": true, "reconnect": true, "page": true,
}

// fillWorkspaceShell completes the shell's real-data widgets — the topbar's list
// search and pending-review button, and the sidebar's data-status card.
//
// It is deliberately best-effort. The sidebar and the topbar are rendered by
// every workspace page, so a widget that cannot read its value must disappear
// rather than fail the page or invent a placeholder.
func (a *app) fillWorkspaceShell(r *http.Request, shell workspaceShell) workspaceShell {
	if r == nil {
		return shell
	}
	if topbarSearchRoutes[r.URL.Path] {
		shell.TopSearch = topbarSearch{
			Action: r.URL.Path,
			Value:  strings.TrimSpace(r.URL.Query().Get("search")),
		}
		params := url.Values{}
		for key, values := range r.URL.Query() {
			if topbarSearchTransient[key] {
				continue
			}
			params[key] = values
		}
		if len(params) > 0 {
			shell.TopSearch.Params = params
		}
	}
	userID, ok := a.currentUserID(r)
	if !ok || a.db == nil {
		return shell
	}
	ctx := r.Context()
	// The button opens the dashboard's "待人工处理流水" panel, so it counts what
	// that panel counts for the month on screen: pending income transactions.
	// An absent or invalid ?period= falls back to the current month, which is
	// also the dashboard's own default.
	if period, err := parsePeriodMonth(strings.TrimSpace(r.URL.Query().Get("period"))); err == nil {
		var pending int64
		err := a.db.WithContext(ctx).Model(&paymentTransaction{}).
			Where("user_id = ? AND direction = ? AND match_status IN ?", userID, "income", pendingMatchStatuses).
			Where("transaction_time >= ? AND transaction_time < ?", period, period.AddDate(0, 1, 0)).
			Count(&pending).Error
		if err == nil {
			shell.PendingReviewCount = int(pending)
			shell.PendingReviewURL = "/rent-dashboard?period=" + period.Format("2006-01") + "#pending-review"
		}
	}
	if a.bankConnections != nil {
		if connected, lastSync, err := a.bankConnections.status(ctx, userID); err == nil {
			if connected {
				shell.StatusTitle = "银行账户已连接"
			} else {
				shell.StatusTitle = "银行账户未连接"
			}
			if lastSync != nil && !lastSync.IsZero() {
				shell.StatusUpdatedAt = lastSync.UTC().Format("2006-01-02 15:04")
			}
		}
	}
	return shell
}

// workspaceNav remains a named compatibility value for the existing template
// tests. The actual markup lives in the embedded partial so new pages do not
// grow another user-visible raw string in a Go file.
var workspaceNav = embeddedWebText("web/templates/partials/workspace-nav.html")

// Partials registered here are available to every page body: pages parse into
// clones of this base, and a {{define}} in a page body stays inside its own
// clone. "collection-settle-form" lives here because /bills and the rent
// workspace's tenant view must render the same inline form.
var workspaceBase = template.Must(template.New("workspace").ParseFS(webFiles,
	"web/templates/partials/workspace-nav.html",
	"web/templates/partials/collection-settle-form.html",
))

// newWorkspacePageTemplate parses a page body into a clone of workspaceBase, so
// the page can call {{template "workspace-nav" .}}. Each page gets its own clone,
// which keeps one page's definitions from leaking into another's.
func newWorkspacePageTemplate(name string, functs template.FuncMap, body string) *template.Template {
	page := template.Must(workspaceBase.Clone()).New(name)
	if len(functs) > 0 {
		page = page.Funcs(functs)
	}
	return template.Must(page.Parse(body))
}

func newEmbeddedWorkspacePageTemplate(name string, funcs template.FuncMap, path string) *template.Template {
	page := template.Must(workspaceBase.Clone()).New(name)
	if len(funcs) > 0 {
		page = page.Funcs(funcs)
	}
	return template.Must(page.Parse(embeddedWebText(path)))
}
