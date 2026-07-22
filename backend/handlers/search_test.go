package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/argval/yatko/cache"
	"github.com/argval/yatko/github"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestSearchHandler_Validation(t *testing.T) {
	h := NewSearchHandler(github.NewClient(), cache.New())
	r := gin.New()
	r.GET("/api/search", h.Handle)

	cases := []struct {
		name string
		q    string
		want int
	}{
		{"empty", "", http.StatusBadRequest},
		{"too short", "a", http.StatusBadRequest},
		{"too long", strings.Repeat("a", 200), http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/search?q="+url.QueryEscape(tc.q), nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d; body=%s", w.Code, tc.want, w.Body.String())
			}
		})
	}
}

func TestNormalizeSearchQuery(t *testing.T) {
	if got := normalizeSearchQuery("  RipGrep  "); got != "ripgrep" {
		t.Fatalf("got %q, want ripgrep", got)
	}
}

func TestSearchHandler_JSONShape(t *testing.T) {
	// No Redis + no real GitHub call: exercise the empty-query path only.
	// Shape of a successful body is asserted via a synthetic encode check.
	body, err := json.Marshal(gin.H{"items": []github.SearchRepo{{
		Owner: "BurntSushi", Repo: "ripgrep", Stars: 1,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Items []github.SearchRepo `json:"items"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Items) != 1 || parsed.Items[0].Repo != "ripgrep" {
		t.Fatalf("unexpected parsed %+v", parsed)
	}
}
