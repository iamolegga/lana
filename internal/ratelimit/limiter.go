package ratelimit

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type Limiter interface {
	Limit(next http.Handler) http.Handler
}

type RateLimiter struct {
	store            sync.Map // key: ip|host|path -> *rate.Limiter
	requestsPerMin   int
	cleanupInterval  time.Duration
	xffIndex         int // index for X-Forwarded-For, supports negative indices
	shutdownComplete chan struct{}
}

type Config struct {
	RequestsPerMinute  int
	CleanupInterval    time.Duration
	XForwardedForIndex int // index for X-Forwarded-For, supports negative indices
}

func New(ctx context.Context, config Config, _ any) *RateLimiter {
	limiter := &RateLimiter{
		store:            sync.Map{},
		requestsPerMin:   config.RequestsPerMinute,
		cleanupInterval:  config.CleanupInterval,
		xffIndex:         config.XForwardedForIndex,
		shutdownComplete: make(chan struct{}),
	}

	go limiter.cleanupLoop(ctx)

	return limiter
}

func (rl *RateLimiter) Limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := rl.getClientIP(r)

		key := ip + "|" + r.Host + "|" + r.URL.Path

		val, _ := rl.store.LoadOrStore(key,
			rate.NewLimiter(
				rate.Every(time.Minute/time.Duration(rl.requestsPerMin)),
				rl.requestsPerMin,
			),
		)

		limiter := val.(*rate.Limiter)

		if !limiter.Allow() {
			slog.Debug("request rate-limited",
				"ip", ip,
				"path", r.URL.Path,
			)

			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (rl *RateLimiter) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(rl.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.cleanup()
		case <-ctx.Done():
			slog.Info("rate limiter cleanup stopped")
			close(rl.shutdownComplete)
			return
		}
	}
}

func (rl *RateLimiter) cleanup() {
	removed := 0
	count := 0

	rl.store.Range(func(key, value any) bool {
		limiter := value.(*rate.Limiter)
		count++

		if limiter.Tokens() == float64(limiter.Burst()) {
			rl.store.Delete(key)
			removed++
		}
		return true
	})

	slog.Debug(
		"cleaning up rate limiters",
		"total_before",
		count,
		"removed",
		removed,
	)
}

func (rl *RateLimiter) Shutdown() {
	<-rl.shutdownComplete
}

func (rl *RateLimiter) getClientIP(r *http.Request) string {
	// Priority 1: Check for CF-Connecting-IP (Cloudflare)
	if cfIP := r.Header.Get("CF-Connecting-IP"); cfIP != "" {
		return cfIP
	}

	// Priority 2: Check for X-Real-IP header (nginx/other proxies)
	if xrip := r.Header.Get("X-Real-IP"); xrip != "" {
		return xrip
	}

	// Priority 3: Check for X-Forwarded-For header (with configurable index)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := splitCSV(xff)
		if len(ips) > 0 {
			// Support positive indices (0 = first, 1 = second, etc.)
			if rl.xffIndex >= 0 && rl.xffIndex < len(ips) {
				return ips[rl.xffIndex]
			}
			// Support negative indices (-1 = last, -2 = second-to-last, etc.)
			if rl.xffIndex < 0 && -rl.xffIndex <= len(ips) {
				return ips[len(ips)+rl.xffIndex]
			}
			// Fallback to first IP if index out of bounds
			return ips[0]
		}
	}

	// Priority 4: Fall back to RemoteAddr
	return r.RemoteAddr
}

func splitCSV(s string) []string {
	var result []string
	for i, start := 0, 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	return result
}
