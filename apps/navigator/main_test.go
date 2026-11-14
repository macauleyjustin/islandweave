package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolveIslandURIAndUI(t *testing.T) {
	r1 := ResolveIslandURI("island://home", "127.0.0.1:47475")
	if r1.Kind != "native" || r1.View != "home" {
		t.Fatalf("unexpected resolution for home: %+v", r1)
	}
	r2 := ResolveIslandURI("island://harbor-news/about.html", "127.0.0.1:47475")
	if r2.Kind != "site" || r2.GatewayURL != "http://127.0.0.1:47475/site/harbor-news/about.html" {
		t.Fatalf("unexpected resolution for site: %+v", r2)
	}
	r3 := ResolveIslandURI("https://example.com", "127.0.0.1:47475")
	if r3.Kind != "https" {
		t.Fatalf("unexpected resolution for https: %+v", r3)
	}

	h, err := NewHandler(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for navigator UI, got %d", rec.Code)
	}
}
