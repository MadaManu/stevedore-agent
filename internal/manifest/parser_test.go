package manifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"stevedore-agent/internal/secrets"
)

func appByName(apps []Application, name string) *Application {
	for i := range apps {
		if apps[i].Metadata.Name == name {
			return &apps[i]
		}
	}
	return nil
}

func TestLoadApplicationsDefaultsNameFromFolder(t *testing.T) {
	tmp := t.TempDir()
	appDir := filepath.Join(tmp, "apps", "demo")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `image:
  repository: nginx
  tag: latest
`
	if err := os.WriteFile(filepath.Join(appDir, "stevedore.yml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	apps, err := LoadApplications(tmp, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 1 {
		t.Fatalf("expected 1 app, got %d", len(apps))
	}
	if apps[0].Metadata.Name != "demo" {
		t.Fatalf("expected default name demo, got %s", apps[0].Metadata.Name)
	}
}

func TestLoadApplicationsAcceptsMatchingMetadataName(t *testing.T) {
	tmp := t.TempDir()
	appDir := filepath.Join(tmp, "apps", "demo")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `metadata:
  name: demo
image:
  repository: nginx
  tag: latest
`
	if err := os.WriteFile(filepath.Join(appDir, "stevedore.yml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	apps, err := LoadApplications(tmp, nil)
	if err != nil {
		t.Fatal(err)
	}
	if apps[0].Metadata.Name != "demo" {
		t.Fatalf("expected folder-derived name demo, got %s", apps[0].Metadata.Name)
	}
}

func TestLoadApplicationsRejectsMismatchedMetadataName(t *testing.T) {
	tmp := t.TempDir()
	appDir := filepath.Join(tmp, "apps", "demo")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `metadata:
  name: not-demo
image:
  repository: nginx
  tag: latest
`
	if err := os.WriteFile(filepath.Join(appDir, "stevedore.yml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadApplications(tmp, nil)
	if err == nil {
		t.Fatal("expected mismatched metadata.name to fail validation")
	}
}

func TestLoadApplicationsResolvesCrossAppReferences(t *testing.T) {
	tmp := t.TempDir()
	apiDir := filepath.Join(tmp, "apps", "demo-api")
	uiDir := filepath.Join(tmp, "apps", "demo-ui")
	if err := os.MkdirAll(apiDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(uiDir, 0o755); err != nil {
		t.Fatal(err)
	}

	api := `image:
  repository: nginx
  tag: 1.31.3
expose:
  enabled: true
  provider: apache
  config:
    domain: demo-api.local
    ssl: false
`
	ui := `image:
  repository: nginx
  tag: 1.31.3
environment:
  API_BASE_URL: "http${demo-api.expose.config.ssl?'s':''}://${demo-api.expose.config.domain}"
  API_HOST: "${demo-api.expose.config.domain}"
`

	if err := os.WriteFile(filepath.Join(apiDir, "stevedore.yml"), []byte(api), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(uiDir, "stevedore.yml"), []byte(ui), 0o644); err != nil {
		t.Fatal(err)
	}

	apps, err := LoadApplications(tmp, nil)
	if err != nil {
		t.Fatal(err)
	}

	uiApp := appByName(apps, "demo-ui")
	if uiApp == nil {
		t.Fatal("demo-ui app not loaded")
	}
	if got, want := uiApp.Environment["API_BASE_URL"], "http://demo-api.local"; got != want {
		t.Fatalf("API_BASE_URL mismatch: want %q got %q", want, got)
	}
	if got, want := uiApp.Environment["API_HOST"], "demo-api.local"; got != want {
		t.Fatalf("API_HOST mismatch: want %q got %q", want, got)
	}
}

func TestLoadApplicationsFailsOnUnknownInterpolationPath(t *testing.T) {
	tmp := t.TempDir()
	apiDir := filepath.Join(tmp, "apps", "demo-api")
	uiDir := filepath.Join(tmp, "apps", "demo-ui")
	if err := os.MkdirAll(apiDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(uiDir, 0o755); err != nil {
		t.Fatal(err)
	}

	api := `image:
  repository: nginx
`
	ui := `image:
  repository: nginx
environment:
  API_BASE_URL: "${demo-api.expose.config.missing}"
`

	if err := os.WriteFile(filepath.Join(apiDir, "stevedore.yml"), []byte(api), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(uiDir, "stevedore.yml"), []byte(ui), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadApplications(tmp, nil); err == nil {
		t.Fatal("expected unresolved interpolation to fail")
	}
}

func TestLoadApplicationsResolvesLocalSecretReferences(t *testing.T) {
	tmp := t.TempDir()
	uiDir := filepath.Join(tmp, "apps", "demo-ui")
	if err := os.MkdirAll(uiDir, 0o755); err != nil {
		t.Fatal(err)
	}

	storePath := filepath.Join(tmp, "secrets.yml")
	if err := os.WriteFile(storePath, []byte("api:\n  key: super-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	ui := `image:
  repository: nginx
environment:
  API_KEY: "${local:api/key}"
`
	if err := os.WriteFile(filepath.Join(uiDir, "stevedore.yml"), []byte(ui), 0o644); err != nil {
		t.Fatal(err)
	}

	provider, err := secrets.NewLocalProvider(storePath)
	if err != nil {
		t.Fatal(err)
	}
	resolver := secrets.NewResolver(map[string]secrets.Provider{"local": provider})

	apps, err := LoadApplications(tmp, resolver)
	if err != nil {
		t.Fatal(err)
	}

	uiApp := appByName(apps, "demo-ui")
	if uiApp == nil {
		t.Fatal("demo-ui app not loaded")
	}
	if got, want := uiApp.Environment["API_KEY"], "super-secret"; got != want {
		t.Fatalf("API_KEY mismatch: want %q got %q", want, got)
	}
}

func TestLoadApplicationsFailsWhenSecretProviderMissing(t *testing.T) {
	tmp := t.TempDir()
	uiDir := filepath.Join(tmp, "apps", "demo-ui")
	if err := os.MkdirAll(uiDir, 0o755); err != nil {
		t.Fatal(err)
	}

	ui := `image:
  repository: nginx
environment:
  API_KEY: "${local:api/key}"
`
	if err := os.WriteFile(filepath.Join(uiDir, "stevedore.yml"), []byte(ui), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadApplications(tmp, nil); err == nil {
		t.Fatal("expected missing secret provider to fail")
	}
}

func TestLoadApplicationsFailsOnDuplicateNetworks(t *testing.T) {
	tmp := t.TempDir()
	appDir := filepath.Join(tmp, "apps", "demo")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}

	content := `image:
  repository: nginx
networks:
  - name: apps
  - name: apps
`
	if err := os.WriteFile(filepath.Join(appDir, "stevedore.yml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadApplications(tmp, nil); err == nil {
		t.Fatal("expected duplicate networks to fail")
	}
}

func TestLoadApplicationsFailsWhenPrimaryNetworkDuplicatesNetworksList(t *testing.T) {
	tmp := t.TempDir()
	appDir := filepath.Join(tmp, "apps", "demo")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}

	content := `image:
  repository: nginx
network:
  name: apps
networks:
  - name: apps
`
	if err := os.WriteFile(filepath.Join(appDir, "stevedore.yml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadApplications(tmp, nil); err == nil {
		t.Fatal("expected duplicate primary network to fail")
	}
}

func TestLoadApplicationsFailsWhenLegacyVolumeMissingRequiredFields(t *testing.T) {
	tmp := t.TempDir()
	appDir := filepath.Join(tmp, "apps", "demo")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}

	content := `image:
  repository: nginx
volume:
  hostPath: /tmp/demo
`
	if err := os.WriteFile(filepath.Join(appDir, "stevedore.yml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadApplications(tmp, nil); err == nil {
		t.Fatal("expected incomplete legacy volume declaration to fail")
	}
}

func TestLoadApplicationsFailsOnDuplicateVolumeMountPaths(t *testing.T) {
	tmp := t.TempDir()
	appDir := filepath.Join(tmp, "apps", "demo")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}

	content := `image:
  repository: nginx
volumes:
  - hostPath: /data/one
    mountPath: /srv/data
  - hostPath: /data/two
    mountPath: /srv/data
`
	if err := os.WriteFile(filepath.Join(appDir, "stevedore.yml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadApplications(tmp, nil); err == nil {
		t.Fatal("expected duplicate mount paths to fail")
	}
}

func TestLoadApplicationsFailsWhenLegacyVolumeDuplicatesVolumesListMountPath(t *testing.T) {
	tmp := t.TempDir()
	appDir := filepath.Join(tmp, "apps", "demo")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}

	content := `image:
  repository: nginx
volume:
  hostPath: /data/primary
  mountPath: /srv/data
volumes:
  - hostPath: /data/secondary
    mountPath: /srv/data
`
	if err := os.WriteFile(filepath.Join(appDir, "stevedore.yml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadApplications(tmp, nil); err == nil {
		t.Fatal("expected duplicate mount path across volume and volumes to fail")
	}
}

func TestLoadApplicationsFailsOnDuplicateAppAcrossHostAndSharedDirs(t *testing.T) {
	tmp := t.TempDir()
	fqdn, err := HostFQDN()
	if err != nil {
		t.Fatal(err)
	}

	sharedAppDir := filepath.Join(tmp, "apps", "demo")
	hostAppDir := filepath.Join(tmp, fqdn, "apps", "demo")
	for _, dir := range []string{sharedAppDir, hostAppDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		content := `image:
  repository: nginx
`
		if err := os.WriteFile(filepath.Join(dir, "stevedore.yml"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	_, err = LoadApplications(tmp, nil)
	if err == nil {
		t.Fatal("expected duplicate app name across host and shared manifests to fail")
	}
	if !strings.Contains(err.Error(), "duplicate application name") {
		t.Fatalf("expected duplicate app name error, got %v", err)
	}
	if !strings.Contains(err.Error(), filepath.Join(sharedAppDir, "stevedore.yml")) || !strings.Contains(err.Error(), filepath.Join(hostAppDir, "stevedore.yml")) {
		t.Fatalf("expected error to include both duplicate manifest paths, got %v", err)
	}
}
