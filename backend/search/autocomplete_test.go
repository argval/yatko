package search

import (
	"testing"

	"github.com/argval/yatko/github"
)

func TestFilterItems(t *testing.T) {
	items := []github.SearchRepo{
		{Owner: "cli", Repo: "cli"},
		{Owner: "charmbracelet", Repo: "bubbletea"},
		{Owner: "clipper", Repo: "tool"},
	}

	got := FilterItems(items, "clip")
	if len(got) != 1 || got[0].Owner != "clipper" {
		t.Fatalf("expected only clipper/tool, got %+v", got)
	}

	got = FilterItems(items, "cli")
	if len(got) != 2 {
		t.Fatalf("expected cli/cli and clipper/tool, got %+v", got)
	}
}

func TestNormalizeQuery(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"  Clip ", "clip"},
		{"https://github.com/astral-sh/uv", "astral-sh/uv"},
		{"http://github.com/astral-sh/uv", "astral-sh/uv"},
		{"https://www.github.com/astral-sh/uv", "astral-sh/uv"},
		{"github.com/astral-sh/uv", "astral-sh/uv"},
		{"https://github.com/astral-sh/uv/", "astral-sh/uv"},
		{"https://github.com/astral-sh/uv.git", "astral-sh/uv"},
		{"https://github.com/astral-sh/uv/releases/tag/0.4.0", "astral-sh/uv"},
		{"HTTPS://GitHub.com/Astral-Sh/UV", "astral-sh/uv"},
		{"astral-sh/uv", "astral-sh/uv"},
		{"uv", "uv"},
	}
	for _, tc := range cases {
		if got := NormalizeQuery(tc.in); got != tc.want {
			t.Fatalf("NormalizeQuery(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
