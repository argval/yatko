package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/argval/yatko/cache"
	"github.com/argval/yatko/github"
	"github.com/argval/yatko/handlers"
	"github.com/argval/yatko/middleware"
)

// defaultRateLimitRPM is the per-IP request budget per minute, overridable
// via RATE_LIMIT_RPM. No-ops without UPSTASH_REDIS_URL, same as caching.
const defaultRateLimitRPM = 120

func main() {
	ghClient := github.NewClient()
	redisCache := cache.New()

	redirectHandler := handlers.NewRedirectHandler(ghClient, redisCache)
	badgeHandler := handlers.NewBadgeHandler(redirectHandler)
	pageHandler := handlers.NewPageHandler(redirectHandler, ghClient, redisCache)
	linkHandler := handlers.NewLinkHandler(redirectHandler)
	releasesHandler := handlers.NewReleasesHandler(ghClient, redisCache)

	r := gin.Default()

	// Fly-Client-IP is set by Fly.io's edge and can't be spoofed by clients,
	// unlike X-Forwarded-For which Gin trusts from anyone by default.
	// TODO: update when migrating off Fly.io - see gin.Platform* constants or SetTrustedProxies.
	r.TrustedPlatform = gin.PlatformFlyIO
	// Belt and suspenders: if Fly-Client-IP is ever absent, fall back to the
	// raw socket address instead of Gin's default (trust X-Forwarded-For from
	// anyone), which would let a client spoof its own rate-limit identity.
	_ = r.SetTrustedProxies(nil)

	frontendOrigin := os.Getenv("FRONTEND_ORIGIN") // e.g. https://yatko.vercel.app
	r.Use(cors.New(cors.Config{
		AllowOriginFunc: func(origin string) bool {
			if origin == "http://localhost:3000" {
				return true
			}
			if frontendOrigin != "" && origin == frontendOrigin {
				return true
			}
			// Allow any *.vercel.app preview deployment
			return strings.HasSuffix(origin, ".vercel.app")
		},
		AllowMethods:     []string{"GET"},
		AllowHeaders:     []string{"Origin"},
		ExposeHeaders:    []string{"Location"},
		AllowCredentials: false,
	}))

	rateLimitRPM := defaultRateLimitRPM
	if v := os.Getenv("RATE_LIMIT_RPM"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			rateLimitRPM = n
		} else {
			fmt.Fprintf(os.Stderr, "warning: invalid RATE_LIMIT_RPM=%q, using default %d\n", v, defaultRateLimitRPM)
		}
	}

	// Rate-limited routes only - /health is exempt so hosting-platform
	// probes (which hit it frequently from an internal IP) are never throttled.
	limited := r.Group("/")
	limited.Use(middleware.RateLimit(redisCache, rateLimitRPM, time.Minute))
	limited.GET("/dl/:owner/:repo", redirectHandler.Handle)
	limited.GET("/dl/:owner/:repo/:version", redirectHandler.HandleVersioned)
	limited.GET("/badge/:owner/:repo", badgeHandler.Handle)
	limited.GET("/api/release/:owner/:repo", pageHandler.Handle)
	limited.GET("/api/link/:owner/:repo", linkHandler.Handle)
	limited.GET("/api/link/:owner/:repo/:version", linkHandler.HandleVersioned)
	limited.GET("/api/release/:owner/:repo/:version", pageHandler.HandleVersioned)
	limited.GET("/api/releases/:owner/:repo", releasesHandler.Handle)

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
