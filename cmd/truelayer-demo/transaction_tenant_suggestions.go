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
	for _, row := range tenants {
		tokens := tenantNameTokens(row.Name)
		if len(tokens) == 0 {
			continue
		}
		payer := strings.Join(payerTokens, " ")
		candidate := strings.Join(tokens, " ")
		ratio := nameEditSimilarity(payer, candidate)
		if payer != candidate && (ratio < .72 || (!sharesDistinctiveNameToken(payerTokens, tokens) && ratio < .86)) {
			continue
		}
		scores = append(scores, scoredTenant{row: row, score: ratio})
	}
	sort.Slice(scores, func(i, j int) bool {
		if scores[i].score != scores[j].score {
			return scores[i].score > scores[j].score
		}
		return scores[i].row.ID < scores[j].row.ID
	})
	result := make([]transactionReviewTenant, 0, min(3, len(scores)))
	for _, candidate := range scores {
		if len(result) == 3 {
			break
		}
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

func sharesDistinctiveNameToken(a, b []string) bool {
	for _, left := range a {
		if len([]rune(left)) < 4 {
			continue
		}
		for _, right := range b {
			if left == right {
				return true
			}
		}
	}
	return false
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
