package config

import (
	"bufio"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"	
)

type Config struct {
	Env                   string
	Port                  string
	DatabaseURL           string
	GooglePlayPackageName string
	AuthMode              string
	AnonymousTokenSecret  string
	AnonymousTokenTTL     time.Duration
	AuthJWTSecret         string
	RequireAPIKey         bool
	AppAPIKey             string
	RTDNVerificationToken string
	AllowedOrigins        string
	TrustedProxyCIDRs     []string
}

func LoadEnv(filenames ...string) error {
	filename := ".env"
	if len(filenames) > 0 {
		filename = filenames[0]
	}
	file, err := os.Open(filename)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("failed to open env file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		_ = os.Setenv(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to scan env file: %w", err)
	}
	return nil
}

func LoadConfig() (*Config, error) {
	dbUser := getEnv("DB_USER", "iap_user")
	dbPassword := getEnv("DB_PASSWORD", "iap_password")
	dbHost := getEnv("DB_HOST", "localhost")
	dbPort := getEnv("DB_PORT", "5432")
	dbName := getEnv("DB_NAME", "iap_db")
	defaultDBURL := buildDatabaseURL(dbUser, dbPassword, dbHost, dbPort, dbName)
	anonymousTTLHours, err := strconv.Atoi(getEnv("ANONYMOUS_TOKEN_TTL_HOURS", "8760"))
	if err != nil || anonymousTTLHours <= 0 {
		return nil, errors.New("ANONYMOUS_TOKEN_TTL_HOURS must be a positive integer")
	}

	cfg := &Config{
		Env:                   getEnv("APP_ENV", "development"),
		Port:                  getEnv("PORT", "8080"),
		DatabaseURL:           getEnv("DATABASE_URL", defaultDBURL),
		GooglePlayPackageName: getEnv("GOOGLE_PLAY_PACKAGE_NAME", ""),
		AuthMode:              getEnv("AUTH_MODE", "anonymous"),
		AnonymousTokenSecret:  getEnv("ANONYMOUS_TOKEN_SECRET", ""),
		AnonymousTokenTTL:     time.Duration(anonymousTTLHours) * time.Hour,
		AuthJWTSecret:         getEnv("AUTH_JWT_SECRET", ""),
		RequireAPIKey:         getEnv("REQUIRE_API_KEY", "false") == "true",
		AppAPIKey:             getEnv("API_KEY", ""),
		RTDNVerificationToken: getEnv("RTDN_VERIFICATION_TOKEN", ""),
		AllowedOrigins:        getEnv("ALLOWED_ORIGINS", "*"),
		TrustedProxyCIDRs:     splitCSV(getEnv("TRUSTED_PROXY_CIDRS", "")),
	}

	if cfg.GooglePlayPackageName == "" {
		return nil, errors.New("GOOGLE_PLAY_PACKAGE_NAME environment variable is required")
	}
	if cfg.AuthMode != "anonymous" && cfg.AuthMode != "jwt" {
		return nil, errors.New("AUTH_MODE must be anonymous or jwt")
	}
	if cfg.AuthMode == "jwt" && cfg.AuthJWTSecret == "" {
		return nil, errors.New("AUTH_JWT_SECRET environment variable is required")
	}
	if cfg.AuthMode == "anonymous" && cfg.AnonymousTokenSecret == "" {
		return nil, errors.New("ANONYMOUS_TOKEN_SECRET environment variable is required")
	}
	if cfg.RequireAPIKey && cfg.AppAPIKey == "" {
		return nil, errors.New("API_KEY environment variable is required when REQUIRE_API_KEY=true")
	}
	if cfg.Env == "production" && cfg.RTDNVerificationToken == "" {
		return nil, errors.New("RTDN_VERIFICATION_TOKEN environment variable is required in production")
	}

	return cfg, nil
}

func buildDatabaseURL(user, password, host, port, name string) string {
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, password),
		Host:   fmt.Sprintf("%s:%s", host, port),
		Path:   "/" + name,
	}
	q := u.Query()
	q.Set("sslmode", "disable")
	u.RawQuery = q.Encode()
	return u.String()
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func splitCSV(value string) []string {
	var out []string
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}
