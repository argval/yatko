package handlers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/argval/yatko/archive"
	"github.com/argval/yatko/cache"
	"github.com/gin-gonic/gin"
)

func TestArchiveSync_UnauthorizedWithoutSecret(t *testing.T) {
	t.Setenv("CACHE_REFRESH_SECRET", "")
	t.Setenv("CRON_SECRET", "")
	h := NewArchiveHandler(archive.New(nil, nil, cache.New(), nil))
	r := gin.New()
	r.GET("/api/archive/sync", h.HandleSync)

	req := httptest.NewRequest(http.MethodGet, "/api/archive/sync", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestArchiveSync_RejectsBadSecret(t *testing.T) {
	t.Setenv("CACHE_REFRESH_SECRET", "s3cret")
	h := NewArchiveHandler(archive.New(
		[]archive.Group{{Aliases: []string{"o/r"}}},
		nil, cache.New(), nil,
	))
	r := gin.New()
	r.GET("/api/archive/sync", h.HandleSync)

	req := httptest.NewRequest(http.MethodGet, "/api/archive/sync?secret=wrong", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestArchiveSync_NotConfigured(t *testing.T) {
	t.Setenv("CACHE_REFRESH_SECRET", "s3cret")
	h := NewArchiveHandler(archive.New(nil, nil, cache.New(), nil))
	r := gin.New()
	r.GET("/api/archive/sync", h.HandleSync)

	req := httptest.NewRequest(http.MethodGet, "/api/archive/sync?secret=s3cret", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
}

func TestArchiveSync_BearerAuthAccepted(t *testing.T) {
	t.Setenv("CACHE_REFRESH_SECRET", "s3cret")
	_ = os.Unsetenv("CRON_SECRET")
	h := NewArchiveHandler(archive.New(nil, nil, cache.New(), nil))
	r := gin.New()
	r.GET("/api/archive/sync", h.HandleSync)

	req := httptest.NewRequest(http.MethodGet, "/api/archive/sync", nil)
	req.Header.Set("Authorization", "Bearer s3cret")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	// Auth passes; archive groups empty → 503 not 401.
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
}

func TestArchiveSync_UnknownRepo(t *testing.T) {
	t.Setenv("CACHE_REFRESH_SECRET", "s3cret")
	h := NewArchiveHandler(archive.New(
		[]archive.Group{{Aliases: []string{"permissionlesstech/bitchat"}}},
		nil, cache.New(), nil,
	))
	r := gin.New()
	r.GET("/api/archive/sync", h.HandleSync)

	req := httptest.NewRequest(http.MethodGet, "/api/archive/sync?secret=s3cret&owner=nope&repo=nope", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
}
