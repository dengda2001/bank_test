package main

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testOfficeFile(t *testing.T, mainPart, contentType string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, content := range map[string]string{"[Content_Types].xml": `<Types><Override ContentType="` + contentType + `"/></Types>`, mainPart: `<document/>`} {
		part, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestExpenseAttachmentValidatesContentAndOfficeContainer(t *testing.T) {
	for _, tc := range []struct{ name, part, kind string }{
		{"record.docx", "word/document.xml", "wordprocessingml.document.main+xml"},
		{"record.xlsx", "xl/workbook.xml", "spreadsheetml.sheet.main+xml"},
		{"record.pptx", "ppt/presentation.xml", "presentationml.presentation.main+xml"},
	} {
		file := testOfficeFile(t, tc.part, tc.kind)
		got, err := parseExpenseAttachment(tc.name, file)
		if err != nil || got.ContentType == "" {
			t.Fatalf("%s: %+v, %v", tc.name, got, err)
		}
		if _, err := parseExpenseAttachment("wrong.docx", testOfficeFile(t, "xl/workbook.xml", "spreadsheetml.sheet.main+xml")); err == nil {
			t.Fatal("wrong Office container accepted")
		}
	}
	if _, err := parseExpenseAttachment("../../receipt.pdf", []byte("%PDF-1.7\nrecord")); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"script.html", []byte("<script>alert(1)</script>")},
		{"fake.pdf", []byte("<html>fake</html>")},
		{"huge.pdf", make([]byte, maxExpenseAttachmentBytes+1)},
	} {
		if _, err := parseExpenseAttachment(tc.name, tc.data); err == nil {
			t.Fatalf("accepted %s", tc.name)
		}
	}
}

func TestExpenseAttachmentMigrationIsFlatRunnerCompatible(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "migrations", "018_expense_attachments.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if len(splitSQLStatements(string(data))) != 1 || strings.Contains(string(data), "--") {
		t.Fatal("attachment migration must be one flat SQL statement")
	}
}

func TestExpenseCreateStoresMultipleAttachmentsOnMySQL(t *testing.T) {
	f := newRoomRentPlanConflictFixture(t)
	t.Cleanup(func() {
		_ = f.db.WithContext(f.ctx).Exec("DELETE FROM expense_attachments WHERE user_id = ?", f.owner.ID).Error
		_ = f.db.WithContext(f.ctx).Exec("DELETE FROM manual_expenses WHERE user_id = ?", f.owner.ID).Error
		_ = f.db.WithContext(f.ctx).Exec("DELETE FROM payment_transactions WHERE user_id = ?", f.owner.ID).Error
	})
	pdf, err := parseExpenseAttachment("receipt.pdf", []byte("%PDF-1.7\nreceipt"))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := parseExpenseAttachment("notes.docx", testOfficeFile(t, "word/document.xml", "wordprocessingml.document.main+xml"))
	if err != nil {
		t.Fatal(err)
	}
	propertyID := f.roomOne.PropertyID
	expense, err := newExpenseService(f.db).createExpense(f.ctx, f.owner.ID, expenseInput{PropertyID: &propertyID, Description: fmt.Sprintf("Attachment expense %d", time.Now().UnixNano()), Category: "维修", Amount: 12, Currency: "EUR", ExpenseDate: "2026-09-24", PaymentMethod: "Manual", Attachments: []expenseAttachmentUpload{pdf, doc}})
	if err != nil {
		t.Fatal(err)
	}
	files, err := listExpenseAttachments(f.ctx, f.db, f.owner.ID, []uint64{expense.ID})
	if err != nil || len(files) != 2 {
		t.Fatalf("files=%+v err=%v", files, err)
	}
	if other, err := listExpenseAttachments(context.Background(), f.db, f.owner.ID+1, []uint64{expense.ID}); err != nil || len(other) != 0 {
		t.Fatalf("other account files=%+v err=%v", other, err)
	}
}
