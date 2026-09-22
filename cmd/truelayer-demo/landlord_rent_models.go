package main

import "time"

type property struct {
	ID         uint64 `gorm:"primaryKey"`
	UserID     uint64
	Name       string
	CityRegion string
	Address    *string
	Timezone   string
	Notes      *string
	Status     string `gorm:"default:active"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (property) TableName() string {
	return "properties"
}

type room struct {
	ID              uint64 `gorm:"primaryKey"`
	UserID          uint64
	PropertyID      uint64
	RoomLabel       string
	RoomType        string
	Capacity        int
	Notes           *string
	Status          string `gorm:"default:active"`
	RentPlanVersion uint64
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (room) TableName() string {
	return "rooms"
}

type roomRentPlan struct {
	ID                 uint64 `gorm:"primaryKey"`
	UserID             uint64
	RoomID             uint64
	EffectiveFromMonth time.Time
	EffectiveToMonth   *time.Time
	MonthlyRentCents   int64
	Currency           string
	DueDay             int
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (roomRentPlan) TableName() string {
	return "room_rent_plans"
}

type roomRentPlanMember struct {
	ID                  uint64 `gorm:"primaryKey"`
	UserID              uint64
	RoomRentPlanID      uint64
	TenantID            uint64
	ResponsibilityCents int64
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

func (roomRentPlanMember) TableName() string {
	return "room_rent_plan_members"
}

type rentCharge struct {
	ID                   uint64 `gorm:"primaryKey"`
	UserID               uint64
	PropertyID           uint64
	RoomID               uint64
	RoomRentPlanID       uint64
	PeriodMonth          time.Time
	DueDate              time.Time
	ExpectedAmountCents  int64
	Currency             string
	RecordStatus         string `gorm:"default:active"`
	PropertyNameSnapshot *string
	RoomLabelSnapshot    *string
	RoomAddressSnapshot  *string
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

func (rentCharge) TableName() string {
	return "rent_charges"
}
