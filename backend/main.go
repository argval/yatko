package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/argval/yatko/cache"
	"github.com/argval/yatko/github"
	"github.com/argval/yatko/handlers"
	"github.com/argval/yatko/middleware"
	"github.com/argval/yatko/ratelimit"
	"github.com/argval/yatko/search"
	"github.com/gin-gonic/gin"
)

// defaultRateLimitRPM is the per-IP request budget per minute, overridable
// via RATE_LIMIT_RPM. No-ops without UPSTASH_REDIS_URL, same as caching.
const defaultRateLimitRPM = 120

// defaultSearchRateLimitRPM caps /api/search below GitHub's ~30/min Search
// quota so one IP cannot drain the shared token. Overridable via
// SEARCH_RATE_LIMIT_RPM. Stacked on top of the global RATE_LIMIT_RPM.
const defaultSearchRateLimitRPM = 20

func main() {
	ghClient := github.NewClient()
	redisCache := cache.New()
	limiter := ratelimit.New()

	redirectHandler := handlers.NewRedirectHandler(ghClient, redisCache)
	pageHandler := handlers.NewPageHandler(redirectHandler, ghClient, redisCache)
	linkHandler := handlers.NewLinkHandler(redirectHandler)
	releasesHandler := handlers.NewReleasesHandler(ghClient, redisCache)
	searchHandler := handlers.NewSearchHandler(search.NewAutocomplete(ghClient, redisCache))

	r := gin.Default()

	// X-Forwarded-For is set by Vercel's edge to the real client IP and
	// can't be spoofed by clients - Vercel overwrites any client-supplied
	// value. https://vercel.com/docs/headers/request-headers#x-forwarded-for
	r.TrustedPlatform = "X-Forwarded-For"
	// Belt and suspenders: if that header is ever absent, fall back to the
	// raw socket address instead of Gin's default (trust X-Forwarded-For from
	// anyone), which would let a client spoof its own rate-limit identity.
	_ = r.SetTrustedProxies(nil)

	// Frontend and backend are Vercel services sharing one origin, so only
	// same-origin requests ever reach this API - no CORS policy needed.

	rateLimitRPM := defaultRateLimitRPM
	if v := os.Getenv("RATE_LIMIT_RPM"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			rateLimitRPM = n
		} else {
			fmt.Fprintf(os.Stderr, "warning: invalid RATE_LIMIT_RPM=%q, using default %d\n", v, defaultRateLimitRPM)
		}
	}

	searchRateLimitRPM := defaultSearchRateLimitRPM
	if v := os.Getenv("SEARCH_RATE_LIMIT_RPM"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			searchRateLimitRPM = n
		} else {
			fmt.Fprintf(os.Stderr, "warning: invalid SEARCH_RATE_LIMIT_RPM=%q, using default %d\n", v, defaultSearchRateLimitRPM)
		}
	}

	// Rate-limited routes only - /health is exempt so hosting-platform
	// probes (which hit it frequently from an internal IP) are never throttled.
	limited := r.Group("/")
	limited.Use(middleware.RateLimit(limiter, rateLimitRPM, time.Minute))
	limited.GET("/dl/:owner/:repo", redirectHandler.Handle)
	limited.GET("/dl/:owner/:repo/:version", redirectHandler.HandleVersioned)
	limited.GET("/api/release/:owner/:repo", pageHandler.Handle)
	limited.GET("/api/link/:owner/:repo", linkHandler.Handle)
	limited.GET("/api/link/:owner/:repo/:version", linkHandler.HandleVersioned)
	limited.GET("/api/release/:owner/:repo/:version", pageHandler.HandleVersioned)
	limited.GET("/api/readme/:owner/:repo", pageHandler.HandleREADME)
	limited.GET("/api/releases/:owner/:repo", releasesHandler.Handle)
	// Prefixed key so this budget does not share the global per-IP counter.
	limited.GET("/api/search",
		middleware.RateLimitPrefixed(limiter, "search:", searchRateLimitRPM, time.Minute),
		searchHandler.Handle,
	)

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("yatko backend starting on :%s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
