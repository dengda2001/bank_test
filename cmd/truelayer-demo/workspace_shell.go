package main

import (
	"html/template"
	"net/http"
	"strings"
)

type tenantPeriodMatchCalendarData struct {
	Options          []billingRentMatchOption
	SelectedTenantID uint64
	SelectedPeriod   string
}

func tenantPeriodMatchCalendar(options []billingRentMatchOption, selectedTenantID uint64, selectedPeriod string) tenantPeriodMatchCalendarData {
	return tenantPeriodMatchCalendarData{Options: options, SelectedTenantID: selectedTenantID, SelectedPeriod: selectedPeriod}
}

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

// fillWorkspaceShell completes the shell's real-data sidebar status card.
//
// It is deliberately best-effort. The sidebar is rendered by every workspace
// page, so a status card that cannot read its value must disappear rather than
// fail the page or invent a placeholder.
func (a *app) fillWorkspaceShell(r *http.Request, shell workspaceShell) workspaceShell {
	if r == nil {
		return shell
	}
	userID, ok := a.currentUserID(r)
	if !ok || a.db == nil {
		return shell
	}
	ctx := r.Context()
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
var workspaceBase = template.Must(template.New("workspace").Funcs(template.FuncMap{
	"tenantPeriodMatchCalendar": tenantPeriodMatchCalendar,
}).ParseFS(webFiles,
	"web/templates/partials/workspace-nav.html",
	"web/templates/partials/collection-settle-form.html",
	"web/templates/partials/tenant-period-calendar.html",
	"web/templates/partials/tenant-form-drawer.html",
	"web/templates/partials/expense-form-drawer.html",
	"web/templates/partials/cash-receipt-drawer.html",
	"web/templates/partials/room-create-drawer.html",
))

// newWorkspacePageTemplate parses a page body into a clone of workspaceBase, so
// the page can call {{template "workspace-nav" .}}. Each page gets its own clone,
// which keeps one page's definitions from leaking into another's.
func newWorkspacePageTemplate(name string, functs template.FuncMap, body string) *template.Template {
	body = withWorkspaceControlAssets(body)
	page := template.Must(workspaceBase.Clone()).New(name)
	if len(functs) > 0 {
		page = page.Funcs(functs)
	}
	return template.Must(page.Parse(body))
}

func newEmbeddedWorkspacePageTemplate(name string, funcs template.FuncMap, path string) *template.Template {
	body := withWorkspaceControlAssets(embeddedWebText(path))
	page := template.Must(workspaceBase.Clone()).New(name)
	if len(funcs) > 0 {
		page = page.Funcs(funcs)
	}
	return template.Must(page.Parse(body))
}

func withWorkspaceControlAssets(body string) string {
	if strings.Contains(body, `href="/static/css/workspace-controls.css"`) {
		return body
	}
	headEnd := strings.Index(body, "</head>")
	if headEnd < 0 {
		return body
	}
	assets := `<link rel="stylesheet" href="/static/css/calendar.css"><link rel="stylesheet" href="/static/css/workspace-controls.css">`
	if !strings.Contains(body, `href="/static/css/pages/entity-drawers.css"`) {
		assets += `<link rel="stylesheet" href="/static/css/pages/entity-drawers.css">`
	}
	if !strings.Contains(body, `href="/static/css/pages/cash-receipts.css"`) {
		assets += `<link rel="stylesheet" href="/static/css/pages/cash-receipts.css">`
	}
	assets += `<script src="/static/js/calendar.js" defer></script><script src="/static/js/workspace-controls.js" defer></script>`
	return body[:headEnd] + assets + body[headEnd:]
}
