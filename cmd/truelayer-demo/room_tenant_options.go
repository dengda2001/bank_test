package main

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"gorm.io/gorm"
)

// roomTenantOccupancy is presentation data only. Plan writes re-read every
// interval while holding tenant locks inside SaveRoomRentPlan.
type roomTenantOccupancy struct {
	RoomID    uint64 `json:"room_id"`
	RoomLabel string `json:"room_label"`
	From      string `json:"from"`
	To        string `json:"to,omitempty"`
}

type roomTenantOccupancyRow struct {
	TenantID           uint64
	RoomID             uint64
	RoomLabel          string
	EffectiveFromMonth time.Time
	EffectiveToMonth   *time.Time
}

func loadRoomTenantOptions(ctx context.Context, db *gorm.DB, userID, targetRoomID uint64, month time.Time) ([]roomRentPlanTenantOption, error) {
	var tenants []tenant
	if err := db.WithContext(ctx).Where("user_id = ?", userID).Order("name ASC, id ASC").Find(&tenants).Error; err != nil {
		return nil, err
	}
	var occupancies []roomTenantOccupancyRow
	if err := db.WithContext(ctx).Table("room_rent_plan_members AS m").
		Select("m.tenant_id, p.room_id, r.room_label, p.effective_from_month, p.effective_to_month").
		Joins("JOIN room_rent_plans AS p ON p.user_id = m.user_id AND p.id = m.room_rent_plan_id").
		Joins("JOIN rooms AS r ON r.user_id = p.user_id AND r.id = p.room_id").
		Where("m.user_id = ?", userID).
		Order("m.tenant_id ASC, p.effective_from_month ASC, p.id ASC").Scan(&occupancies).Error; err != nil {
		return nil, err
	}
	byTenant := make(map[uint64][]roomTenantOccupancy)
	for _, row := range occupancies {
		item := roomTenantOccupancy{RoomID: row.RoomID, RoomLabel: row.RoomLabel, From: row.EffectiveFromMonth.Format("2006-01")}
		if row.EffectiveToMonth != nil {
			item.To = row.EffectiveToMonth.Format("2006-01")
		}
		byTenant[row.TenantID] = append(byTenant[row.TenantID], item)
	}
	monthValue := monthStart(month).Format("2006-01")
	options := make([]roomRentPlanTenantOption, 0, len(tenants))
	for _, row := range tenants {
		intervals := byTenant[row.ID]
		if intervals == nil {
			intervals = []roomTenantOccupancy{}
		}
		encoded, err := json.Marshal(intervals)
		if err != nil {
			return nil, err
		}
		option := roomRentPlanTenantOption{ID: row.ID, Name: firstNonEmpty(row.DisplayAlias, row.Name), Status: row.Status, OccupanciesJSON: string(encoded)}
		if conflict, found := firstRoomTenantConflict(intervals, targetRoomID, monthValue); found {
			option.ConflictRoomName = conflict.RoomLabel
			option.ConflictRoomID = conflict.RoomID
			option.UnbindURL = "/rooms/" + strconv.FormatUint(conflict.RoomID, 10) + "?period=" + monthValue + "&rent=1"
		}
		options = append(options, option)
	}
	return options, nil
}

func firstRoomTenantConflict(intervals []roomTenantOccupancy, targetRoomID uint64, month string) (roomTenantOccupancy, bool) {
	for _, interval := range intervals {
		if interval.RoomID != targetRoomID && (interval.To == "" || interval.To >= month) {
			return interval, true
		}
	}
	return roomTenantOccupancy{}, false
}
