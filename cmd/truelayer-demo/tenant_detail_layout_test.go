package main

import (
	"strings"
	"testing"
)

// styleRuleFor returns the declaration block of the first rule written for
// selector. The selector is matched together with its opening brace, so
// ".profile-list" cannot match ".profile-list dt".
func styleRuleFor(t *testing.T, page, selector string) string {
	t.Helper()
	start := strings.Index(page, selector+" {")
	if start < 0 {
		t.Fatalf("tenant detail page has no style rule for %q", selector)
	}
	rest := page[start:]
	end := strings.Index(rest, "}")
	if end < 0 {
		t.Fatalf("style rule for %q is unterminated", selector)
	}
	return rest[:end]
}

// .panel.surface is a bare card: it brings no padding of its own, so its header
// insets itself to 18px 20px and every body block has to do the same. The
// detail page's blocks were the ones that never did, which left the profile
// labels and values sitting on the card border and the last row pressed against
// its bottom edge.
func TestTenantDetailPanelBodiesMatchTheHeaderEdge(t *testing.T) {
	var body strings.Builder
	if err := tenantDetailTemplate.Execute(&body, tenantDetailPageData{}); err != nil {
		t.Fatal(err)
	}
	page := body.String()

	for selector, wantPadding := range map[string]string{
		".profile-list":   "padding: 18px 20px;",
		".payer-list":     "padding: 18px 20px;",
		".history-filter": "padding: 16px 20px 0;",
		".pagination":     "padding: 0 20px 18px;",
	} {
		rule := styleRuleFor(t, page, selector)
		if !strings.Contains(rule, wantPadding) {
			t.Fatalf("%s does not inset itself to the header edge, so its content sits on the card border: want %q in %s", selector, wantPadding, rule)
		}
	}
}
