package archive

import (
	"testing"
)

func TestParseGroups(t *testing.T) {
	groups, err := ParseGroups("permissionlesstech/bitchat|argval/bitchat;permissionlesstech/bitchat-android|argval/bitchat-android")
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 {
		t.Fatalf("got %d groups, want 2", len(groups))
	}
	if groups[0].Canonical() != "permissionlesstech/bitchat" {
		t.Fatalf("canonical = %q", groups[0].Canonical())
	}
	if !groups[0].Contains("argval", "bitchat") {
		t.Fatal("expected argval/bitchat in group 0")
	}
	if !groups[1].Contains("permissionlesstech", "bitchat-android") {
		t.Fatal("expected android upstream in group 1")
	}
}

func TestParseGroups_EmptyAndDash(t *testing.T) {
	for _, s := range []string{"", "-", "  "} {
		g, err := ParseGroups(s)
		if err != nil {
			t.Fatalf("%q: %v", s, err)
		}
		if len(g) != 0 {
			t.Fatalf("%q: got %d groups", s, len(g))
		}
	}
}

func TestParseGroups_Invalid(t *testing.T) {
	if _, err := ParseGroups("nonsplit"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := ParseGroups("a/b/c"); err == nil {
		t.Fatal("expected error for extra slash")
	}
}

func TestParseGroups_Dedup(t *testing.T) {
	g, err := ParseGroups("owner/repo|owner/repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(g) != 1 || len(g[0].Aliases) != 1 {
		t.Fatalf("got %+v", g)
	}
}
