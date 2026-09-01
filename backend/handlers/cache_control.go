package handlers

import "github.com/gin-gonic/gin"

// Match the cache package's one-hour soft TTL and 24-hour hard TTL. Browsers
// revalidate every request; Vercel's CDN serves stale data while it refreshes.
const publicDataCacheControl = "public, max-age=0, s-maxage=3600, stale-while-revalidate=82800"

func setPublicDataCacheControl(c *gin.Context) {
	c.Header("Cache-Control", publicDataCacheControl)
}
