package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type e2EReportWriter func(e2eReport) error

type e2EBusinessArtifacts struct {
	PropertyID    uint64
	RoomIDs       []uint64
	TenantIDs     []uint64
	PrimaryUserID uint64
}

var e2ePlanVersionPattern = regexp.MustCompile(`name="plan_version" value="([0-9]+)"`)

func runE2EBusinessScenarios(ctx context.Context, options e2eOptions, manifest e2eFixtureManifest, report *e2eReport, writeReport e2EReportWriter, artifacts *e2EBusinessArtifacts) error {
	if report == nil {
		return errors.New("E2E report is required")
	}
	if err := manifest.validate(); err != nil {
		return fmt.Errorf("validate E2E fixture manifest: %w", err)
	}
	client, err := newE2EHTTPClient(options.BaseURL)
	if err != nil {
		return fmt.Errorf("create primary E2E HTTP client: %w", err)
	}
	db, err := sql.Open("mysql", options.MySQLDSN)
	if err != nil {
		return fmt.Errorf("open E2E fixture database: %w", err)
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("connect E2E fixture database: %w", err)
	}
	var userID uint64
	if err := db.QueryRowContext(ctx, "SELECT id FROM users WHERE username = ? LIMIT 1", options.Username).Scan(&userID); err != nil || userID == 0 {
		return errors.New("run-scoped E2E account was not found in the allowlisted database")
	}
	if artifacts != nil {
		artifacts.PrimaryUserID = userID
	}
	report.Status = "running"
	if err := persistE2EProgress(report, writeReport); err != nil {
		return err
	}
	if err := recordE2EScenario(report, client.authenticationScenario(ctx, options.Username, options.Password), writeReport); err != nil {
		return err
	}
	if artifacts != nil {
		artifacts.TenantIDs = make([]uint64, 0, 4)
		artifacts.RoomIDs = make([]uint64, 0, 4)
	}

	propertyName := manifest.RunID + " Rosewood Court"
	propertyScenario := e2eScenarioReport{Name: "create-property-and-rooms", Status: "running", Steps: []e2eStepReport{}}
	propertyResponse, requestErr := client.do(ctx, http.MethodPost, "/properties", url.Values{
		"action": {"save"}, "name": {propertyName}, "city_region": {"Dublin 8"},
		"address": {manifest.RunID + " Rosewood Court, Dublin"}, "timezone": {"Europe/Dublin"},
		"notes": {manifest.RunID + " E2E"},
	})
	propertyStep := e2eHTTPStep(http.MethodPost, "/properties", map[string]any{"redirect_contains": "message=property_saved"}, propertyResponse, requestErr)
	if requestErr == nil {
		propertyStep.Passed = propertyResponse.StatusCode == http.StatusFound && strings.Contains(propertyResponse.Location, "message=property_saved")
		if !propertyStep.Passed {
			propertyStep.Error = "property form did not save successfully"
		}
	}
	propertyScenario.Steps = append(propertyScenario.Steps, propertyStep)
	propertyID, lookupErr := e2eFindPropertyID(ctx, db, userID, propertyName)
	propertyLookup := e2eStepReport{Method: "SQL", Path: "properties by user_id and exact run-scoped name", Expected: map[string]any{"one_row": true}, Actual: map[string]any{"id_found": propertyID > 0}, Passed: lookupErr == nil && propertyID > 0}
	if lookupErr != nil {
		propertyLookup.Error = "run-scoped property could not be read back"
	}
	propertyScenario.Steps = append(propertyScenario.Steps, propertyLookup)
	if !propertyStep.Passed || !propertyLookup.Passed {
		propertyScenario.Status, propertyScenario.Error = "failed", "property creation failed"
		if err := recordE2EScenario(report, propertyScenario, writeReport); err != nil {
			return err
		}
	}
	if artifacts != nil {
		artifacts.PropertyID = propertyID
	}

	tenants := []e2eTenantFixture{manifest.TenantA, manifest.TenantB, manifest.TenantC, manifest.TenantD}
	tenantIDs := make([]uint64, 0, len(tenants))
	tenantScenario := e2eScenarioReport{Name: "create-tenant-profiles", Status: "running", Steps: []e2eStepReport{}}
	for _, tenant := range tenants {
		response, requestErr := client.do(ctx, http.MethodPost, "/tenants", url.Values{
			"name": {tenant.Name}, "display_alias": {tenant.Alias}, "email": {tenant.Email},
			"payer_id": {tenant.PayerID}, "payer_name_hint": {tenant.PayerName}, "status": {"active"},
		})
		step := e2eHTTPStep(http.MethodPost, "/tenants", map[string]any{"redirect_contains": "message=tenant_added"}, response, requestErr)
		if requestErr == nil {
			step.Passed = response.StatusCode == http.StatusFound && strings.Contains(response.Location, "message=tenant_added")
			if !step.Passed {
				step.Error = "tenant profile form did not save successfully"
			}
		}
		tenantScenario.Steps = append(tenantScenario.Steps, step)
		id, lookupErr := e2eFindTenantID(ctx, db, userID, tenant.Name)
		lookup := e2eStepReport{Method: "SQL", Path: "tenants by user_id and exact run-scoped name", Expected: map[string]any{"one_row": true}, Actual: map[string]any{"id_found": id > 0}, Passed: lookupErr == nil && id > 0}
		if lookupErr != nil {
			lookup.Error = "run-scoped tenant could not be read back"
		}
		tenantScenario.Steps = append(tenantScenario.Steps, lookup)
		if !step.Passed || !lookup.Passed {
			tenantScenario.Status, tenantScenario.Error = "failed", "tenant profile creation failed"
			if err := recordE2EScenario(report, tenantScenario, writeReport); err != nil {
				return err
			}
		}
		tenantIDs = append(tenantIDs, id)
	}
	if artifacts != nil {
		artifacts.TenantIDs = append([]uint64(nil), tenantIDs...)
	}
	if err := recordE2EScenario(report, passedE2EScenario(tenantScenario), writeReport); err != nil {
		return err
	}

	roomSpecs := []struct {
		label, typ string
	}{
		{label: "01", typ: "双人间"},
		{label: "02", typ: "单人间"},
		{label: "03", typ: "单人间"},
		{label: "A", typ: "单人间"},
	}
	roomIDs := make([]uint64, 0, len(roomSpecs))
	roomScenario := e2eScenarioReport{Name: "create-physical-rooms", Status: "running", Steps: []e2eStepReport{}}
	for _, room := range roomSpecs {
		response, requestErr := client.do(ctx, http.MethodPost, "/rooms", url.Values{
			"action": {"save"}, "property_id": {strconv.FormatUint(propertyID, 10)}, "room_label": {room.label},
			"room_type": {room.typ}, "capacity": {"2"}, "period": {manifest.Period}, "notes": {manifest.RunID + " E2E"},
		})
		step := e2eHTTPStep(http.MethodPost, "/rooms", map[string]any{"status_code": http.StatusFound}, response, requestErr)
		if requestErr == nil {
			step.Passed = response.StatusCode == http.StatusFound && strings.Contains(response.Location, "room_saved")
			if !step.Passed {
				step.Error = "physical room form did not save successfully"
			}
		}
		roomScenario.Steps = append(roomScenario.Steps, step)
		id, lookupErr := e2eFindRoomID(ctx, db, userID, propertyID, room.label)
		lookup := e2eStepReport{Method: "SQL", Path: "rooms by user_id, property_id and exact label", Expected: map[string]any{"one_row": true}, Actual: map[string]any{"id_found": id > 0}, Passed: lookupErr == nil && id > 0}
		if lookupErr != nil {
			lookup.Error = "run-scoped room could not be read back"
		}
		roomScenario.Steps = append(roomScenario.Steps, lookup)
		if !step.Passed || !lookup.Passed {
			roomScenario.Status, roomScenario.Error = "failed", "physical room creation failed"
			if err := recordE2EScenario(report, roomScenario, writeReport); err != nil {
				return err
			}
		}
		roomIDs = append(roomIDs, id)
	}
	if artifacts != nil {
		artifacts.RoomIDs = append([]uint64(nil), roomIDs...)
	}
	if err := recordE2EScenario(report, passedE2EScenario(roomScenario), writeReport); err != nil {
		return err
	}

	planScenario := e2eScenarioReport{Name: "save-room-rent-plans", Status: "running", Steps: []e2eStepReport{}}
	planCases := []struct {
		roomID    uint64
		period    string
		rent      string
		dueDay    int
		tenantIDs []uint64
		amounts   []string
	}{
		{roomIDs[0], manifest.Period, "1200.00", 5, []uint64{tenantIDs[0], tenantIDs[1]}, []string{"700.00", "500.00"}},
		{roomIDs[1], manifest.Period, "900.00", manifest.OverdueDueDay, []uint64{tenantIDs[2]}, []string{"900.00"}},
		{roomIDs[3], manifest.FuturePeriod, "760.00", 20, []uint64{tenantIDs[3]}, []string{"760.00"}},
	}
	for _, plan := range planCases {
		step, planErr := e2eSaveRoomRentPlan(ctx, client, plan.roomID, plan.period, plan.rent, plan.dueDay, plan.tenantIDs, plan.amounts)
		planScenario.Steps = append(planScenario.Steps, step)
		if planErr != nil {
			planScenario.Status, planScenario.Error = "failed", "room rent plan could not be saved"
			if err := recordE2EScenario(report, planScenario, writeReport); err != nil {
				return err
			}
		}
	}
	if err := recordE2EScenario(report, passedE2EScenario(planScenario), writeReport); err != nil {
		return err
	}

	futureBefore, err := e2eCountRentCharges(ctx, db, userID, manifest.FuturePeriod)
	if err != nil {
		return fmt.Errorf("count future rent charges before preview: %w", err)
	}
	futureResponse, futureErr := client.do(ctx, http.MethodGet, "/rent-dashboard?view=rooms&period="+url.QueryEscape(manifest.FuturePeriod), nil)
	futureStep := e2eHTTPStep(http.MethodGet, "/rent-dashboard?view=rooms&period="+manifest.FuturePeriod, map[string]any{"status_code": http.StatusOK, "contains_forecast": true, "future_charge_count": 0}, futureResponse, futureErr)
	futureAfter, countErr := e2eCountRentCharges(ctx, db, userID, manifest.FuturePeriod)
	if futureErr == nil && countErr == nil {
		containsForecast := strings.Contains(string(futureResponse.Body), "预计")
		futureStep.Passed = futureResponse.StatusCode == http.StatusOK && containsForecast && futureBefore == 0 && futureAfter == 0
		futureStep.Actual.(map[string]any)["contains_forecast"] = containsForecast
		futureStep.Actual.(map[string]any)["future_charge_count_before"] = futureBefore
		futureStep.Actual.(map[string]any)["future_charge_count_after"] = futureAfter
		if !futureStep.Passed {
			futureStep.Error = "future preview wrote a rent charge or did not show forecast data"
		}
	} else {
		futureStep.Passed = false
		futureStep.Error = "future preview or rent charge count failed"
	}
	if err := recordE2EScenario(report, scenarioFromSteps("future-plan-preview-does-not-write-facts", []e2eStepReport{futureStep}), writeReport); err != nil {
		return err
	}

	if err := e2eCreateAndAllocate(ctx, db, userID, client, manifest, roomIDs, tenantIDs); err != nil {
		return recordE2EScenario(report, e2eScenarioReport{Name: "payment-allocation-and-third-party-payment", Status: "failed", Error: err.Error()}, writeReport)
	}
	paymentScenario := e2eScenarioReport{Name: "payment-allocation-and-third-party-payment", Status: "passed", Steps: []e2eStepReport{{Method: "POST", Path: "/transactions/allocate", Expected: map[string]any{"shared_room_rent": "1200.00", "tenant_a": "700.00", "tenant_b": "500.00", "payer": tenants[0].Name}, Actual: map[string]any{"allocation_saved": true, "payer_is_tenant_a": true}, Passed: true}}}
	if err := recordE2EScenario(report, paymentScenario, writeReport); err != nil {
		return err
	}

	workspaceScenario := verifyE2EWorkspaceViews(ctx, client, manifest, roomIDs, tenantIDs)
	if err := recordE2EScenario(report, workspaceScenario, writeReport); err != nil {
		return err
	}
	legacyScenario := verifyE2ELegacyTenanciesRoute(ctx, client)
	if err := recordE2EScenario(report, legacyScenario, writeReport); err != nil {
		return err
	}

	secondClient, err := newE2EHTTPClient(options.BaseURL)
	if err != nil {
		return fmt.Errorf("create second E2E HTTP client: %w", err)
	}
	if err := recordE2EScenario(report, secondClient.authenticationScenario(ctx, options.SecondUsername, options.SecondPassword), writeReport); err != nil {
		return err
	}
	isolation := verifyE2ECrossUserIsolation(ctx, secondClient, manifest, roomIDs[0], tenantIDs[0])
	if err := recordE2EScenario(report, isolation, writeReport); err != nil {
		return err
	}

	report.Status = "business_passed"
	report.FinishedAt = time.Now().UTC()
	return persistE2EProgress(report, writeReport)
}

func passedE2EScenario(scenario e2eScenarioReport) e2eScenarioReport {
	if scenario.Status != "failed" {
		scenario.Status = "passed"
	}
	return scenario
}

func scenarioFromSteps(name string, steps []e2eStepReport) e2eScenarioReport {
	scenario := e2eScenarioReport{Name: name, Status: "passed", Steps: steps}
	for _, step := range steps {
		if !step.Passed {
			scenario.Status, scenario.Error = "failed", firstE2EError(step.Error, "scenario check failed")
			break
		}
	}
	return scenario
}

func e2eSaveRoomRentPlan(ctx context.Context, client *e2eHTTPClient, roomID uint64, period, rent string, dueDay int, tenantIDs []uint64, amounts []string) (e2eStepReport, error) {
	path := fmt.Sprintf("/rooms/%d?period=%s&rent=1", roomID, url.QueryEscape(period))
	response, err := client.do(ctx, http.MethodGet, path, nil)
	if err != nil || response.StatusCode != http.StatusOK {
		return e2eHTTPStep(http.MethodGet, path, map[string]any{"status_code": http.StatusOK}, response, err), errors.New("rent-plan editor was unavailable")
	}
	matches := e2ePlanVersionPattern.FindSubmatch(response.Body)
	if len(matches) != 2 {
		return e2eHTTPStep(http.MethodGet, path, map[string]any{"plan_version": "present"}, response, nil), errors.New("rent-plan timeline version was missing")
	}
	form := url.Values{"action": {"save"}, "period": {period}, "effective_month": {period}, "monthly_rent": {rent}, "due_day": {strconv.Itoa(dueDay)}, "plan_version": {string(matches[1])}}
	for index, tenantID := range tenantIDs {
		form.Add("tenant_id", strconv.FormatUint(tenantID, 10))
		form.Add("responsibility", amounts[index])
	}
	path = fmt.Sprintf("/rooms/%d/rent-plan", roomID)
	response, err = client.do(ctx, http.MethodPost, path, form)
	step := e2eHTTPStep(http.MethodPost, path, map[string]any{"redirect_contains": "message=rent_plan_saved"}, response, err)
	if err == nil {
		step.Passed = response.StatusCode == http.StatusFound && strings.Contains(response.Location, "message=rent_plan_saved")
		if !step.Passed {
			step.Error = "rent plan was rejected by the room editor"
		}
	}
	if !step.Passed {
		return step, errors.New("rent-plan form did not save successfully")
	}
	return step, nil
}

func e2eCreateAndAllocate(ctx context.Context, db *sql.DB, userID uint64, client *e2eHTTPClient, manifest e2eFixtureManifest, roomIDs, tenantIDs []uint64) error {
	sharedTxID, err := e2eInsertPaymentTransaction(ctx, db, userID, manifest, "shared-room-payment", manifest.Expected.SharedRoomRentCents, tenantIDs[0])
	if err != nil {
		return err
	}
	sharedForm := url.Values{"transaction_id": {strconv.FormatUint(sharedTxID, 10)}, "idempotency_key": {manifest.RunID + "-shared-allocation"}}
	for index, data := range []struct {
		tenantID uint64
		cents    int64
	}{{tenantIDs[0], manifest.Expected.TenantAAmountCents}, {tenantIDs[1], manifest.Expected.TenantBAmountCents}} {
		sharedForm.Add("allocation_kind", "rent")
		sharedForm.Add("tenant_id", strconv.FormatUint(data.tenantID, 10))
		sharedForm.Add("period", manifest.Period)
		sharedForm.Add("amount", e2eCentsToAmount(data.cents))
		sharedForm.Add("note", fmt.Sprintf("%s shared responsibility %d", manifest.RunID, index+1))
	}
	response, err := client.do(ctx, http.MethodPost, "/transactions/allocate", sharedForm)
	if err != nil || response.StatusCode != http.StatusFound || !strings.Contains(response.Location, "message=allocation_saved") {
		return errors.New("shared-room payment allocation failed")
	}
	partialTxID, err := e2eInsertPaymentTransaction(ctx, db, userID, manifest, "tenant-c-partial-payment", manifest.Expected.TenantCPaidCents, tenantIDs[2])
	if err != nil {
		return err
	}
	partialForm := url.Values{
		"transaction_id": {strconv.FormatUint(partialTxID, 10)}, "idempotency_key": {manifest.RunID + "-partial-allocation"},
		"allocation_kind": {"rent"}, "tenant_id": {strconv.FormatUint(tenantIDs[2], 10)},
		"period": {manifest.Period}, "amount": {e2eCentsToAmount(manifest.Expected.TenantCPaidCents)}, "note": {manifest.RunID + " partial payment"},
	}
	response, err = client.do(ctx, http.MethodPost, "/transactions/allocate", partialForm)
	if err != nil || response.StatusCode != http.StatusFound || !strings.Contains(response.Location, "message=allocation_saved") {
		return errors.New("partial payment allocation failed")
	}
	var aPaid, bPaid int64
	if err := db.QueryRowContext(ctx, "SELECT SUM(CASE WHEN tenant_id = ? THEN paid_amount_cents ELSE 0 END), SUM(CASE WHEN tenant_id = ? THEN paid_amount_cents ELSE 0 END) FROM rent_obligations WHERE user_id = ? AND period_month = ?", tenantIDs[0], tenantIDs[1], userID, e2ePeriodDate(manifest.Period)).Scan(&aPaid, &bPaid); err != nil {
		return fmt.Errorf("verify payment projections: %w", err)
	}
	if aPaid != manifest.Expected.TenantAAmountCents || bPaid != manifest.Expected.TenantBAmountCents {
		return errors.New("shared payment did not fully cover both rent responsibilities")
	}
	rows, err := db.QueryContext(ctx, `SELECT rc.room_id, ro.expected_amount_cents, ro.paid_amount_cents
		FROM rent_obligations AS ro JOIN rent_charges AS rc ON rc.user_id = ro.user_id AND rc.id = ro.rent_charge_id
		WHERE ro.user_id = ? AND ro.tenant_id = ? AND ro.period_month = ? ORDER BY rc.room_id`, userID, tenantIDs[2], e2ePeriodDate(manifest.Period))
	if err != nil {
		return fmt.Errorf("read tenant C room obligations: %w", err)
	}
	defer rows.Close()
	cAmounts := map[uint64][2]int64{}
	for rows.Next() {
		var roomID uint64
		var expected, paid int64
		if err := rows.Scan(&roomID, &expected, &paid); err != nil {
			return fmt.Errorf("read tenant C room obligation row: %w", err)
		}
		cAmounts[roomID] = [2]int64{expected, paid}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate tenant C room obligations: %w", err)
	}
	if len(cAmounts) != 1 || cAmounts[roomIDs[1]] != [2]int64{manifest.Expected.TenantCCents, manifest.Expected.TenantCPaidCents} {
		return errors.New("tenant C responsibility was mis-sized, duplicated, or assigned to the wrong room")
	}
	return nil
}

func e2eInsertPaymentTransaction(ctx context.Context, db *sql.DB, userID uint64, manifest e2eFixtureManifest, key string, amountCents int64, matchedTenantID uint64) (uint64, error) {
	stableKey := manifest.RunID + "-" + key
	result, err := db.ExecContext(ctx, `INSERT INTO payment_transactions
	(user_id, source, source_batch_id, provider_transaction_id, stable_transaction_key, account_id, account_name, direction, amount_cents, currency, transaction_time, description, reference, payer_id, payer_name, payer_name_kind, matched_tenant_id, match_status)
	VALUES (?, 'e2e', ?, ?, ?, ?, ?, 'income', ?, 'EUR', ?, ?, ?, ?, ?, 'matched', ?, 'matched')`,
		userID, manifest.RunID, stableKey, stableKey, manifest.RunID+"-account", manifest.RunID+" E2E account", amountCents,
		manifest.GeneratedAt.UTC(), manifest.RunID+" rent payment", manifest.RunID+" rent", manifest.RunID+"-payer-a", manifest.TenantA.PayerName, matchedTenantID)
	if err != nil {
		return 0, fmt.Errorf("seed run-scoped payment transaction: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil || id <= 0 {
		return 0, errors.New("run-scoped payment transaction ID was not returned")
	}
	return uint64(id), nil
}

func verifyE2EWorkspaceViews(ctx context.Context, client *e2eHTTPClient, manifest e2eFixtureManifest, roomIDs, tenantIDs []uint64) e2eScenarioReport {
	scenario := e2eScenarioReport{Name: "workspace-and-tenant-responsibility-views", Status: "running", Steps: []e2eStepReport{}}
	checks := []struct {
		path    string
		markers []string
		absent  []string
		name    string
	}{
		{path: "/rent-dashboard?view=rooms&period=" + url.QueryEscape(manifest.Period), markers: []string{"EUR 1200.00", "01"}, name: "room view displays shared-room rent"},
		{path: fmt.Sprintf("/rooms/%d?period=%s", roomIDs[0], manifest.Period), markers: []string{"入住与租金", "他人代付", "EUR 1200.00"}, name: "room detail is the plan view and shows third-party payment"},
		{path: "/rent-dashboard?view=tenants&status=outstanding&period=" + url.QueryEscape(manifest.Period) + "&search=" + url.QueryEscape(manifest.TenantC.Name), markers: []string{manifest.TenantC.Alias, "未结清责任"}, absent: []string{manifest.TenantA.Name, manifest.TenantB.Name}, name: "outstanding excludes settled tenants and includes tenant C"},
		{path: "/rent-dashboard?view=tenants&status=overdue&period=" + url.QueryEscape(manifest.Period) + "&search=" + url.QueryEscape(manifest.TenantC.Name), markers: []string{manifest.TenantC.Alias, "逾期"}, name: "overdue filter shows tenant C's partial responsibility"},
		{path: fmt.Sprintf("/tenants/%d?from_month=%s&to_month=%s", tenantIDs[1], manifest.Period, manifest.Period), markers: []string{"本人被代付", "EUR 500.00"}, name: "tenant B shows third-party payment"},
		{path: fmt.Sprintf("/tenants/%d?from_month=%s&to_month=%s", tenantIDs[2], manifest.Period, manifest.Period), markers: []string{"02", fmt.Sprintf("/rooms/%d?period=%s", roomIDs[1], manifest.Period)}, absent: []string{"/rooms/" + strconv.FormatUint(roomIDs[2], 10) + "?period=" + manifest.Period}, name: "tenant C shows one room responsibility and its room link"},
	}
	for _, check := range checks {
		response, err := client.do(ctx, http.MethodGet, check.path, nil)
		step := e2eHTTPStep(http.MethodGet, check.path, map[string]any{"status_code": http.StatusOK, "contains": check.markers, "absent": check.absent}, response, err)
		if err == nil {
			body := string(response.Body)
			pass := response.StatusCode == http.StatusOK
			for _, marker := range check.markers {
				pass = pass && strings.Contains(body, marker)
			}
			for _, marker := range check.absent {
				pass = pass && !strings.Contains(body, marker)
			}
			step.Passed = pass
			step.Actual.(map[string]any)["response_body_bytes"] = len(response.Body)
			if !pass {
				missing := make([]string, 0, len(check.markers))
				for _, marker := range check.markers {
					if !strings.Contains(body, marker) {
						missing = append(missing, marker)
					}
				}
				step.Actual.(map[string]any)["missing_markers"] = missing
				step.Error = check.name + " did not match expected content"
			}
		}
		scenario.Steps = append(scenario.Steps, step)
		if !step.Passed {
			scenario.Status, scenario.Error = "failed", check.name
			return scenario
		}
	}
	return passedE2EScenario(scenario)
}

func verifyE2ELegacyTenanciesRoute(ctx context.Context, client *e2eHTTPClient) e2eScenarioReport {
	scenario := e2eScenarioReport{Name: "legacy-tenancies-route-removed", Status: "running", Steps: []e2eStepReport{}}
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		response, err := client.do(ctx, method, "/tenancies", url.Values{})
		step := e2eHTTPStep(method, "/tenancies", map[string]any{"status_code": http.StatusNotFound}, response, err)
		if err == nil {
			step.Passed = response.StatusCode == http.StatusNotFound
			if !step.Passed {
				step.Error = "legacy tenancies route is still registered"
			}
		}
		scenario.Steps = append(scenario.Steps, step)
		if !step.Passed {
			scenario.Status, scenario.Error = "failed", "legacy tenancies route remains available"
			return scenario
		}
	}
	return passedE2EScenario(scenario)
}

func verifyE2ECrossUserIsolation(ctx context.Context, client *e2eHTTPClient, manifest e2eFixtureManifest, roomID, tenantID uint64) e2eScenarioReport {
	scenario := e2eScenarioReport{Name: "cross-user-data-isolation", Status: "running", Steps: []e2eStepReport{}}
	for _, check := range []struct {
		method, path string
		status       int
		markers      []string
	}{
		{http.MethodGet, fmt.Sprintf("/rooms/%d?period=%s", roomID, manifest.Period), http.StatusNotFound, []string{manifest.TenantA.Name, manifest.TenantB.Name}},
		{http.MethodGet, fmt.Sprintf("/tenants/%d?from_month=%s&to_month=%s", tenantID, manifest.Period, manifest.Period), http.StatusNotFound, []string{manifest.TenantA.Name, manifest.TenantA.PayerName}},
	} {
		response, err := client.do(ctx, check.method, check.path, nil)
		step := e2eHTTPStep(check.method, check.path, map[string]any{"status_code": check.status, "foreign_markers_absent": true}, response, err)
		if err == nil {
			body := string(response.Body)
			markersAbsent := true
			for _, marker := range check.markers {
				markersAbsent = markersAbsent && !strings.Contains(body, marker)
			}
			step.Passed = response.StatusCode == check.status && markersAbsent
			step.Actual.(map[string]any)["foreign_markers_absent"] = markersAbsent
			if !step.Passed {
				step.Error = "second account could access primary-account data"
			}
		}
		scenario.Steps = append(scenario.Steps, step)
		if !step.Passed {
			scenario.Status, scenario.Error = "failed", "cross-user isolation check failed"
			return scenario
		}
	}
	return passedE2EScenario(scenario)
}

func e2eFindPropertyID(ctx context.Context, db *sql.DB, userID uint64, name string) (uint64, error) {
	var id uint64
	err := db.QueryRowContext(ctx, "SELECT id FROM properties WHERE user_id = ? AND name = ? LIMIT 1", userID, name).Scan(&id)
	return id, err
}

func e2eFindTenantID(ctx context.Context, db *sql.DB, userID uint64, name string) (uint64, error) {
	var id uint64
	err := db.QueryRowContext(ctx, "SELECT id FROM tenants WHERE user_id = ? AND name = ? LIMIT 1", userID, name).Scan(&id)
	return id, err
}

func e2eFindRoomID(ctx context.Context, db *sql.DB, userID, propertyID uint64, label string) (uint64, error) {
	var id uint64
	err := db.QueryRowContext(ctx, "SELECT id FROM rooms WHERE user_id = ? AND property_id = ? AND room_label = ? LIMIT 1", userID, propertyID, label).Scan(&id)
	return id, err
}

func e2eCountRentCharges(ctx context.Context, db *sql.DB, userID uint64, period string) (int64, error) {
	var count int64
	err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM rent_charges WHERE user_id = ? AND period_month = ?", userID, e2ePeriodDate(period)).Scan(&count)
	return count, err
}

func e2ePeriodDate(period string) string { return period + "-01" }

func e2eCentsToAmount(cents int64) string { return fmt.Sprintf("%d.%02d", cents/100, cents%100) }

func recordE2EScenario(report *e2eReport, scenario e2eScenarioReport, writeReport e2EReportWriter) error {
	if report == nil {
		return errors.New("E2E report is required")
	}
	report.Scenarios = append(report.Scenarios, scenario)
	report.FinishedAt = time.Now().UTC()
	if scenario.Status != "passed" {
		report.Status = "failed"
		report.Error = firstE2EError(scenario.Error, "E2E scenario failed: "+scenario.Name)
		if err := persistE2EProgress(report, writeReport); err != nil {
			return err
		}
		return errors.New(report.Error)
	}
	return persistE2EProgress(report, writeReport)
}

func persistE2EProgress(report *e2eReport, writeReport e2EReportWriter) error {
	if writeReport == nil {
		return nil
	}
	if err := writeReport(*report); err != nil {
		return fmt.Errorf("write E2E progress report: %w", err)
	}
	return nil
}

func firstE2EError(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}
