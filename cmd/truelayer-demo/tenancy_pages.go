package main

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

type tenancyRoomOption struct {
	ID           uint64
	PropertyID   uint64
	PropertyName string
	RoomLabel    string
	Label        string
}

type tenancyTenantOption struct {
	ID   uint64
	Name string
}

type tenancyFormData struct {
	RoomID       uint64
	Period       string
	StartDate    string
	ContractDate string
	MoveInDate   string
	EndDate      string
	MonthlyRent  string
	DueDay       int
	TenantIDs    map[uint64]bool
	Shares       map[uint64]string
}

func (a *app) serveTenanciesPage(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		a.createTenancyFromPage(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := a.scopedPageUser(w, r)
	if !ok {
		return
	}
	period, err := parsePeriodMonth(strings.TrimSpace(r.URL.Query().Get("period")))
	if err != nil {
		http.Error(w, "period is invalid", http.StatusBadRequest)
		return
	}
	statusFilter := firstNonEmpty(strings.TrimSpace(r.URL.Query().Get("status")), "all")
	if statusFilter != "all" && statusFilter != "active" && statusFilter != "ended" && statusFilter != "upcoming" {
		http.Error(w, "tenancy status filter is invalid", http.StatusBadRequest)
		return
	}
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	repo := newLandlordRentRepository(a.db)
	agreements, err := repo.listTenancyAgreements(r.Context(), userID, agreementQuery{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	rooms, err := repo.listRooms(r.Context(), userID, roomQuery{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	properties, err := repo.listProperties(r.Context(), userID, propertyQuery{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	parties, err := repo.listAgreementParties(r.Context(), userID, agreementPartyQuery{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var tenants []tenant
	if err := a.db.WithContext(r.Context()).Where("user_id = ?", userID).Order("name ASC, id ASC").Find(&tenants).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	roomByID := make(map[uint64]room, len(rooms))
	for _, row := range rooms {
		roomByID[row.ID] = row
	}
	propertyByID := make(map[uint64]property, len(properties))
	for _, row := range properties {
		propertyByID[row.ID] = row
	}
	tenantByID := make(map[uint64]tenant, len(tenants))
	for _, row := range tenants {
		tenantByID[row.ID] = row
	}
	partiesByAgreement := make(map[uint64][]tenancyPartyView)
	for _, party := range parties {
		tenantRow := tenantByID[party.TenantID]
		partiesByAgreement[party.AgreementID] = append(partiesByAgreement[party.AgreementID], tenancyPartyView{
			TenantID: party.TenantID, TenantName: firstNonEmpty(tenantRow.DisplayAlias, tenantRow.Name, "未绑定租客"),
			ResponsibilityCents: party.ResponsibilityCents, Responsibility: pageCurrencyAmount(party.ResponsibilityCents, "EUR"),
			JoinedAt: pageDate(party.JoinedAt), LeftAt: pageDate(party.LeftAt),
		})
	}
	rows := make([]tenancyPageRow, 0, len(agreements))
	for _, agreement := range agreements {
		roomRow, roomExists := roomByID[agreement.RoomID]
		propertyRow, propertyExists := propertyByID[roomRow.PropertyID]
		if !roomExists || !propertyExists {
			continue
		}
		status, statusLabel := tenancyStatusInMonth(agreement, period)
		if statusFilter != "all" && statusFilter != status {
			continue
		}
		row := tenancyPageRow{
			ID: agreement.ID, RecordLabel: fmt.Sprintf("LEASE-%06d", agreement.ID), RoomID: agreement.RoomID,
			RoomLabel: roomRow.RoomLabel, PropertyID: propertyRow.ID, PropertyName: propertyRow.Name,
			StartDate: agreement.StartDate.Format(dateLayout), EndDate: pageDate(agreement.EndDate),
			ContractDate: pageDate(agreement.ContractDate), MoveInDate: pageDate(agreement.MoveInDate),
			MonthlyRentCents: agreement.MonthlyRentCents,
			MonthlyRent:      pageCurrencyAmount(agreement.MonthlyRentCents, agreement.Currency), Currency: agreement.Currency,
			DueDay: agreement.DueDay, Status: status, StatusLabel: statusLabel, Parties: partiesByAgreement[agreement.ID],
		}
		if !tenancyRowMatches(row, search) {
			continue
		}
		rows = append(rows, row)
	}
	// The repository order is stable by room; leases are easier to scan newest first.
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].StartDate != rows[j].StartDate {
			return rows[i].StartDate > rows[j].StartDate
		}
		return rows[i].ID > rows[j].ID
	})
	tableRows := expandTenancyTableRows(rows)
	roomOptions := make([]tenancyRoomOption, 0, len(rooms))
	for _, row := range rooms {
		propertyRow := propertyByID[row.PropertyID]
		if row.Status != "active" || propertyRow.Status != "active" {
			continue
		}
		roomOptions = append(roomOptions, tenancyRoomOption{
			ID: row.ID, PropertyID: row.PropertyID, PropertyName: propertyRow.Name, RoomLabel: row.RoomLabel,
			Label: propertyRow.Name + " · " + row.RoomLabel,
		})
	}
	tenantOptions := make([]tenancyTenantOption, 0, len(tenants))
	for _, row := range tenants {
		if row.Status != "active" {
			continue
		}
		tenantOptions = append(tenantOptions, tenancyTenantOption{ID: row.ID, Name: firstNonEmpty(row.DisplayAlias, row.Name)})
	}
	formDate := time.Now().UTC()
	form := tenancyFormData{Period: monthStart(formDate).Format("2006-01"), StartDate: formDate.Format(dateLayout), ContractDate: formDate.Format(dateLayout), MoveInDate: formDate.Format(dateLayout), DueDay: 1, TenantIDs: map[uint64]bool{}, Shares: map[uint64]string{}}
	data := tenancyPageData{
		workspaceShell: canonicalPageShell(a, r, "tenancies", "租约管理"),
		Rows:           rows, TableRows: tableRows, Period: period.Format("2006-01"), StatusFilter: statusFilter, Search: search,
		ShowCreate: r.URL.Query().Get("add") == "1", Rooms: roomOptions, Tenants: tenantOptions,
		Form: form, Message: r.URL.Query().Get("message"), Error: r.URL.Query().Get("error"),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tenancyPageTemplate.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func expandTenancyTableRows(rows []tenancyPageRow) []tenancyPageRow {
	result := make([]tenancyPageRow, 0, len(rows))
	for _, row := range rows {
		if len(row.Parties) == 0 {
			row.Responsibility = row.MonthlyRent
			result = append(result, row)
			continue
		}
		for _, party := range row.Parties {
			partyRow := row
			partyRow.TenantID = party.TenantID
			partyRow.TenantName = party.TenantName
			partyRow.Responsibility = party.Responsibility
			partyRow.Parties = nil
			result = append(result, partyRow)
		}
	}
	return result
}

func tenancyStatusInMonth(agreement tenancyAgreement, period time.Time) (string, string) {
	start := monthStart(period)
	end := start.AddDate(0, 1, 0).Add(-time.Nanosecond)
	if agreement.StartDate.After(end) {
		return "upcoming", "待开始"
	}
	if agreement.EndDate != nil && agreement.EndDate.Before(start) {
		return "ended", "已结束"
	}
	if agreement.Status != "active" {
		return "ended", "已结束"
	}
	return "active", "执行中"
}

func tenancyRowMatches(row tenancyPageRow, search string) bool {
	needle := strings.ToLower(strings.TrimSpace(search))
	if needle == "" {
		return true
	}
	if strings.Contains(strings.ToLower(row.RecordLabel+" "+row.PropertyName+" "+row.RoomLabel), needle) {
		return true
	}
	for _, party := range row.Parties {
		if strings.Contains(strings.ToLower(party.TenantName), needle) {
			return true
		}
	}
	return false
}

func tenancyReturnURL(values url.Values, errorCode string) string {
	query := url.Values{}
	query.Set("period", validatedPeriodValue(values.Get("period")))
	status := firstNonEmpty(strings.TrimSpace(values.Get("status")), "all")
	if status != "all" && status != "active" && status != "ended" && status != "upcoming" {
		status = "all"
	}
	query.Set("status", status)
	if search := strings.TrimSpace(values.Get("search")); search != "" {
		query.Set("search", search)
	}
	if errorCode != "" {
		query.Set("add", "1")
		query.Set("error", errorCode)
	}
	return "/tenancies?" + query.Encode()
}

func parseTenancyArrangement(values url.Values) (rentArrangementInput, error) {
	roomID, err := parsePositiveUint(values.Get("room_id"))
	if err != nil {
		return rentArrangementInput{}, fmt.Errorf("room_id: %w", err)
	}
	effectiveMonth, err := parsePeriodMonth(strings.TrimSpace(values.Get("effective_month")))
	if err != nil {
		return rentArrangementInput{}, fmt.Errorf("effective_month: %w", err)
	}
	startDate, err := parseDate(values.Get("start_date"))
	if err != nil {
		return rentArrangementInput{}, fmt.Errorf("start_date: %w", err)
	}
	contractDate := startDate
	if raw := strings.TrimSpace(values.Get("contract_date")); raw != "" {
		contractDate, err = parseDate(raw)
		if err != nil {
			return rentArrangementInput{}, fmt.Errorf("contract_date: %w", err)
		}
	}
	moveInDate := startDate
	if raw := strings.TrimSpace(values.Get("move_in_date")); raw != "" {
		moveInDate, err = parseDate(raw)
		if err != nil {
			return rentArrangementInput{}, fmt.Errorf("move_in_date: %w", err)
		}
	}
	var endDate *time.Time
	if raw := strings.TrimSpace(values.Get("end_date")); raw != "" {
		parsed, parseErr := parseDate(raw)
		if parseErr != nil {
			return rentArrangementInput{}, fmt.Errorf("end_date: %w", parseErr)
		}
		endDate = &parsed
	}
	monthlyRent, err := parsePositiveAmount(values.Get("monthly_rent"))
	if err != nil {
		return rentArrangementInput{}, err
	}
	dueDay := 1
	if raw := strings.TrimSpace(values.Get("due_day")); raw != "" {
		dueDay, err = strconv.Atoi(raw)
		if err != nil || dueDay < 1 || dueDay > 31 {
			return rentArrangementInput{}, errors.New("due day must be between 1 and 31")
		}
	}
	tenantIDs := make([]uint64, 0)
	seen := make(map[uint64]bool)
	for _, raw := range values["tenant_ids"] {
		tenantID, parseErr := parsePositiveUint(raw)
		if parseErr != nil {
			return rentArrangementInput{}, fmt.Errorf("tenant_ids: %w", parseErr)
		}
		if !seen[tenantID] {
			seen[tenantID] = true
			tenantIDs = append(tenantIDs, tenantID)
		}
	}
	responsibilities := make([]rentResponsibilityInput, 0, len(tenantIDs))
	sharesProvided := false
	sharesComplete := true
	for _, tenantID := range tenantIDs {
		raw := strings.TrimSpace(values.Get("responsibility_" + strconv.FormatUint(tenantID, 10)))
		if raw == "" {
			sharesComplete = false
			continue
		}
		sharesProvided = true
		amount, parseErr := parsePositiveAmount(raw)
		if parseErr != nil {
			return rentArrangementInput{}, fmt.Errorf("tenant responsibility: %w", parseErr)
		}
		responsibilities = append(responsibilities, rentResponsibilityInput{TenantID: tenantID, AmountCents: moneyToCents(amount)})
	}
	if sharesProvided && !sharesComplete {
		return rentArrangementInput{}, errors.New("enter a responsibility for every selected tenant or leave all blank")
	}
	if !sharesProvided {
		responsibilities = nil
	}
	return rentArrangementInput{
		RoomID: roomID, EffectiveMonth: effectiveMonth, StartDate: startDate, ContractDate: &contractDate, MoveInDate: &moveInDate, EndDate: endDate,
		MonthlyRentCents: moneyToCents(monthlyRent), Currency: ledgerCurrencyEUR, DueDay: dueDay,
		TenantIDs: tenantIDs, Responsibilities: responsibilities,
	}, nil
}

func (a *app) createTenancyFromPage(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := a.currentUserID(r)
	if !ok || a.db == nil {
		http.Error(w, "database session required", http.StatusServiceUnavailable)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, tenancyReturnURL(r.Form, "lease_failed"), http.StatusFound)
		return
	}
	input, err := parseTenancyArrangement(r.Form)
	if err == nil && len(input.TenantIDs) == 0 {
		err = errors.New("at least one tenant is required")
	}
	if err == nil {
		_, err = newLandlordDomainService(a.db).saveRentArrangement(r.Context(), userID, input)
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		code := "lease_failed"
		if errors.Is(err, errArrangementHistoryLocked) {
			code = "lease_locked"
		}
		http.Redirect(w, r, tenancyReturnURL(r.Form, code), http.StatusFound)
		return
	}
	query := url.Values{}
	query.Set("period", validatedPeriodValue(r.Form.Get("period")))
	status := firstNonEmpty(r.Form.Get("status"), "all")
	if status != "all" && status != "active" && status != "ended" && status != "upcoming" {
		status = "all"
	}
	query.Set("status", status)
	if search := strings.TrimSpace(r.Form.Get("search")); search != "" {
		query.Set("search", search)
	}
	query.Set("message", "lease_saved")
	http.Redirect(w, r, "/tenancies?"+query.Encode(), http.StatusFound)
}
