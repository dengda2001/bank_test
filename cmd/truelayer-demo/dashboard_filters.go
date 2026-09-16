package main

import (
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

const (
	dashboardDefaultPageSize = 12
	dashboardMaxPageSize     = 50
	// dashboardDefaultSort is what the list falls back to when no column heading
	// has been clicked: outstanding bills first. The sort is chosen by clicking a
	// table heading, so this value never appears as a form control.
	dashboardDefaultSort = "status"
)

type rentDashboardFilters struct {
	Search   string
	Status   string
	Sort     string
	Page     int
	PageSize int
}

func defaultRentDashboardFilters() rentDashboardFilters {
	return rentDashboardFilters{Status: "all", Sort: dashboardDefaultSort, Page: 1, PageSize: dashboardDefaultPageSize}
}

func rentDashboardFiltersFromQuery(q url.Values) (rentDashboardFilters, error) {
	filters := defaultRentDashboardFilters()
	filters.Search = strings.TrimSpace(q.Get("search"))
	if len([]rune(filters.Search)) > 191 {
		return rentDashboardFilters{}, errors.New("dashboard search is too long")
	}
	if value := strings.TrimSpace(q.Get("status")); value != "" {
		filters.Status = value
	}
	if value := strings.TrimSpace(q.Get("sort")); value != "" {
		filters.Sort = value
	}
	if value := strings.TrimSpace(q.Get("page")); value != "" {
		page, err := strconv.Atoi(value)
		if err != nil || page < 1 {
			return rentDashboardFilters{}, errors.New("dashboard page is invalid")
		}
		filters.Page = page
	}
	if value := strings.TrimSpace(q.Get("page_size")); value != "" {
		pageSize, err := strconv.Atoi(value)
		if err != nil || pageSize < 1 || pageSize > dashboardMaxPageSize {
			return rentDashboardFilters{}, errors.New("dashboard page size is invalid")
		}
		filters.PageSize = pageSize
	}
	if err := validateRentDashboardFilters(filters); err != nil {
		return rentDashboardFilters{}, err
	}
	return filters, nil
}

func validateRentDashboardFilters(filters rentDashboardFilters) error {
	switch filters.Status {
	case "all", "paid", "partial", "unpaid", "open", "overdue", "needs_review":
	default:
		return errors.New("dashboard status is invalid")
	}
	switch filters.Sort {
	case dashboardDefaultSort, "tenant_asc", "tenant_desc", "amount_desc", "amount_asc", "due_asc", "due_desc":
	default:
		return errors.New("dashboard sort is invalid")
	}
	if filters.Page < 1 || filters.PageSize < 1 || filters.PageSize > dashboardMaxPageSize {
		return errors.New("dashboard pagination is invalid")
	}
	return nil
}

func dashboardStatusMatches(status, filter string) bool {
	switch filter {
	case "all":
		return true
	case "unpaid":
		return status == "open" || status == "overdue" || status == "partial"
	default:
		return status == filter
	}
}

func dashboardSearchMatches(row rentDashboardRow, search string) bool {
	search = strings.ToLower(strings.TrimSpace(search))
	if search == "" {
		return true
	}
	for _, value := range []string{row.TenantName, row.TenantAlias, row.RoomLabel, row.RoomAddress} {
		if strings.Contains(strings.ToLower(value), search) {
			return true
		}
	}
	return false
}

func filterAndSortRentDashboardRows(rows []rentDashboardRow, filters rentDashboardFilters) []rentDashboardRow {
	filtered := make([]rentDashboardRow, 0, len(rows))
	for _, row := range rows {
		if !dashboardStatusMatches(row.Status, filters.Status) || !dashboardSearchMatches(row, filters.Search) {
			continue
		}
		filtered = append(filtered, row)
	}
	statusPriority := map[string]int{"overdue": 0, "needs_review": 1, "partial": 2, "open": 3, "paid": 4}
	sort.SliceStable(filtered, func(i, j int) bool {
		left, right := filtered[i], filtered[j]
		switch filters.Sort {
		case "tenant_asc":
			if strings.ToLower(left.TenantName) != strings.ToLower(right.TenantName) {
				return strings.ToLower(left.TenantName) < strings.ToLower(right.TenantName)
			}
		case "tenant_desc":
			if strings.ToLower(left.TenantName) != strings.ToLower(right.TenantName) {
				return strings.ToLower(left.TenantName) > strings.ToLower(right.TenantName)
			}
		case "amount_desc":
			if left.ExpectedCents != right.ExpectedCents {
				return left.ExpectedCents > right.ExpectedCents
			}
		case "amount_asc":
			if left.ExpectedCents != right.ExpectedCents {
				return left.ExpectedCents < right.ExpectedCents
			}
		case "due_asc":
			if !left.DueDateValue.Equal(right.DueDateValue) {
				return left.DueDateValue.Before(right.DueDateValue)
			}
		case "due_desc":
			if !left.DueDateValue.Equal(right.DueDateValue) {
				return left.DueDateValue.After(right.DueDateValue)
			}
		default:
			if statusPriority[left.Status] != statusPriority[right.Status] {
				return statusPriority[left.Status] < statusPriority[right.Status]
			}
		}
		if strings.ToLower(left.TenantName) != strings.ToLower(right.TenantName) {
			return strings.ToLower(left.TenantName) < strings.ToLower(right.TenantName)
		}
		return left.TenantID < right.TenantID
	})
	return filtered
}

func paginateRentDashboardRows(rows []rentDashboardRow, page, pageSize int) ([]rentDashboardRow, int) {
	if page < 1 || pageSize < 1 {
		return []rentDashboardRow{}, 0
	}
	totalPages := (len(rows) + pageSize - 1) / pageSize
	if totalPages == 0 || page > totalPages {
		return []rentDashboardRow{}, totalPages
	}
	start := (page - 1) * pageSize
	end := start + pageSize
	if end > len(rows) {
		end = len(rows)
	}
	return rows[start:end], totalPages
}

func rentDashboardURL(period, search, status, sortValue string, page, pageSize int) string {
	values := url.Values{"period": []string{period}}
	if strings.TrimSpace(search) != "" {
		values.Set("search", search)
	}
	if status != "" && status != "all" {
		values.Set("status", status)
	}
	if sortValue != "" && sortValue != dashboardDefaultSort {
		values.Set("sort", sortValue)
	}
	if page > 1 {
		values.Set("page", strconv.Itoa(page))
	}
	if pageSize > 0 && pageSize != dashboardDefaultPageSize {
		values.Set("page_size", strconv.Itoa(pageSize))
	}
	return "/rent-dashboard?" + values.Encode()
}
