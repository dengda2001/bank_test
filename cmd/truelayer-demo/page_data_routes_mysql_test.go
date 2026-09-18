package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCanonicalPropertyPagesAreUserScopedOnMySQL(t *testing.T) {
	db, sqlDB := openLedgerMySQLTestDB(t)
	if err := runMigrations(sqlDB, "../../migrations"); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	ctx := context.Background()
	owner := user{Username: fmt.Sprintf("page-owner-%d", time.Now().UnixNano()), PasswordHash: "test"}
	other := user{Username: fmt.Sprintf("page-other-%d", time.Now().UnixNano()), PasswordHash: "test"}
	if err := db.WithContext(ctx).Create(&owner).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.WithContext(ctx).Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	repo := newLandlordRentRepository(db)
	ownerProperty, err := repo.createProperty(ctx, owner.ID, property{Name: "Owner only"})
	if err != nil {
		t.Fatal(err)
	}
	otherProperty, err := repo.createProperty(ctx, other.ID, property{Name: "Other only"})
	if err != nil {
		t.Fatal(err)
	}
	a := testApp()
	a.db = db
	a.cfg.Environment = "sandbox"
	handler := newAppMux(&a)
	cookie := userSessionCookie(a.cfg, owner.ID, owner.Username, time.Now().Add(sessionTTL))
	request := func(method, path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, nil)
		req.AddCookie(cookie)
		handler.ServeHTTP(rec, req)
		return rec
	}
	list := request(http.MethodGet, "/properties")
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), ownerProperty.Name) || strings.Contains(list.Body.String(), otherProperty.Name) {
		t.Fatalf("scoped property list status=%d body=%s", list.Code, list.Body.String())
	}
	otherDetail := request(http.MethodGet, fmt.Sprintf("/properties/%d", otherProperty.ID))
	if otherDetail.Code != http.StatusNotFound {
		t.Fatalf("cross-user property detail status=%d want %d", otherDetail.Code, http.StatusNotFound)
	}
}
