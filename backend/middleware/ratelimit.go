package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/yoink/cache"
)

// RateLimit throttles requests per client IP to limit requests per window,
// returning 429 with a Retry-After header once exceeded. No-ops when the
// cache has no Redis configured (see Cache.Allow).
func RateLimit(c *cache.Cache, limit int, window time.Duration) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		allowed, retryAfter, err := c.Allow(ctx.Request.Context(), ctx.ClientIP(), limit, window)
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
