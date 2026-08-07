package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"stevedore-agent/internal/config"
)

func testDeps() systemDeps {
	return systemDeps{
		readFile: os.ReadFile,
		readDir:  os.ReadDir,
		stat:     os.Stat,
		glob:     filepath.Glob,
		hostFQDN: func() (string, error) { return "host.example.com", nil },
		lookPath: func(name string) (string, error) { return "/usr/bin/" + name, nil },
		runCmd:   func(string, ...string) ([]byte, error) { return []byte("ok"), nil },
		access:   func(string, uint32) error { return nil },
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func settingsByPath(settings []config.EffectiveSetting) map[string]config.EffectiveSetting {
	out := make(map[string]config.EffectiveSetting, len(settings))
	for _, setting := range settings {
		out[setting.Path] = setting
	}
	return out
}

func TestRunWithDeps_LocalConfigPassesAndReportsOrigins(t *testing.T) {
	home := t.TempDir()
	repo := filepath.Join(home, "repo")
	logs := filepath.Join(home, "logs")
	apache := filepath.Join(home, "apache")
	secretsFile := filepath.Join(home, "secrets.local.yml")

	writeFile(t, filepath.Join(repo, "apps", "demo-ui", "stevedore.yml"), `image:
  repository: nginx
  tag: 1.31.3
`)
	writeFile(t, secretsFile, "api:\n  token: demo-token\n")
	if err := os.MkdirAll(logs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(apache, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(home, config.ConfigFileName), `logging:
  dir: `+logs+`
source:
  local:
    path: `+repo+`
poll:
  interval: 45s
exposure:
  apache:
    sitesDir: `+apache+`
secrets:
  providers:
    local:
      file: ./secrets.local.yml
`)

	t.Setenv(config.HomeEnvVar, home)
	t.Setenv(config.DebugEnvVar, "true")

	report := runWithDeps(testDeps())
	if report.FailureCount() != 0 {
		t.Fatalf("expected zero failures, got %d\n%s", report.FailureCount(), RenderText(report))
	}
	if report.WarningCount() != 0 {
		t.Fatalf("expected zero warnings, got %d\n%s", report.WarningCount(), RenderText(report))
	}

	settings := settingsByPath(report.Settings)
	if got := settings["logging.debug"].Origin.Source; got != config.SettingSourceEnvVar {
		t.Fatalf("expected logging.debug origin env, got %s", got)
	}
	if got := settings["logging.dir"].Origin.Source; got != config.SettingSourceConfigFile {
		t.Fatalf("expected logging.dir origin config, got %s", got)
	}
	if settings["secrets.providers.local.file"].Value != secretsFile {
		t.Fatalf("expected resolved secrets path %q, got %q", secretsFile, settings["secrets.providers.local.file"].Value)
	}
	if _, ok := settings["exposure.apache.sitesDir"]; !ok {
		t.Fatal("expected exposure.apache.sitesDir in effective settings")
	}
	if !strings.Contains(RenderText(report), "logging.debug = true (environment variable STEVEDORE_DEBUG)") {
		t.Fatal("expected rendered report to include logging.debug origin")
	}
	if !strings.Contains(RenderText(report), "Summary: ") {
		t.Fatal("expected rendered report to include summary")
	}
}

func TestRunWithDeps_ShowsResolvedSecretsPathWhenRelativePathIsWrong(t *testing.T) {
	home := t.TempDir()
	repo := filepath.Join(home, "repo")
	logs := filepath.Join(home, "logs")

	writeFile(t, filepath.Join(repo, "apps", "demo-ui", "stevedore.yml"), `image:
  repository: nginx
`)
	if err := os.MkdirAll(logs, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(home, config.ConfigFileName), `logging:
  dir: `+logs+`
source:
  local:
    path: `+repo+`
secrets:
  providers:
    local:
      file: ./.home/secrets.local.yml
`)

	t.Setenv(config.HomeEnvVar, home)

	report := runWithDeps(testDeps())
	if report.FailureCount() == 0 {
		t.Fatalf("expected at least one failure\n%s", RenderText(report))
	}

	var secretsCheck *CheckResult
	for i := range report.Checks {
		if report.Checks[i].Name == "local secrets store" {
			secretsCheck = &report.Checks[i]
			break
		}
	}
	if secretsCheck == nil {
		t.Fatal("expected local secrets store check")
	}
	if secretsCheck.Status != StatusFail {
		t.Fatalf("expected local secrets store check to fail, got %s", secretsCheck.Status)
	}
	resolved := filepath.Join(home, ".home", "secrets.local.yml")
	if !strings.Contains(strings.Join(secretsCheck.Tried, "\n"), resolved) {
		t.Fatalf("expected tried paths to include resolved path %q, got %v", resolved, secretsCheck.Tried)
	}
	if !strings.Contains(strings.Join(secretsCheck.Tried, "\n"), "raw config value: ./.home/secrets.local.yml") {
		t.Fatalf("expected tried paths to include raw config value, got %v", secretsCheck.Tried)
	}
}

func TestRunWithDeps_FailsWhenDockerDaemonIsNotReachable(t *testing.T) {
	home := t.TempDir()
	repo := filepath.Join(home, "repo")
	logs := filepath.Join(home, "logs")

	writeFile(t, filepath.Join(repo, "apps", "demo-ui", "stevedore.yml"), `image:
  repository: nginx
`)
	if err := os.MkdirAll(logs, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(home, config.ConfigFileName), `logging:
  dir: `+logs+`
source:
  local:
    path: `+repo+`
`)

	t.Setenv(config.HomeEnvVar, home)

	deps := testDeps()
	deps.runCmd = func(name string, args ...string) ([]byte, error) {
		if name == "/usr/bin/docker" && len(args) == 1 && args[0] == "info" {
			return []byte("Cannot connect to the Docker daemon at unix:///var/run/docker.sock"), fmt.Errorf("exit status 1")
		}
		return []byte("ok"), nil
	}

	report := runWithDeps(deps)
	var dockerDaemon *CheckResult
	for i := range report.Checks {
		if report.Checks[i].Name == "docker daemon" {
			dockerDaemon = &report.Checks[i]
			break
		}
	}
	if dockerDaemon == nil {
		t.Fatal("expected docker daemon check")
	}
	if dockerDaemon.Status != StatusFail {
		t.Fatalf("expected docker daemon check to fail, got %s", dockerDaemon.Status)
	}
}

func TestRunWithDeps_SkipsDockerDaemonCheckWhenDockerBinaryMissing(t *testing.T) {
	home := t.TempDir()
	repo := filepath.Join(home, "repo")
	logs := filepath.Join(home, "logs")

	writeFile(t, filepath.Join(repo, "apps", "demo-ui", "stevedore.yml"), `image:
  repository: nginx
`)
	if err := os.MkdirAll(logs, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(home, config.ConfigFileName), `logging:
  dir: `+logs+`
source:
  local:
    path: `+repo+`
`)

	t.Setenv(config.HomeEnvVar, home)

	deps := testDeps()
	deps.lookPath = func(name string) (string, error) {
		if name == "docker" {
			return "", fmt.Errorf("not found")
		}
		return "/usr/bin/" + name, nil
	}

	report := runWithDeps(deps)
	for _, check := range report.Checks {
		if check.Name == "docker daemon" {
			t.Fatal("did not expect docker daemon check when docker binary is missing")
		}
	}
}

func TestRunWithDeps_GitWorkdirDefaultsToStevedoreHomeGitSource(t *testing.T) {
	home := t.TempDir()
	logs := filepath.Join(home, "logs")
	if err := os.MkdirAll(logs, 0o755); err != nil {
		t.Fatal(err)
	}

	writeFile(t, filepath.Join(home, config.ConfigFileName), `logging:
  dir: `+logs+`
source:
  git:
    url: https://example.com/org/repo.git
`)

	t.Setenv(config.HomeEnvVar, home)

	report := runWithDeps(testDeps())
	if report.FailureCount() != 0 {
		t.Fatalf("expected zero failures, got %d\n%s", report.FailureCount(), RenderText(report))
	}

	settings := settingsByPath(report.Settings)
	workdir := settings["source.git.workdir"]
	want := filepath.Join(home, config.DefaultGitSourceDirName)
	if workdir.Value != want {
		t.Fatalf("expected source.git.workdir=%q, got %q", want, workdir.Value)
	}
	if workdir.Origin.Source != config.SettingSourceDerived {
		t.Fatalf("expected source.git.workdir origin=derived, got %s", workdir.Origin.Source)
	}

	var gitWorkdirCheck *CheckResult
	for i := range report.Checks {
		if report.Checks[i].Name == "git workdir" {
			gitWorkdirCheck = &report.Checks[i]
			break
		}
	}
	if gitWorkdirCheck == nil {
		t.Fatal("expected git workdir check")
	}
	if len(gitWorkdirCheck.Tried) == 0 || gitWorkdirCheck.Tried[0] != want {
		t.Fatalf("expected git workdir check to try %q, got %v", want, gitWorkdirCheck.Tried)
	}
}
