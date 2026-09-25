package main

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

var errLandlordRentUserRequired = errors.New("landlord rent userID is required")

type landlordRentRepository struct {
	db *gorm.DB
}

type propertyQuery struct {
	Status         string
	IncludeDeleted bool
}

type roomQuery struct {
	PropertyID     uint64
	Status         string
	IncludeDeleted bool
}

type roomRentPlanQuery struct {
	RoomID                       uint64
	EffectiveFromMonthOnOrBefore *time.Time
	EffectiveToMonthOnOrAfter    *time.Time
}

type roomRentPlanMemberQuery struct {
	RoomRentPlanID uint64
	TenantID       uint64
}

type rentChargeQuery struct {
	PropertyID     uint64
	RoomID         uint64
	RoomRentPlanID uint64
	PeriodMonth    *time.Time
	IncludeVoided  bool
}

type rentObligationQuery struct {
	RentChargeID  uint64
	PropertyID    uint64
	RoomID        uint64
	TenantID      uint64
	PeriodMonth   *time.Time
	IncludeVoided bool
}

type paymentAllocationQuery struct {
	PaymentTransactionID uint64
	RentObligationID     uint64
	TenantID             uint64
	Status               string
}

type manualExpenseQuery struct {
	PropertyID    uint64
	RoomID        uint64
	FromDate      *time.Time
	ToDate        *time.Time
	IncludeVoided bool
}

func newLandlordRentRepository(db *gorm.DB) *landlordRentRepository {
	return &landlordRentRepository{db: db}
}

func (r *landlordRentRepository) scoped(ctx context.Context, userID uint64) (*gorm.DB, error) {
	if userID == 0 {
		return nil, errLandlordRentUserRequired
	}
	if r == nil || r.db == nil {
		return nil, errors.New("landlord rent database is required")
	}
	return r.db.WithContext(ctx).Where("user_id = ?", userID), nil
}

func (r *landlordRentRepository) findProperty(ctx context.Context, userID, propertyID uint64) (property, error) {
	query, err := r.scoped(ctx, userID)
	if err != nil {
		return property{}, err
	}
	var row property
	if err := query.Where("id = ?", propertyID).First(&row).Error; err != nil {
		return property{}, err
	}
	return row, nil
}

func (r *landlordRentRepository) listProperties(ctx context.Context, userID uint64, filters propertyQuery) ([]property, error) {
	query, err := r.scoped(ctx, userID)
	if err != nil {
		return nil, err
	}
	if filters.Status != "" {
		query = query.Where("status = ?", filters.Status)
	}
	if !filters.IncludeDeleted {
		query = query.Where("deleted_at IS NULL")
	}
	rows := make([]property, 0)
	if err := query.Order("name ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *landlordRentRepository) createProperty(ctx context.Context, userID uint64, row property) (property, error) {
	if _, err := r.scoped(ctx, userID); err != nil {
		return property{}, err
	}
	row.ID = 0
	row.UserID = userID
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return property{}, err
	}
	return row, nil
}

func (r *landlordRentRepository) findRoom(ctx context.Context, userID, roomID uint64) (room, error) {
	query, err := r.scoped(ctx, userID)
	if err != nil {
		return room{}, err
	}
	var row room
	if err := query.Where("id = ?", roomID).First(&row).Error; err != nil {
		return room{}, err
	}
	return row, nil
}

func (r *landlordRentRepository) listRooms(ctx context.Context, userID uint64, filters roomQuery) ([]room, error) {
	query, err := r.scoped(ctx, userID)
	if err != nil {
		return nil, err
	}
	if filters.PropertyID != 0 {
		query = query.Where("property_id = ?", filters.PropertyID)
	}
	if filters.Status != "" {
		query = query.Where("status = ?", filters.Status)
	}
	if !filters.IncludeDeleted {
		query = query.Where("rooms.deleted_at IS NULL AND EXISTS (SELECT 1 FROM properties AS parent WHERE parent.id = rooms.property_id AND parent.user_id = rooms.user_id AND parent.deleted_at IS NULL)")
	}
	rows := make([]room, 0)
	if err := query.Order("property_id ASC, room_label ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *landlordRentRepository) createRoom(ctx context.Context, userID uint64, row room) (room, error) {
	parent, err := r.findProperty(ctx, userID, row.PropertyID)
	if err != nil {
		return room{}, err
	}
	if parent.DeletedAt != nil {
		return room{}, gorm.ErrRecordNotFound
	}
	row.ID = 0
	row.UserID = userID
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return room{}, err
	}
	return row, nil
}

func (r *landlordRentRepository) findRoomRentPlan(ctx context.Context, userID, planID uint64) (roomRentPlan, error) {
	query, err := r.scoped(ctx, userID)
	if err != nil {
		return roomRentPlan{}, err
	}
	var row roomRentPlan
	if err := query.Where("id = ?", planID).First(&row).Error; err != nil {
		return roomRentPlan{}, err
	}
	return row, nil
}

func (r *landlordRentRepository) listRoomRentPlans(ctx context.Context, userID uint64, filters roomRentPlanQuery) ([]roomRentPlan, error) {
	query, err := r.scoped(ctx, userID)
	if err != nil {
		return nil, err
	}
	if filters.RoomID != 0 {
		query = query.Where("room_id = ?", filters.RoomID)
	}
	if filters.EffectiveFromMonthOnOrBefore != nil {
		query = query.Where("effective_from_month <= ?", *filters.EffectiveFromMonthOnOrBefore)
	}
	if filters.EffectiveToMonthOnOrAfter != nil {
		query = query.Where("(effective_to_month IS NULL OR effective_to_month >= ?)", *filters.EffectiveToMonthOnOrAfter)
	}
	rows := make([]roomRentPlan, 0)
	if err := query.Order("room_id ASC, effective_from_month DESC, id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *landlordRentRepository) createRoomRentPlan(ctx context.Context, userID uint64, row roomRentPlan) (roomRentPlan, error) {
	if _, err := r.findRoom(ctx, userID, row.RoomID); err != nil {
		return roomRentPlan{}, err
	}
	row.ID = 0
	row.UserID = userID
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return roomRentPlan{}, err
	}
	return row, nil
}

func (r *landlordRentRepository) listRoomRentPlanMembers(ctx context.Context, userID uint64, filters roomRentPlanMemberQuery) ([]roomRentPlanMember, error) {
	query, err := r.scoped(ctx, userID)
	if err != nil {
		return nil, err
	}
	if filters.RoomRentPlanID != 0 {
		query = query.Where("room_rent_plan_id = ?", filters.RoomRentPlanID)
	}
	if filters.TenantID != 0 {
		query = query.Where("tenant_id = ?", filters.TenantID)
	}
	rows := make([]roomRentPlanMember, 0)
	if err := query.Order("room_rent_plan_id ASC, tenant_id ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *landlordRentRepository) createRoomRentPlanMember(ctx context.Context, userID uint64, row roomRentPlanMember) (roomRentPlanMember, error) {
	if _, err := r.findRoomRentPlan(ctx, userID, row.RoomRentPlanID); err != nil {
		return roomRentPlanMember{}, err
	}
	var tenantRow tenant
	query, err := r.scoped(ctx, userID)
	if err != nil {
		return roomRentPlanMember{}, err
	}
	if err := query.Where("id = ?", row.TenantID).First(&tenantRow).Error; err != nil {
		return roomRentPlanMember{}, err
	}
	row.ID = 0
	row.UserID = userID
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return roomRentPlanMember{}, err
	}
	return row, nil
}

func (r *landlordRentRepository) findRentCharge(ctx context.Context, userID, chargeID uint64) (rentCharge, error) {
	query, err := r.scoped(ctx, userID)
	if err != nil {
		return rentCharge{}, err
	}
	var row rentCharge
	if err := query.Where("id = ?", chargeID).First(&row).Error; err != nil {
		return rentCharge{}, err
	}
	return row, nil
}

func (r *landlordRentRepository) listRentCharges(ctx context.Context, userID uint64, filters rentChargeQuery) ([]rentCharge, error) {
	query, err := r.scoped(ctx, userID)
	if err != nil {
		return nil, err
	}
	if filters.PropertyID != 0 {
		query = query.Where("property_id = ?", filters.PropertyID)
	}
	if filters.RoomID != 0 {
		query = query.Where("room_id = ?", filters.RoomID)
	}
	if filters.RoomRentPlanID != 0 {
		query = query.Where("room_rent_plan_id = ?", filters.RoomRentPlanID)
	}
	if filters.PeriodMonth != nil {
		query = query.Where("period_month = ?", monthStart(*filters.PeriodMonth))
	}
	if !filters.IncludeVoided {
		query = query.Where("record_status = ?", "active")
	}
	rows := make([]rentCharge, 0)
	if err := query.Order("period_month ASC, room_id ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *landlordRentRepository) createRentCharge(ctx context.Context, userID uint64, row rentCharge) (rentCharge, error) {
	propertyRow, err := r.findProperty(ctx, userID, row.PropertyID)
	if err != nil {
		return rentCharge{}, err
	}
	roomRow, err := r.findRoom(ctx, userID, row.RoomID)
	if err != nil {
		return rentCharge{}, err
	}
	if roomRow.PropertyID != propertyRow.ID {
		return rentCharge{}, gorm.ErrRecordNotFound
	}
	planRow, err := r.findRoomRentPlan(ctx, userID, row.RoomRentPlanID)
	if err != nil {
		return rentCharge{}, err
	}
	if planRow.RoomID != roomRow.ID {
		return rentCharge{}, gorm.ErrRecordNotFound
	}
	row.ID = 0
	row.UserID = userID
	row.PeriodMonth = monthStart(row.PeriodMonth)
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return rentCharge{}, err
	}
	return row, nil
}

func (r *landlordRentRepository) findRentObligation(ctx context.Context, userID, obligationID uint64) (rentObligation, error) {
	query, err := r.scoped(ctx, userID)
	if err != nil {
		return rentObligation{}, err
	}
	var row rentObligation
	if err := query.Where("id = ?", obligationID).First(&row).Error; err != nil {
		return rentObligation{}, err
	}
	return row, nil
}

func (r *landlordRentRepository) listRentObligations(ctx context.Context, userID uint64, filters rentObligationQuery) ([]rentObligation, error) {
	if userID == 0 {
		return nil, errLandlordRentUserRequired
	}
	if r == nil || r.db == nil {
		return nil, errors.New("landlord rent database is required")
	}
	query := r.db.WithContext(ctx).Table("rent_obligations AS ro").Where("ro.user_id = ?", userID)
	if filters.PropertyID != 0 || filters.RoomID != 0 {
		query = query.Joins("JOIN rent_charges AS rc ON rc.id = ro.rent_charge_id AND rc.user_id = ro.user_id")
		if filters.PropertyID != 0 {
			query = query.Where("rc.property_id = ?", filters.PropertyID)
		}
		if filters.RoomID != 0 {
			query = query.Where("rc.room_id = ?", filters.RoomID)
		}
	}
	if filters.RentChargeID != 0 {
		query = query.Where("ro.rent_charge_id = ?", filters.RentChargeID)
	}
	if filters.TenantID != 0 {
		query = query.Where("ro.tenant_id = ?", filters.TenantID)
	}
	if filters.PeriodMonth != nil {
		query = query.Where("ro.period_month = ?", monthStart(*filters.PeriodMonth))
	}
	if !filters.IncludeVoided {
		query = query.Where("ro.record_status = ?", obligationRecordActive)
	}
	rows := make([]rentObligation, 0)
	if err := query.Select("ro.*").Order("ro.period_month ASC, ro.tenant_id ASC, ro.id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *landlordRentRepository) createRentObligation(ctx context.Context, userID uint64, row rentObligation) (rentObligation, error) {
	query, err := r.scoped(ctx, userID)
	if err != nil {
		return rentObligation{}, err
	}
	var tenantRow tenant
	if err := query.Where("id = ?", row.TenantID).First(&tenantRow).Error; err != nil {
		return rentObligation{}, err
	}
	if row.RentChargeID != 0 {
		if _, err := r.findRentCharge(ctx, userID, row.RentChargeID); err != nil {
			return rentObligation{}, err
		}
	}
	row.ID = 0
	row.UserID = userID
	row.PeriodMonth = monthStart(row.PeriodMonth)
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return rentObligation{}, err
	}
	return row, nil
}

func (r *landlordRentRepository) listPaymentAllocations(ctx context.Context, userID uint64, filters paymentAllocationQuery) ([]paymentAllocation, error) {
	query, err := r.scoped(ctx, userID)
	if err != nil {
		return nil, err
	}
	if filters.PaymentTransactionID != 0 {
		query = query.Where("payment_transaction_id = ?", filters.PaymentTransactionID)
	}
	if filters.RentObligationID != 0 {
		query = query.Where("rent_obligation_id = ?", filters.RentObligationID)
	}
	if filters.TenantID != 0 {
		query = query.Where("tenant_id = ?", filters.TenantID)
	}
	if filters.Status != "" {
		query = query.Where("status = ?", filters.Status)
	}
	rows := make([]paymentAllocation, 0)
	if err := query.Order("created_at ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *landlordRentRepository) findManualExpense(ctx context.Context, userID, expenseID uint64) (manualExpense, error) {
	query, err := r.scoped(ctx, userID)
	if err != nil {
		return manualExpense{}, err
	}
	var row manualExpense
	if err := query.Where("id = ?", expenseID).First(&row).Error; err != nil {
		return manualExpense{}, err
	}
	return row, nil
}

func (r *landlordRentRepository) listManualExpenses(ctx context.Context, userID uint64, filters manualExpenseQuery) ([]manualExpense, error) {
	query, err := r.scoped(ctx, userID)
	if err != nil {
		return nil, err
	}
	if filters.PropertyID != 0 {
		query = query.Where("property_id = ?", filters.PropertyID)
	}
	if filters.RoomID != 0 {
		query = query.Where("room_id = ?", filters.RoomID)
	}
	if filters.FromDate != nil {
		query = query.Where("expense_date >= ?", *filters.FromDate)
	}
	if filters.ToDate != nil {
		query = query.Where("expense_date < ?", *filters.ToDate)
	}
	if !filters.IncludeVoided {
		query = query.Where("record_status = ?", "active")
	}
	rows := make([]manualExpense, 0)
	if err := query.Order("expense_date DESC, id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *landlordRentRepository) createManualExpense(ctx context.Context, userID uint64, row manualExpense) (manualExpense, error) {
	if _, err := r.scoped(ctx, userID); err != nil {
		return manualExpense{}, err
	}
	if row.PropertyID != nil {
		if _, err := r.findProperty(ctx, userID, *row.PropertyID); err != nil {
			return manualExpense{}, err
		}
	}
	if row.RoomID != nil {
		roomRow, err := r.findRoom(ctx, userID, *row.RoomID)
		if err != nil {
			return manualExpense{}, err
		}
		if row.PropertyID != nil && roomRow.PropertyID != *row.PropertyID {
			return manualExpense{}, gorm.ErrRecordNotFound
		}
	}
	row.ID = 0
	row.UserID = userID
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return manualExpense{}, err
	}
	return row, nil
}
