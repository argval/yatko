package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCodeArchiveURL(t *testing.T) {
	tests := []struct {
		name, owner, repo, tag, want string
	}{
		{
			name:  "default branch",
			owner: "cli",
			repo:  "cli",
			want:  "https://github.com/cli/cli/archive/HEAD.zip",
		},
		{
			name:  "tag is escaped",
			owner: "acme",
			repo:  "tool",
			tag:   "release/v1.0?draft",
			want:  "https://github.com/acme/tool/archive/refs/tags/release%2Fv1.0%3Fdraft.zip",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := codeArchiveURL(tt.owner, tt.repo, tt.tag); got != tt.want {
				t.Fatalf("codeArchiveURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHandleCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/code/:owner/:repo", HandleCode)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/code/cli/cli?tag=v2.83.1", nil))

	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusFound)
	}
	if got, want := w.Header().Get("Location"), "https://github.com/cli/cli/archive/refs/tags/v2.83.1.zip"; got != want {
		t.Fatalf("Location = %q, want %q", got, want)
	}
}
