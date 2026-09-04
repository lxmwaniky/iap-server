package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lxmwaniky/iap-server/config"
)

func TestClientIP_IgnoresForwardedHeadersWithoutTrustedProxy(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.10:1234"
	req.Header.Set("X-Forwarded-For", "198.51.100.99")

	ip := clientIP(req, nil)

	if ip != "203.0.113.10" {
		t.Fatalf("expected remote address IP, got %s", ip)
	}
}

func TestClientIP_UsesForwardedHeaderFromTrustedProxy(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.5:1234"
	req.Header.Set("X-Forwarded-For", "198.51.100.99, 10.0.0.5")

	ip := clientIP(req, []string{"10.0.0.0/8"})

	if ip != "198.51.100.99" {
		t.Fatalf("expected forwarded client IP, got %s", ip)
	}
}

func TestAPIKeyAuth_UsesConfiguredSecret(t *testing.T) {
	cfg := &config.Config{RequireAPIKey: true, AppAPIKey: "secret"}
	called := false
	handler := APIKeyAuth(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-Key", "secret")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d", rec.Code)
	}
	if !called {
		t.Fatal("expected wrapped handler to be called")
	}
}

func TestEnableCORS_EchoesAllowedOrigin(t *testing.T) {
	cfg := &config.Config{AllowedOrigins: "https://a.example, https://b.example"}
	handler := EnableCORS(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	req.Header.Set("Origin", "https://b.example")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://b.example" {
		t.Fatalf("expected matching origin, got %s", got)
	}
	if got := rec.Header().Get("Vary"); got != "Origin" {
		t.Fatalf("expected Vary Origin, got %s", got)
	}
}
