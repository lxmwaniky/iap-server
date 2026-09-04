package main

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lxmwaniky/iap-server/config"
	"github.com/lxmwaniky/iap-server/internal/auth"
	delivery "github.com/lxmwaniky/iap-server/internal/delivery/http"
	"github.com/lxmwaniky/iap-server/internal/infrastructure/playbilling"
	"github.com/lxmwaniky/iap-server/internal/infrastructure/repository"
	"github.com/lxmwaniky/iap-server/internal/usecase"
)

func main() {
	if err := config.LoadEnv(); err != nil {
		log.Fatalf("env load error: %v", err)
	}
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	var logger *slog.Logger
	if cfg.Env == "production" {
		logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))
	} else {
		logger = slog.New(slog.NewTextHandler(os.Stdout, nil))
	}
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pgDB, err := repository.NewPostgresDB(cfg.DatabaseURL)
	if err != nil {
		slog.Error("failed to initialize postgres connection", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := pgDB.Close(); err != nil {
			slog.Error("failed to close database safely", "error", err)
		}
	}()

	playClient, err := playbilling.NewClient(ctx, cfg.GooglePlayPackageName)
	if err != nil {
		slog.Error("failed to initialize google play client", "error", err)
		os.Exit(1)
	}

	purchaseRepo := repository.NewPostgresPurchaseRepository(pgDB.DB)
	authenticator := newAuthenticator(cfg)
	iapUsecase := usecase.NewIAPUsecase(purchaseRepo, playClient)
	limiter := delivery.NewIPLimiter(cfg.TrustedProxyCIDRs...)
	limiter.StartCleanup(ctx)
	router := delivery.NewRouter(iapUsecase, authenticator, cfg, limiter)

	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		slog.Info("server starting", "port", cfg.Port)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server listener failed", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Warn("received termination signal")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("server graceful shutdown failed", "error", err)
		return
	}
	slog.Info("server stopped gracefully")
}

func newAuthenticator(cfg *config.Config) delivery.Authenticator {
	if cfg.AuthMode == "jwt" {
		return auth.NewJWTAuthenticator(cfg.AuthJWTSecret)
	}
	return auth.NewAnonymousAuthenticator(cfg.AnonymousTokenSecret, cfg.AnonymousTokenTTL)
}
