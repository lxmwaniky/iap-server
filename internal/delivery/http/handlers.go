package http

import (
	"context"
	"crypto/hmac"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/lxmwaniky/iap-server/internal/domain"
)

type Authenticator interface {
	Authenticate(ctx context.Context, token string) (*domain.User, error)
}

type anonymousSessionIssuer interface {
	NewSession() (*domain.AnonymousSession, error)
}

type Handler struct {
	usecase       domain.IAPUsecase
	authenticator Authenticator
}

type verifyPurchaseRequest struct {
	ProductID     string              `json:"product_id"`
	PurchaseToken string              `json:"purchase_token"`
	Type          domain.PurchaseType `json:"type"`
}

func NewHandler(usecase domain.IAPUsecase, authenticator Authenticator) *Handler {
	return &Handler{
		usecase:       usecase,
		authenticator: authenticator,
	}
}

func (h *Handler) CreateAnonymousSession(w http.ResponseWriter, r *http.Request) {
	issuer, ok := h.authenticator.(anonymousSessionIssuer)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Not found"})
		return
	}
	session, err := issuer.NewSession()
	if err != nil {
		slog.Error("failed to create anonymous session", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal server error"})
		return
	}
	writeJSON(w, http.StatusCreated, session)
}

func (h *Handler) VerifyPurchase(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticate(w, r)
	if !ok {
		return
	}

	var req verifyPurchaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Warn("invalid verify purchase payload", "error", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request payload"})
		return
	}
	if req.ProductID == "" || req.PurchaseToken == "" || req.Type == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "product_id, purchase_token, and type are required"})
		return
	}

	result, err := h.usecase.VerifyPurchase(r.Context(), domain.VerifyPurchaseRequest{
		UserID:        user.ID,
		ProductID:     req.ProductID,
		PurchaseToken: req.PurchaseToken,
		Type:          req.Type,
	})
	if err != nil {
		h.writeUsecaseError(w, err, "failed to verify purchase")
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) GetEntitlement(w http.ResponseWriter, r *http.Request) {
	user, ok := h.authenticate(w, r)
	if !ok {
		return
	}

	productID := r.PathValue("productID")
	if productID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "product_id is required"})
		return
	}

	entitlement, err := h.usecase.GetEntitlement(r.Context(), user.ID, productID)
	if err != nil {
		h.writeUsecaseError(w, err, "failed to get entitlement")
		return
	}
	if entitlement == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Entitlement not found"})
		return
	}
	writeJSON(w, http.StatusOK, entitlement)
}

func (h *Handler) HandleRTDN(w http.ResponseWriter, r *http.Request) {
	token := bearerToken(r.Header.Get("Authorization"))
	expected := r.Context().Value(rtdnTokenKey{})
	if expectedToken, ok := expected.(string); ok && expectedToken != "" && !constantTimeEqual(token, expectedToken) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	notification, err := decodePlayNotification(r)
	if err != nil {
		slog.Warn("invalid rtdn payload", "error", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid notification payload"})
		return
	}
	if notification.ReceivedAt.IsZero() {
		notification.ReceivedAt = time.Now().UTC()
	}
	if err := h.usecase.ProcessNotification(r.Context(), notification); err != nil {
		h.writeUsecaseError(w, err, "failed to process notification")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "OK"})
}

func (h *Handler) Healthz(w http.ResponseWriter, r *http.Request) {
	if err := h.usecase.Ping(r.Context()); err != nil {
		slog.Error("health check failed", "error", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "DOWN"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "UP"})
}

func (h *Handler) authenticate(w http.ResponseWriter, r *http.Request) (*domain.User, bool) {
	token := authTokenFromRequest(r)
	if token == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return nil, false
	}

	user, err := h.authenticator.Authenticate(r.Context(), token)
	if err != nil {
		slog.Warn("unauthorized request", "error", err)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return nil, false
	}
	return user, true
}

func authTokenFromRequest(r *http.Request) string {
	if token := bearerToken(r.Header.Get("Authorization")); token != "" {
		return token
	}
	return r.Header.Get("X-IAP-User-Token")
}

func bearerToken(authHeader string) string {
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}
	return ""
}

func constantTimeEqual(got, want string) bool {
	return hmac.Equal([]byte(got), []byte(want))
}

func (h *Handler) writeUsecaseError(w http.ResponseWriter, err error, logMessage string) {
	switch {
	case errors.Is(err, domain.ErrInvalidPurchase):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid purchase"})
	case errors.Is(err, domain.ErrPurchaseOwnedByAnotherUser):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "Purchase belongs to another user"})
	default:
		slog.Error(logMessage, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal server error"})
	}
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

type pubsubPushRequest struct {
	Message struct {
		Data       string            `json:"data"`
		MessageID  string            `json:"messageId"`
		Attributes map[string]string `json:"attributes"`
	} `json:"message"`
}

type playDeveloperNotification struct {
	PackageName              string `json:"packageName"`
	SubscriptionNotification *struct {
		NotificationType int    `json:"notificationType"`
		PurchaseToken    string `json:"purchaseToken"`
		SubscriptionID   string `json:"subscriptionId"`
	} `json:"subscriptionNotification"`
	OneTimeProductNotification *struct {
		NotificationType int    `json:"notificationType"`
		PurchaseToken    string `json:"purchaseToken"`
		SKU              string `json:"sku"`
	} `json:"oneTimeProductNotification"`
}

func decodePlayNotification(r *http.Request) (*domain.PlayNotification, error) {
	var push pubsubPushRequest
	if err := json.NewDecoder(r.Body).Decode(&push); err != nil {
		return nil, err
	}
	if push.Message.Data == "" {
		return nil, errors.New("missing pubsub message data")
	}

	raw, err := base64.StdEncoding.DecodeString(push.Message.Data)
	if err != nil {
		return nil, err
	}

	var playNotification playDeveloperNotification
	if err := json.Unmarshal(raw, &playNotification); err != nil {
		return nil, err
	}

	notification := &domain.PlayNotification{
		MessageID:   push.Message.MessageID,
		PackageName: playNotification.PackageName,
		ReceivedAt:  time.Now().UTC(),
	}
	if notification.MessageID == "" && push.Message.Attributes != nil {
		notification.MessageID = push.Message.Attributes["message_id"]
	}
	if sub := playNotification.SubscriptionNotification; sub != nil {
		notification.EventType = "SUBSCRIPTION"
		notification.ProductID = sub.SubscriptionID
		notification.PurchaseToken = sub.PurchaseToken
		return notification, nil
	}
	if product := playNotification.OneTimeProductNotification; product != nil {
		notification.EventType = "ONE_TIME_PRODUCT"
		notification.ProductID = product.SKU
		notification.PurchaseToken = product.PurchaseToken
		return notification, nil
	}
	return nil, errors.New("unsupported play notification type")
}
