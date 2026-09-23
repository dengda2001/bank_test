package main

import (
	"bytes"
	"mime/multipart"
	"net/http/httptest"
	"testing"
)

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
	if err := parseRentMatchBatchForm(request); err != nil {
		t.Fatal(err)
	}
	if request.Form.Get("transaction_id") != "71" || len(allocationFormValues(request.Form, "tenant_id")) != 1 {
		t.Fatalf("browser FormData was not parsed: %v", request.Form)
	}
}
