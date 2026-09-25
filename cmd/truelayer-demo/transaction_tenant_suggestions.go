package main

import (
	"sort"
	"strings"
	"unicode"
)

// manualTenantSuggestions is presentation-only. A similar name never becomes
// identity evidence for background allocation.
func manualTenantSuggestions(payerName, payerNameKind string, tenants []tenant) []transactionReviewTenant {
	if payerNameKind != "confirmed" {
		return nil
	}
	payerTokens := tenantNameTokens(payerName)
	if len(payerTokens) == 0 {
		return nil
	}
	type scoredTenant struct {
		row   tenant
		score float64
	}
	scores := make([]scoredTenant, 0)
	payer := strings.Join(payerTokens, " ")
	for _, row := range tenants {
		best := float64(0)
		for _, name := range []string{row.Name, row.DisplayAlias} {
			tokens := tenantNameTokens(name)
			if len(tokens) == 0 {
				continue
			}
			candidate := strings.Join(tokens, " ")
			ratio := nameEditSimilarity(payer, candidate)
			overlap := distinctiveNameTokenOverlap(payerTokens, tokens)
			if payer == candidate || ratio >= .72 && (overlap > 0 || ratio >= .86) || overlap >= 2 {
				best = max(best, ratio)
				if overlap >= 2 {
					best = max(best, .72)
				}
			}
		}
		if best > 0 {
			scores = append(scores, scoredTenant{row: row, score: best})
		}
	}
	if len(scores) == 0 {
		// A bank name can contain extra given/family names. A distinctive payer
		// word is a navigation hint only, never identity evidence.
		for _, row := range tenants {
			best := float64(0)
			for _, name := range []string{row.Name, row.DisplayAlias} {
				for _, candidate := range tenantNameTokens(name) {
					if !isDistinctiveNameToken(candidate) {
						continue
					}
					for _, payerToken := range payerTokens {
						if !isDistinctiveNameToken(payerToken) {
							continue
						}
						switch {
						case candidate == payerToken:
							best = max(best, 1)
						case len([]rune(payerToken)) >= 5 && strings.HasPrefix(candidate, payerToken):
							best = max(best, .8)
						case len([]rune(payerToken)) >= 5 && strings.Contains(candidate, payerToken):
							best = max(best, .6)
						}
					}
				}
			}
			if best > 0 {
				scores = append(scores, scoredTenant{row: row, score: best})
			}
		}
	}
	sort.Slice(scores, func(i, j int) bool {
		if scores[i].score != scores[j].score {
			return scores[i].score > scores[j].score
		}
		return scores[i].row.ID < scores[j].row.ID
	})
	result := make([]transactionReviewTenant, 0, len(scores))
	for _, candidate := range scores {
		result = append(result, transactionReviewTenant{ID: candidate.row.ID, Name: firstNonEmpty(candidate.row.DisplayAlias, candidate.row.Name)})
	}
	return result
}

func tenantNameTokens(name string) []string {
	parts := strings.FieldsFunc(strings.ToLower(name), func(r rune) bool { return !unicode.IsLetter(r) })
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		switch part {
		case "mr", "mrs", "ms", "miss", "dr", "na":
			continue
		}
		result = append(result, part)
	}
	return result
}

func distinctiveNameTokenOverlap(a, b []string) int {
	seen := make(map[string]bool)
	for _, left := range a {
		if !isDistinctiveNameToken(left) {
			continue
		}
		for _, right := range b {
			if left == right {
				seen[left] = true
			}
		}
	}
	return len(seen)
}

func isDistinctiveNameToken(token string) bool {
	if len([]rune(token)) < 4 {
		return false
	}
	switch token {
	case "bank", "from", "rent", "payment", "transfer", "sepa", "credit", "direct", "debit":
		return false
	}
	return true
}

func nameEditSimilarity(a, b string) float64 {
	left, right := []rune(a), []rune(b)
	if len(left) == 0 || len(right) == 0 {
		return 0
	}
	previous := make([]int, len(right)+1)
	current := make([]int, len(right)+1)
	for j := range previous {
		previous[j] = j
	}
	for i, l := range left {
		current[0] = i + 1
		for j, r := range right {
			cost := 0
			if l != r {
				cost = 1
			}
			current[j+1] = min(previous[j+1]+1, current[j]+1, previous[j]+cost)
		}
		previous, current = current, previous
	}
	return 1 - float64(previous[len(right)])/float64(max(len(left), len(right)))
}
