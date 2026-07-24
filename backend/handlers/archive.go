package handlers

import (
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/argval/yatko/archive"
	"github.com/gin-gonic/gin"
)

// ArchiveHandler serves authenticated archive sync endpoints.
type ArchiveHandler struct {
	svc *archive.Service
}

// NewArchiveHandler wires the archive sync HTTP API.
func NewArchiveHandler(svc *archive.Service) *ArchiveHandler {
	return &ArchiveHandler{svc: svc}
}

// HandleSync handles GET|POST /api/archive/sync.
// Auth: CACHE_REFRESH_SECRET (or CRON_SECRET) via Bearer token or ?secret=.
// Optional query owner+repo selects a single group; otherwise syncs all.
func (h *ArchiveHandler) HandleSync(c *gin.Context) {
	if !archiveSyncAuthorized(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	if h.svc == nil || !h.svc.Enabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "archive not configured (set ARCHIVE_GROUPS)"})
		return
	}

	owner := c.Query("owner")
	repo := c.Query("repo")
	if owner != "" && repo != "" {
		g := h.svc.FindGroup(owner, repo)
		if g == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "repo not in ARCHIVE_GROUPS"})
			return
		}
		res, err := h.svc.SyncGroup(c.Request.Context(), *g)
		if err != nil {
			log.Printf("archive sync %s/%s: %v", owner, repo, err)
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "results": []archive.SyncResult{*res}})
		return
	}

	results, err := h.svc.SyncAll(c.Request.Context())
	if err != nil {
		log.Printf("archive sync all: %v", err)
		status := http.StatusBadGateway
		if len(results) > 0 {
			status = http.StatusMultiStatus
		}
		c.JSON(status, gin.H{"ok": false, "results": results, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "results": results})
}

func archiveSyncAuthorized(c *gin.Context) bool {
	secrets := make([]string, 0, 2)
	if s := os.Getenv("CACHE_REFRESH_SECRET"); s != "" {
		secrets = append(secrets, s)
	}
	if s := os.Getenv("CRON_SECRET"); s != "" {
		secrets = append(secrets, s)
	}
	if len(secrets) == 0 {
		return false
	}
	candidates := []string{c.Query("secret")}
	if auth := c.GetHeader("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		candidates = append(candidates, strings.TrimPrefix(auth, "Bearer "))
	}
	for _, want := range secrets {
		for _, got := range candidates {
			if got != "" && got == want {
				return true
			}
		}
	}
	return false
}
