package main

import (
	"context"
	"sort"
	"strconv"
	"time"

	"gorm.io/gorm"
)

type transactionReviewRoommateGroup struct {
	AnchorName   string
	PropertyName string
	RoomLabel    string
	Period       string
	RoomURL      string
	Roommates    []transactionReviewTenant
}

type transactionReviewRoommateRow struct {
	AnchorID     uint64
	RoommateID   uint64
	RoomID       uint64
	PropertyName string
	RoomLabel    string
}

func transactionReviewRoommateAnchors(selectedTenantID uint64, payerName string, tenants []tenant) []tenant {
	if selectedTenantID != 0 {
		for _, row := range tenants {
			if row.ID == selectedTenantID {
				return []tenant{row}
			}
		}
		return nil
	}
	return tenantsByName(tenants, payerName)
}

func transactionReviewRoommateMonth(source paymentTransaction, requestedMonth string) time.Time {
	if requestedMonth != "" {
		if month, err := parsePeriodMonth(requestedMonth); err == nil {
			return month
		}
	}
	if period := transactionPeriodForModel(source); period.Month != nil {
		return monthStart(*period.Month)
	}
	if source.TransactionTime != nil {
		return dublinCurrentMonth(*source.TransactionTime)
	}
	return dublinCurrentMonth(time.Now())
}

// Roommate suggestions are read-only navigation. They never select an
// allocation target or establish a payer relationship.
func loadTransactionReviewRoommates(ctx context.Context, db *gorm.DB, userID uint64, anchors, tenants []tenant, month time.Time) ([]transactionReviewRoommateGroup, error) {
	if len(anchors) == 0 {
		return nil, nil
	}
	anchorIDs := make([]uint64, 0, len(anchors))
	anchorNames := make(map[uint64]string, len(anchors))
	for _, row := range anchors {
		anchorIDs = append(anchorIDs, row.ID)
		anchorNames[row.ID] = firstNonEmpty(row.DisplayAlias, row.Name)
	}
	tenantNames := make(map[uint64]string, len(tenants))
	for _, row := range tenants {
		tenantNames[row.ID] = firstNonEmpty(row.DisplayAlias, row.Name)
	}
	var rows []transactionReviewRoommateRow
	err := db.WithContext(ctx).Table("room_rent_plan_members AS anchor").
		Select("anchor.tenant_id AS anchor_id, roommate.tenant_id AS roommate_id, plan.room_id, property.name AS property_name, room.room_label").
		Joins("JOIN room_rent_plans AS plan ON plan.user_id = anchor.user_id AND plan.id = anchor.room_rent_plan_id").
		Joins("JOIN room_rent_plan_members AS roommate ON roommate.user_id = plan.user_id AND roommate.room_rent_plan_id = plan.id").
		Joins("JOIN rooms AS room ON room.user_id = plan.user_id AND room.id = plan.room_id").
		Joins("JOIN properties AS property ON property.user_id = room.user_id AND property.id = room.property_id").
		Where("anchor.user_id = ? AND anchor.tenant_id IN ? AND roommate.tenant_id <> anchor.tenant_id AND plan.effective_from_month <= ? AND (plan.effective_to_month IS NULL OR plan.effective_to_month >= ?)", userID, anchorIDs, monthStart(month), monthStart(month)).
		Order("property.name ASC, room.room_label ASC, anchor.tenant_id ASC, roommate.tenant_id ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	groups := make([]transactionReviewRoommateGroup, 0)
	groupByKey := make(map[[2]uint64]int)
	seenRoommate := make(map[[3]uint64]bool)
	for _, row := range rows {
		name := tenantNames[row.RoommateID]
		if name == "" || anchorNames[row.AnchorID] == "" {
			continue
		}
		key := [2]uint64{row.AnchorID, row.RoomID}
		index, exists := groupByKey[key]
		if !exists {
			index = len(groups)
			groupByKey[key] = index
			groups = append(groups, transactionReviewRoommateGroup{
				AnchorName: anchorNames[row.AnchorID], PropertyName: row.PropertyName,
				RoomLabel: row.RoomLabel, Period: monthStart(month).Format("2006-01"),
				RoomURL: "/rooms/" + strconv.FormatUint(row.RoomID, 10) + "?period=" + monthStart(month).Format("2006-01"),
			})
		}
		roommateKey := [3]uint64{row.AnchorID, row.RoomID, row.RoommateID}
		if seenRoommate[roommateKey] {
			continue
		}
		seenRoommate[roommateKey] = true
		groups[index].Roommates = append(groups[index].Roommates, transactionReviewTenant{ID: row.RoommateID, Name: name})
	}
	for index := range groups {
		sort.Slice(groups[index].Roommates, func(i, j int) bool {
			return groups[index].Roommates[i].Name < groups[index].Roommates[j].Name
		})
	}
	return groups, nil
}
