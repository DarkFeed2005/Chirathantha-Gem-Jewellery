// Package config loads runtime configuration from environment variables.
// Nothing here talks to the network or the database — it's pure, so it's
// trivial to unit test and to override in CI.
package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port string

	DatabaseURL       string
	DBMaxConns        int32
	DBMinConns        int32
	DBConnectTimeout  time.Duration
	DBMaxConnLifetime time.Duration

	// JWTSecret signs and verifies access tokens (see internal/middleware).
	// Required, like DatabaseURL — an insecure baked-in default here would
	// mean every deployment that forgets to set it shares the same key.
	JWTSecret []byte
	// AccessTokenTTL controls how long a signup/login/refresh-issued
	// access token is valid — short by design (default 15m), since it
	// travels in a cookie sent on every request and a compromised one is
	// only useful for this long. RefreshTokenTTL controls the much
	// longer-lived, revocable, DB-backed refresh token used to silently
	// mint new access tokens without forcing a re-login — see
	// AuthService.RefreshAccessToken and migrations/0002_refresh_tokens.sql.
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration

	// AllowedOrigins configures CORS for the Next.js frontend.
	AllowedOrigins []string

	// --- Notification providers (all optional) ---------------------------
	// Any of these left unset means that channel falls back to
	// LogNotifier — see buildNotifier in cmd/api/main.go. Unlike
	// DatabaseURL/JWTSecret, the app is fully usable without these; it
	// just won't actually deliver email/SMS.
	SendGridAPIKey   string
	EmailFromAddress string
	EmailFromName    string

	TwilioAccountSID string
	TwilioAuthToken  string
	TwilioFromNumber string
}

// Load reads configuration from the environment, applying sane defaults
// for local development. DATABASE_URL and JWT_SECRET are the only
// variables that MUST be set explicitly — we refuse to guess credentials
// or secrets.
func Load() (*Config, error) {
	loadDotEnv(".env")

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, fmt.Errorf("config: DATABASE_URL is required, e.g. " +
			"postgres://user:pass@localhost:5432/gemstore?sslmode=disable")
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if len(jwtSecret) < 32 {
		return nil, fmt.Errorf("config: JWT_SECRET is required and must be at least 32 characters " +
			"(e.g. `openssl rand -base64 32`)")
	}

	cfg := &Config{
		Port:              getEnv("PORT", "8080"),
		DatabaseURL:       dbURL,
		DBMaxConns:        getEnvInt32("DB_MAX_CONNS", 10),
		DBMinConns:        getEnvInt32("DB_MIN_CONNS", 2),
		DBConnectTimeout:  getEnvDuration("DB_CONNECT_TIMEOUT", 5*time.Second),
		DBMaxConnLifetime: getEnvDuration("DB_MAX_CONN_LIFETIME", time.Hour),
		JWTSecret:         []byte(jwtSecret),
		AccessTokenTTL:    getEnvDuration("ACCESS_TOKEN_TTL", 15*time.Minute),
		RefreshTokenTTL:   getEnvDuration("REFRESH_TOKEN_TTL", 7*24*time.Hour),
		AllowedOrigins:    []string{getEnv("FRONTEND_ORIGIN", "http://localhost:3000")},

		SendGridAPIKey:   os.Getenv("SENDGRID_API_KEY"),
		EmailFromAddress: getEnv("EMAIL_FROM_ADDRESS", "orders@example.com"),
		EmailFromName:    getEnv("EMAIL_FROM_NAME", "Gem & Jewelry Store"),

		TwilioAccountSID: os.Getenv("TWILIO_ACCOUNT_SID"),
		TwilioAuthToken:  os.Getenv("TWILIO_AUTH_TOKEN"),
		TwilioFromNumber: os.Getenv("TWILIO_PHONE_NUMBER"),
	}

	return cfg, nil
}

// loadDotEnv populates any unset environment variable from a `KEY=VALUE`
// file, so `go run ./cmd/api` works from a fresh checkout without the
// shell gymnastics the README used to prescribe (`export $(cat .env | xargs)`).
//
// Deliberately NOT a hard requirement, and deliberately never overwrites:
//   - A missing .env is not an error. Real deployments (containers, CI,
//     systemd) inject the environment directly and have no .env at all, so
//     requiring one would break them.
//   - Variables already present in the real environment win. That keeps
//     `DATABASE_URL=... go run ./cmd/api` and container env injection
//     authoritative, rather than a stale committed .env silently winning
//     over what the operator explicitly asked for.
//
// This is a deliberate ~40-line stdlib-only parser rather than a godotenv
// dependency: the whole format is "skip blanks/comments, split on the first
// =", and taking a dependency for that isn't worth it.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")

		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}

		value = strings.TrimSpace(value)
		// Strip one layer of matching quotes, so a value containing
		// spaces or a '#' survives intact.
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}

		if _, alreadySet := os.LookupEnv(key); !alreadySet {
			_ = os.Setenv(key, value)
		}
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt32(key string, fallback int32) int32 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(v, 10, 32)
	if err != nil {
		return fallback
	}
	return int32(parsed)
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return parsed
}
