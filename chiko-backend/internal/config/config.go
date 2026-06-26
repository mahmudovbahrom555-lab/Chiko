package config

import (
	"fmt"
	"os"
)

type Config struct {
	Port              string
	Env               string
	SupabaseURL       string
	SupabaseJWTSecret string
	SupabaseServiceKey string
	DatabaseURL       string
	FCMKey           string // OAuth2 Bearer token for FCM v1 API
	FCMProjectID     string // Firebase project ID, required for FCM v1 URL
	// SLA для возвратов (часов), читается из feature_flags при старте
	ReturnSLAHours int
}

func Load() (*Config, error) {
	cfg := &Config{
		Port:               getEnv("PORT", "8080"),
		Env:                getEnv("ENV", "development"),
		SupabaseURL:        mustEnv("SUPABASE_URL"),
		SupabaseJWTSecret:  mustEnv("SUPABASE_JWT_SECRET"),
		SupabaseServiceKey: mustEnv("SUPABASE_SERVICE_KEY"),
		DatabaseURL:        mustEnv("DATABASE_URL"),
		FCMKey:             getEnv("FCM_KEY", ""),
		FCMProjectID:       getEnv("FCM_PROJECT_ID", ""),
		ReturnSLAHours:     48,
	}
	return cfg, nil
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic(fmt.Sprintf("required env var %s is not set", key))
	}
	return v
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
