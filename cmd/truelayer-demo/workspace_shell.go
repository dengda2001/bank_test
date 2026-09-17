package main

import "html/template"

// workspaceShell is the page chrome every workspace page shares: the sidebar
// navigation and, on narrow screens, the compact bar that opens it as a drawer.
//
// Every page data struct embeds it, so a template keeps reading {{.Username}} /
// {{.ActivePage}} exactly as before while the markup behind them lives in one
// place. Before this the <aside class="sidebar"> block had seven live copies and
// they had drifted apart: the two cash-receipt pages rendered no nav-count
// badges, and the footer line carried three different captions.
type workspaceShell struct {
	// ActivePage selects the highlighted nav item: "rent-dashboard", "billing",
	// "tenants" or "expenses".
	ActivePage  string
	Username    string
	Environment string
	// FootNote is the second line of the sidebar footer, after "当前用户：<user>".
	FootNote string
	// ShowNavCounts is false on the pages that never rendered the count badges
	// (cash receipt entry, cash receipt void, tenant detail). Rendering them
	// there would change those pages' desktop appearance.
	ShowNavCounts bool
	// NavLabel is the <nav> landmark label. Only the dashboard set one; the other
	// pages rendered a bare <nav>.
	NavLabel     string
	TenantCount  int
	IncomeCount  int
	ExpenseCount int
}

// workspaceNav remains a named compatibility value for the existing template
// tests. The actual markup lives in the embedded partial so new pages do not
// grow another user-visible raw string in a Go file.
var workspaceNav = embeddedWebText("web/templates/partials/workspace-nav.html")

var workspaceBase = template.Must(template.New("workspace").ParseFS(webFiles, "web/templates/partials/workspace-nav.html"))

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
