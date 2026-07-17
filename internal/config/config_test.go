package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, ConfigFileName)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadLocalSourceDefaults(t *testing.T) {
	p := writeConfig(t, `source:
  local:
    path: /srv/apps-repo
`)

	cfg, err := LoadFromPath(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Source.IsGit() {
		t.Fatal("expected local source")
	}
	if cfg.Poll.Interval != DefaultInterval {
		t.Fatalf("expected default interval %s, got %s", DefaultInterval, cfg.Poll.Interval)
	}
	if cfg.ApacheSitesDir != DefaultApacheSitesDir {
		t.Fatalf("expected default apache dir, got %s", cfg.ApacheSitesDir)
	}
	if cfg.RepoRoot() != "/srv/apps-repo" {
		t.Fatalf("unexpected repo root: %s", cfg.RepoRoot())
	}
	if cfg.Logging.Dir != DefaultLogDir {
		t.Fatalf("expected default log dir, got %s", cfg.Logging.Dir)
	}
	if cfg.Logging.Debug {
		t.Fatal("expected debug false by default")
	}
}

func TestEnvOverridesTakePrecedenceOverFile(t *testing.T) {
	p := writeConfig(t, `logging:
  dir: /file/logs
  debug: false
source:
  local:
    path: /srv/apps-repo
poll:
  interval: 30s
apacheSitesDir: /file/sites
`)

	t.Setenv(LogDirEnvVar, "/env/logs")
	t.Setenv(DebugEnvVar, "true")
	t.Setenv(IntervalEnvVar, "5s")
	t.Setenv(ApacheSitesDirEnvVar, "/env/sites")

	cfg, err := LoadFromPath(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Logging.Dir != "/env/logs" {
		t.Fatalf("expected env log dir, got %s", cfg.Logging.Dir)
	}
	if !cfg.Logging.Debug {
		t.Fatal("expected env debug=true to win over file")
	}
	if cfg.Poll.Interval != 5*time.Second {
		t.Fatalf("expected env interval 5s, got %s", cfg.Poll.Interval)
	}
	if cfg.ApacheSitesDir != "/env/sites" {
		t.Fatalf("expected env apache dir, got %s", cfg.ApacheSitesDir)
	}
}

func TestFileValuesUsedWhenNoEnv(t *testing.T) {
	p := writeConfig(t, `logging:
  dir: /file/logs
  debug: true
source:
  local:
    path: /srv/apps-repo
`)

	cfg, err := LoadFromPath(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Logging.Dir != "/file/logs" {
		t.Fatalf("expected file log dir, got %s", cfg.Logging.Dir)
	}
	if !cfg.Logging.Debug {
		t.Fatal("expected file debug=true")
	}
}

func TestLoadGitSourceDefaultsAndTokenAuth(t *testing.T) {
	t.Setenv("GIT_TOKEN", "secret-token")
	p := writeConfig(t, `source:
  git:
    url: https://example.com/org/repo.git
    auth:
      token:
        value: ${GIT_TOKEN}
poll:
  interval: 15s
`)

	cfg, err := LoadFromPath(p)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Source.IsGit() {
		t.Fatal("expected git source")
	}
	if cfg.Poll.Interval != 15*time.Second {
		t.Fatalf("expected 15s interval, got %s", cfg.Poll.Interval)
	}
	if cfg.Source.Git.Branch != "main" {
		t.Fatalf("expected default branch main, got %s", cfg.Source.Git.Branch)
	}
	if cfg.Source.Git.Workdir == "" {
		t.Fatal("expected git workdir default to be set")
	}
	if cfg.Source.Git.Auth.Method() != AuthToken {
		t.Fatalf("expected token auth, got %s", cfg.Source.Git.Auth.Method())
	}
	if cfg.Source.Git.Auth.Token.Value != "secret-token" {
		t.Fatalf("expected token env expansion, got %q", cfg.Source.Git.Auth.Token.Value)
	}
	if cfg.RepoRoot() != cfg.Source.Git.Workdir {
		t.Fatalf("git repo root should equal workdir")
	}
}

func TestLoadGitSourceNoAuth(t *testing.T) {
	p := writeConfig(t, `source:
  git:
    url: https://example.com/org/repo.git
`)
	cfg, err := LoadFromPath(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Source.Git.Auth.Method() != AuthNone {
		t.Fatalf("expected no auth, got %s", cfg.Source.Git.Auth.Method())
	}
}

func TestValidateRejectsInvalidSources(t *testing.T) {
	cases := map[string]string{
		"no source":              "poll:\n  interval: 5s\n",
		"both sources":           "source:\n  local:\n    path: ./apps\n  git:\n    url: https://example.com/repo.git\n",
		"local without path":     "source:\n  local: {}\n",
		"git without url":        "source:\n  git:\n    auth:\n      token:\n        value: abc\n",
		"multiple auth methods":  "source:\n  git:\n    url: https://example.com/repo.git\n    auth:\n      token:\n        value: abc\n      ssh:\n        keyPath: /key\n",
		"token without value":    "source:\n  git:\n    url: https://example.com/repo.git\n    auth:\n      token: {}\n",
		"basic missing password": "source:\n  git:\n    url: https://example.com/repo.git\n    auth:\n      basic:\n        username: alice\n",
		"ssh missing key":        "source:\n  git:\n    url: git@example.com:org/repo.git\n    auth:\n      ssh: {}\n",
	}

	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			p := writeConfig(t, content)
			if _, err := LoadFromPath(p); err == nil {
				t.Fatalf("expected validation error for %s", name)
			}
		})
	}
}

func TestLoadFromPathHonoursHomeEnvOverride(t *testing.T) {
	home := t.TempDir()
	p := filepath.Join(home, ConfigFileName)
	if err := os.WriteFile(p, []byte(`source:
  local:
    path: ./apps
`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(HomeEnvVar, home)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Path() != p {
		t.Fatalf("expected config path %s, got %s", p, cfg.Path())
	}
}

func TestLocalSecretsFileResolvesRelativeToConfigDir(t *testing.T) {
	configDir := t.TempDir()
	configPath := filepath.Join(configDir, ConfigFileName)
	content := `source:
  local:
    path: /srv/apps-repo
secrets:
  providers:
    local:
      file: ./secrets/store.yml
`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFromPath(configPath)
	if err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(configDir, "secrets", "store.yml")
	if got := cfg.LocalSecretsFile(); got != want {
		t.Fatalf("expected local secrets file %q, got %q", want, got)
	}
}

func TestValidateRejectsLocalSecretsProviderWithoutFile(t *testing.T) {
	p := writeConfig(t, `source:
  local:
    path: /srv/apps-repo
secrets:
  providers:
    local: {}
`)

	if _, err := LoadFromPath(p); err == nil {
		t.Fatal("expected validation error when local secrets provider file is missing")
	}
}
