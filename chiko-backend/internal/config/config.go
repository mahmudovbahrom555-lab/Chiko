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
	// Path to Firebase service account JSON key (downloaded from Firebase Console).
	// firebase-admin-go uses it to obtain and auto-refresh OAuth2 tokens.
	// Leave empty to disable push (local dev without Firebase).
	FCMServiceAccountJSON string
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
		DatabaseURL:           mustEnv("DATABASE_URL"),
		FCMServiceAccountJSON: getEnv("FCM_SERVICE_ACCOUNT_JSON", ""),
		ReturnSLAHours:        48,
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
