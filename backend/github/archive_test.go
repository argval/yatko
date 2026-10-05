package github

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func archiveFixture(t *testing.T, names []string, padding bool) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	if padding {
		f, err := w.CreateHeader(&zip.FileHeader{Name: "padding", Method: zip.Store})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write(make([]byte, archiveTailLimit*2)); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range names {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(name, "/") {
			continue
		}
		if _, err := f.Write([]byte("fixture")); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestZIPPlatformDirectory(t *testing.T) {
	for _, tt := range []struct {
		name    string
		entries []string
		want    string
	}{
		{"mac app", []string{"App.app/Contents/Info.plist", "App.app/Contents/MacOS/App"}, "macos"},
		{"nested app", []string{"release/App.app/Contents/MacOS/App", "release/App.app/Contents/Info.plist"}, "macos"},
		{"source", []string{"src/main.swift", "README.md"}, ""},
		{"resource forks", []string{"__MACOSX/App.app/Contents/Info.plist", "__MACOSX/App.app/Contents/MacOS/App"}, ""},
		{"plist only", []string{"App.app/Contents/Info.plist"}, ""},
		{"executable only", []string{"App.app/Contents/MacOS/App"}, ""},
		{"empty executable directory", []string{"App.app/Contents/Info.plist", "App.app/Contents/MacOS/"}, ""},
		{"different bundles", []string{"A.app/Contents/Info.plist", "B.app/Contents/MacOS/B"}, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			data := archiveFixture(t, tt.entries, true)
			start := int64(len(data) - archiveTailLimit)
			if got := zipPlatform(data[start:], start, int64(len(data))); got != tt.want {
				t.Fatalf("platform = %q, want %q", got, tt.want)
			}
		})
	}
	if got := zipPlatform([]byte("not a zip"), 0, 9); got != "" {
		t.Fatalf("invalid ZIP = %q", got)
	}
}

type archiveTransport func(*http.Request) (*http.Response, error)

func (f archiveTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestZIPPlatformRange(t *testing.T) {
	data := archiveFixture(t, []string{"App.app/Contents/Info.plist", "App.app/Contents/MacOS/App"}, true)
	start := len(data) - archiveTailLimit
	for _, tt := range []struct {
		name         string
		status       int
		contentRange string
		body         []byte
		want         string
	}{
		{"partial content", 206, fmt.Sprintf("bytes %d-%d/%d", start, len(data)-1, len(data)), data[start:], "macos"},
		{"ignores range", 200, "", data, ""},
		{"wrong offset", 206, "bytes 0-99/100", data[start:], ""},
		{"truncated", 206, fmt.Sprintf("bytes %d-%d/%d", start, len(data)-1, len(data)), data[start : len(data)-1], ""},
		{"not found", 404, "", nil, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := &Client{token: "secret", httpClient: &http.Client{Transport: archiveTransport(func(r *http.Request) (*http.Response, error) {
				if got, want := r.Header.Get("Range"), fmt.Sprintf("bytes=%d-%d", start, len(data)-1); got != want {
					t.Fatalf("Range = %q, want %q", got, want)
				}
				if r.Header.Get("Authorization") != "" {
					t.Fatal("token sent to download URL")
				}
				h := make(http.Header)
				h.Set("Content-Range", tt.contentRange)
				return &http.Response{StatusCode: tt.status, Header: h, Body: io.NopCloser(bytes.NewReader(tt.body))}, nil
			})}}
			asset := Asset{BrowserDownloadURL: "https://github.com/acme/app/releases/download/v1/App.zip", Size: int64(len(data))}
			if got, _ := c.ZIPPlatform(context.Background(), asset); got != tt.want {
				t.Fatalf("platform = %q, want %q", got, tt.want)
			}
		})
	}
}
