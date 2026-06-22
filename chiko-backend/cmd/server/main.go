package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/chiko/backend/internal/config"
	"github.com/chiko/backend/internal/middleware"
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	if os.Getenv("ENV") != "production" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load config")
	}

	mux := http.NewServeMux()
	registerRoutes(mux, cfg)

	// Цепочка middleware (порядок важен: Recovery → Logger → RateLimit → Auth → handler)
	handler := middleware.Recovery(
		middleware.Logger(
			mux,
		),
	)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Info().Str("addr", srv.Addr).Str("env", cfg.Env).Msg("server starting")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("server error")
		}
	}()

	<-quit
	log.Info().Msg("shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal().Err(err).Msg("forced shutdown")
	}

	log.Info().Msg("server stopped")
}

func registerRoutes(mux *http.ServeMux, cfg *config.Config) {
	authMiddleware := middleware.Auth(cfg.SupabaseJWTSecret)
	rateLimitMiddleware := middleware.RateLimit(100) // 100 req/min для REST

	// Health check — без auth, для load balancer / k8s probe
	mux.HandleFunc("GET /health", handleHealth)

	// Защищённые маршруты — обёрнуты в auth + rate limit
	protected := func(h http.HandlerFunc) http.Handler {
		return authMiddleware(rateLimitMiddleware(http.HandlerFunc(h)))
	}

	// Заглушки — заполняются в шагах 2.x – 4.x
	mux.Handle("GET /api/catalog/products",       protected(handleNotImplemented))
	mux.Handle("POST /api/catalog/products",      protected(handleNotImplemented))
	mux.Handle("GET /api/catalog/categories",     protected(handleNotImplemented))
	mux.Handle("GET /api/orders",                 protected(handleNotImplemented))
	mux.Handle("GET /api/debt/balance/{chat_id}", protected(handleNotImplemented))
	mux.Handle("GET /api/analytics/dashboard",    protected(handleNotImplemented))

	// WebSocket — шаг 2.1
	mux.Handle("GET /ws", authMiddleware(http.HandlerFunc(handleNotImplemented)))

	// Гостевой каталог — без auth (шаг 4.2)
	mux.HandleFunc("GET /api/guest/catalog/{producer_token}", handleNotImplemented)

	_ = cfg // используется в будущих шагах
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

func handleNotImplemented(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, `{"error":"not_implemented"}`, http.StatusNotImplemented)
}
