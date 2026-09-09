package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type user struct {
	ID           uint64 `gorm:"primaryKey"`
	Username     string
	PasswordHash string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type authService struct {
	db  *gorm.DB
	cfg config
}

func newAuthService(db *gorm.DB, cfg config) *authService {
	return &authService{db: db, cfg: cfg}
}

func (s *authService) seedDefaultUser(ctx context.Context) error {
	username := strings.TrimSpace(s.cfg.AdminUsername)
	if username == "" {
		return errors.New("APP_ADMIN_USERNAME is required")
	}
	if s.cfg.AdminPassword == "" {
		return errors.New("APP_ADMIN_PASSWORD is required")
	}
	var existing user
	err := s.db.WithContext(ctx).Where("username = ?", username).First(&existing).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("find default user: %w", err)
	}
	hash, err := hashPassword(s.cfg.AdminPassword)
	if err != nil {
		return err
	}
	if err := s.db.WithContext(ctx).Create(&user{
		Username:     username,
		PasswordHash: hash,
	}).Error; err != nil {
		return fmt.Errorf("seed default user: %w", err)
	}
	return nil
}

func (s *authService) authenticate(ctx context.Context, username, password string) (user, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return user{}, errors.New("invalid credentials")
	}
	var found user
	err := s.db.WithContext(ctx).Where("username = ?", username).First(&found).Error
	if err != nil {
		return user{}, errors.New("invalid credentials")
	}
	if !verifyPassword(found.PasswordHash, password) {
		return user{}, errors.New("invalid credentials")
	}
	return found, nil
}

func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}

func verifyPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func userSessionCookie(cfg config, userID uint64, username string, expires time.Time) *http.Cookie {
	expiresUnix := strconv.FormatInt(expires.Unix(), 10)
	userIDText := strconv.FormatUint(userID, 10)
	value := strings.Join([]string{"v2", userIDText, username, expiresUnix, userSessionSignature(cfg, userIDText, username, expiresUnix)}, "|")
	return &http.Cookie{
		Name:     "rentops_session",
		Value:    value,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
}

func userSessionSignature(cfg config, userID, username, expiresUnix string) string {
	mac := hmac.New(sha256.New, []byte(cfg.SessionSecret))
	io.WriteString(mac, "v2")
	io.WriteString(mac, "|")
	io.WriteString(mac, userID)
	io.WriteString(mac, "|")
	io.WriteString(mac, username)
	io.WriteString(mac, "|")
	io.WriteString(mac, expiresUnix)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
