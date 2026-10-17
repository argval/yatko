package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/argval/yatko/cache"
	"github.com/argval/yatko/github"
	"github.com/argval/yatko/jev"
	"github.com/gin-gonic/gin"
)

// primaryRouter serves the visitor fixture release and a fake TypeSafe API.
// The fake reads OS and CPU from filename tokens, except that it calls the
// Windows zip "operating system not stated", so Jev and the rules disagree for
// Windows visitors. status != 200 makes every TypeSafe call fail.
func primaryRouter(t *testing.T, status int) *gin.Engine {
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
			return respond(http.StatusOK, fakeJevAnswers(t, r))
		case r.URL.Path == "/repos/acme/tool/releases/latest":
			return respond(http.StatusOK, visitorReleaseJSON)
		}
		t.Fatalf("unexpected request: %s", r.URL.String())
		return nil, nil
	})
	redirect := NewRedirectHandler(github.NewClient(), cache.New())
	redirect.Primary = &jev.Primary{Client: jev.NewClientFromEnv(), Cache: cache.New()}
	router := gin.New()
	router.GET("/dl/:owner/:repo", redirect.Handle)
	return router
}

func fakeJevAnswers(t *testing.T, r *http.Request) string {
	var req struct {
		Questions map[string]struct {
			Instructions map[string]string `json:"instructions"`
		} `json:"questions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		t.Fatal(err)
	}
	answers := map[string]any{}
	for id, q := range req.Questions {
		file := q.Instructions["file"]
		pick := func(option string) map[string]any {
			return map[string]any{"type": "choice", "choice": option, "probabilities": map[string]float64{option: 1}}
		}
		switch {
		case strings.HasPrefix(id, "os_"):
			os := "operating system not stated"
			switch {
			case strings.Contains(file, "linux"):
				os = "Linux"
			case strings.Contains(file, "darwin"):
				os = "macOS"
			}
			answers[id] = pick(os)
		case strings.HasPrefix(id, "cpu_"):
			cpu := "64-bit x86 (x86_64, x64, amd64)"
			if strings.Contains(file, "arm64") {
				cpu = "64-bit ARM (arm64, aarch64)"
			}
			answers[id] = pick(cpu)
		default:
			answers[id] = pick("the main program, in its standard build")
		}
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
