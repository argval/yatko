package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/argval/yatko/cache"
	"github.com/argval/yatko/github"
	"github.com/argval/yatko/jev"
	"github.com/gin-gonic/gin"
)

// primaryRouter serves one release and a local fake of the Choice API.
// Windows receives a successful abstention even though rule ranking has a match.
func primaryRouter(t *testing.T, status int) *gin.Engine {
	t.Helper()
	return primaryRouterWithChoice(t, status, "")
}

func primaryRouterWithChoice(t *testing.T, status int, forced string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	for _, k := range []string{"REDIS_URL", "KV_URL", "UPSTASH_REDIS_URL", "TYPESAFE_API", "JEV_MODEL"} {
		t.Setenv(k, "")
	}
	t.Setenv("TYPESAFE_API_KEY", "test-key")
	old := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = old })
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		respond := func(code int, body string) (*http.Response, error) {
			return &http.Response{StatusCode: code, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
		}
		switch {
		case r.URL.Host == "api.typesafe.ai":
			if status != http.StatusOK {
				return respond(status, `{"error":"down"}`)
			}
			return respond(http.StatusOK, fakeJevAnswers(t, r, forced))
		case r.URL.Path == "/repos/acme/tool/releases/latest":
			return respond(http.StatusOK, visitorReleaseJSON)
		}
		t.Fatalf("unexpected request: %s", r.URL.String())
		return nil, nil
	})
	gh := github.NewClient()
	store := cache.New()
	redirect := NewRedirectHandler(gh, store)
	redirect.Primary = &jev.Primary{Client: jev.NewClientFromEnv(), Cache: store}
	router := gin.New()
	router.GET("/dl/:owner/:repo", redirect.Handle)
	router.GET("/api/link/:owner/:repo", NewLinkHandler(redirect).Handle)
	router.GET("/api/release/:owner/:repo", NewPageHandler(redirect, gh, store).Handle)
	return router
}

func fakeJevAnswers(t *testing.T, r *http.Request, forced string) string {
	t.Helper()
	var req struct {
		State struct {
			Visitor struct {
				OS struct {
					Value string `json:"value"`
				} `json:"operating_system"`
			} `json:"visitor"`
		} `json:"state"`
		Questions map[string]struct {
			Type     string                     `json:"type"`
			Criteria map[string]json.RawMessage `json:"criteria"`
		} `json:"questions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		t.Fatal(err)
	}
	choice := forced
	if choice == "" {
		choice = "tool-linux-amd64.tar.gz"
		if strings.EqualFold(req.State.Visitor.OS.Value, "Windows") {
			choice = jev.NoneOption
		}
	}
	answers := map[string]any{}
	for id, q := range req.Questions {
		if q.Type == "noul" {
			answers[id] = map[string]any{"type": "noul", "noul": 1}
			continue
		}
		probabilities := map[string]float64{}
		for option := range q.Criteria {
			probabilities[option] = 0
		}
		probabilities[choice] = 1
		answers[id] = map[string]any{"type": "choice", "choice": choice, "probabilities": probabilities}
	}
	out, _ := json.Marshal(map[string]any{"model": "jev-test", "answers": answers, "usage": map[string]int{"input_tokens": 1}})
	return string(out)
}

func download(router *gin.Engine, ua string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/dl/acme/tool", nil)
	req.Header.Set("User-Agent", ua)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

const windowsUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/130.0 Safari/537.36"

// With Jev primary, /dl follows Jev: it agrees on Linux and abstains on the
// Windows zip it cannot place, where the rules would have picked it.
func TestPrimaryJevDecidesDownloads(t *testing.T) {
	router := primaryRouter(t, http.StatusOK)
	if w := download(router, "curl/8.5.0"); w.Code != http.StatusFound || w.Header().Get("Location") != "https://example.com/linux.tgz" {
		t.Fatalf("linux: %d %q", w.Code, w.Header().Get("Location"))
	}
	if w := download(router, windowsUA); w.Header().Get("Location") == "https://example.com/win.zip" {
		t.Fatal("windows: the rule pick was used although Jev answered")
	}
}

// When TypeSafe fails, the rule picker answers and downloads keep working.
func TestPrimaryFallsBackToRulesWhenJevFails(t *testing.T) {
	router := primaryRouter(t, http.StatusInternalServerError)
	if w := download(router, windowsUA); w.Code != http.StatusFound || w.Header().Get("Location") != "https://example.com/win.zip" {
		t.Fatalf("fallback: %d %q", w.Code, w.Header().Get("Location"))
	}
}

// A pin keeps the full release as Choice context and cannot silently switch files.
func TestPrimaryPinnedDownloadUsesSameChoice(t *testing.T) {
	router := primaryRouter(t, http.StatusOK)
	for _, tc := range []struct{ pin, want string }{
		{"tool-linux-amd64.tar.gz", "https://example.com/linux.tgz"},
		{"tool-windows-amd64.zip", "https://github.com/acme/tool/releases/tag/v1.0.0"},
		{"missing.zip", "https://github.com/acme/tool/releases/tag/v1.0.0"},
	} {
		req := httptest.NewRequest(http.MethodGet, "/dl/acme/tool?platform=linux&arch=amd64&asset="+tc.pin, nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64)")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusFound || w.Header().Get("Location") != tc.want {
			t.Fatalf("pin %q: %d %q", tc.pin, w.Code, w.Header().Get("Location"))
		}
	}
}

func TestPrimaryLinkAbstentionDoesNotUseRules(t *testing.T) {
	router := primaryRouter(t, http.StatusOK)
	req := httptest.NewRequest(http.MethodGet, "/api/link/acme/tool?platform=windows&arch=amd64", nil)
	req.Header.Set("User-Agent", windowsUA)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var link LinkResponse
	if err := json.Unmarshal(w.Body.Bytes(), &link); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusNotFound || link.Filename != "" || link.URL != "https://github.com/acme/tool/releases/tag/v1.0.0" {
		t.Fatalf("abstention = %d %+v", w.Code, link)
	}
}

func TestPrimaryReleaseDoesNotPrecomputeChoices(t *testing.T) {
	// A broken fake would fail this request if page loading invoked Choice.
	router := primaryRouter(t, http.StatusInternalServerError)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/release/acme/tool", nil))
	var body map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusOK || body["picks"] != nil {
		t.Fatalf("release = %d %s", w.Code, w.Body.String())
	}
}

func TestPrimaryPinnedAbstentionDoesNotUseRules(t *testing.T) {
	router := primaryRouter(t, http.StatusOK)
	req := httptest.NewRequest(http.MethodGet, "/dl/acme/tool?platform=windows&arch=amd64&asset=tool-windows-amd64.zip", nil)
	req.Header.Set("User-Agent", windowsUA)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusFound || w.Header().Get("Location") != "https://github.com/acme/tool/releases/tag/v1.0.0" {
		t.Fatalf("pinned abstention = %d %q", w.Code, w.Header().Get("Location"))
	}
}

func TestPrimaryPinnedServiceFailureUsesConservativeRules(t *testing.T) {
	router := primaryRouter(t, http.StatusInternalServerError)
	for _, tc := range []struct{ pin, want string }{
		{"tool-windows-amd64.zip", "https://example.com/win.zip"},
		{"tool-linux-amd64.tar.gz", "https://github.com/acme/tool/releases/tag/v1.0.0"},
	} {
		req := httptest.NewRequest(http.MethodGet, "/dl/acme/tool?platform=windows&arch=amd64&asset="+tc.pin, nil)
		req.Header.Set("User-Agent", windowsUA)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusFound || w.Header().Get("Location") != tc.want {
			t.Fatalf("pinned failure %q = %d %q", tc.pin, w.Code, w.Header().Get("Location"))
		}
	}
}

func TestPrimaryScriptAbstentionReturns404(t *testing.T) {
	router := primaryRouterWithChoice(t, http.StatusOK, jev.NoneOption)
	w := download(router, "curl/8.5.0")
	if w.Code != http.StatusNotFound || w.Header().Get("Location") != "" {
		t.Fatalf("script abstention = %d %q", w.Code, w.Header().Get("Location"))
	}
}

// Jev abstains for Windows while the rules pick the zip: that disagreement is
// the production tuning signal and must be logged with Jev's reasons.
func TestPrimaryLogsDisagreementWithRules(t *testing.T) {
	router := primaryRouter(t, http.StatusOK)
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	download(router, windowsUA)
	if out := buf.String(); !strings.Contains(out, "jev_primary_diff") || !strings.Contains(out, `rules="tool-windows-amd64.zip"`) || !strings.Contains(out, "jev chose") {
		t.Fatalf("log = %q", out)
	}
}
