package archive

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/argval/yatko/cache"
	"github.com/argval/yatko/github"
)

func TestOverlay_RewritesAndInjectsSource(t *testing.T) {
	rel := &github.Release{
		TagName: "1.0.0",
		Assets: []github.Asset{
			{Name: "app.apk", BrowserDownloadURL: "https://github.com/x/app.apk", Size: 10},
		},
	}
	m := &Manifest{
		Tag: "1.0.0",
		Assets: []ManifestAsset{
			{Name: "app.apk", URL: "https://blob.example/app.apk", Size: 99, ContentType: "application/vnd.android.package-archive"},
			{Name: "repo-1.0.0-source.zip", URL: "https://blob.example/src.zip", Size: 50, ContentType: "application/zip"},
		},
	}
	out := Overlay(rel, m)
	if out.Assets[0].BrowserDownloadURL != "https://blob.example/app.apk" {
		t.Fatalf("apk url = %q", out.Assets[0].BrowserDownloadURL)
	}
	if rel.Assets[0].BrowserDownloadURL != "https://github.com/x/app.apk" {
		t.Fatal("Overlay mutated original release")
	}
	if len(out.Assets) != 2 {
		t.Fatalf("want injected source, got %d assets", len(out.Assets))
	}
	if out.Assets[1].Name != "repo-1.0.0-source.zip" {
		t.Fatalf("source name = %q", out.Assets[1].Name)
	}
}

func TestResolve_GitHubDownUsesManifest(t *testing.T) {
	c := cache.New() // L1-only is enough for permanent manifest tests
	ctx := context.Background()
	groups := []Group{{Aliases: []string{"permissionlesstech/bitchat", "argval/bitchat"}}}
	svc := New(groups, nil, c, nil)

	m := &Manifest{
		Canonical: "permissionlesstech/bitchat",
		Tag:       "v1.7.0",
		SyncedAt:  time.Now().UTC(),
		Assets: []ManifestAsset{
			{Name: "bitchat-v1.7.0-source.zip", URL: "https://blob.example/src.zip", Size: 1, ContentType: "application/zip"},
		},
	}
	if err := SaveManifest(ctx, c, groups[0], m); err != nil {
		t.Fatal(err)
	}

	got, err := svc.Resolve(ctx, "argval", "bitchat", "", nil, errors.New("github down"))
	if err != nil {
		t.Fatal(err)
	}
	if got.TagName != "v1.7.0" || len(got.Assets) != 1 {
		t.Fatalf("unexpected %+v", got)
	}
	if got.Assets[0].BrowserDownloadURL != "https://blob.example/src.zip" {
		t.Fatalf("url = %q", got.Assets[0].BrowserDownloadURL)
	}
}

func TestResolve_VersionMismatchSkipsArchive(t *testing.T) {
	c := cache.New()
	ctx := context.Background()
	groups := []Group{{Aliases: []string{"o/r"}}}
	svc := New(groups, nil, c, nil)
	_ = SaveManifest(ctx, c, groups[0], &Manifest{Tag: "1.0.0", Assets: []ManifestAsset{{Name: "a.zip", URL: "https://blob/a"}}})

	fetchErr := errors.New("boom")
	got, err := svc.Resolve(ctx, "o", "r", "9.9.9", nil, fetchErr)
	if !errors.Is(err, fetchErr) {
		t.Fatalf("err = %v", err)
	}
	if got != nil {
		t.Fatal("expected nil release")
	}
}

func TestResolve_PassthroughNonArchived(t *testing.T) {
	svc := New(nil, nil, cache.New(), nil)
	rel := &github.Release{TagName: "1"}
	got, err := svc.Resolve(context.Background(), "a", "b", "", rel, nil)
	if err != nil || got != rel {
		t.Fatalf("got %v %v", got, err)
	}
}

func TestSaveAndLoadManifest_SharedAcrossAliases(t *testing.T) {
	c := cache.New()
	ctx := context.Background()
	g := Group{Aliases: []string{"up/repo", "fork/repo"}}
	m := &Manifest{Canonical: "up/repo", Tag: "t", Assets: []ManifestAsset{{Name: "f", URL: "u"}}}
	if err := SaveManifest(ctx, c, g, m); err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]string{{"up", "repo"}, {"fork", "repo"}} {
		got, err := LoadManifest(ctx, c, pair[0], pair[1])
		if err != nil || got == nil || got.Tag != "t" {
			t.Fatalf("%s/%s: got %+v err=%v", pair[0], pair[1], got, err)
		}
	}
}
