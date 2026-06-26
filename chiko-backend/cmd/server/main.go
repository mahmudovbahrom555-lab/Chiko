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

	"github.com/chiko/backend/internal/analytics"
	"github.com/chiko/backend/internal/catalog"
	"github.com/chiko/backend/internal/demand"
	chatpkg "github.com/chiko/backend/internal/chat"
	"github.com/chiko/backend/internal/config"
	"github.com/chiko/backend/internal/debt"
	"github.com/chiko/backend/internal/guest"
	"github.com/chiko/backend/internal/middleware"
	"github.com/chiko/backend/internal/order"
	"github.com/chiko/backend/internal/push"
	"github.com/chiko/backend/internal/subscription"
	"github.com/chiko/backend/internal/users"
	"github.com/chiko/backend/internal/ws"
	"github.com/chiko/backend/pkg/db"
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

	// ── Database ──────────────────────────────────────────────────────────────
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool, err := db.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to database")
	}
	defer pool.Close()
	log.Info().Msg("database connected")

	// ── WebSocket Hub ─────────────────────────────────────────────────────────
	hub := ws.NewHub()
	go hub.Run(ctx)

	// ── Push notifications (Шаг 4.1) ─────────────────────────────────────────
	pushSvc := push.NewService(cfg.FCMServiceAccountJSON, pool)

	// ── Background jobs ───────────────────────────────────────────────────────
	debtSvc := debt.NewService(pool, hub)
	debt.StartSLAJob(ctx, debtSvc, pushSvc)
	subscription.StartTrialExpiryJob(ctx, pool)

	// ── HTTP server ───────────────────────────────────────────────────────────
	mux := http.NewServeMux()
	registerRoutes(mux, cfg, pool, hub, debtSvc, pushSvc)

	handler := middleware.Recovery(middleware.Logger(mux))

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second, // voice uploads may take longer
		IdleTimeout:  60 * time.Second,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Info().Str("addr", srv.Addr).Str("env", cfg.Env).Msg("server starting")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("server error")
		}
	}()

	<-quit
	log.Info().Msg("shutting down...")

	// Shutdown order matters: stop accepting new HTTP connections FIRST,
	// then cancel the context so background goroutines (hub, jobs) exit cleanly.
	// Reversing the order would close WS connections before in-flight handlers finish.
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()

	if err := srv.Shutdown(shutCtx); err != nil {
		log.Fatal().Err(err).Msg("forced shutdown")
	}
	cancel()
	log.Info().Msg("server stopped")
}

func registerRoutes(mux *http.ServeMux, cfg *config.Config, pool *db.Pool, hub *ws.Hub, debtSvc *debt.Service, pushSvc *push.Service) {
	authMW := middleware.Auth(cfg.SupabaseJWTSecret)
	rateMW := middleware.RateLimit(100)

	protected := func(h http.HandlerFunc) http.Handler {
		return authMW(rateMW(http.HandlerFunc(h)))
	}

	// ── Health ────────────────────────────────────────────────────────────────
	mux.HandleFunc("GET /health", handleHealth)

	// ── WebSocket (Шаг 2.1) ───────────────────────────────────────────────────
	mux.Handle("GET /ws", authMW(ws.Handler(hub, pool)))

	// ── Шаг 1.5: Bootstrap + Users ────────────────────────────────────────────
	storageURL := os.Getenv("SUPABASE_STORAGE_URL")
	chatSvc := chatpkg.NewService(pool, hub, storageURL, cfg.SupabaseServiceKey)
	chatHandler := chatpkg.NewHandler(chatSvc)
	usersHandler := users.NewHandler(pool, chatSvc)

	mux.Handle("POST /api/auth/bootstrap",          authMW(http.HandlerFunc(usersHandler.Bootstrap)))
	mux.Handle("PUT /api/users/push-token",         protected(usersHandler.UpdatePushToken))
	mux.Handle("GET /api/producers/me/guest-link",  protected(usersHandler.GetGuestLink))

	// ── Шаг 1.5: Chats + Messages ─────────────────────────────────────────────
	mux.Handle("GET /api/chats",              protected(chatHandler.ListChats))
	mux.Handle("POST /api/chats",             protected(chatHandler.CreateChat))
	mux.Handle("GET /api/messages",           protected(chatHandler.ListMessages))
	mux.Handle("POST /api/messages",          protected(chatHandler.SendText))
	mux.Handle("POST /api/messages/voice",    protected(chatHandler.SendVoice))

	// ── Каталог (Шаг 2.2) ────────────────────────────────────────────────────
	cat := catalog.NewHandler(catalog.NewService(pool))

	mux.Handle("GET /api/catalog/categories",                protected(cat.ListCategories))
	mux.Handle("POST /api/catalog/categories",               protected(cat.CreateCategory))
	mux.Handle("GET /api/catalog/products",                  protected(cat.ListProducts))
	mux.Handle("POST /api/catalog/products",                 protected(cat.CreateProduct))
	mux.Handle("PUT /api/catalog/products/{id}",             protected(cat.UpdateProduct))
	mux.Handle("DELETE /api/catalog/products/{id}",          protected(cat.DeleteProduct))
	mux.Handle("GET /api/catalog/template",                  protected(cat.DownloadTemplate))
	mux.Handle("POST /api/catalog/import",                   protected(cat.ImportProducts))
	mux.Handle("GET /api/catalog/export",                    protected(cat.ExportCatalog))
	mux.Handle("PUT /api/producers/{id}/currency",           protected(cat.SetCurrency))
	mux.Handle("GET /api/catalog/currencies",                protected(cat.SearchCurrencies))

	// demandSvc создаётся здесь, чтобы его можно было передать в order.Handler.
	demandSvc := demand.NewService(pool, hub)

	// ── Заказы (Шаг 3.1) ─────────────────────────────────────────────────────
	ord := order.NewHandler(order.NewService(pool, hub), pushSvc, demandSvc)

	mux.Handle("POST /api/orders",                           protected(ord.CreateDraft))
	mux.Handle("GET /api/orders",                            protected(ord.ListByChat))
	mux.Handle("GET /api/orders/{id}",                       protected(ord.GetOrder))
	mux.Handle("PUT /api/orders/{id}/items",                 protected(ord.UpsertItem))
	mux.Handle("DELETE /api/orders/{id}/items/{item_id}",    protected(ord.RemoveItem))
	mux.Handle("POST /api/orders/{id}/confirm",              protected(ord.Confirm))
	mux.Handle("POST /api/orders/repeat",                    protected(ord.Repeat))

	// ── Долг (Шаг 3.2) ────────────────────────────────────────────────────────
	dbt := debt.NewHandler(debtSvc, pushSvc)

	mux.Handle("GET /api/debt/balance/{chat_id}",                    protected(dbt.GetBalance))
	mux.Handle("GET /api/debt/history/{chat_id}",                    protected(dbt.ListHistory))
	mux.Handle("POST /api/debt/delivery",                            protected(dbt.CreateDelivery))
	mux.Handle("POST /api/debt/payment",                             protected(dbt.CreatePayment))
	mux.Handle("POST /api/debt/transactions/{id}/confirm",           protected(dbt.ConfirmTx))
	mux.Handle("POST /api/debt/transactions/{id}/dispute",           protected(dbt.DisputeTx))
	mux.Handle("POST /api/debt/returns",                             protected(dbt.CreateReturnRequest))
	mux.Handle("POST /api/debt/returns/{id}/correct",                protected(dbt.CreateReturnCorrection))
	mux.Handle("POST /api/debt/transactions/{id}/dispute-correction", protected(dbt.DisputeCorrection))

	// ── Список спроса — Вариант Б ────────────────────────────────────────────
	dem := demand.NewHandler(demandSvc)
	mux.Handle("GET /api/demand",                    protected(dem.List))
	mux.Handle("POST /api/demand",                   protected(dem.Add))
	mux.Handle("PUT /api/demand/{id}",               protected(dem.Update))
	mux.Handle("DELETE /api/demand/{id}",            protected(dem.Remove))
	mux.Handle("GET /api/demand/suggestions",        protected(dem.GetSuggestions))
	mux.Handle("POST /api/demand/create-draft",      protected(dem.CreateDraft))

	// ── Аналитика (Шаг 4.2) ──────────────────────────────────────────────────
	an := analytics.NewHandler(analytics.NewService(pool))
	mux.Handle("GET /api/analytics/dashboard", protected(an.GetDashboard))

	// ── Гостевой каталог (Шаг 4.2) — без auth, но с rate limiting ───────────
	gs := guest.NewHandler(guest.NewService(pool))
	guestRate := rateMW // reuse same 100 req/min limiter
	mux.Handle("GET /api/guest/catalog/{producer_token}", guestRate(http.HandlerFunc(gs.GetCatalog)))
	mux.Handle("POST /api/guest/cart",                    guestRate(http.HandlerFunc(gs.UpsertCart)))
	mux.Handle("GET /api/guest/cart/{session_id}",        guestRate(http.HandlerFunc(gs.GetCart)))
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}
