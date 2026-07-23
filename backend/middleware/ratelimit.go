package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/argval/yatko/ratelimit"
	"github.com/gin-gonic/gin"
)

// RateLimit throttles requests per client IP to limit requests per window,
// returning 429 with a Retry-After header once exceeded. No-ops when the
// limiter has no Redis configured; Redis errors fall back to an in-process
// window inside ratelimit.Limiter.Allow.
func RateLimit(l *ratelimit.Limiter, limit int, window time.Duration) gin.HandlerFunc {
	return rateLimit(l, "", limit, window)
}

// RateLimitPrefixed is like RateLimit but namespaces the Redis key so a
// tighter per-endpoint budget (e.g. /api/search) can coexist with the global
// per-IP limit without sharing the same counter.
func RateLimitPrefixed(l *ratelimit.Limiter, keyPrefix string, limit int, window time.Duration) gin.HandlerFunc {
	return rateLimit(l, keyPrefix, limit, window)
}

func rateLimit(l *ratelimit.Limiter, keyPrefix string, limit int, window time.Duration) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		key := keyPrefix + ctx.ClientIP()
		allowed, retryAfter, err := l.Allow(ctx.Request.Context(), key, limit, window)
		if err != nil {
			ctx.Next()
			return
		}
		if !allowed {
			ctx.Header("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
			ctx.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded, try again later"})
			return
		}
		ctx.Next()
	}
}
