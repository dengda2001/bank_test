package main

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var e2eRunIDPattern = regexp.MustCompile(`^rentops-e2e-[0-9]{8}-[0-9]{6}-[a-f0-9]{8}$`)

type e2eFixtureManifest struct {
	SchemaVersion int                 `json:"schema_version"`
	RunID         string              `json:"run_id"`
	GeneratedAt   time.Time           `json:"generated_at"`
	Period        string              `json:"period"`
	FuturePeriod  string              `json:"future_period"`
	OverdueDueDay int                 `json:"overdue_due_day"`
	TenantA       e2eTenantFixture    `json:"tenant_a"`
	TenantB       e2eTenantFixture    `json:"tenant_b"`
	TenantC       e2eTenantFixture    `json:"tenant_c"`
	TenantD       e2eTenantFixture    `json:"tenant_d"`
	Expected      e2EExpectedSnapshot `json:"expected"`
}

type e2eTenantFixture struct {
	Name      string `json:"name"`
	Alias     string `json:"alias"`
	Email     string `json:"email"`
	PayerID   string `json:"payer_id"`
	PayerName string `json:"payer_name"`
}

type e2EExpectedSnapshot struct {
	SharedRoomRentCents int64 `json:"shared_room_rent_cents"`
	TenantAAmountCents  int64 `json:"tenant_a_amount_cents"`
	TenantBAmountCents  int64 `json:"tenant_b_amount_cents"`
	TenantCCents        int64 `json:"tenant_c_cents"`
	TenantCPaidCents    int64 `json:"tenant_c_paid_cents"`
}

func validateE2ERunID(value string) error {
	if !e2eRunIDPattern.MatchString(strings.TrimSpace(value)) {
		return errors.New("run ID must match rentops-e2e-YYYYMMDD-HHMMSS-xxxxxxxx")
	}
	return nil
}

func newE2EFixtureManifest(runID string, now time.Time) (e2eFixtureManifest, error) {
	if err := validateE2ERunID(runID); err != nil {
		return e2eFixtureManifest{}, err
	}
	location, err := time.LoadLocation("Europe/Dublin")
	if err != nil {
		return e2eFixtureManifest{}, fmt.Errorf("load Dublin timezone: %w", err)
	}
	local := now.In(location)
	current := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, location)
	future := current.AddDate(0, 1, 0)
	dueDay := local.Day() - 1
	if dueDay < 1 {
		dueDay = 1
	}
	manifest := e2eFixtureManifest{
		SchemaVersion: 1,
		RunID:         runID,
		GeneratedAt:   now.UTC(),
		Period:        current.Format("2006-01"),
		FuturePeriod:  future.Format("2006-01"),
		OverdueDueDay: dueDay,
		TenantA:       e2eTenantFixture{Name: runID + " Aoife Murphy", Alias: "Aoife", Email: runID + "-a@example.test", PayerID: runID + "-payer-a", PayerName: runID + " Aoife Murphy"},
		TenantB:       e2eTenantFixture{Name: runID + " Brian Doyle", Alias: "Brian", Email: runID + "-b@example.test", PayerID: runID + "-payer-b", PayerName: runID + " Brian Doyle"},
		TenantC:       e2eTenantFixture{Name: runID + " Chen Xi", Alias: "Chen", Email: runID + "-c@example.test", PayerID: runID + "-payer-c", PayerName: runID + " Chen Xi"},
		TenantD:       e2eTenantFixture{Name: runID + " Dara Wu", Alias: "Dara", Email: runID + "-d@example.test", PayerID: runID + "-payer-d", PayerName: runID + " Dara Wu"},
		Expected:      e2EExpectedSnapshot{SharedRoomRentCents: 120000, TenantAAmountCents: 70000, TenantBAmountCents: 50000, TenantCCents: 90000, TenantCPaidCents: 10000},
	}
	if err := manifest.validate(); err != nil {
		return e2eFixtureManifest{}, err
	}
	return manifest, nil
}

func (manifest e2eFixtureManifest) validate() error {
	if err := validateE2ERunID(manifest.RunID); err != nil {
		return err
	}
	if manifest.SchemaVersion != 1 {
		return errors.New("unsupported E2E fixture schema version")
	}
	period, err := time.Parse("2006-01", manifest.Period)
	if err != nil || period.Format("2006-01") != manifest.Period {
		return errors.New("current E2E period is invalid")
	}
	future, err := time.Parse("2006-01", manifest.FuturePeriod)
	if err != nil || !future.Equal(period.AddDate(0, 1, 0)) {
		return errors.New("future E2E period must immediately follow current period")
	}
	if manifest.OverdueDueDay < 1 || manifest.OverdueDueDay > 31 {
		return errors.New("overdue fixture due day is invalid")
	}
	for _, tenant := range []e2eTenantFixture{manifest.TenantA, manifest.TenantB, manifest.TenantC, manifest.TenantD} {
		for _, value := range []string{tenant.Name, tenant.Email, tenant.PayerID, tenant.PayerName} {
			if !strings.HasPrefix(value, manifest.RunID) {
				return errors.New("tenant fixtures must be run-ID scoped")
			}
		}
	}
	if manifest.Expected.TenantAAmountCents+manifest.Expected.TenantBAmountCents != manifest.Expected.SharedRoomRentCents ||
		manifest.Expected.TenantCCents <= manifest.Expected.TenantCPaidCents {
		return errors.New("rent fixture amounts are inconsistent")
	}
	return nil
}
