package main

// paginationLink is a page number rendered beside the previous/next controls.
// Ellipsis entries keep long lists compact while leaving nearby pages clickable.
type paginationLink struct {
	Number   int
	Current  bool
	Ellipsis bool
	URL      string
}

func paginationLinks(current, total int, pageURL func(int) string) []paginationLink {
	if total <= 0 {
		return nil
	}
	if current < 1 {
		current = 1
	}
	if current > total {
		current = total
	}
	if total <= 7 {
		links := make([]paginationLink, 0, total)
		for page := 1; page <= total; page++ {
			links = appendPaginationPage(links, page, current, pageURL)
		}
		return links
	}

	start, end := current-1, current+1
	if current <= 3 {
		start, end = 2, 4
	} else if current >= total-2 {
		start, end = total-3, total-1
	}
	links := []paginationLink{{Number: 1, Current: current == 1}}
	if current != 1 && pageURL != nil {
		links[0].URL = pageURL(1)
	}
	if start > 2 {
		links = append(links, paginationLink{Ellipsis: true})
	}
	for page := start; page <= end; page++ {
		links = appendPaginationPage(links, page, current, pageURL)
	}
	if end < total-1 {
		links = append(links, paginationLink{Ellipsis: true})
	}
	links = appendPaginationPage(links, total, current, pageURL)
	return links
}

func appendPaginationPage(links []paginationLink, page, current int, pageURL func(int) string) []paginationLink {
	link := paginationLink{Number: page, Current: page == current}
	if !link.Current && pageURL != nil {
		link.URL = pageURL(page)
	}
	return append(links, link)
}
