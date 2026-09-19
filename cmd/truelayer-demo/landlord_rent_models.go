package main

import "time"

type property struct {
	ID           uint64 `gorm:"primaryKey"`
	UserID       uint64
	Name         string
	CityRegion   string
	Address      *string
	Timezone     string
	Notes        *string
	Status       string `gorm:"default:active"`
	InactiveFrom *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (property) TableName() string {
	return "properties"
}

type room struct {
	ID               uint64 `gorm:"primaryKey"`
	UserID           uint64
	PropertyID       uint64
	RoomLabel        string
	RoomType         string
	Capacity         int
	MonthlyRentCents int64
	DueDay           int `gorm:"default:1"`
	Notes            *string
	Status           string `gorm:"default:active"`
	ActiveFrom       time.Time
	InactiveFrom     *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (room) TableName() string {
	return "rooms"
}

type tenancyAgreement struct {
	ID               uint64 `gorm:"primaryKey"`
	UserID           uint64
	RoomID           uint64
	ContractDate     *time.Time
	MoveInDate       *time.Time
	StartDate        time.Time
	EndDate          *time.Time
	MonthlyRentCents int64
	Currency         string
	DueDay           int
	Status           string `gorm:"default:active"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (tenancyAgreement) TableName() string {
	return "tenancy_agreements"
}

type agreementParty struct {
	ID                  uint64 `gorm:"primaryKey"`
	UserID              uint64
	AgreementID         uint64
	TenantID            uint64
	ResponsibilityCents int64
	JoinedAt            *time.Time
	LeftAt              *time.Time
	Status              string `gorm:"default:active"`
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

func (agreementParty) TableName() string {
	return "agreement_parties"
}

type rentCharge struct {
	ID                   uint64 `gorm:"primaryKey"`
	UserID               uint64
	PropertyID           uint64
	RoomID               uint64
	TenancyAgreementID   uint64
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
