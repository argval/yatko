package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/argval/yatko/cache"
	"github.com/argval/yatko/github"
	"github.com/argval/yatko/jev"
	"github.com/argval/yatko/picker"
	"github.com/gin-gonic/gin"
)

// Shadow mode must leave the /dl redirect untouched and call the shadow
// picker once per bucket, even when the same request repeats.
func TestShadow_SameRedirectAndOneCallPerBucket(t *testing.T) {
	gin.SetMode(gin.TestMode)
	plain := visitorRouter(t) // also installs the fake GitHub transport

	shadowed := NewRedirectHandler(github.NewClient(), cache.New())
	var calls atomic.Int32
	shadowed.Shadow = func(context.Context, string, []github.Asset, picker.Platform, picker.Arch, picker.PickOpts) (picker.AssetDecision, jev.Usage, error) {
		calls.Add(1)
		return picker.AssetDecision{}, jev.Usage{}, nil
	}
	router := gin.New()
	router.GET("/dl/:owner/:repo", shadowed.Handle)

	get := func(r http.Handler) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/dl/acme/tool", nil)
		req.Header.Set("User-Agent", "curl/8.5.0")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	want := get(plain)
	for range 2 {
		got := get(router)
		if got.Code != want.Code || got.Header().Get("Location") != want.Header().Get("Location") {
			t.Fatalf("shadow changed response: %d %q, want %d %q",
				got.Code, got.Header().Get("Location"), want.Code, want.Header().Get("Location"))
		}
	}
	shadowed.shadowWG.Wait()
	if n := calls.Load(); n != 1 {
		t.Fatalf("shadow calls = %d, want 1 (bucket dedupe)", n)
	}
}
