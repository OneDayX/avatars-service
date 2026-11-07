package handlers

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

func CORS(allowedOrigins string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" {
				allowed := allowedOrigins == "*"
				for _, candidate := range strings.Split(allowedOrigins, ",") {
					if strings.TrimSpace(candidate) == origin {
						allowed = true
						break
					}
				}
				if allowed {
					if allowedOrigins == "*" {
						w.Header().Set("Access-Control-Allow-Origin", "*")
					} else {
						w.Header().Set("Access-Control-Allow-Origin", origin)
						w.Header().Add("Vary", "Origin")
					}
					w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
					w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-User-ID, If-None-Match")
				}
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

type rateWindow struct {
	started time.Time
	count   int
}

type RateLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	clients map[string]rateWindow
	now     func() time.Time
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{limit: limit, window: window, clients: make(map[string]rateWindow), now: time.Now}
}

func (l *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !l.allow(r) {
			w.Header().Set("Retry-After", "60")
			writeError(w, http.StatusTooManyRequests, "Rate limit exceeded", "Try again later")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (l *RateLimiter) allow(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	key := host + ":" + r.Header.Get("X-User-ID")
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	window := l.clients[key]
	if now.Sub(window.started) >= l.window || window.started.IsZero() {
		window = rateWindow{started: now}
	}
	window.count++
	l.clients[key] = window
	if len(l.clients) > 10000 {
		for client, value := range l.clients {
			if now.Sub(value.started) >= l.window {
				delete(l.clients, client)
			}
		}
	}
	return window.count <= l.limit
}
