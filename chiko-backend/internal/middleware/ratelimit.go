package middleware

import (
	"net/http"
	"sync"

	"github.com/google/uuid"
	"golang.org/x/time/rate"
)

// RateLimit ограничивает количество запросов per user_id.
// REST: 100 req/min, WebSocket events: 500/min — передаётся через r и eventsPerMin.
func RateLimit(requestsPerMin int) func(http.Handler) http.Handler {
	type entry struct {
		limiter *rate.Limiter
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

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID, ok := UserIDFromCtx(r.Context())
			if !ok {
				// Неаутентифицированный запрос — пропускаем без лимита (Auth middleware должен отклонить раньше)
				next.ServeHTTP(w, r)
				return
			}

			mu.Lock()
			e, exists := clients[userID]
			if !exists {
				e = &entry{limiter: rate.NewLimiter(rps, burst)}
				clients[userID] = e
			}
			mu.Unlock()

			if !e.limiter.Allow() {
				http.Error(w, `{"error":"rate_limit_exceeded"}`, http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
