// Package config loads settings from environment variables (12-factor style).
// See auth.go for the service configuration.
package config

import "os"

func getenv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
