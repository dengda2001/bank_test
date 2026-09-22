package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	rentWorkspaceViewProperties = "properties"
	rentWorkspaceViewRooms      = "rooms"
	rentWorkspaceViewTenants    = "tenants"
	rentWorkspaceMaxPageSize    = 50
)

type rentWorkspaceFilters struct {
	PeriodMonth time.Time
	View        string
	PropertyID  uint64
	RoomID      uint64
	Search      string
	Status      string
	Sort        string
	Page        int
	PageSize    int
}

func defaultRentWorkspaceFilters(period time.Time) rentWorkspaceFilters {
	return rentWorkspaceFilters{
		PeriodMonth: monthStart(period),
		View:        rentWorkspaceViewProperties,
		Status:      "all",
		Sort:        dashboardDefaultSort,
		Page:        1,
		PageSize:    dashboardDefaultPageSize,
	}
}

func rentWorkspaceFiltersFromQuery(q url.Values) (rentWorkspaceFilters, error) {
	period, err := parsePeriodMonth(strings.TrimSpace(q.Get("period")))
	if err != nil {
		return rentWorkspaceFilters{}, errors.New("workspace period is invalid")
	}
	filters := defaultRentWorkspaceFilters(period)
	if value := strings.TrimSpace(q.Get("view")); value != "" {
		filters.View = value
	}
	if value := strings.TrimSpace(q.Get("property_id")); value != "" {
		filters.PropertyID, err = parseWorkspaceID(value, "property_id")
		if err != nil {
			return rentWorkspaceFilters{}, err
		}
	}
	if value := strings.TrimSpace(q.Get("room_id")); value != "" {
		filters.RoomID, err = parseWorkspaceID(value, "room_id")
		if err != nil {
			return rentWorkspaceFilters{}, err
		}
	}
	filters.Search = strings.TrimSpace(q.Get("search"))
	if len([]rune(filters.Search)) > 191 {
		return rentWorkspaceFilters{}, errors.New("workspace search is too long")
	}
	if value := strings.TrimSpace(q.Get("status")); value != "" {
		filters.Status = value
	}
	if value := strings.TrimSpace(q.Get("sort")); value != "" {
		filters.Sort = value
	}
	if value := strings.TrimSpace(q.Get("page")); value != "" {
		filters.Page, err = strconv.Atoi(value)
		if err != nil || filters.Page < 1 {
			return rentWorkspaceFilters{}, errors.New("workspace page is invalid")
		}
	}
	if value := strings.TrimSpace(q.Get("page_size")); value != "" {
		filters.PageSize, err = strconv.Atoi(value)
		if err != nil || filters.PageSize < 1 || filters.PageSize > rentWorkspaceMaxPageSize {
			return rentWorkspaceFilters{}, errors.New("workspace page size is invalid")
		}
	}
	if err := validateRentWorkspaceFilters(filters); err != nil {
		return rentWorkspaceFilters{}, err
	}
	return filters, nil
}

func parseWorkspaceID(value, field string) (uint64, error) {
	id, err := strconv.ParseUint(value, 10, 64)
	if err != nil || id == 0 {
		return 0, fmt.Errorf("workspace %s is invalid", field)
	}
	return id, nil
}

func validateRentWorkspaceFilters(filters rentWorkspaceFilters) error {
	switch filters.View {
	case rentWorkspaceViewProperties, rentWorkspaceViewRooms, rentWorkspaceViewTenants:
	default:
		return errors.New("workspace view is invalid")
	}
	switch filters.Status {
	case "all", "outstanding", "needs_review", "overdue", "partial", "open", "paid", "forecast", "vacant":
	default:
		return errors.New("workspace status is invalid")
	}
	switch filters.Sort {
	case dashboardDefaultSort, "name_asc", "name_desc", "balance_desc", "balance_asc", "due_asc", "due_desc":
	default:
		return errors.New("workspace sort is invalid")
	}
	if filters.Page < 1 || filters.PageSize < 1 || filters.PageSize > rentWorkspaceMaxPageSize {
		return errors.New("workspace pagination is invalid")
	}
	return nil
}

func rentWorkspaceURL(filters rentWorkspaceFilters, page int) string {
	values := url.Values{}
	period := monthStart(filters.PeriodMonth)
	values.Set("period", period.Format("2006-01"))
	values.Set("view", firstNonEmpty(filters.View, rentWorkspaceViewProperties))
	if filters.PropertyID != 0 {
		values.Set("property_id", strconv.FormatUint(filters.PropertyID, 10))
	}
	if filters.RoomID != 0 {
		values.Set("room_id", strconv.FormatUint(filters.RoomID, 10))
	}
	if filters.Search != "" {
		values.Set("search", filters.Search)
	}
	if filters.Status != "" && filters.Status != "all" {
		values.Set("status", filters.Status)
	}
	if filters.Sort != "" && filters.Sort != dashboardDefaultSort {
		values.Set("sort", filters.Sort)
	}
	if page > 1 {
		values.Set("page", strconv.Itoa(page))
	}
	if filters.PageSize > 0 && filters.PageSize != dashboardDefaultPageSize {
		values.Set("page_size", strconv.Itoa(filters.PageSize))
	}
	return "/rent-dashboard?" + values.Encode()
}

type rentWorkspaceInput struct {
	UserID       uint64
	PeriodMonth  time.Time
	Properties   []property
	Rooms        []room
	Plans        []roomRentPlan
	Parties      []roomRentPlanMember
	Charges      []rentCharge
	Obligations  []rentObligation
	Tenants      []tenant
	Transactions []paymentTransaction
	Allocations  []paymentAllocation
	CashReceipts []cashReceipt
	Expenses     []manualExpense
	Now          time.Time
}

type rentWorkspaceSummary struct {
	ExpectedCents          int64
	ForecastCents          int64
	PaidCents              int64
	BalanceCents           int64
	ExpenseCents           int64
	OtherIncomeCents       int64
	NetCents               int64
	CollectionPercent      int
	TotalRooms             int
	PaidRooms              int
	UnpaidRooms            int
	VacantRooms            int
	ResponsibilityCount    int
	ForecastCount          int
	FollowupCount          int
	ExpectedAmount         string
	ForecastAmount         string
	ForecastExpectedAmount string
	PaidAmount             string
	BalanceAmount          string
	ProjectedBalanceAmount string
	ExpenseAmount          string
	OtherIncomeAmount      string
	NetAmount              string
}

// rentWorkspaceDimensionSummary is the chips row above the list panel. It is
// aggregated from the rows the active view has already loaded and filtered
// (design.md §2.4) -- never from a second query, so the chips cannot disagree
// with the numbers in the table directly beneath them. The three buckets reuse
// the workspace's own status vocabulary (the same words the status filter
// offers), so a chip and its filter always describe the same rows.
type rentWorkspaceDimensionSummary struct {
	Total    int
	Overdue  int
	Partial  int
	Paid     int
	Forecast int
}

type rentWorkspaceData struct {
	Filters          rentWorkspaceFilters
	IsFuturePeriod   bool
	Summary          rentWorkspaceSummary
	DimensionSummary rentWorkspaceDimensionSummary
	PropertyOptions  []rentWorkspacePropertyOption
	PropertyRows     []rentWorkspacePropertyRow
	PropertyTreeRows []rentWorkspacePropertyTreeRow
	RoomRows         []rentWorkspaceRoomRow
	RoomTreeRows     []rentWorkspaceRoomTreeRow
	TenantRows       []rentWorkspaceTenantRow
	TotalRows        int
	FilteredCount    int
	TotalPages       int
	Page             int
	PageSize         int
}

type rentWorkspacePropertyTreeRow struct {
	Property rentWorkspacePropertyRow
	Rooms    []rentWorkspaceRoomTreeRow
}

type rentWorkspaceRoomTreeRow struct {
	Room    rentWorkspaceRoomRow
	Tenants []rentWorkspaceTenantRow
}

type rentWorkspacePropertyOption struct {
	ID   uint64
	Name string
}

type rentWorkspacePropertyRow struct {
	PropertyID             uint64
	Name                   string
	Address                string
	Status                 string
	StatusLabel            string
	TotalRooms             int
	PaidRooms              int
	UnpaidRooms            int
	VacantRooms            int
	ExpectedCents          int64
	ForecastCents          int64
	PaidCents              int64
	BalanceCents           int64
	ExpenseCents           int64
	OtherIncomeCents       int64
	NetCents               int64
	CollectionPercent      int
	ExpectedAmount         string
	ForecastAmount         string
	ProjectedBalanceAmount string
	IsForecast             bool
	PaidAmount             string
	BalanceAmount          string
	ExpenseAmount          string
	OtherIncomeAmount      string
	NetAmount              string
}

type rentWorkspaceRoomRow struct {
	RoomID                 uint64
	PropertyID             uint64
	PropertyName           string
	RoomLabel              string
	Address                string
	Status                 string
	StatusLabel            string
	TenantCount            int
	ExpectedCents          int64
	ForecastCents          int64
	PaidCents              int64
	BalanceCents           int64
	ExpectedAmount         string
	ForecastAmount         string
	ProjectedBalanceAmount string
	IsForecast             bool
	PaidAmount             string
	BalanceAmount          string
	DueDate                string
	DueDateValue           time.Time
	CollectionPercent      int
	HasCharge              bool
}

type rentWorkspaceTenantRow struct {
	TenantID          uint64
	PropertyID        uint64
	PropertyName      string
	RoomID            uint64
	RoomLabel         string
	RoomAddress       string
	TenantName        string
	TenantAlias       string
	ObligationID      uint64
	Period            string
	DueDate           string
	DueDateValue      time.Time
	ExpectedCents     int64
	PaidCents         int64
	BalanceCents      int64
	ExpectedAmount    string
	PaidAmount        string
	BalanceAmount     string
	CollectionPercent int
	Status            string
	StatusLabel       string
	PaidByOther       bool
	Payments          []rentPaymentDetail
	IsForecast        bool
}

type rentWorkspaceRoomAggregate struct {
	Room             room
	Property         property
	HasRentPlan      bool
	HasPlanMembers   bool
	PlanMemberCount  int
	Charge           *rentCharge
	Obligations      []rentWorkspaceTenantRow
	ExpectedCents    int64
	ForecastCents    int64
	PaidCents        int64
	BalanceCents     int64
	ExpenseCents     int64
	OtherIncomeCents int64
	Status           string
	DueDate          time.Time
	IsForecast       bool
}

func buildRentWorkspace(input rentWorkspaceInput, filters rentWorkspaceFilters) (rentWorkspaceData, error) {
	if input.UserID == 0 {
		return rentWorkspaceData{}, errors.New("workspace userID is required")
	}
	if filters.PeriodMonth.IsZero() {
		filters.PeriodMonth = monthStart(input.PeriodMonth)
	}
	filters.PeriodMonth = monthStart(filters.PeriodMonth)
	if filters.View == "" {
		filters.View = rentWorkspaceViewProperties
	}
	if filters.Status == "" {
		filters.Status = "all"
	}
	if filters.Sort == "" {
		filters.Sort = dashboardDefaultSort
	}
	if filters.Page == 0 {
		filters.Page = 1
	}
	if filters.PageSize == 0 {
		filters.PageSize = dashboardDefaultPageSize
	}
	if err := validateRentWorkspaceFilters(filters); err != nil {
		return rentWorkspaceData{}, err
	}
	if input.Now.IsZero() {
		input.Now = time.Now().UTC()
	}

	propertyByID := make(map[uint64]property)
	for _, row := range input.Properties {
		if row.UserID == input.UserID && row.ID != 0 {
			propertyByID[row.ID] = row
		}
	}
	roomByID := make(map[uint64]room)
	roomPropertyByID := make(map[uint64]uint64)
	for _, row := range input.Rooms {
		if row.UserID != input.UserID || row.ID == 0 || propertyByID[row.PropertyID].ID == 0 {
			continue
		}
		roomPropertyByID[row.ID] = row.PropertyID
		roomByID[row.ID] = row
	}

	planByID := make(map[uint64]roomRentPlan)
	planCoversMonthByRoom := make(map[uint64]bool)
	plansByRoom := make(map[uint64][]roomRentPlan)
	planMemberCountByRoom := make(map[uint64]int)
	membersByPlan := make(map[uint64][]roomRentPlanMember)
	for _, row := range input.Plans {
		if row.UserID != input.UserID || row.ID == 0 || roomByID[row.RoomID].ID == 0 {
			continue
		}
		planByID[row.ID] = row
		if roomRentPlanCoversMonth(row, filters.PeriodMonth) {
			planCoversMonthByRoom[row.RoomID] = true
			plansByRoom[row.RoomID] = append(plansByRoom[row.RoomID], row)
		}
	}
	for _, row := range input.Parties {
		if row.UserID == input.UserID {
			membersByPlan[row.RoomRentPlanID] = append(membersByPlan[row.RoomRentPlanID], row)
		}
		if row.UserID != input.UserID {
			continue
		}
		if _, ok := planByID[row.RoomRentPlanID]; ok {
			plan := planByID[row.RoomRentPlanID]
			if roomRentPlanCoversMonth(plan, filters.PeriodMonth) {
				planMemberCountByRoom[plan.RoomID]++
			}
		}
	}
	roomHasPlanMembers := make(map[uint64]bool)
	for roomID, count := range planMemberCountByRoom {
		roomHasPlanMembers[roomID] = count > 0
	}

	chargeByRoom := make(map[uint64]rentCharge)
	chargeByID := make(map[uint64]rentCharge)
	for _, row := range input.Charges {
		if row.UserID != input.UserID || row.RecordStatus != obligationRecordActive || row.RoomID == 0 || monthStart(row.PeriodMonth) != filters.PeriodMonth {
			continue
		}
		if roomByID[row.RoomID].ID == 0 || roomByID[row.RoomID].PropertyID != row.PropertyID || propertyByID[row.PropertyID].ID == 0 {
			continue
		}
		chargeByRoom[row.RoomID] = row
		chargeByID[row.ID] = row
	}

	tenantByID := make(map[uint64]tenant)
	for _, row := range input.Tenants {
		if row.UserID == input.UserID && row.ID != 0 {
			tenantByID[row.ID] = row
		}
	}
	transactionsByID := make(map[uint64]paymentTransaction)
	for _, row := range input.Transactions {
		if row.UserID == input.UserID && row.ID != 0 {
			transactionsByID[row.ID] = row
		}
	}
	allocationsByObligation := make(map[uint64][]paymentAllocation)
	for _, row := range input.Allocations {
		if row.UserID == input.UserID && row.RentObligationID != nil && ledgerAllocationIsEffective(row) && ledgerAllocationKind(row) == allocationKindRent {
			allocationsByObligation[*row.RentObligationID] = append(allocationsByObligation[*row.RentObligationID], row)
		}
	}
	cashByObligation := make(map[uint64][]cashReceipt)
	for _, row := range input.CashReceipts {
		if row.UserID == input.UserID && row.RentObligationID != 0 {
			cashByObligation[row.RentObligationID] = append(cashByObligation[row.RentObligationID], row)
		}
	}

	obligationsByRoom := make(map[uint64][]rentObligation)
	for _, row := range input.Obligations {
		if row.UserID != input.UserID || row.RecordStatus != obligationRecordActive || monthStart(row.PeriodMonth) != filters.PeriodMonth {
			continue
		}
		charge, ok := chargeByID[row.RentChargeID]
		if !ok || charge.RoomID == 0 || roomByID[charge.RoomID].ID == 0 {
			continue
		}
		obligationsByRoom[charge.RoomID] = append(obligationsByRoom[charge.RoomID], row)
	}

	roomExpenseCents, propertyExpenseCents := workspaceExpenseTotals(input, propertyByID, roomPropertyByID, filters.PeriodMonth)
	aggregates := make([]rentWorkspaceRoomAggregate, 0, len(roomByID))
	for _, row := range input.Rooms {
		roomRow, ok := roomByID[row.ID]
		if !ok {
			continue
		}
		propertyRow := propertyByID[roomRow.PropertyID]
		aggregate := rentWorkspaceRoomAggregate{
			Room:            roomRow,
			Property:        propertyRow,
			HasRentPlan:     planCoversMonthByRoom[roomRow.ID],
			HasPlanMembers:  roomHasPlanMembers[roomRow.ID],
			PlanMemberCount: planMemberCountByRoom[roomRow.ID],
			ExpenseCents:    roomExpenseCents[roomRow.ID],
		}
		charge, hasCharge := chargeByRoom[roomRow.ID]
		if hasCharge {
			chargeCopy := charge
			aggregate.Charge = &chargeCopy
		}
		for _, obligation := range obligationsByRoom[roomRow.ID] {
			projected := projectRentObligation(obligation, allocationsByObligation[obligation.ID], cashByObligation[obligation.ID], input.Now)
			currency := firstNonEmpty(projected.Currency, charge.Currency, ledgerCurrencyEUR)
			if !strings.EqualFold(currency, ledgerCurrencyEUR) {
				aggregate.Status = workspaceWorstStatus(aggregate.Status, "needs_review")
				continue
			}
			balance := maxInt64(projected.ExpectedAmountCents-projected.PaidAmountCents, 0)
			name := firstNonEmpty(stringValue(projected.TenantNameSnapshot), tenantByID[projected.TenantID].Name, "Unknown tenant")
			payments, paidByOther := workspacePayments(projected, allocationsByObligation[projected.ID], cashByObligation[projected.ID], transactionsByID)
			tenantRow := rentWorkspaceTenantRow{
				TenantID:          projected.TenantID,
				PropertyID:        propertyRow.ID,
				PropertyName:      propertyRow.Name,
				RoomID:            roomRow.ID,
				RoomLabel:         roomRow.RoomLabel,
				RoomAddress:       propertyAddress(propertyRow),
				TenantName:        name,
				TenantAlias:       tenantByID[projected.TenantID].DisplayAlias,
				ObligationID:      projected.ID,
				Period:            filters.PeriodMonth.Format("2006-01"),
				DueDate:           projected.DueDate.Format(dateLayout),
				DueDateValue:      projected.DueDate,
				ExpectedCents:     projected.ExpectedAmountCents,
				PaidCents:         projected.PaidAmountCents,
				BalanceCents:      balance,
				ExpectedAmount:    formatMoney(centsToMoney(projected.ExpectedAmountCents), currency, 2),
				PaidAmount:        formatMoney(centsToMoney(projected.PaidAmountCents), currency, 2),
				BalanceAmount:     formatMoney(centsToMoney(balance), currency, 2),
				CollectionPercent: collectionPercent(projected.ExpectedAmountCents, projected.PaidAmountCents),
				Status:            projected.Status,
				StatusLabel:       workspaceStatusLabel(projected.Status),
				PaidByOther:       paidByOther,
				Payments:          payments,
			}
			aggregate.Obligations = append(aggregate.Obligations, tenantRow)
			aggregate.ExpectedCents += projected.ExpectedAmountCents
			aggregate.PaidCents += projected.PaidAmountCents
			aggregate.BalanceCents += balance
			aggregate.Status = workspaceWorstStatus(aggregate.Status, projected.Status)
			if aggregate.DueDate.IsZero() || projected.DueDate.Before(aggregate.DueDate) {
				aggregate.DueDate = projected.DueDate
			}
		}
		if len(aggregate.Obligations) == 0 && filters.PeriodMonth.After(monthStart(input.Now)) && !hasCharge && aggregate.HasRentPlan && aggregate.HasPlanMembers {
			roomPlans := plansByRoom[roomRow.ID]
			if len(roomPlans) != 1 {
				aggregate.Status = "needs_review"
			} else {
				roomPlan := roomPlans[0]
				plan, err := buildRentChargePlan(roomPlan, membersByPlan[roomPlan.ID], filters.PeriodMonth)
				if err != nil {
					aggregate.Status = "needs_review"
				} else {
					validPreview := true
					for _, responsibility := range plan.Responsibilities {
						tenantRow, exists := tenantByID[responsibility.TenantID]
						if !exists {
							validPreview = false
							break
						}
						name := firstNonEmpty(tenantRow.Name, "Unknown tenant")
						dueDate := dueDateForMonth(filters.PeriodMonth, roomPlan.DueDay)
						amount := formatMoney(centsToMoney(responsibility.AmountCents), plan.Currency, 2)
						aggregate.Obligations = append(aggregate.Obligations, rentWorkspaceTenantRow{
							TenantID: responsibility.TenantID, PropertyID: propertyRow.ID, PropertyName: propertyRow.Name,
							RoomID: roomRow.ID, RoomLabel: roomRow.RoomLabel, RoomAddress: propertyAddress(propertyRow),
							TenantName: name, TenantAlias: tenantRow.DisplayAlias, Period: filters.PeriodMonth.Format("2006-01"),
							DueDate: dueDate.Format(dateLayout), DueDateValue: dueDate,
							ExpectedCents: responsibility.AmountCents, ExpectedAmount: amount, PaidAmount: "—", BalanceAmount: "—",
							Status: "forecast", StatusLabel: workspaceStatusLabel("forecast"), IsForecast: true,
						})
					}
					if validPreview {
						aggregate.IsForecast = true
						aggregate.Status = "forecast"
						aggregate.DueDate = dueDateForMonth(filters.PeriodMonth, roomPlan.DueDay)
						for _, row := range aggregate.Obligations {
							if row.IsForecast {
								aggregate.ForecastCents += row.ExpectedCents
							}
						}
					} else {
						aggregate.Obligations = nil
						aggregate.Status = "needs_review"
					}
				}
			}
		}
		if len(aggregate.Obligations) == 0 {
			if aggregate.HasRentPlan && aggregate.HasPlanMembers {
				aggregate.Status = "needs_review"
			} else {
				aggregate.Status = "vacant"
			}
		}
		if aggregate.Status == "" {
			aggregate.Status = "needs_review"
		}
		aggregates = append(aggregates, aggregate)
	}

	propertyRows := make([]rentWorkspacePropertyRow, 0, len(propertyByID))
	roomRows := make([]rentWorkspaceRoomRow, 0, len(aggregates))
	tenantRows := make([]rentWorkspaceTenantRow, 0)
	propertyOptions := make([]rentWorkspacePropertyOption, 0, len(propertyByID))
	for _, propertyRow := range propertyByID {
		propertyOptions = append(propertyOptions, rentWorkspacePropertyOption{ID: propertyRow.ID, Name: propertyRow.Name})
	}
	sort.SliceStable(propertyOptions, func(i, j int) bool {
		if strings.ToLower(propertyOptions[i].Name) != strings.ToLower(propertyOptions[j].Name) {
			return strings.ToLower(propertyOptions[i].Name) < strings.ToLower(propertyOptions[j].Name)
		}
		return propertyOptions[i].ID < propertyOptions[j].ID
	})
	for _, aggregate := range aggregates {
		roomRows = append(roomRows, workspaceRoomRow(aggregate))
		tenantRows = append(tenantRows, aggregate.Obligations...)
	}
	for _, propertyRow := range propertyByID {
		row := rentWorkspacePropertyRow{PropertyID: propertyRow.ID, Name: propertyRow.Name, Address: stringValue(propertyRow.Address)}
		forecastRooms := 0
		actualRentRooms := 0
		for _, aggregate := range aggregates {
			if aggregate.Property.ID != propertyRow.ID || !workspaceScopeMatches(aggregate.Property.ID, aggregate.Room.ID, filters) {
				continue
			}
			row.TotalRooms++
			row.ExpectedCents += aggregate.ExpectedCents
			row.ForecastCents += aggregate.ForecastCents
			row.PaidCents += aggregate.PaidCents
			row.BalanceCents += aggregate.BalanceCents
			row.OtherIncomeCents += aggregate.OtherIncomeCents
			row.Status = workspaceWorstStatus(row.Status, aggregate.Status)
			if aggregate.Status == "vacant" {
				row.VacantRooms++
			} else if aggregate.ForecastCents > 0 {
				forecastRooms++
			} else if aggregate.ExpectedCents > 0 && aggregate.Status == "paid" {
				row.PaidRooms++
			} else if aggregate.ExpectedCents > 0 {
				row.UnpaidRooms++
			}
			if aggregate.ExpectedCents > 0 {
				actualRentRooms++
			}
		}
		row.ExpenseCents = propertyExpenseCents[propertyRow.ID]
		row.NetCents = row.PaidCents + row.OtherIncomeCents - row.ExpenseCents
		row.CollectionPercent = collectionPercent(row.ExpectedCents+row.ForecastCents, row.PaidCents)
		row.StatusLabel = workspaceStatusLabel(row.Status)
		if row.Status == "" {
			row.Status = "vacant"
			row.StatusLabel = workspaceStatusLabel(row.Status)
		}
		row.IsForecast = forecastRooms > 0 && actualRentRooms == 0 && row.Status == "forecast"
		row.ExpectedAmount = formatWorkspaceAmount(row.ExpectedCents + row.ForecastCents)
		row.ForecastAmount = formatWorkspaceAmount(row.ForecastCents)
		row.ProjectedBalanceAmount = formatWorkspaceAmount(maxInt64(row.ExpectedCents+row.ForecastCents-row.PaidCents, 0))
		if row.IsForecast {
			row.PaidAmount = "—"
			row.BalanceAmount = "—"
		}
		propertyRows = append(propertyRows, row)
	}

	summary := rentWorkspaceSummary{}
	for _, row := range propertyRows {
		summary.ExpectedCents += row.ExpectedCents
		summary.ForecastCents += row.ForecastCents
		summary.PaidCents += row.PaidCents
		summary.BalanceCents += row.BalanceCents
		summary.ExpenseCents += row.ExpenseCents
		summary.OtherIncomeCents += row.OtherIncomeCents
		summary.TotalRooms += row.TotalRooms
		summary.PaidRooms += row.PaidRooms
		summary.UnpaidRooms += row.UnpaidRooms
		summary.VacantRooms += row.VacantRooms
	}
	for _, tenantRow := range tenantRows {
		if tenantRow.IsForecast {
			summary.ForecastCount++
		} else {
			summary.ResponsibilityCount++
		}
		if !tenantRow.IsForecast && tenantRow.BalanceCents > 0 && tenantRow.Status != "needs_review" {
			summary.FollowupCount++
		}
	}
	summary.NetCents = summary.PaidCents + summary.OtherIncomeCents - summary.ExpenseCents
	summary.CollectionPercent = collectionPercent(summary.ExpectedCents+summary.ForecastCents, summary.PaidCents)
	summary.ExpectedAmount = formatWorkspaceAmount(summary.ExpectedCents)
	summary.ForecastAmount = formatWorkspaceAmount(summary.ForecastCents)
	summary.ForecastExpectedAmount = formatWorkspaceAmount(summary.ExpectedCents + summary.ForecastCents)
	summary.PaidAmount = formatWorkspaceAmount(summary.PaidCents)
	summary.BalanceAmount = formatWorkspaceAmount(summary.BalanceCents)
	summary.ProjectedBalanceAmount = formatWorkspaceAmount(maxInt64(summary.ExpectedCents+summary.ForecastCents-summary.PaidCents, 0))
	summary.ExpenseAmount = formatWorkspaceAmount(summary.ExpenseCents)
	summary.OtherIncomeAmount = formatWorkspaceAmount(summary.OtherIncomeCents)
	summary.NetAmount = formatWorkspaceAmount(summary.NetCents)
	for index := range propertyRows {
		propertyRows[index].ExpectedAmount = formatWorkspaceAmount(propertyRows[index].ExpectedCents + propertyRows[index].ForecastCents)
		propertyRows[index].ForecastAmount = formatWorkspaceAmount(propertyRows[index].ForecastCents)
		propertyRows[index].ProjectedBalanceAmount = formatWorkspaceAmount(maxInt64(propertyRows[index].ExpectedCents+propertyRows[index].ForecastCents-propertyRows[index].PaidCents, 0))
		if propertyRows[index].IsForecast {
			propertyRows[index].PaidAmount = "—"
			propertyRows[index].BalanceAmount = "—"
		} else {
			propertyRows[index].PaidAmount = formatWorkspaceAmount(propertyRows[index].PaidCents)
			propertyRows[index].BalanceAmount = formatWorkspaceAmount(propertyRows[index].BalanceCents)
		}
		propertyRows[index].ExpenseAmount = formatWorkspaceAmount(propertyRows[index].ExpenseCents)
		propertyRows[index].OtherIncomeAmount = formatWorkspaceAmount(propertyRows[index].OtherIncomeCents)
		propertyRows[index].NetAmount = formatWorkspaceAmount(propertyRows[index].NetCents)
	}

	propertyTotalRows := len(propertyRows)
	roomTotalRows := len(roomRows)
	tenantTotalRows := len(tenantRows)
	allRoomRows := append([]rentWorkspaceRoomRow(nil), roomRows...)
	allTenantRows := append([]rentWorkspaceTenantRow(nil), tenantRows...)
	propertyRows = filterAndSortWorkspaceProperties(propertyRows, filters)
	roomRows = filterAndSortWorkspaceRooms(roomRows, filters)
	tenantRows = filterAndSortWorkspaceTenants(tenantRows, filters)
	propertyFilteredCount, roomFilteredCount, tenantFilteredCount := len(propertyRows), len(roomRows), len(tenantRows)
	// Chips are read off the filtered-but-not-yet-paginated slice, so they describe
	// the same set of rows the table below them is showing.
	dimensionSummary := workspaceDimensionSummaryForView(filters.View, propertyRows, roomRows, tenantRows)
	propertyRows, propertyPages := paginateWorkspaceProperties(propertyRows, filters.Page, filters.PageSize)
	roomRows, roomPages := paginateWorkspaceRooms(roomRows, filters.Page, filters.PageSize)
	tenantRows, tenantPages := paginateWorkspaceTenants(tenantRows, filters.Page, filters.PageSize)
	roomTreeRows := workspaceRoomTreeRows(roomRows, allTenantRows)
	propertyTreeRows := workspacePropertyTreeRows(propertyRows, allRoomRows, allTenantRows, filters)
	totalRows, filteredCount, totalPages := workspacePageCounts(filters.View, propertyPages, roomPages, tenantPages, propertyTotalRows, roomTotalRows, tenantTotalRows, propertyFilteredCount, roomFilteredCount, tenantFilteredCount)
	return rentWorkspaceData{
		Filters:          filters,
		IsFuturePeriod:   filters.PeriodMonth.After(monthStart(input.Now)),
		Summary:          summary,
		DimensionSummary: dimensionSummary,
		PropertyOptions:  propertyOptions,
		PropertyRows:     propertyRows,
		PropertyTreeRows: propertyTreeRows,
		RoomRows:         roomRows,
		RoomTreeRows:     roomTreeRows,
		TenantRows:       tenantRows,
		TotalRows:        totalRows,
		FilteredCount:    filteredCount,
		TotalPages:       totalPages,
		Page:             filters.Page,
		PageSize:         filters.PageSize,
	}, nil
}

func workspaceRoomTreeRows(rooms []rentWorkspaceRoomRow, tenants []rentWorkspaceTenantRow) []rentWorkspaceRoomTreeRow {
	type workspaceRoomKey struct {
		propertyID uint64
		roomID     uint64
	}
	tenantsByRoom := make(map[workspaceRoomKey][]rentWorkspaceTenantRow, len(rooms))
	for _, tenantRow := range tenants {
		key := workspaceRoomKey{propertyID: tenantRow.PropertyID, roomID: tenantRow.RoomID}
		tenantsByRoom[key] = append(tenantsByRoom[key], tenantRow)
	}
	tree := make([]rentWorkspaceRoomTreeRow, 0, len(rooms))
	for _, roomRow := range rooms {
		key := workspaceRoomKey{propertyID: roomRow.PropertyID, roomID: roomRow.RoomID}
		children := append([]rentWorkspaceTenantRow(nil), tenantsByRoom[key]...)
		sort.SliceStable(children, func(i, j int) bool {
			if strings.ToLower(children[i].TenantName) != strings.ToLower(children[j].TenantName) {
				return strings.ToLower(children[i].TenantName) < strings.ToLower(children[j].TenantName)
			}
			return children[i].ObligationID < children[j].ObligationID
		})
		tree = append(tree, rentWorkspaceRoomTreeRow{Room: roomRow, Tenants: children})
	}
	return tree
}

func workspacePropertyTreeRows(properties []rentWorkspacePropertyRow, rooms []rentWorkspaceRoomRow, tenants []rentWorkspaceTenantRow, filters rentWorkspaceFilters) []rentWorkspacePropertyTreeRow {
	roomTreesByProperty := make(map[uint64][]rentWorkspaceRoomTreeRow)
	for _, roomTree := range workspaceRoomTreeRows(rooms, tenants) {
		if filters.RoomID != 0 && roomTree.Room.RoomID != filters.RoomID {
			continue
		}
		roomTreesByProperty[roomTree.Room.PropertyID] = append(roomTreesByProperty[roomTree.Room.PropertyID], roomTree)
	}
	tree := make([]rentWorkspacePropertyTreeRow, 0, len(properties))
	for _, propertyRow := range properties {
		propertyRooms := append([]rentWorkspaceRoomTreeRow(nil), roomTreesByProperty[propertyRow.PropertyID]...)
		sort.SliceStable(propertyRooms, func(i, j int) bool {
			if strings.ToLower(propertyRooms[i].Room.RoomLabel) != strings.ToLower(propertyRooms[j].Room.RoomLabel) {
				return strings.ToLower(propertyRooms[i].Room.RoomLabel) < strings.ToLower(propertyRooms[j].Room.RoomLabel)
			}
			return propertyRooms[i].Room.RoomID < propertyRooms[j].Room.RoomID
		})
		tree = append(tree, rentWorkspacePropertyTreeRow{
			Property: propertyRow,
			Rooms:    propertyRooms,
		})
	}
	return tree
}

func workspaceScopeMatches(propertyID, roomID uint64, filters rentWorkspaceFilters) bool {
	if filters.PropertyID != 0 && propertyID != filters.PropertyID {
		return false
	}
	if filters.RoomID != 0 && roomID != filters.RoomID {
		return false
	}
	return true
}

func propertyAddress(propertyRow property) string {
	if propertyRow.Address != nil {
		return *propertyRow.Address
	}
	return ""
}

func workspaceExpenseTotals(input rentWorkspaceInput, properties map[uint64]property, roomPropertyByID map[uint64]uint64, period time.Time) (map[uint64]int64, map[uint64]int64) {
	roomTotals := make(map[uint64]int64)
	propertyTotals := make(map[uint64]int64)
	start := monthStart(period)
	end := start.AddDate(0, 1, 0)
	for _, expense := range input.Expenses {
		if expense.UserID != input.UserID || expense.RecordStatus == obligationRecordVoided || expense.ExpenseDate.Before(start) || !expense.ExpenseDate.Before(end) || !strings.EqualFold(strings.TrimSpace(expense.Currency), ledgerCurrencyEUR) {
			continue
		}
		propertyID := uint64(0)
		if expense.PropertyID != nil && properties[*expense.PropertyID].ID != 0 {
			propertyID = *expense.PropertyID
		}
		if expense.RoomID != nil {
			if inferred := roomPropertyByID[*expense.RoomID]; inferred != 0 {
				roomTotals[*expense.RoomID] += expense.AmountCents
				if propertyID == 0 {
					propertyID = inferred
				}
			}
		}
		if propertyID != 0 {
			propertyTotals[propertyID] += expense.AmountCents
		}
	}
	return roomTotals, propertyTotals
}

func workspacePayments(obligation rentObligation, allocations []paymentAllocation, receipts []cashReceipt, transactions map[uint64]paymentTransaction) ([]rentPaymentDetail, bool) {
	rows := make([]rentPaymentDetailRow, 0, len(allocations)+len(receipts))
	paidByOther := false
	for _, allocation := range allocations {
		if allocation.TenantID != nil && *allocation.TenantID != obligation.TenantID {
			continue
		}
		transaction, ok := transactions[allocation.PaymentTransactionID]
		if !ok || transaction.Direction != "income" {
			continue
		}
		if transaction.MatchedTenantID != nil && *transaction.MatchedTenantID != obligation.TenantID {
			paidByOther = true
		}
		rows = append(rows, rentPaymentDetailRow{PaymentID: transaction.ID, AmountCents: allocation.AmountCents, Currency: transaction.Currency, Source: transaction.Source, TransactionTime: transaction.TransactionTime, Description: transaction.Description, Reference: transaction.Reference, ConfirmationSource: allocation.ConfirmationSource})
	}
	for _, receipt := range receipts {
		if !cashReceiptIsEffective(receipt) {
			continue
		}
		rows = append(rows, rentPaymentDetailRow{PaymentID: receipt.ID, AmountCents: receipt.AmountCents, Currency: receipt.Currency, Source: "cash", TransactionTime: &receipt.ReceivedAt, Description: "现金租金补录", Reference: receipt.ReceiptNumber, ConfirmationSource: "manual_cash"})
	}
	sortRentPaymentDetailRows(rows)
	payments := make([]rentPaymentDetail, 0, len(rows))
	for _, row := range rows {
		payments = append(payments, rentPaymentDetailFromRow(row))
	}
	return payments, paidByOther
}

func workspaceRoomRow(aggregate rentWorkspaceRoomAggregate) rentWorkspaceRoomRow {
	row := rentWorkspaceRoomRow{
		RoomID:                 aggregate.Room.ID,
		PropertyID:             aggregate.Property.ID,
		PropertyName:           aggregate.Property.Name,
		RoomLabel:              aggregate.Room.RoomLabel,
		Address:                propertyAddress(aggregate.Property),
		Status:                 aggregate.Status,
		StatusLabel:            workspaceStatusLabel(aggregate.Status),
		TenantCount:            maxInt(aggregate.PlanMemberCount, len(aggregate.Obligations)),
		ExpectedCents:          aggregate.ExpectedCents,
		ForecastCents:          aggregate.ForecastCents,
		PaidCents:              aggregate.PaidCents,
		BalanceCents:           aggregate.BalanceCents,
		ExpectedAmount:         formatWorkspaceAmount(aggregate.ExpectedCents + aggregate.ForecastCents),
		ForecastAmount:         formatWorkspaceAmount(aggregate.ForecastCents),
		ProjectedBalanceAmount: formatWorkspaceAmount(maxInt64(aggregate.ExpectedCents+aggregate.ForecastCents-aggregate.PaidCents, 0)),
		IsForecast:             aggregate.IsForecast,
		PaidAmount:             formatWorkspaceAmount(aggregate.PaidCents),
		BalanceAmount:          formatWorkspaceAmount(aggregate.BalanceCents),
		HasCharge:              aggregate.Charge != nil,
		CollectionPercent:      collectionPercent(aggregate.ExpectedCents+aggregate.ForecastCents, aggregate.PaidCents),
	}
	if aggregate.IsForecast {
		row.PaidAmount = "—"
		row.BalanceAmount = "—"
		row.CollectionPercent = 0
	}
	if aggregate.ExpectedCents > 0 || aggregate.ForecastCents > 0 {
		row.DueDateValue = aggregate.DueDate
		row.DueDate = aggregate.DueDate.Format(dateLayout)
	}
	if aggregate.IsForecast {
		row.PaidAmount = "—"
		row.BalanceAmount = "—"
		row.CollectionPercent = 0
	}
	if aggregate.Status == "vacant" {
		row.ExpectedAmount = "—"
		row.PaidAmount = "—"
		row.BalanceAmount = "—"
		row.DueDate = "—"
	}
	return row
}

func formatWorkspaceAmount(cents int64) string {
	return formatMoney(centsToMoney(cents), ledgerCurrencyEUR, 2)
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func collectionPercent(expected, paid int64) int {
	if expected <= 0 {
		return 0
	}
	percent := int(paid * 100 / expected)
	if percent > 100 {
		return 100
	}
	if percent < 0 {
		return 0
	}
	return percent
}

// workspaceDimensionSummaryForView counts the active view's own rows. "Total"
// counts every row the table is about to render (空置 / 未到期 included), while
// the three buckets split them by the workspace status the row already carries,
// so 逾期未缴 + 部分缴纳 + 已缴满 never exceeds the total and every bucket maps
// one-to-one onto an option of the status filter next to it.
func workspaceDimensionSummaryForView(view string, properties []rentWorkspacePropertyRow, rooms []rentWorkspaceRoomRow, tenants []rentWorkspaceTenantRow) rentWorkspaceDimensionSummary {
	summary := rentWorkspaceDimensionSummary{}
	switch view {
	case rentWorkspaceViewRooms:
		for _, row := range rooms {
			summary.Total++
			summary.addStatus(row.Status)
		}
	case rentWorkspaceViewTenants:
		for _, row := range tenants {
			summary.Total++
			summary.addStatus(row.Status)
		}
	default:
		for _, row := range properties {
			summary.Total++
			summary.addStatus(row.Status)
		}
	}
	return summary
}

func (summary *rentWorkspaceDimensionSummary) addStatus(status string) {
	switch status {
	case "overdue":
		summary.Overdue++
	case "partial":
		summary.Partial++
	case "paid":
		summary.Paid++
	case "forecast":
		summary.Forecast++
	}
}

var workspaceStatusPriority = map[string]int{
	"needs_review": 0,
	"overdue":      1,
	"partial":      2,
	"open":         3,
	"paid":         4,
	"forecast":     5,
	"vacant":       6,
}

func workspaceWorstStatus(current, candidate string) string {
	if current == "" {
		return candidate
	}
	if workspaceStatusPriority[candidate] < workspaceStatusPriority[current] {
		return candidate
	}
	return current
}

func workspaceStatusLabel(status string) string {
	if status == "vacant" {
		return "空置"
	}
	if status == "needs_review" {
		return "待处理"
	}
	if status == "forecast" {
		return "预计"
	}
	return rentStatusLabel(status)
}

func workspaceStatusMatches(status, filter string) bool {
	if filter == "all" || filter == "" {
		return true
	}
	if filter == "outstanding" {
		return status == "overdue" || status == "partial" || status == "open"
	}
	return status == filter
}

func workspaceTextMatches(search string, values ...string) bool {
	search = strings.ToLower(strings.TrimSpace(search))
	if search == "" {
		return true
	}
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), search) {
			return true
		}
	}
	return false
}

func filterAndSortWorkspaceProperties(rows []rentWorkspacePropertyRow, filters rentWorkspaceFilters) []rentWorkspacePropertyRow {
	filtered := make([]rentWorkspacePropertyRow, 0, len(rows))
	for _, row := range rows {
		if filters.PropertyID != 0 && row.PropertyID != filters.PropertyID {
			continue
		}
		if !workspaceStatusMatches(row.Status, filters.Status) || !workspaceTextMatches(filters.Search, row.Name, row.Address) {
			continue
		}
		filtered = append(filtered, row)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		left, right := filtered[i], filtered[j]
		if filters.Sort == "name_asc" && left.Name != right.Name {
			return strings.ToLower(left.Name) < strings.ToLower(right.Name)
		}
		if filters.Sort == "name_desc" && left.Name != right.Name {
			return strings.ToLower(left.Name) > strings.ToLower(right.Name)
		}
		if filters.Sort == "balance_desc" && left.BalanceCents != right.BalanceCents {
			return left.BalanceCents > right.BalanceCents
		}
		if filters.Sort == "balance_asc" && left.BalanceCents != right.BalanceCents {
			return left.BalanceCents < right.BalanceCents
		}
		if workspaceStatusPriority[left.Status] != workspaceStatusPriority[right.Status] {
			return workspaceStatusPriority[left.Status] < workspaceStatusPriority[right.Status]
		}
		if left.BalanceCents != right.BalanceCents {
			return left.BalanceCents > right.BalanceCents
		}
		if left.ExpectedCents+left.ForecastCents != right.ExpectedCents+right.ForecastCents {
			return left.ExpectedCents+left.ForecastCents > right.ExpectedCents+right.ForecastCents
		}
		if strings.ToLower(left.Name) != strings.ToLower(right.Name) {
			return strings.ToLower(left.Name) < strings.ToLower(right.Name)
		}
		return left.PropertyID < right.PropertyID
	})
	return filtered
}

func filterAndSortWorkspaceRooms(rows []rentWorkspaceRoomRow, filters rentWorkspaceFilters) []rentWorkspaceRoomRow {
	filtered := make([]rentWorkspaceRoomRow, 0, len(rows))
	for _, row := range rows {
		if filters.PropertyID != 0 && row.PropertyID != filters.PropertyID || filters.RoomID != 0 && row.RoomID != filters.RoomID {
			continue
		}
		if !workspaceStatusMatches(row.Status, filters.Status) || !workspaceTextMatches(filters.Search, row.PropertyName, row.RoomLabel, row.Address) {
			continue
		}
		filtered = append(filtered, row)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		left, right := filtered[i], filtered[j]
		if filters.Sort == "name_asc" || filters.Sort == "name_desc" {
			leftName := strings.ToLower(left.PropertyName + " " + left.RoomLabel)
			rightName := strings.ToLower(right.PropertyName + " " + right.RoomLabel)
			if leftName != rightName {
				if filters.Sort == "name_desc" {
					return leftName > rightName
				}
				return leftName < rightName
			}
		}
		if filters.Sort == "balance_desc" && left.BalanceCents != right.BalanceCents {
			return left.BalanceCents > right.BalanceCents
		}
		if filters.Sort == "balance_asc" && left.BalanceCents != right.BalanceCents {
			return left.BalanceCents < right.BalanceCents
		}
		if workspaceStatusPriority[left.Status] != workspaceStatusPriority[right.Status] {
			return workspaceStatusPriority[left.Status] < workspaceStatusPriority[right.Status]
		}
		if left.BalanceCents != right.BalanceCents {
			return left.BalanceCents > right.BalanceCents
		}
		if left.ExpectedCents+left.ForecastCents != right.ExpectedCents+right.ForecastCents {
			return left.ExpectedCents+left.ForecastCents > right.ExpectedCents+right.ForecastCents
		}
		if left.DueDateValue.IsZero() != right.DueDateValue.IsZero() {
			return !left.DueDateValue.IsZero()
		}
		if !left.DueDateValue.Equal(right.DueDateValue) {
			return left.DueDateValue.Before(right.DueDateValue)
		}
		if strings.ToLower(left.PropertyName) != strings.ToLower(right.PropertyName) {
			return strings.ToLower(left.PropertyName) < strings.ToLower(right.PropertyName)
		}
		if strings.ToLower(left.RoomLabel) != strings.ToLower(right.RoomLabel) {
			return strings.ToLower(left.RoomLabel) < strings.ToLower(right.RoomLabel)
		}
		return left.RoomID < right.RoomID
	})
	return filtered
}

func filterAndSortWorkspaceTenants(rows []rentWorkspaceTenantRow, filters rentWorkspaceFilters) []rentWorkspaceTenantRow {
	filtered := make([]rentWorkspaceTenantRow, 0, len(rows))
	for _, row := range rows {
		if filters.PropertyID != 0 && row.PropertyID != filters.PropertyID || filters.RoomID != 0 && row.RoomID != filters.RoomID {
			continue
		}
		if !workspaceStatusMatches(row.Status, filters.Status) || !workspaceTextMatches(filters.Search, row.TenantName, row.TenantAlias, row.PropertyName, row.RoomLabel, row.RoomAddress) {
			continue
		}
		filtered = append(filtered, row)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		left, right := filtered[i], filtered[j]
		if filters.Sort == "name_asc" || filters.Sort == "name_desc" {
			leftName := strings.ToLower(left.TenantName)
			rightName := strings.ToLower(right.TenantName)
			if leftName != rightName {
				if filters.Sort == "name_desc" {
					return leftName > rightName
				}
				return leftName < rightName
			}
		}
		if filters.Sort == "balance_desc" && left.BalanceCents != right.BalanceCents {
			return left.BalanceCents > right.BalanceCents
		}
		if filters.Sort == "balance_asc" && left.BalanceCents != right.BalanceCents {
			return left.BalanceCents < right.BalanceCents
		}
		if filters.Sort == "due_asc" && !left.DueDateValue.Equal(right.DueDateValue) {
			return left.DueDateValue.Before(right.DueDateValue)
		}
		if filters.Sort == "due_desc" && !left.DueDateValue.Equal(right.DueDateValue) {
			return left.DueDateValue.After(right.DueDateValue)
		}
		if workspaceStatusPriority[left.Status] != workspaceStatusPriority[right.Status] {
			return workspaceStatusPriority[left.Status] < workspaceStatusPriority[right.Status]
		}
		if left.BalanceCents != right.BalanceCents {
			return left.BalanceCents > right.BalanceCents
		}
		if left.ExpectedCents != right.ExpectedCents {
			return left.ExpectedCents > right.ExpectedCents
		}
		if strings.ToLower(left.TenantName) != strings.ToLower(right.TenantName) {
			return strings.ToLower(left.TenantName) < strings.ToLower(right.TenantName)
		}
		if left.RoomID != right.RoomID {
			return left.RoomID < right.RoomID
		}
		return left.ObligationID < right.ObligationID
	})
	return filtered
}

func paginateWorkspaceProperties(rows []rentWorkspacePropertyRow, page, pageSize int) ([]rentWorkspacePropertyRow, int) {
	start, end, total := workspacePageBounds(len(rows), page, pageSize)
	return rows[start:end], total
}

func paginateWorkspaceRooms(rows []rentWorkspaceRoomRow, page, pageSize int) ([]rentWorkspaceRoomRow, int) {
	start, end, total := workspacePageBounds(len(rows), page, pageSize)
	return rows[start:end], total
}

func paginateWorkspaceTenants(rows []rentWorkspaceTenantRow, page, pageSize int) ([]rentWorkspaceTenantRow, int) {
	start, end, total := workspacePageBounds(len(rows), page, pageSize)
	return rows[start:end], total
}

func workspacePageBounds(length, page, pageSize int) (int, int, int) {
	if page < 1 || pageSize < 1 || length == 0 {
		return 0, 0, 0
	}
	totalPages := (length + pageSize - 1) / pageSize
	if page > totalPages {
		return 0, 0, totalPages
	}
	start := (page - 1) * pageSize
	end := start + pageSize
	if end > length {
		end = length
	}
	return start, end, totalPages
}

func workspacePageCounts(view string, propertyPages, roomPages, tenantPages, propertyRows, roomRows, tenantRows, propertyFilteredCount, roomFilteredCount, tenantFilteredCount int) (int, int, int) {
	switch view {
	case rentWorkspaceViewRooms:
		return roomRows, roomFilteredCount, roomPages
	case rentWorkspaceViewTenants:
		return tenantRows, tenantFilteredCount, tenantPages
	default:
		return propertyRows, propertyFilteredCount, propertyPages
	}
}

type rentWorkspaceService struct {
	db *gorm.DB
}

func newRentWorkspaceService(db *gorm.DB) *rentWorkspaceService {
	return &rentWorkspaceService{db: db}
}

func (s *rentWorkspaceService) load(ctx context.Context, userID uint64, filters rentWorkspaceFilters) (rentWorkspaceData, error) {
	if userID == 0 {
		return rentWorkspaceData{}, errors.New("workspace userID is required")
	}
	if s == nil || s.db == nil {
		return rentWorkspaceData{}, errors.New("workspace database is required")
	}
	if filters.PeriodMonth.IsZero() {
		filters.PeriodMonth = monthStart(time.Now().UTC())
	}
	filters.PeriodMonth = monthStart(filters.PeriodMonth)
	if err := validateRentWorkspaceFilters(filters); err != nil {
		return rentWorkspaceData{}, err
	}
	if filters.PropertyID != 0 {
		var propertyCount int64
		if err := s.db.WithContext(ctx).Model(&property{}).Where("user_id = ? AND id = ?", userID, filters.PropertyID).Count(&propertyCount).Error; err != nil {
			return rentWorkspaceData{}, err
		}
		if propertyCount == 0 {
			return rentWorkspaceData{}, gorm.ErrRecordNotFound
		}
	}
	if filters.RoomID != 0 {
		var roomCount int64
		if err := s.db.WithContext(ctx).Model(&room{}).Where("user_id = ? AND id = ?", userID, filters.RoomID).Count(&roomCount).Error; err != nil {
			return rentWorkspaceData{}, err
		}
		if roomCount == 0 {
			return rentWorkspaceData{}, gorm.ErrRecordNotFound
		}
	}
	// A started month uses the shared persisted rent facts. Future months stay
	// unmaterialized here and are rendered as arrangement-based forecasts below.
	if err := newMonthlyRentFactsService(s.db).ensureMonthlyRentFacts(ctx, userID, filters.PeriodMonth, rentFactsIntentRead); err != nil {
		return rentWorkspaceData{}, err
	}
	input := rentWorkspaceInput{UserID: userID, PeriodMonth: filters.PeriodMonth, Now: time.Now().UTC()}
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).Order("name ASC, id ASC").Find(&input.Properties).Error; err != nil {
		return rentWorkspaceData{}, err
	}
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).Order("property_id ASC, room_label ASC, id ASC").Find(&input.Rooms).Error; err != nil {
		return rentWorkspaceData{}, err
	}
	if err := s.db.WithContext(ctx).Where("user_id = ? AND effective_from_month <= ? AND (effective_to_month IS NULL OR effective_to_month >= ?)", userID, filters.PeriodMonth, filters.PeriodMonth).Find(&input.Plans).Error; err != nil {
		return rentWorkspaceData{}, err
	}
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).Find(&input.Parties).Error; err != nil {
		return rentWorkspaceData{}, err
	}
	if err := s.db.WithContext(ctx).Where("user_id = ? AND period_month = ? AND record_status = ?", userID, filters.PeriodMonth, obligationRecordActive).Find(&input.Charges).Error; err != nil {
		return rentWorkspaceData{}, err
	}
	if err := s.db.WithContext(ctx).Where("user_id = ? AND period_month = ? AND record_status = ?", userID, filters.PeriodMonth, obligationRecordActive).Find(&input.Obligations).Error; err != nil {
		return rentWorkspaceData{}, err
	}
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).Find(&input.Tenants).Error; err != nil {
		return rentWorkspaceData{}, err
	}
	if err := s.db.WithContext(ctx).Where("user_id = ? AND expense_date >= ? AND expense_date < ? AND record_status = ?", userID, filters.PeriodMonth, filters.PeriodMonth.AddDate(0, 1, 0), obligationRecordActive).Find(&input.Expenses).Error; err != nil {
		return rentWorkspaceData{}, err
	}
	obligationIDs := make([]uint64, 0, len(input.Obligations))
	for _, row := range input.Obligations {
		obligationIDs = append(obligationIDs, row.ID)
	}
	if len(obligationIDs) > 0 {
		if err := s.db.WithContext(ctx).Where("user_id = ? AND rent_obligation_id IN ?", userID, obligationIDs).Find(&input.Allocations).Error; err != nil {
			return rentWorkspaceData{}, err
		}
		if err := s.db.WithContext(ctx).Where("user_id = ? AND rent_obligation_id IN ?", userID, obligationIDs).Find(&input.CashReceipts).Error; err != nil {
			return rentWorkspaceData{}, err
		}
		transactionIDs := make([]uint64, 0, len(input.Allocations))
		seen := make(map[uint64]struct{})
		for _, row := range input.Allocations {
			if _, ok := seen[row.PaymentTransactionID]; !ok {
				seen[row.PaymentTransactionID] = struct{}{}
				transactionIDs = append(transactionIDs, row.PaymentTransactionID)
			}
		}
		if len(transactionIDs) > 0 {
			if err := s.db.WithContext(ctx).Where("user_id = ? AND id IN ?", userID, transactionIDs).Find(&input.Transactions).Error; err != nil {
				return rentWorkspaceData{}, err
			}
		}
	}
	return buildRentWorkspace(input, filters)
}
