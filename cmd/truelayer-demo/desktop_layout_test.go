package main

import (
	"strings"
	"testing"
)

func TestDesktopWorkspaceShellUsesFigmaTokens(t *testing.T) {
	for _, marker := range []string{
		"--sidebar: oklch(19% .018 240)",
		"--accent: oklch(58% .16 145)",
		"grid-template-columns: 236px minmax(0, 1fr)",
		".topbar {",
		".workspace-toolbar {",
		"background: var(--sidebar)",
	} {
		if !strings.Contains(workspacePageCSS, marker) {
			t.Fatalf("desktop shell missing Figma marker %q", marker)
		}
	}
	if strings.Contains(strings.Split(workspacePageCSS, "@media (max-width: 640px)")[0], "position: sticky") {
		t.Fatal("desktop shell must keep sticky/frozen rules in the mobile stylesheet tail")
	}
}

func TestWorkspaceNavExposesDesktopSections(t *testing.T) {
	for _, marker := range []string{
		`<div class="nav-label">工作台</div>`,
		`href="/bills"`,
		`href="/transactions"`,
		`href="/properties"`,
		`href="/bank"`,
	} {
		if !strings.Contains(workspaceNav, marker) {
			t.Fatalf("workspace nav missing %q", marker)
		}
	}
}
