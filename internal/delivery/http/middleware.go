package http

import (
	"context"
	"crypto/hmac"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/lxmwaniky/iap-server/config"
)

type rtdnTokenKey struct{}

type limiterEntry struct {
	count      int
	windowEnds time.Time
	lastActive time.Time
}

type IPLimiter struct {
	mu                sync.Mutex
	ips               map[string]*limiterEntry
	trustedProxyCIDRs []string
}

func NewIPLimiter(trustedProxyCIDRs ...string) *IPLimiter {
	return &IPLimiter{
		ips:               make(map[string]*limiterEntry),
		trustedProxyCIDRs: trustedProxyCIDRs,
	}
}

func (l *IPLimiter) StartCleanup(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Minute)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				l.cleanup()
			}
		}
	}()
}

func (l *IPLimiter) RateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r, l.trustedProxyCIDRs)
		if !l.allow(ip) {
			slog.Warn("rate limit exceeded", "ip", ip, "path", r.URL.Path)
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "Too many requests. Please slow down."})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (l *IPLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	entry, ok := l.ips[ip]
	if !ok {
		entry = &limiterEntry{
			count:      1,
			windowEnds: now.Add(10 * time.Second),
			lastActive: now,
		}
		l.ips[ip] = entry
		return true
	}
	if now.After(entry.windowEnds) {
		entry.count = 1
		entry.windowEnds = now.Add(10 * time.Second)
		entry.lastActive = now
		return true
	}

	entry.count++
	entry.lastActive = now
	return entry.count <= 5
}

func (l *IPLimiter) cleanup() {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	for ip, entry := range l.ips {
		if now.Sub(entry.lastActive) > 10*time.Minute {
			delete(l.ips, ip)
		}
	}
}

func APIKeyAuth(cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !cfg.RequireAPIKey {
				next.ServeHTTP(w, r)
				return
			}
			if !secretEqual(r.Header.Get("X-API-Key"), cfg.AppAPIKey) {
				slog.Warn("unauthorized api key attempt", "ip", r.RemoteAddr, "path", r.URL.Path)
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)

		fields := []any{
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.status,
			"duration_ms", time.Since(start).Milliseconds(),
			"ip", clientIP(r, nil),
		}
		switch {
		case rw.status >= 500:
			slog.Error("request failed", fields...)
		case rw.status >= 400:
			slog.Warn("request client error", fields...)
		default:
			slog.Info("request completed", fields...)
		}
	})
}

func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func EnableCORS(cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if origin := allowedOrigin(r.Header.Get("Origin"), cfg.AllowedOrigins); origin != "" {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				if origin != "*" {
					w.Header().Set("Vary", "Origin")
				}
			}
			w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-API-Key, X-IAP-User-Token")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusOK)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func allowedOrigin(requestOrigin, configuredOrigins string) string {
	configuredOrigins = strings.TrimSpace(configuredOrigins)
	if configuredOrigins == "*" {
		return "*"
	}
	for _, origin := range strings.Split(configuredOrigins, ",") {
		if strings.TrimSpace(origin) == requestOrigin {
			return requestOrigin
		}
	}
	return ""
}

func WithRTDNToken(cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), rtdnTokenKey{}, cfg.RTDNVerificationToken)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func clientIP(r *http.Request, trustedProxyCIDRs []string) string {
	remoteIP := remoteAddrIP(r.RemoteAddr)
	if remoteIP == "" {
		return r.RemoteAddr
	}
	if trustedProxy(remoteIP, trustedProxyCIDRs) {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			if len(parts) > 0 {
				return strings.TrimSpace(parts[0])
			}
		}
		if ip := r.Header.Get("X-Real-IP"); ip != "" {
			return ip
		}
	}
	return remoteIP
}

func remoteAddrIP(remoteAddr string) string {
	ip, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return ip
}

func trustedProxy(remoteIP string, trustedProxyCIDRs []string) bool {
	addr, err := netip.ParseAddr(remoteIP)
	if err != nil {
		return false
	}
	for _, cidr := range trustedProxyCIDRs {
		prefix, err := netip.ParsePrefix(cidr)
		if err != nil {
			continue
		}
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

func secretEqual(got, want string) bool {
	return hmac.Equal([]byte(got), []byte(want))
}
