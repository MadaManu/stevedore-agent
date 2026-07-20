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
	// Apache is optional — no default.
	if cfg.Exposure.Apache != nil {
		t.Fatal("expected no apache exposure config by default")
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
exposure:
  apache:
    sitesDir: /file/sites
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
	if cfg.Exposure.Apache == nil || cfg.Exposure.Apache.SitesDir != "/env/sites" {
		t.Fatalf("expected env apache sites dir, got %v", cfg.Exposure.Apache)
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
	wantDefaultWorkdir := filepath.Join(DefaultHomeDir, DefaultGitSourceDirName)
	if cfg.Source.Git.Workdir != wantDefaultWorkdir {
		t.Fatalf("expected default git workdir %q, got %q", wantDefaultWorkdir, cfg.Source.Git.Workdir)
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

func TestLoadGitSourceDefaultsWorkdirFromStevedoreHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv(HomeEnvVar, home)
	p := writeConfig(t, `source:
  git:
    url: https://example.com/org/repo.git
`)

	cfg, err := LoadFromPath(p)
	if err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(home, DefaultGitSourceDirName)
	if cfg.Source.Git.Workdir != want {
		t.Fatalf("expected git workdir %q, got %q", want, cfg.Source.Git.Workdir)
	}
	if got := cfg.SettingOrigin("source.git.workdir").Source; got != SettingSourceDerived {
		t.Fatalf("expected source.git.workdir origin=derived, got %s", got)
	}
}

func TestLoadGitSourceWorkdirEnvOverrideBeatsHomeDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv(HomeEnvVar, home)
	t.Setenv(WorkdirEnvVar, "/env/override-workdir")
	p := writeConfig(t, `source:
  git:
    url: https://example.com/org/repo.git
`)

	cfg, err := LoadFromPath(p)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Source.Git.Workdir != "/env/override-workdir" {
		t.Fatalf("expected env workdir override, got %q", cfg.Source.Git.Workdir)
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

func TestApacheExposureIsOptional(t *testing.T) {
	p := writeConfig(t, `source:
  local:
    path: /srv/apps-repo
`)
	cfg, err := LoadFromPath(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Exposure.Apache != nil {
		t.Fatal("expected no apache exposure config when not specified")
	}
}

func TestApacheExposureConfiguredFromFile(t *testing.T) {
	p := writeConfig(t, `source:
  local:
    path: /srv/apps-repo
exposure:
  apache:
    sitesDir: /etc/apache2/sites-enabled
`)
	cfg, err := LoadFromPath(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Exposure.Apache == nil {
		t.Fatal("expected apache exposure config to be set")
	}
	if cfg.Exposure.Apache.SitesDir != "/etc/apache2/sites-enabled" {
		t.Fatalf("unexpected sitesDir: %s", cfg.Exposure.Apache.SitesDir)
	}
}

func TestApacheExposureActivatedByEnvVar(t *testing.T) {
	p := writeConfig(t, `source:
  local:
    path: /srv/apps-repo
`)
	t.Setenv(ApacheSitesDirEnvVar, "/tmp/apache-sites")

	cfg, err := LoadFromPath(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Exposure.Apache == nil {
		t.Fatal("expected apache exposure config to be created by env var")
	}
	if cfg.Exposure.Apache.SitesDir != "/tmp/apache-sites" {
		t.Fatalf("unexpected sitesDir from env: %s", cfg.Exposure.Apache.SitesDir)
	}
	if cfg.SettingOrigin("exposure.apache.sitesDir").Source != SettingSourceEnvVar {
		t.Fatalf("expected env origin for apache sitesDir")
	}
}

func TestApacheExposureEnvVarOverridesFile(t *testing.T) {
	p := writeConfig(t, `source:
  local:
    path: /srv/apps-repo
exposure:
  apache:
    sitesDir: /file/sites
`)
	t.Setenv(ApacheSitesDirEnvVar, "/env/sites")

	cfg, err := LoadFromPath(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Exposure.Apache == nil || cfg.Exposure.Apache.SitesDir != "/env/sites" {
		t.Fatalf("expected env to override file apache sitesDir, got %v", cfg.Exposure.Apache)
	}
}

func TestValidateRejectsApacheExposureWithoutSitesDir(t *testing.T) {
	p := writeConfig(t, `source:
  local:
    path: /srv/apps-repo
exposure:
  apache: {}
`)
	if _, err := LoadFromPath(p); err == nil {
		t.Fatal("expected validation error when apache exposure sitesDir is missing")
	}
}

func TestEffectiveSettingsIncludesApacheWhenConfigured(t *testing.T) {
	p := writeConfig(t, `source:
  local:
    path: /srv/apps-repo
exposure:
  apache:
    sitesDir: /etc/apache2/sites-enabled
`)
	cfg, err := LoadFromPath(p)
	if err != nil {
		t.Fatal(err)
	}
	settings := map[string]EffectiveSetting{}
	for _, s := range cfg.EffectiveSettings() {
		settings[s.Path] = s
	}
	apache, ok := settings["exposure.apache.sitesDir"]
	if !ok {
		t.Fatal("expected exposure.apache.sitesDir in effective settings")
	}
	if apache.Value != "/etc/apache2/sites-enabled" {
		t.Fatalf("unexpected apache sitesDir value: %s", apache.Value)
	}
	if apache.Origin.Source != SettingSourceConfigFile {
		t.Fatalf("expected config file origin, got %s", apache.Origin.Source)
	}
}

func TestEffectiveSettingsOmitsApacheWhenNotConfigured(t *testing.T) {
	p := writeConfig(t, `source:
  local:
    path: /srv/apps-repo
`)
	cfg, err := LoadFromPath(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range cfg.EffectiveSettings() {
		if s.Path == "exposure.apache.sitesDir" {
			t.Fatal("expected exposure.apache.sitesDir to be absent when apache is not configured")
		}
	}
}

func TestEffectiveSettingsReportOriginsAndResolvedSecretsFile(t *testing.T) {
	p := writeConfig(t, `logging:
  dir: /file/logs
source:
  local:
    path: /srv/apps-repo
secrets:
  providers:
    local:
      file: ./secrets/store.yml
`)

	t.Setenv(DebugEnvVar, "true")

	cfg, err := LoadFromPath(p)
	if err != nil {
		t.Fatal(err)
	}

	settings := map[string]EffectiveSetting{}
	for _, setting := range cfg.EffectiveSettings() {
		settings[setting.Path] = setting
	}

	if got := settings["logging.dir"].Origin.Source; got != SettingSourceConfigFile {
		t.Fatalf("expected logging.dir origin=config, got %s", got)
	}
	if got := settings["logging.debug"].Origin.Source; got != SettingSourceEnvVar {
		t.Fatalf("expected logging.debug origin=env, got %s", got)
	}
	if got := settings["poll.interval"].Origin.Source; got != SettingSourceDefault {
		t.Fatalf("expected poll.interval origin=default, got %s", got)
	}

	secretsSetting := settings["secrets.providers.local.file"]
	wantSecretsPath := filepath.Join(filepath.Dir(p), "secrets", "store.yml")
	if secretsSetting.Value != wantSecretsPath {
		t.Fatalf("expected resolved secrets path %q, got %q", wantSecretsPath, secretsSetting.Value)
	}
	if secretsSetting.Note == "" {
		t.Fatal("expected secrets setting note to mention relative path resolution")
	}

	if got := settings["source.repoRoot"].Origin.Source; got != SettingSourceDerived {
		t.Fatalf("expected source.repoRoot origin=derived, got %s", got)
	}
	if settings["source.type"].Value != "local" {
		t.Fatalf("expected source.type=local, got %q", settings["source.type"].Value)
	}
}

func TestEffectiveSettingsRedactGitSecretValues(t *testing.T) {
	t.Setenv("GIT_TOKEN", "super-secret-token")
	p := writeConfig(t, `source:
  git:
    url: https://example.com/org/repo.git
    auth:
      token:
        value: ${GIT_TOKEN}
`)

	cfg, err := LoadFromPath(p)
	if err != nil {
		t.Fatal(err)
	}

	settings := map[string]EffectiveSetting{}
	for _, setting := range cfg.EffectiveSettings() {
		settings[setting.Path] = setting
	}

	token := settings["source.git.auth.token.value"]
	if !token.Sensitive {
		t.Fatal("expected token setting to be marked sensitive")
	}
	if token.Value != "<redacted>" {
		t.Fatalf("expected redacted token value, got %q", token.Value)
	}
	if token.Origin.Source != SettingSourceConfigFile {
		t.Fatalf("expected token origin=config, got %s", token.Origin.Source)
	}
}
