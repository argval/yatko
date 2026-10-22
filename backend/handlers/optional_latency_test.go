package handlers

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/argval/yatko/cache"
	"github.com/argval/yatko/github"
	"github.com/argval/yatko/jev"
	"github.com/argval/yatko/picker"
	"github.com/gin-gonic/gin"
)

// Optional metadata must not delay fallback or cancel a page's concurrent lookup.
func TestOptionalRepoMetadataLatency(t *testing.T) {
	for _, pageFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "pick first", true: "page first"}[pageFirst], func(t *testing.T) {
			primaryRouter(t, http.StatusServiceUnavailable)
			started := make(chan struct{}, 2)
			transport := http.DefaultTransport
			http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != "/repos/acme/tool" {
					return transport.RoundTrip(req)
				}
				started <- struct{}{}
				select {
				case <-time.After(800 * time.Millisecond):
					return transport.RoundTrip(req)
				case <-req.Context().Done():
					return nil, req.Context().Err()
				}
			})
			gh, store := github.NewClient(), cache.New()
			h := NewRedirectHandler(gh, store)
			h.Primary = &jev.Primary{Client: jev.NewClientFromEnv(), Cache: store}
			var release github.Release
			if err := json.Unmarshal([]byte(primaryReleaseJSON), &release); err != nil {
				t.Fatal(err)
			}
			router := gin.New()
			router.GET("/api/repo/:owner/:repo", NewPageHandler(h, gh, store).HandleRepoMeta)
			pageDone := make(chan *httptest.ResponseRecorder, 1)
			page := func() {
				w := httptest.NewRecorder()
				router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/repo/acme/tool", nil))
				pageDone <- w
			}
			pickDone := make(chan assetPick, 1)
			pick := func() {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodGet, "/?platform=windows&arch=amd64", nil)
				pickDone <- h.pick(c, "acme", "tool", &release)
			}
			begin := time.Now()
			if pageFirst {
				go page()
			} else {
				go pick()
			}
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("metadata fetch did not start")
			}
			if pageFirst {
				go pick()
			} else {
				go page()
			}
			p := <-pickDone
			if elapsed := time.Since(begin); elapsed > 500*time.Millisecond {
				t.Errorf("optional metadata delayed rule fallback: %s", elapsed)
			}
			if !p.Decision.ShouldAutoSelect() || p.Decision.Asset.Name != "tool-windows-amd64.zip" {
				t.Errorf("lost fallback: %+v", p.Decision)
			}
			w := <-pageDone
			if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "A tool for testing") {
				t.Fatalf("required metadata lookup failed: %d %s", w.Code, w.Body)
			}
		})
	}
}

// The production cache must bound a stalled TLS handshake and keep the local offer.
func TestRememberDownloadRedisLatency(t *testing.T) {
	for _, unavailable := range []bool{false, true} {
		t.Run(map[bool]string{false: "stalled", true: "unavailable"}[unavailable], func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			connections := make(chan net.Conn, 16)
			go func() {
				defer close(connections)
				for {
					conn, err := listener.Accept()
					if err != nil {
						return
					}
					connections <- conn
				}
			}()
			t.Cleanup(func() {
				_ = listener.Close()
				for conn := range connections {
					_ = conn.Close()
				}
			})
			if unavailable {
				_ = listener.Close()
			}
			t.Setenv("REDIS_URL", "rediss://"+listener.Addr().String()+"?dial_timeout=1s&read_timeout=1s&write_timeout=1s&max_retries=10&min_retry_backoff=100ms&max_retry_backoff=100ms")
			t.Setenv("KV_URL", "")
			t.Setenv("UPSTASH_REDIS_URL", "")
			h := &RedirectHandler{cache: cache.New(), Primary: &jev.Primary{}}
			asset := github.Asset{ID: 1, Name: "tool-windows-amd64.zip", State: "uploaded", Size: 10,
				BrowserDownloadURL: "https://example.com/tool.zip"}
			release := &github.Release{TagName: "v1", Assets: []github.Asset{asset}}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodGet, "/?platform=windows&arch=amd64", nil)
			ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
			defer cancel()
			c.Request = c.Request.WithContext(ctx)
			p := resolveAssetPick(c)
			p.Decision = picker.DecideAsset(release.Assets, p.Platform, p.Arch, p.opts())
			if !p.Decision.ShouldAutoSelect() {
				t.Fatal("fixture did not select an asset")
			}
			begin := time.Now()
			done := make(chan struct{})
			go func() {
				h.rememberDownload(c, "acme", "tool", release, p)
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(500 * time.Millisecond):
				_ = listener.Close()
				for conn := range connections {
					_ = conn.Close()
				}
				<-done
				t.Fatal("offer persistence exceeded its optional budget")
			}
			if elapsed := time.Since(begin); elapsed > 500*time.Millisecond {
				t.Errorf("offer persistence took %s", elapsed)
			}
			pin, _ := gin.CreateTestContext(httptest.NewRecorder())
			pin.Request = httptest.NewRequest(http.MethodGet, "/?platform=windows&arch=amd64&asset="+asset.Name, nil)
			d, ok := h.offeredDownload(pin, "acme", "tool", release, resolveAssetPick(pin))
			if !ok || !d.ShouldAutoSelect() || d.Asset.BrowserDownloadURL != asset.BrowserDownloadURL {
				t.Fatalf("Redis failure lost the local offer: ok=%v decision=%+v", ok, d)
			}
			if len(c.Errors) == 0 {
				t.Error("expected the Redis failure to remain observable")
			}
		})
	}
}
