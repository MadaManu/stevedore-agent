package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHashManifestsChangesOnEdit(t *testing.T) {
	tmp := t.TempDir()
	appDir := filepath.Join(tmp, "apps", "demo")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(appDir, "stevedore.yml")
	if err := os.WriteFile(manifestPath, []byte("image:\n  repository: nginx\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	first, err := HashManifests(tmp)
	if err != nil {
		t.Fatal(err)
	}

	second, err := HashManifests(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("hash should be stable across unchanged reads")
	}

	if err := os.WriteFile(manifestPath, []byte("image:\n  repository: httpd\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	third, err := HashManifests(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if third == first {
		t.Fatal("hash should change when a manifest changes")
	}
}

func TestHashManifestsChangesOnAdd(t *testing.T) {
	tmp := t.TempDir()
	appDir := filepath.Join(tmp, "apps", "demo")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "stevedore.yml"), []byte("image:\n  repository: nginx\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := HashManifests(tmp)
	if err != nil {
		t.Fatal(err)
	}

	otherDir := filepath.Join(tmp, "apps", "extra")
	if err := os.MkdirAll(otherDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(otherDir, "stevedore.yml"), []byte("image:\n  repository: redis\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	after, err := HashManifests(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("hash should change when a new manifest is added")
	}
}
