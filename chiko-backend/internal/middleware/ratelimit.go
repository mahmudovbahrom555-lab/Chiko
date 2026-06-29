package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/time/rate"
)

// RateLimit ограничивает количество запросов per user_id.
// Хранит lastSeen и периодически чистит неактивные записи (TTL 10 min)
// чтобы избежать memory leak при большом числе уникальных пользователей.
func RateLimit(requestsPerMin int) func(http.Handler) http.Handler {
	type entry struct {
		limiter  *rate.Limiter
		lastSeen time.Time
	}

	var (
		mu      sync.Mutex
		clients = make(map[uuid.UUID]*entry)
	)

	rps := rate.Limit(float64(requestsPerMin) / 60.0)
	burst := requestsPerMin / 10
	if burst < 5 {
		burst = 5
	}

	// Фоновая очистка: удаляем записи без активности >10 минут.
	// Запускается один раз при создании middleware (not per-request).
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			mu.Lock()
			for id, e := range clients {
				if time.Since(e.lastSeen) > 10*time.Minute {
					delete(clients, id)
				}
			}
			mu.Unlock()
		}
	}()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID, ok := UserIDFromCtx(r.Context())
			if !ok {
				// Неаутентифицированный запрос — пропускаем без лимита.
				// Auth middleware должен отклонить его раньше.
				// Гостевые endpoints (buy-list, guest catalog) проходят сюда
				// — они должны быть обёрнуты в rateMW НАПРЯМУЮ, а не через protected().
				next.ServeHTTP(w, r)
				return
			}

			mu.Lock()
			e, exists := clients[userID]
			if !exists {
				e = &entry{limiter: rate.NewLimiter(rps, burst)}
				clients[userID] = e
			}
			e.lastSeen = time.Now()
			mu.Unlock()

			if !e.limiter.Allow() {
				http.Error(w, `{"error":"rate_limit_exceeded"}`, http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
