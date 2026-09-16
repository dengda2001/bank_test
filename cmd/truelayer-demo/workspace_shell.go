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

// workspaceNav is the single source of truth for the sidebar. It is defined once
// and every page template is cloned from workspaceBase, so all seven pages share
// this definition instead of carrying a copy.
//
// The drawer is pure CSS: an invisible checkbox holds the open state, the compact
// bar and the scrim are <label for> toggles, and the sidebar slides in on
// `#nav-drawer:checked`. No script is involved, so the markup is assertable from
// a Go template test and the navigation still works with JavaScript disabled.
//
// The checkbox deliberately does NOT carry the `hidden` attribute: `hidden` is
// `display: none`, which cannot take focus, and a drawer that only a pointer can
// open is worse than the plain sidebar links it replaced. `.nav-drawer-input`
// hides it visually while leaving it focusable — see workspacePageCSS.
const workspaceNav = `{{define "workspace-nav"}}<input type="checkbox" id="nav-drawer" class="nav-drawer-input">
    <label for="nav-drawer" class="nav-compact-bar" aria-label="展开导航"><span class="mark">R</span><span class="brand-title">RentOps</span><span class="nav-burger">☰</span></label>
    <label for="nav-drawer" class="nav-scrim" aria-hidden="true"></label>
    <aside class="sidebar" aria-label="Main navigation">
      <div class="side-brand"><div class="mark">R</div><div><div class="brand-title">RentOps</div><div class="brand-meta">{{.Environment}} workspace</div></div></div>
      <nav class="nav"{{if .NavLabel}} aria-label="{{.NavLabel}}"{{end}}>
        <a href="/rent-dashboard"{{if eq .ActivePage "rent-dashboard"}} class="active"{{end}}><span class="glyph">总</span><span>月度总览</span>{{if .ShowNavCounts}}<span class="nav-count">{{.TenantCount}}</span>{{end}}</a>
        <a href="/billing"{{if eq .ActivePage "billing"}} class="active"{{end}}><span class="glyph">流</span><span>银行流水</span>{{if .ShowNavCounts}}<span class="nav-count">{{.IncomeCount}}</span>{{end}}</a>
        <a href="/tenants"{{if eq .ActivePage "tenants"}} class="active"{{end}}><span class="glyph">租</span><span>租客管理</span>{{if .ShowNavCounts}}<span class="nav-count">{{.TenantCount}}</span>{{end}}</a>
        <a href="/expenses"{{if eq .ActivePage "expenses"}} class="active"{{end}}><span class="glyph">支</span><span>支出记录</span>{{if .ShowNavCounts}}<span class="nav-count">{{.ExpenseCount}}</span>{{end}}</a>
      </nav>
      <div class="side-foot">当前用户：{{.Username}}<br>{{.FootNote}}</div>
    </aside>{{end}}`

var workspaceBase = template.Must(template.New("workspace").Parse(workspaceNav))

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
