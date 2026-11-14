package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDesktopUIServesIndex(t *testing.T) {
	h, err := NewHandler(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for desktop UI, got %d", rec.Code)
	}
}
