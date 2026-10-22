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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/argval/yatko/cache"
	"github.com/argval/yatko/github"
	"github.com/argval/yatko/jev"
	"github.com/gin-gonic/gin"
)

// primaryRouter serves one release and a local fake of the Choice API.
// Windows receives a successful abstention even though rule ranking has a match.
func primaryRouter(t *testing.T, status int, wait ...time.Duration) *gin.Engine {
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
		case r.URL.Path == "/repos/acme/tool":
			return respond(http.StatusOK, `{"description":"A tool for testing","stargazers_count":1,"owner":{"avatar_url":""}}`)
		case r.URL.Path == "/repos/acme/tool/releases/latest":
			return respond(http.StatusOK, primaryReleaseJSON)
		case strings.HasPrefix(r.URL.Path, "/repos/acme/tool/releases/tags/"):
			return respond(http.StatusOK, primaryReleaseJSON)
		}
		t.Fatalf("unexpected request: %s", r.URL.String())
		return nil, nil
	})
	gh := github.NewClient()
	store := cache.New()
	redirect := NewRedirectHandler(gh, store)
	redirect.Primary = &jev.Primary{Client: jev.NewClientFromEnv(), Cache: store}
	if len(wait) > 0 {
		redirect.Primary.Wait = wait[0]
	}
	router := gin.New()
	router.GET("/dl/:owner/:repo", redirect.Handle)
	router.GET("/dl/:owner/:repo/:version", redirect.HandleVersioned)
	router.GET("/api/link/:owner/:repo", NewLinkHandler(redirect).Handle)
	router.GET("/api/release/:owner/:repo", NewPageHandler(redirect, gh, store).Handle)
	return router
}

// primaryReleaseJSON adds a portable Windows build, so two Windows files run.
var primaryReleaseJSON = strings.Replace(visitorReleaseJSON, `{"name":"tool-darwin-arm64.dmg"`,
	`{"name":"tool-windows-amd64-portable.zip","browser_download_url":"https://example.com/win-portable.zip","size":11,"download_count":1},{"name":"tool-darwin-arm64.dmg"`, 1)

// fakeJevAnswers answers Describe from filename tokens. A Choice offered
// Windows files answers "not enough information", so Jev abstains for Windows
// visitors while the rules pick the zip; any other Choice picks its first option.
func fakeJevAnswers(t *testing.T, r *http.Request) string {
	t.Helper()
	var req struct {
		State struct {
			Product string `json:"product"`
		} `json:"state"`
		Questions map[string]struct {
			Instructions struct{ File string }      `json:"instructions"`
			Criteria     map[string]json.RawMessage `json:"criteria"`
		} `json:"questions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		t.Fatal(err)
	}
	if req.State.Product != "A tool for testing" {
		t.Errorf("product = %q, want the repo description", req.State.Product)
	}
	pick := func(option string, criteria map[string]json.RawMessage) map[string]any {
		probabilities := map[string]float64{}
		for name := range criteria {
			probabilities[name] = 0
		}
		probabilities[option] = 1
		return map[string]any{"type": "choice", "choice": option, "probabilities": probabilities}
	}
	answers := map[string]any{}
	for id, q := range req.Questions {
		file := q.Instructions.File
		switch {
		case id == "choice":
			choice := ""
			for option := range q.Criteria {
				if choice == "" || strings.Contains(option, "windows") {
					choice = option
				}
			}
			if strings.Contains(choice, "windows") {
				choice = jev.InsufficientOption
			}
			probabilities := map[string]float64{}
			for option := range q.Criteria {
				probabilities[option] = 0
			}
			probabilities[choice] = 1
			answers[id] = map[string]any{"type": "choice", "choice": choice, "probabilities": probabilities}
		case strings.HasPrefix(id, "os_"):
			os := "operating system not stated"
			switch {
			case strings.Contains(file, "linux"):
				os = "Linux"
			case strings.Contains(file, "darwin"):
				os = "macOS"
			case strings.Contains(file, "windows"):
				os = "Windows"
			}
			answers[id] = pick(os, q.Criteria)
		case strings.HasPrefix(id, "cpu_"):
			cpu := "64-bit x86 (x86_64, x64, amd64)"
			if strings.Contains(file, "arm64") {
				cpu = "64-bit ARM (arm64, aarch64)"
			}
			answers[id] = pick(cpu, q.Criteria)
		default:
			answers[id] = pick("the main program, in its standard build", q.Criteria)
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

// With Jev primary, /dl follows Jev: it agrees on Linux and abstains between
// two Windows builds, where the rules would have picked the zip.
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
	router := primaryRouter(t, http.StatusOK)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/dl/acme/tool?platform=windows&arch=amd64", nil)
	req.Header.Set("User-Agent", "curl/8.5.0")
	router.ServeHTTP(w, req)
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
	if out := buf.String(); !strings.Contains(out, "jev_primary_diff") || !strings.Contains(out, `rules="tool-windows-amd64.zip"`) || !strings.Contains(out, `jev=""`) || !strings.Contains(out, "jev_reasons=\"jev") {
		t.Fatalf("log = %q", out)
	}
}

// A rule fallback already offered by /api/link survives Jev's later abstention.
func TestPrimaryOfferedDownloadSurvivesWarmAbstention(t *testing.T) {
	router := primaryRouter(t, http.StatusOK, 20*time.Millisecond)
	gate, finished := make(chan struct{}), make(chan struct{})
	var release sync.Once
	defer release.Do(func() { close(gate) })
	var calls atomic.Int32
	transport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "api.typesafe.ai" {
			return transport.RoundTrip(req)
		}
		<-gate
		response, err := transport.RoundTrip(req)
		if calls.Add(1) == 2 {
			close(finished)
		}
		return response, err
	})
	request := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("User-Agent", windowsUA)
		router.ServeHTTP(w, req)
		return w
	}
	first := request("/api/link/acme/tool?platform=windows&arch=amd64")
	var link LinkResponse
	if err := json.Unmarshal(first.Body.Bytes(), &link); err != nil {
		t.Fatal(err)
	}
	if first.Code != http.StatusOK || link.Filename != "tool-windows-amd64.zip" {
		t.Fatalf("cold fallback: %d %s", first.Code, first.Body)
	}
	release.Do(func() { close(gate) })
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("Jev did not finish warming")
	}
	if warmed := request("/api/link/acme/tool?platform=windows&arch=amd64"); warmed.Code != http.StatusNotFound {
		t.Fatalf("warm Jev must still abstain: %d %s", warmed.Code, warmed.Body)
	}
	for _, tc := range []struct{ query, want string }{
		{"platform=windows&arch=amd64&asset=tool-windows-amd64.zip", link.URL},
		{"platform=windows&arch=arm64&asset=tool-windows-amd64.zip", "https://github.com/acme/tool/releases/tag/v1.0.0"},
		{"platform=linux&arch=amd64&asset=tool-windows-amd64.zip", "https://github.com/acme/tool/releases/tag/v1.0.0"},
		{"platform=windows&arch=amd64&asset=tool-windows-amd64-portable.zip", "https://github.com/acme/tool/releases/tag/v1.0.0"},
	} {
		w := request("/dl/acme/tool/v1.0.0?" + tc.query)
		if w.Code != http.StatusFound || w.Header().Get("Location") != tc.want {
			t.Errorf("pin %s: %d %q, want %q", tc.query, w.Code, w.Header().Get("Location"), tc.want)
		}
	}
}

// Supplying a pin during a service failure cannot mint an enduring offer.
func TestPrimaryPinnedLinkCannotCreateOffer(t *testing.T) {
	router := primaryRouter(t, http.StatusInternalServerError)
	transport := http.DefaultTransport
	var recovered atomic.Bool
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "api.typesafe.ai" && recovered.Load() {
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Request: req,
				Body: io.NopCloser(strings.NewReader(fakeJevAnswers(t, req)))}, nil
		}
		return transport.RoundTrip(req)
	})
	request := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("User-Agent", windowsUA)
		router.ServeHTTP(w, req)
		return w
	}
	pin := "platform=windows&arch=amd64&asset=tool-windows-amd64-portable.zip"
	if w := request("/api/link/acme/tool?" + pin); w.Code != http.StatusOK {
		t.Fatalf("precondition: pinned rule fallback failed: %d %s", w.Code, w.Body)
	}
	recovered.Store(true)
	if w := request("/api/link/acme/tool?platform=windows&arch=amd64"); w.Code != http.StatusNotFound {
		t.Fatalf("recovered Jev must abstain: %d %s", w.Code, w.Body)
	}
	if w := request("/dl/acme/tool?" + pin); w.Header().Get("Location") != "https://github.com/acme/tool/releases/tag/v1.0.0" {
		t.Fatalf("client-supplied pin created an offer: %d %q", w.Code, w.Header().Get("Location"))
	}
}
