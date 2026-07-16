package manifest

import (
	"os"
	"path/filepath"
	"testing"
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
	if err := os.WriteFile(filepath.Join(appDir, "stevedore.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	apps, err := LoadApplications(tmp)
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
	if err := os.WriteFile(filepath.Join(appDir, "stevedore.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	apps, err := LoadApplications(tmp)
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
	if err := os.WriteFile(filepath.Join(appDir, "stevedore.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadApplications(tmp)
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

	if err := os.WriteFile(filepath.Join(apiDir, "stevedore.yaml"), []byte(api), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(uiDir, "stevedore.yaml"), []byte(ui), 0o644); err != nil {
		t.Fatal(err)
	}

	apps, err := LoadApplications(tmp)
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

	if err := os.WriteFile(filepath.Join(apiDir, "stevedore.yaml"), []byte(api), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(uiDir, "stevedore.yaml"), []byte(ui), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadApplications(tmp); err == nil {
		t.Fatal("expected unresolved interpolation to fail")
	}
}
