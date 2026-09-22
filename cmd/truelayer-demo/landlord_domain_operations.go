package main

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	errPropertyNameRequired = errors.New("property name is required")
	errRoomLabelRequired    = errors.New("room label is required")
)

type propertyInput struct {
	Name       string
	CityRegion string
	Address    string
	Timezone   string
	Notes      string
}

type roomInput struct {
	PropertyID uint64
	RoomLabel  string
	RoomType   string
	Capacity   int
	Notes      string
}

type landlordDomainService struct {
	db *gorm.DB
}

func newLandlordDomainService(db *gorm.DB) *landlordDomainService {
	return &landlordDomainService{db: db}
}

func (s *landlordDomainService) createProperty(ctx context.Context, userID uint64, input propertyInput) (property, error) {
	if userID == 0 {
		return property{}, errLandlordRentUserRequired
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return property{}, errPropertyNameRequired
	}
	if len([]rune(input.Name)) > 191 || len([]rune(input.CityRegion)) > 191 || len([]rune(input.Address)) > 1000 || len([]rune(input.Timezone)) > 64 || len([]rune(input.Notes)) > 2000 {
		return property{}, errors.New("property fields are too long")
	}
	timezone := firstNonEmpty(strings.TrimSpace(input.Timezone), "Europe/Dublin")
	if _, err := time.LoadLocation(timezone); err != nil {
		return property{}, errors.New("property timezone is invalid")
	}
	row := property{UserID: userID, Name: input.Name, CityRegion: strings.TrimSpace(input.CityRegion), Address: nullableString(strings.TrimSpace(input.Address)), Timezone: timezone, Notes: nullableString(strings.TrimSpace(input.Notes)), Status: "active"}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return property{}, err
	}
	return row, nil
}

func (s *landlordDomainService) updateProperty(ctx context.Context, userID, propertyID uint64, input propertyInput) (property, error) {
	if userID == 0 || propertyID == 0 {
		return property{}, errors.New("userID and propertyID are required")
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return property{}, errPropertyNameRequired
	}
	if len([]rune(input.Name)) > 191 || len([]rune(input.CityRegion)) > 191 || len([]rune(input.Address)) > 1000 || len([]rune(input.Timezone)) > 64 || len([]rune(input.Notes)) > 2000 {
		return property{}, errors.New("property fields are too long")
	}
	input.Timezone = firstNonEmpty(strings.TrimSpace(input.Timezone), "Europe/Dublin")
	if _, err := time.LoadLocation(input.Timezone); err != nil {
		return property{}, errors.New("property timezone is invalid")
	}
	var row property
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", propertyID, userID).First(&row).Error; err != nil {
			return err
		}
		return tx.Model(&row).Updates(map[string]any{
			"name": input.Name, "city_region": strings.TrimSpace(input.CityRegion),
			"address": nullableString(strings.TrimSpace(input.Address)), "timezone": input.Timezone,
			"notes": nullableString(strings.TrimSpace(input.Notes)),
		}).Error
	})
	return row, err
}

func (s *landlordDomainService) createRoom(ctx context.Context, userID uint64, input roomInput) (room, error) {
	if userID == 0 || input.PropertyID == 0 {
		return room{}, errors.New("userID and propertyID are required")
	}
	input.RoomLabel = strings.TrimSpace(input.RoomLabel)
	if input.RoomLabel == "" {
		return room{}, errRoomLabelRequired
	}
	input.RoomType = strings.TrimSpace(input.RoomType)
	if len([]rune(input.RoomLabel)) > 191 || len([]rune(input.RoomType)) > 64 || len([]rune(input.Notes)) > 2000 || input.Capacity < 0 || input.Capacity > 100 {
		return room{}, errors.New("room details are invalid")
	}
	if input.RoomType == "" {
		input.RoomType = "其他"
	}
	if input.Capacity == 0 {
		input.Capacity = 1
	}
	var row room
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var propertyRow property
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND id = ?", userID, input.PropertyID).First(&propertyRow).Error; err != nil {
			return err
		}
		row = room{UserID: userID, PropertyID: input.PropertyID, RoomLabel: input.RoomLabel, RoomType: input.RoomType, Capacity: input.Capacity, Notes: nullableString(strings.TrimSpace(input.Notes)), Status: "active"}
		return tx.Create(&row).Error
	})
	return row, err
}

func (s *landlordDomainService) updateRoom(ctx context.Context, userID, roomID uint64, input roomInput) (room, error) {
	if userID == 0 || roomID == 0 || input.PropertyID == 0 {
		return room{}, errors.New("userID, roomID, and propertyID are required")
	}
	input.RoomLabel = strings.TrimSpace(input.RoomLabel)
	input.RoomType = strings.TrimSpace(input.RoomType)
	if input.RoomLabel == "" {
		return room{}, errRoomLabelRequired
	}
	if len([]rune(input.RoomLabel)) > 191 || len([]rune(input.RoomType)) > 64 || len([]rune(input.Notes)) > 2000 || input.Capacity < 0 || input.Capacity > 100 {
		return room{}, errors.New("room details are invalid")
	}
	var row room
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND id = ?", userID, roomID).First(&row).Error; err != nil {
			return err
		}
		if row.PropertyID != input.PropertyID {
			var chargeCount int64
			if err := tx.Model(&rentCharge{}).Where("user_id = ? AND room_id = ?", userID, roomID).Count(&chargeCount).Error; err != nil {
				return err
			}
			if chargeCount > 0 {
				return ErrRoomPropertyLocked
			}
			var target property
			if err := tx.Where("user_id = ? AND id = ?", userID, input.PropertyID).First(&target).Error; err != nil {
				return err
			}
		}
		updates := map[string]any{
			"property_id": input.PropertyID,
			"room_label":  input.RoomLabel,
			"room_type":   firstNonEmpty(input.RoomType, "其他"),
			"notes":       nullableString(strings.TrimSpace(input.Notes)),
		}
		if input.Capacity > 0 {
			updates["capacity"] = input.Capacity
		}
		return tx.Model(&row).Updates(updates).Error
	})
	return row, err
}

func (s *landlordDomainService) deactivateProperty(ctx context.Context, userID, propertyID uint64) error {
	if userID == 0 || propertyID == 0 {
		return errors.New("userID and propertyID are required")
	}
	return s.db.WithContext(ctx).Model(&property{}).Where("user_id = ? AND id = ?", userID, propertyID).Update("status", "inactive").Error
}

func (s *landlordDomainService) deactivateRoom(ctx context.Context, userID, roomID uint64, _ time.Time) error {
	if userID == 0 || roomID == 0 {
		return errors.New("userID and roomID are required")
	}
	return s.db.WithContext(ctx).Model(&room{}).Where("user_id = ? AND id = ?", userID, roomID).Update("status", "inactive").Error
}
