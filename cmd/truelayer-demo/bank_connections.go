package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type bankConnection struct {
	ID                     uint64 `gorm:"primaryKey"`
	UserID                 uint64
	Provider               string
	Environment            string
	RefreshTokenCiphertext string
	RefreshTokenNonce      string
	TokenStorageMode       string
	SavedAt                *time.Time
	LastSyncAt             *time.Time
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type bankConnectionStore struct {
	db  *gorm.DB
	cfg config
}

func (s *bankConnectionStore) hasRefreshToken(ctx context.Context, userID uint64) (bool, error) {
	if userID == 0 {
		return false, errors.New("userID is required")
	}
	var row bankConnection
	err := s.db.WithContext(ctx).Where("user_id = ? AND provider = ? AND environment = ?", userID, "truelayer", s.cfg.Environment).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(row.RefreshTokenCiphertext) != "", nil
}

func newBankConnectionStore(db *gorm.DB, cfg config) *bankConnectionStore {
	return &bankConnectionStore{db: db, cfg: cfg}
}

func (s *bankConnectionStore) saveRefreshToken(ctx context.Context, userID uint64, refreshToken string) error {
	if userID == 0 {
		return errors.New("userID is required")
	}
	if strings.TrimSpace(refreshToken) == "" {
		return errors.New("refresh token is required")
	}
	ciphertext, nonce, mode, err := encodeRefreshToken(s.cfg, refreshToken)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	conn := bankConnection{
		UserID:                 userID,
		Provider:               "truelayer",
		Environment:            s.cfg.Environment,
		RefreshTokenCiphertext: ciphertext,
		RefreshTokenNonce:      nonce,
		TokenStorageMode:       mode,
		SavedAt:                &now,
	}
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "provider"}, {Name: "environment"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"refresh_token_ciphertext",
			"refresh_token_nonce",
			"token_storage_mode",
			"saved_at",
			"updated_at",
		}),
	}).Create(&conn).Error
}

func (s *bankConnectionStore) loadRefreshToken(ctx context.Context, userID uint64) (string, error) {
	if userID == 0 {
		return "", errors.New("userID is required")
	}
	var conn bankConnection
	err := s.db.WithContext(ctx).
		Where("user_id = ? AND provider = ? AND environment = ?", userID, "truelayer", s.cfg.Environment).
		First(&conn).Error
	if err != nil {
		return "", err
	}
	return decodeRefreshToken(s.cfg, conn.RefreshTokenCiphertext, conn.RefreshTokenNonce, conn.TokenStorageMode)
}

func (s *bankConnectionStore) markLastSync(ctx context.Context, userID uint64) error {
	if userID == 0 {
		return errors.New("userID is required")
	}
	now := time.Now().UTC()
	return s.db.WithContext(ctx).Model(&bankConnection{}).
		Where("user_id = ? AND provider = ? AND environment = ?", userID, "truelayer", s.cfg.Environment).
		Updates(map[string]any{"last_sync_at": now}).Error
}

func encodeRefreshToken(cfg config, refreshToken string) (ciphertext, nonceText, mode string, err error) {
	if cfg.AllowPlaintextTokens && cfg.BankTokenEncryptionKey == "" && cfg.Environment != "live" {
		return refreshToken, "", "plaintext_dev", nil
	}
	key, err := tokenEncryptionKeyBytes(cfg.BankTokenEncryptionKey)
	if err != nil {
		return "", "", "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", "", "", fmt.Errorf("create token cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", "", "", fmt.Errorf("create token gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", "", "", fmt.Errorf("generate token nonce: %w", err)
	}
	sealed := gcm.Seal(nil, nonce, []byte(refreshToken), nil)
	return base64.RawStdEncoding.EncodeToString(sealed), base64.RawStdEncoding.EncodeToString(nonce), "encrypted", nil
}

func decodeRefreshToken(cfg config, ciphertext, nonceText, mode string) (string, error) {
	if mode == "plaintext_dev" {
		if cfg.Environment == "live" || !cfg.AllowPlaintextTokens {
			return "", errors.New("plaintext token storage is not allowed")
		}
		return ciphertext, nil
	}
	if mode != "encrypted" {
		return "", fmt.Errorf("unsupported token storage mode %q", mode)
	}
	key, err := tokenEncryptionKeyBytes(cfg.BankTokenEncryptionKey)
	if err != nil {
		return "", err
	}
	sealed, err := base64.RawStdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("decode token ciphertext: %w", err)
	}
	nonce, err := base64.RawStdEncoding.DecodeString(nonceText)
	if err != nil {
		return "", fmt.Errorf("decode token nonce: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create token cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create token gcm: %w", err)
	}
	plain, err := gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt refresh token: %w", err)
	}
	return string(plain), nil
}
