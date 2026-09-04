package http

import (
	"net/http"

	"github.com/lxmwaniky/iap-server/config"
	"github.com/lxmwaniky/iap-server/internal/domain"
)

func NewRouter(usecase domain.IAPUsecase, authenticator Authenticator, cfg *config.Config, limiter *IPLimiter) http.Handler {
	mux := http.NewServeMux()
	handler := NewHandler(usecase, authenticator)
	authMiddleware := APIKeyAuth(cfg)

	mux.HandleFunc("POST /api/v1/iap/anonymous-sessions", handler.CreateAnonymousSession)
	mux.Handle("POST /api/v1/iap/verify", authMiddleware(http.HandlerFunc(handler.VerifyPurchase)))
	mux.Handle("GET /api/v1/iap/entitlements/{productID}", authMiddleware(http.HandlerFunc(handler.GetEntitlement)))
	mux.Handle("POST /api/v1/iap/rtdn", WithRTDNToken(cfg)(http.HandlerFunc(handler.HandleRTDN)))
	mux.HandleFunc("GET /healthz", handler.Healthz)

	wrapped := limiter.RateLimit(mux)
	wrapped = Logger(wrapped)
	wrapped = SecurityHeaders(wrapped)
	return EnableCORS(cfg)(wrapped)
}
