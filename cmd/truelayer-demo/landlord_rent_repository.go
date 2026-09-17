package main

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

var errLandlordRentUserRequired = errors.New("landlord rent userID is required")

type landlordRentRepository struct {
	db *gorm.DB
}

type propertyQuery struct {
	Status string
}

type roomQuery struct {
	PropertyID uint64
	Status     string
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
	rows := make([]room, 0)
	if err := query.Order("property_id ASC, room_label ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *landlordRentRepository) createRoom(ctx context.Context, userID uint64, row room) (room, error) {
	if _, err := r.findProperty(ctx, userID, row.PropertyID); err != nil {
		return room{}, err
	}
	row.ID = 0
	row.UserID = userID
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return room{}, err
	}
	return row, nil
}
