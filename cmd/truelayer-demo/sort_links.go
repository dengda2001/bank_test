package main

import "strings"

// tableSortLink is a clickable table heading that re-sorts the list below it.
//
// Clicking a column that is already sorted flips it to its paired direction;
// clicking any other column adopts that column's primary direction. The arrow
// always shows what a click will produce, so even an unsorted table tells the
// reader which way each column would go.
type tableSortLink struct {
	URL    string
	Active bool
	Arrow  string
}

// sortLinkFor builds the heading link for one column. baseURL turns a sort value
// into a link. primary and secondary are the column's two sort values; leave
// secondary empty for a column that only sorts one way.
func sortLinkFor(baseURL func(sortValue string) string, current, primary, secondary string) tableSortLink {
	// A single-direction column has nothing to flip to, so it gets no arrow: an
	// arrow would promise a toggle that a second click cannot deliver. The
	// highlight still tells the reader which order the list is in.
	if secondary == "" {
		return tableSortLink{URL: baseURL(primary), Active: current == primary}
	}
	next := primary
	switch current {
	case primary:
		next = secondary
	case secondary:
		next = primary
	}
	arrow := "▼"
	if strings.HasSuffix(next, "_asc") {
		arrow = "▲"
	}
	return tableSortLink{
		URL:    baseURL(next),
		Active: current == primary || current == secondary,
		Arrow:  arrow,
	}
}

// normalisedSort resolves an empty sort value to the column the list falls back
// to, so that heading renders as the active one instead of leaving the table
// with no visible sort at all.
func normalisedSort(current, fallback string) string {
	if strings.TrimSpace(current) == "" {
		return fallback
	}
	return current
}
