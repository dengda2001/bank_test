package main

import "testing"

func TestManualTenantSuggestionsKeepOverlappingMultiPartNames(t *testing.T) {
	tenants := []tenant{
		{ID: 1, Name: "Durga Nagendra", DisplayAlias: "Durga Nagendra"},
		{ID: 2, Name: "Nagendra Gonugunta"},
		{ID: 3, Name: "Another Tenant"},
	}

	got := manualTenantSuggestions("DURGA NAGENDRA GONUGUNTA", "confirmed", tenants)
	seen := make(map[uint64]int)
	for _, suggestion := range got {
		seen[suggestion.ID]++
	}
	if seen[1] != 1 || seen[2] != 1 || len(got) != 2 {
		t.Fatalf("multi-part payer suggestions = %+v, want Durga and Nagendra once each", got)
	}
}
