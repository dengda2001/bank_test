package main

import (
	"bytes"
	"mime/multipart"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestTransactionActionReadsBrowserFormDataAndOrdinaryForms(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range map[string]string{"transaction_id": "677", "reason": "首页暂不处理", "return_to": "/rent-dashboard?period=2026-09"} {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	multipartRequest := httptest.NewRequest("POST", "/transactions/defer", &body)
	multipartRequest.Header.Set("Content-Type", writer.FormDataContentType())
	id, err := transactionActionIDFromRequest(multipartRequest)
	if err != nil || id != 677 || multipartRequest.Form.Get("reason") != "首页暂不处理" {
		t.Fatalf("browser defer form id=%d fields=%v err=%v", id, multipartRequest.Form, err)
	}
	ordinaryRequest := httptest.NewRequest("POST", "/transactions/defer", strings.NewReader(url.Values{"transaction_id": {"677"}}.Encode()))
	ordinaryRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if id, err := transactionActionIDFromRequest(ordinaryRequest); err != nil || id != 677 {
		t.Fatalf("ordinary form id=%d err=%v", id, err)
	}
	missingID := httptest.NewRequest("POST", "/transactions/defer", strings.NewReader(url.Values{"reason": {"首页暂不处理"}}.Encode()))
	missingID.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if _, err := transactionActionIDFromRequest(missingID); err == nil {
		t.Fatal("missing transaction ID was accepted")
	}
}

func TestBatchMatchReadsBrowserFormData(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range map[string]string{
		"transaction_id": "71", "return_to": "/rent-dashboard?period=2026-09",
		"request_key": "batch-71", "match_tenant": "9",
		"tenant_id[]": "9", "period[]": "2026-09", "amount[]": "600.00",
	} {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("POST", "/transactions/confirm-batch", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	if err := parseTransactionForm(request); err != nil {
		t.Fatal(err)
	}
	if request.Form.Get("transaction_id") != "71" || len(allocationFormValues(request.Form, "tenant_id")) != 1 {
		t.Fatalf("browser FormData was not parsed: %v", request.Form)
	}
}

func TestExactShareRevokeReadsBrowserFormData(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range map[string]string{"allocation_id": "5", "transaction_id": "4", "idempotency_key": "share-5"} {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("POST", "/transactions/revoke-allocation", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	if err := parseTransactionForm(request); err != nil {
		t.Fatal(err)
	}
	if request.Form.Get("allocation_id") != "5" || request.Form.Get("transaction_id") != "4" {
		t.Fatalf("browser revoke FormData was not parsed: %v", request.Form)
	}
}
