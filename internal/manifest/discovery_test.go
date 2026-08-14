package manifest

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverManifestPathsWithDeps_FallsBackToSharedApps(t *testing.T) {
	tmp := t.TempDir()
	sharedManifest := filepath.Join(tmp, "apps", "demo", "stevedore.yml")
	if err := os.MkdirAll(filepath.Dir(sharedManifest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sharedManifest, []byte("image:\n  repository: nginx\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	paths, err := discoverManifestPathsWithDeps(tmp, discoveryDeps{
		glob:     filepath.Glob,
		stat:     os.Stat,
		hostFQDN: func() (string, error) { return "host.example.com", nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0] != sharedManifest {
		t.Fatalf("expected shared manifest only, got %v", paths)
	}
}

func TestDiscoverManifestPathsWithDeps_CombinesHostAndSharedApps(t *testing.T) {
	tmp := t.TempDir()
	hostManifest := filepath.Join(tmp, "host.example.com", "apps", "host-only", "stevedore.yml")
	sharedManifest := filepath.Join(tmp, "apps", "shared", "stevedore.yml")
	for _, manifestPath := range []string{hostManifest, sharedManifest} {
		if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(manifestPath, []byte("image:\n  repository: nginx\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	paths, err := discoverManifestPathsWithDeps(tmp, discoveryDeps{
		glob:     filepath.Glob,
		stat:     os.Stat,
		hostFQDN: func() (string, error) { return "host.example.com", nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 {
		t.Fatalf("expected 2 manifests, got %d (%v)", len(paths), paths)
	}
	if paths[0] != sharedManifest || paths[1] != hostManifest {
		t.Fatalf("unexpected manifest set/order: %v", paths)
	}
}

func TestDiscoverManifestPathsWithDeps_FailsWhenFQDNLookupFails(t *testing.T) {
	tmp := t.TempDir()

	_, err := discoverManifestPathsWithDeps(tmp, discoveryDeps{
		glob: filepath.Glob,
		stat: os.Stat,
		hostFQDN: func() (string, error) {
			return "", errors.New("boom")
		},
	})
	if err == nil {
		t.Fatal("expected fqdn failure to be returned")
	}
}
