package config

import (
	"bufio"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestLoadEnv_ReturnsScannerError(t *testing.T) {
	path := t.TempDir() + "/.env"
	longLine := strings.Repeat("A", 70*1024)
	if err := os.WriteFile(path, []byte(longLine), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	err := LoadEnv(path)

	if err == nil {
		t.Fatal("expected scanner error")
	}
	if !errors.Is(err, bufio.ErrTooLong) {
		t.Fatalf("expected bufio.ErrTooLong, got %v", err)
	}
}

func TestLoadConfig_AnonymousModeDoesNotRequireJWTSecret(t *testing.T) {
	t.Setenv("AUTH_MODE", "anonymous")
	t.Setenv("GOOGLE_PLAY_PACKAGE_NAME", "com.example.app")
	t.Setenv("ANONYMOUS_TOKEN_SECRET", "anonymous-secret")
	t.Setenv("AUTH_JWT_SECRET", "")
	t.Setenv("APP_ENV", "development")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.AuthMode != "anonymous" {
		t.Fatalf("expected anonymous auth mode, got %s", cfg.AuthMode)
	}
}

func TestLoadConfig_JWTModeRequiresJWTSecret(t *testing.T) {
	t.Setenv("AUTH_MODE", "jwt")
	t.Setenv("GOOGLE_PLAY_PACKAGE_NAME", "com.example.app")
	t.Setenv("AUTH_JWT_SECRET", "")
	t.Setenv("APP_ENV", "development")

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected missing JWT secret error")
	}
}

func TestLoadConfig_AnonymousModeRequiresTokenSecret(t *testing.T) {
	t.Setenv("AUTH_MODE", "anonymous")
	t.Setenv("GOOGLE_PLAY_PACKAGE_NAME", "com.example.app")
	t.Setenv("ANONYMOUS_TOKEN_SECRET", "")
	t.Setenv("APP_ENV", "development")

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected missing anonymous token secret error")
	}
}

func TestLoadConfig_DoesNotExposeGoogleCredentialsPath(t *testing.T) {
	t.Setenv("AUTH_MODE", "anonymous")
	t.Setenv("GOOGLE_PLAY_PACKAGE_NAME", "com.example.app")
	t.Setenv("ANONYMOUS_TOKEN_SECRET", "anonymous-secret")
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "/tmp/service-account.json")
	t.Setenv("APP_ENV", "development")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.GooglePlayPackageName != "com.example.app" {
		t.Fatalf("expected package name to load, got %s", cfg.GooglePlayPackageName)
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
