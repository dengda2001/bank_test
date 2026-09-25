package main

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

type transactionReviewPropertyOption struct {
	ID   uint64
	Name string
}

type transactionReviewRoomOption struct {
	ID          uint64
	PropertyID  uint64
	Name        string
	OccupantIDs string
}

type transactionReviewOccupantRow struct {
	RoomID   uint64
	TenantID uint64
}

// This is navigation evidence for one month. Confirmation still validates
// the owner, tenant, room plan, bill, and bank balance under its write locks.
func loadTransactionReviewLocations(ctx context.Context, db *gorm.DB, userID uint64, month time.Time) ([]transactionReviewPropertyOption, []transactionReviewRoomOption, error) {
	var properties []property
	if err := db.WithContext(ctx).Where("user_id = ? AND deleted_at IS NULL", userID).Order("name ASC, id ASC").Find(&properties).Error; err != nil {
		return nil, nil, err
	}
	var rooms []room
	if err := db.WithContext(ctx).Where("user_id = ? AND deleted_at IS NULL", userID).Order("property_id ASC, room_label ASC, id ASC").Find(&rooms).Error; err != nil {
		return nil, nil, err
	}
	month = monthStart(month)
	var occupants []transactionReviewOccupantRow
	if err := db.WithContext(ctx).Table("room_rent_plan_members AS member").
		Select("plan.room_id, member.tenant_id").
		Joins("JOIN room_rent_plans AS plan ON plan.user_id = member.user_id AND plan.id = member.room_rent_plan_id").
		Where("member.user_id = ? AND plan.effective_from_month <= ? AND (plan.effective_to_month IS NULL OR plan.effective_to_month >= ?)", userID, month, month).
		Scan(&occupants).Error; err != nil {
		return nil, nil, err
	}
	byRoom := make(map[uint64]map[uint64]bool)
	for _, occupant := range occupants {
		if byRoom[occupant.RoomID] == nil {
			byRoom[occupant.RoomID] = make(map[uint64]bool)
		}
		byRoom[occupant.RoomID][occupant.TenantID] = true
	}
	propertyOptions := make([]transactionReviewPropertyOption, 0, len(properties))
	visibleProperties := make(map[uint64]bool, len(properties))
	for _, row := range properties {
		visibleProperties[row.ID] = true
		name := row.Name
		if address := strings.TrimSpace(stringValue(row.Address)); address != "" {
			name += " · " + address
		}
		propertyOptions = append(propertyOptions, transactionReviewPropertyOption{ID: row.ID, Name: name})
	}
	roomOptions := make([]transactionReviewRoomOption, 0, len(rooms))
	for _, row := range rooms {
		if !visibleProperties[row.PropertyID] {
			continue
		}
		ids := make([]uint64, 0, len(byRoom[row.ID]))
		for tenantID := range byRoom[row.ID] {
			ids = append(ids, tenantID)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		values := make([]string, 0, len(ids))
		for _, tenantID := range ids {
			values = append(values, strconv.FormatUint(tenantID, 10))
		}
		roomOptions = append(roomOptions, transactionReviewRoomOption{ID: row.ID, PropertyID: row.PropertyID, Name: row.RoomLabel, OccupantIDs: strings.Join(values, ",")})
	}
	return propertyOptions, roomOptions, nil
}
