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
	if got := NormalizeQuery("  Clip "); got != "clip" {
		t.Fatalf("got %q", got)
	}
}
