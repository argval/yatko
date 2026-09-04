package picker

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCatalogMatchesShared(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	shared := filepath.Join(filepath.Dir(thisFile), "..", "..", "shared", "picker", "catalog.json")
	data, err := os.ReadFile(shared)
	if err != nil {
		t.Fatalf("read %s: %v", shared, err)
	}
	if !bytes.Equal(bytes.TrimSpace(data), bytes.TrimSpace(catalogJSON)) {
		t.Fatal("backend/picker/catalog.json is out of sync with shared/picker/catalog.json — copy the shared file into backend/picker/")
	}
}

func TestResolvePreferUsesCatalog(t *testing.T) {
	if got := ResolvePrefer("app-image"); got != "appimage" {
		t.Fatalf("ResolvePrefer(app-image) = %q", got)
	}
	if got := ResolvePrefer(".DMG"); got != "dmg" {
		t.Fatalf("ResolvePrefer(.DMG) = %q", got)
	}
}
