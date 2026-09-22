package main

import (
	"errors"
	"sort"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrRentPlanFactsLocked = errors.New("rent plan facts have payment or dunning history")
	ErrRentFactsConflict   = errors.New("rent facts changed while acquiring financial locks")
)

func rentFactsLockedFromMonth(tx *gorm.DB, userID, roomID uint64, fromMonth time.Time) (bool, error) {
	if tx == nil || userID == 0 || roomID == 0 || fromMonth.IsZero() {
		return false, errors.New("transaction, user, room, and month are required")
	}
	fromMonth = monthStart(fromMonth)
	var locked bool
	err := tx.Raw(`
		SELECT EXISTS (
			SELECT 1
			FROM rent_obligations ro
			JOIN rent_charges rc
			  ON rc.user_id = ro.user_id
			 AND rc.id = ro.rent_charge_id
			WHERE rc.user_id = ?
			  AND rc.room_id = ?
			  AND rc.period_month >= ?
			  AND (
				EXISTS (SELECT 1 FROM payment_allocations pa WHERE pa.user_id = ro.user_id AND pa.rent_obligation_id = ro.id)
				OR EXISTS (SELECT 1 FROM cash_receipts cr WHERE cr.user_id = ro.user_id AND cr.rent_obligation_id = ro.id)
				OR EXISTS (SELECT 1 FROM dunning_send_attempts da WHERE da.user_id = ro.user_id AND da.rent_obligation_id = ro.id)
			  )
		)`, userID, roomID, fromMonth).Scan(&locked).Error
	return locked, err
}

// lockRentObligationRoomsInTx discovers the affected rooms, locks them in ID
// order, then locks and revalidates the charges and obligations. Callers that
// write bank allocations must lock their payment transaction before calling
// this helper; cash and dunning paths use lockRoomForRentObligation instead.
func lockRentObligationRoomsInTx(tx *gorm.DB, userID uint64, obligationIDs []uint64) (map[uint64]rentObligation, error) {
	if tx == nil || userID == 0 {
		return nil, errors.New("transaction and user are required")
	}
	seen := make(map[uint64]struct{}, len(obligationIDs))
	ids := make([]uint64, 0, len(obligationIDs))
	for _, id := range obligationIDs {
		if id == 0 {
			return nil, errors.New("rent obligation is required")
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return map[uint64]rentObligation{}, nil
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	var discoveredObligations []rentObligation
	if err := tx.Where("user_id = ? AND id IN ?", userID, ids).Order("id ASC").Find(&discoveredObligations).Error; err != nil {
		return nil, err
	}
	if len(discoveredObligations) != len(ids) {
		return nil, ErrRentFactsConflict
	}
	chargeIDs := make([]uint64, 0, len(discoveredObligations))
	seenCharges := make(map[uint64]struct{}, len(discoveredObligations))
	discoveredObligationByID := make(map[uint64]rentObligation, len(discoveredObligations))
	for _, obligation := range discoveredObligations {
		discoveredObligationByID[obligation.ID] = obligation
		if obligation.RentChargeID == 0 {
			return nil, ErrRentFactsConflict
		}
		if _, exists := seenCharges[obligation.RentChargeID]; !exists {
			seenCharges[obligation.RentChargeID] = struct{}{}
			chargeIDs = append(chargeIDs, obligation.RentChargeID)
		}
	}
	sort.Slice(chargeIDs, func(i, j int) bool { return chargeIDs[i] < chargeIDs[j] })
	var discoveredCharges []rentCharge
	if err := tx.Where("user_id = ? AND id IN ?", userID, chargeIDs).Order("id ASC").Find(&discoveredCharges).Error; err != nil {
		return nil, err
	}
	if len(discoveredCharges) != len(chargeIDs) {
		return nil, ErrRentFactsConflict
	}
	discoveredChargeByID := make(map[uint64]rentCharge, len(discoveredCharges))
	roomIDs := make([]uint64, 0, len(discoveredCharges))
	for _, charge := range discoveredCharges {
		discoveredChargeByID[charge.ID] = charge
		roomIDs = append(roomIDs, charge.RoomID)
	}
	sort.Slice(roomIDs, func(i, j int) bool { return roomIDs[i] < roomIDs[j] })
	uniqueRooms := roomIDs[:0]
	for _, roomID := range roomIDs {
		if len(uniqueRooms) == 0 || uniqueRooms[len(uniqueRooms)-1] != roomID {
			uniqueRooms = append(uniqueRooms, roomID)
		}
	}
	var rooms []room
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND id IN ?", userID, uniqueRooms).Order("id ASC").Find(&rooms).Error; err != nil {
		return nil, err
	}
	if len(rooms) != len(uniqueRooms) {
		return nil, ErrRentFactsConflict
	}
	roomByID := make(map[uint64]room, len(rooms))
	for _, roomRow := range rooms {
		roomByID[roomRow.ID] = roomRow
	}
	for _, charge := range discoveredCharges {
		if roomByID[charge.RoomID].ID == 0 || roomByID[charge.RoomID].PropertyID != charge.PropertyID {
			return nil, ErrRentFactsConflict
		}
	}
	// A plan edit may have completed while this writer waited for the room
	// locks. Lock charge rows next, then re-read obligations after those locks.
	var charges []rentCharge
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND id IN ?", userID, chargeIDs).Order("id ASC").Find(&charges).Error; err != nil {
		return nil, err
	}
	if len(charges) != len(chargeIDs) {
		return nil, ErrRentFactsConflict
	}
	chargeByID := make(map[uint64]rentCharge, len(charges))
	for _, charge := range charges {
		discovered := discoveredChargeByID[charge.ID]
		if discovered.ID == 0 || charge.RoomID != discovered.RoomID || charge.PropertyID != discovered.PropertyID || charge.RoomRentPlanID != discovered.RoomRentPlanID || !charge.PeriodMonth.Equal(discovered.PeriodMonth) {
			return nil, ErrRentFactsConflict
		}
		if roomByID[charge.RoomID].ID == 0 || roomByID[charge.RoomID].PropertyID != charge.PropertyID {
			return nil, ErrRentFactsConflict
		}
		chargeByID[charge.ID] = charge
	}
	var obligations []rentObligation
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND id IN ?", userID, ids).Order("id ASC").Find(&obligations).Error; err != nil {
		return nil, err
	}
	if len(obligations) != len(ids) {
		return nil, ErrRentFactsConflict
	}
	lockedObligations := make(map[uint64]rentObligation, len(obligations))
	for _, obligation := range obligations {
		discovered := discoveredObligationByID[obligation.ID]
		charge := chargeByID[obligation.RentChargeID]
		if discovered.ID == 0 || charge.ID == 0 || obligation.RentChargeID != discovered.RentChargeID || obligation.RoomRentPlanID != discovered.RoomRentPlanID || obligation.RoomRentPlanMemberID != discovered.RoomRentPlanMemberID || obligation.TenantID != discovered.TenantID || !obligation.PeriodMonth.Equal(discovered.PeriodMonth) || obligation.RentChargeID != charge.ID || obligation.RoomRentPlanID != charge.RoomRentPlanID || !obligation.PeriodMonth.Equal(charge.PeriodMonth) {
			return nil, ErrRentFactsConflict
		}
		lockedObligations[obligation.ID] = obligation
	}
	return lockedObligations, nil
}

// lockRoomForRentObligation serializes a financial write with rent-plan edits.
// Every write first discovers the charge without locking, then locks the room,
// charge, and obligation in that order and revalidates their relationships.
func lockRoomForRentObligation(tx *gorm.DB, userID, obligationID uint64) (rentObligation, rentCharge, error) {
	if tx == nil || userID == 0 || obligationID == 0 {
		return rentObligation{}, rentCharge{}, errors.New("transaction, user, and rent obligation are required")
	}
	var obligation rentObligation
	if err := tx.Where("user_id = ? AND id = ?", userID, obligationID).First(&obligation).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return rentObligation{}, rentCharge{}, ErrRentFactsConflict
		}
		return rentObligation{}, rentCharge{}, err
	}
	if obligation.RentChargeID == 0 {
		return rentObligation{}, rentCharge{}, gorm.ErrRecordNotFound
	}
	var charge rentCharge
	if err := tx.Where("user_id = ? AND id = ?", userID, obligation.RentChargeID).First(&charge).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return rentObligation{}, rentCharge{}, ErrRentFactsConflict
		}
		return rentObligation{}, rentCharge{}, err
	}
	var roomRow room
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND id = ?", userID, charge.RoomID).First(&roomRow).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return rentObligation{}, rentCharge{}, ErrRentFactsConflict
		}
		return rentObligation{}, rentCharge{}, err
	}
	if roomRow.PropertyID != charge.PropertyID {
		return rentObligation{}, rentCharge{}, ErrRentFactsConflict
	}
	var currentCharge rentCharge
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND id = ?", userID, charge.ID).First(&currentCharge).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return rentObligation{}, rentCharge{}, ErrRentFactsConflict
		}
		return rentObligation{}, rentCharge{}, err
	}
	if currentCharge.RoomID != charge.RoomID || currentCharge.PropertyID != charge.PropertyID || currentCharge.RoomRentPlanID != charge.RoomRentPlanID || !currentCharge.PeriodMonth.Equal(charge.PeriodMonth) || roomRow.ID != currentCharge.RoomID || roomRow.PropertyID != currentCharge.PropertyID {
		return rentObligation{}, rentCharge{}, ErrRentFactsConflict
	}
	var currentObligation rentObligation
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND id = ?", userID, obligationID).First(&currentObligation).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return rentObligation{}, rentCharge{}, ErrRentFactsConflict
		}
		return rentObligation{}, rentCharge{}, err
	}
	if currentObligation.RentChargeID != currentCharge.ID || currentObligation.RoomRentPlanID != currentCharge.RoomRentPlanID || !currentObligation.PeriodMonth.Equal(currentCharge.PeriodMonth) || currentObligation.TenantID != obligation.TenantID || currentObligation.RoomRentPlanMemberID != obligation.RoomRentPlanMemberID {
		return rentObligation{}, rentCharge{}, ErrRentFactsConflict
	}
	return currentObligation, currentCharge, nil
}

func deleteUnlockedRentFactsFromMonth(tx *gorm.DB, userID, roomID uint64, fromMonth time.Time) error {
	if tx == nil || userID == 0 || roomID == 0 || fromMonth.IsZero() {
		return errors.New("transaction, user, room, and month are required")
	}
	fromMonth = monthStart(fromMonth)
	locked, err := rentFactsLockedFromMonth(tx, userID, roomID, fromMonth)
	if err != nil {
		return err
	}
	if locked {
		return ErrRentPlanFactsLocked
	}
	chargeIDs := tx.Model(&rentCharge{}).
		Select("id").
		Where("user_id = ? AND room_id = ? AND period_month >= ?", userID, roomID, fromMonth)
	if err := tx.Where("user_id = ? AND rent_charge_id IN (?)", userID, chargeIDs).Delete(&rentObligation{}).Error; err != nil {
		return err
	}
	return tx.Where("user_id = ? AND room_id = ? AND period_month >= ?", userID, roomID, fromMonth).Delete(&rentCharge{}).Error
}
