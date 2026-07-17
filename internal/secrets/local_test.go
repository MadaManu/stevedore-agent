package secrets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLocalProviderResolveFromYAML(t *testing.T) {
	dir := t.TempDir()
	store := filepath.Join(dir, "secrets.yml")
	if err := os.WriteFile(store, []byte(`db:
  password: s3cr3t
nested:
  list:
    - one
    - two
`), 0o600); err != nil {
		t.Fatal(err)
	}

	provider, err := NewLocalProvider(store)
	if err != nil {
		t.Fatal(err)
	}

	got, err := provider.Resolve("db/password")
	if err != nil {
		t.Fatal(err)
	}
	if got != "s3cr3t" {
		t.Fatalf("expected secret value, got %q", got)
	}

	listValue, err := provider.Resolve("nested/list/1")
	if err != nil {
		t.Fatal(err)
	}
	if listValue != "two" {
		t.Fatalf("expected list item 'two', got %q", listValue)
	}
}

func TestLocalProviderResolveFromJSON(t *testing.T) {
	dir := t.TempDir()
	store := filepath.Join(dir, "secrets.json")
	if err := os.WriteFile(store, []byte(`{"api":{"token":"abc123"}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	provider, err := NewLocalProvider(store)
	if err != nil {
		t.Fatal(err)
	}

	got, err := provider.Resolve("api/token")
	if err != nil {
		t.Fatal(err)
	}
	if got != "abc123" {
		t.Fatalf("expected secret value, got %q", got)
	}
}

func TestLocalProviderResolveFailsForMissingPath(t *testing.T) {
	dir := t.TempDir()
	store := filepath.Join(dir, "secrets.yml")
	if err := os.WriteFile(store, []byte("db:\n  password: s3cr3t\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	provider, err := NewLocalProvider(store)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := provider.Resolve("db/username"); err == nil {
		t.Fatal("expected missing secret path to fail")
	}
}
