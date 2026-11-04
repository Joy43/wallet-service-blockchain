package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"
)

type AuthConfig struct {
	HTTPAddr        string
	GRPCAddr        string
	DatabaseURL     string
	NATSURL         string
	RedisURL        string
	JWTPrivateKey   string // base64(PEM(PKCS#8 Ed25519)); see `make keygen`
	JWTKeyID        string
	JWTIssuer       string
	AccessTTL       time.Duration
	RefreshTTL      time.Duration
	UserCacheTTL    time.Duration
	LoginLimit      int
	LoginWindow     time.Duration
	BcryptCost      int
	AuditRetention  time.Duration
	ShutdownTimeout time.Duration
	LogLevel        slog.Level
}

func LoadAuth() (AuthConfig, error) {
	cfg := AuthConfig{
		HTTPAddr:      getenv("AUTH_HTTP_ADDR", ":8081"),
		GRPCAddr:      getenv("AUTH_GRPC_LISTEN_ADDR", ":9090"),
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		NATSURL:       getenv("NATS_URL", "nats://localhost:4222"),
		RedisURL:      getenv("REDIS_URL", "redis://localhost:6379/0"),
		JWTPrivateKey: os.Getenv("JWT_PRIVATE_KEY"),
		JWTKeyID:      getenv("JWT_KEY_ID", "auth-1"),
		JWTIssuer:     getenv("JWT_ISSUER", "auth-service"),
	}
	if cfg.DatabaseURL == "" {
		return AuthConfig{}, errors.New("DATABASE_URL is required")
	}
	if cfg.JWTPrivateKey == "" {
		return AuthConfig{}, errors.New("JWT_PRIVATE_KEY is required (generate one with: make keygen)")
	}

	var err error
	durations := []struct {
		dst      *time.Duration
		key, def string
	}{
		{&cfg.AccessTTL, "ACCESS_TOKEN_TTL", "15m"},
		{&cfg.RefreshTTL, "REFRESH_TOKEN_TTL", "720h"},
		{&cfg.UserCacheTTL, "USER_CACHE_TTL", "5m"},
		{&cfg.LoginWindow, "LOGIN_RATE_WINDOW", "1m"},
		{&cfg.AuditRetention, "AUDIT_RETENTION", "2160h"},
		{&cfg.ShutdownTimeout, "SHUTDOWN_TIMEOUT", "15s"},
	}
	for _, d := range durations {
		if *d.dst, err = time.ParseDuration(getenv(d.key, d.def)); err != nil {
			return AuthConfig{}, fmt.Errorf("parse %s: %w", d.key, err)
		}
	}
	if cfg.LoginLimit, err = strconv.Atoi(getenv("LOGIN_RATE_LIMIT", "10")); err != nil {
		return AuthConfig{}, fmt.Errorf("parse LOGIN_RATE_LIMIT: %w", err)
	}
	if cfg.BcryptCost, err = strconv.Atoi(getenv("BCRYPT_COST", "12")); err != nil {
		return AuthConfig{}, fmt.Errorf("parse BCRYPT_COST: %w", err)
	}
	if err := cfg.LogLevel.UnmarshalText([]byte(getenv("LOG_LEVEL", "INFO"))); err != nil {
		return AuthConfig{}, fmt.Errorf("parse LOG_LEVEL: %w", err)
	}
	return cfg, nil
}
