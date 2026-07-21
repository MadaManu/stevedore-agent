package stevedore

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderSystemdService_IncludesRequiredDirectives(t *testing.T) {
	text, err := renderSystemdService("/usr/local/bin/stevedore-agent", "/root/.docker", "/root")
	if err != nil {
		t.Fatal(err)
	}

	required := []string{
		"StartLimitIntervalSec=900",
		"StartLimitBurst=5",
		"Restart=on-failure",
		"RestartSec=30",
		"ExecStart=/usr/local/bin/stevedore-agent run",
	}
	for _, item := range required {
		if !strings.Contains(text, item) {
			t.Fatalf("expected rendered service to include %q", item)
		}
	}
}

func TestRenderSystemdService_IncludesDockerConfig(t *testing.T) {
	oldConfigPath := dockerConfigPath
	oldHomePath := dockerHomePath
	defer func() {
		dockerConfigPath = oldConfigPath
		dockerHomePath = oldHomePath
	}()

	dockerConfigPath = "/root/.docker"
	dockerHomePath = "/root"

	text, err := renderSystemdService("/usr/local/bin/stevedore-agent", dockerConfigPath, dockerHomePath)
	if err != nil {
		t.Fatal(err)
	}

	required := []string{
		"Environment=DOCKER_CONFIG=/root/.docker",
		"Environment=HOME=/root",
	}
	for _, item := range required {
		if !strings.Contains(text, item) {
			t.Fatalf("expected rendered service to include %q, got:\n%s", item, text)
		}
	}
}

func TestRenderSystemdService_OmitsDockerConfigWhenEmpty(t *testing.T) {
	oldConfigPath := dockerConfigPath
	oldHomePath := dockerHomePath
	defer func() {
		dockerConfigPath = oldConfigPath
		dockerHomePath = oldHomePath
	}()

	dockerConfigPath = ""
	dockerHomePath = ""

	text, err := renderSystemdService("/usr/local/bin/stevedore-agent", dockerConfigPath, dockerHomePath)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(text, "Environment=DOCKER_CONFIG=") {
		t.Fatalf("expected Docker config to be omitted when not set, got:\n%s", text)
	}
	if strings.Contains(text, "Environment=HOME=") {
		t.Fatalf("expected HOME to be omitted when not set, got:\n%s", text)
	}
}

func TestRunInstallService_WritesUnitAndEnablesService(t *testing.T) {
	oldServiceName := serviceName
	oldSystemdDir := systemdDirPath
	oldHome := stevedoreHomePath
	oldLog := stevedoreLogDir
	oldResolve := resolveExecutablePath
	oldWrite := writeFileFn
	oldRun := runCommandFn
	oldDockerConfig := dockerConfigPath
	oldDockerHome := dockerHomePath
	defer func() {
		serviceName = oldServiceName
		systemdDirPath = oldSystemdDir
		stevedoreHomePath = oldHome
		stevedoreLogDir = oldLog
		resolveExecutablePath = oldResolve
		writeFileFn = oldWrite
		runCommandFn = oldRun
		dockerConfigPath = oldDockerConfig
		dockerHomePath = oldDockerHome
	}()

	tmp := t.TempDir()
	var commandCalls []string
	homeDir := filepath.Join(tmp, "home")
	t.Setenv("HOME", homeDir)

	serviceName = "stevedore-agent"
	systemdDirPath = tmp
	stevedoreHomePath = "/etc/stevedore"
	stevedoreLogDir = "/var/log/stevedore"
	dockerConfigPath = ""
	dockerHomePath = ""

	resolveExecutablePath = func() (string, error) {
		return "/usr/local/bin/stevedore-agent", nil
	}
	writeFileFn = os.WriteFile
	runCommandFn = func(name string, args ...string) ([]byte, error) {
		commandCalls = append(commandCalls, name+" "+strings.Join(args, " "))
		return []byte("ok"), nil
	}

	var out bytes.Buffer
	if err := runInstallService(&out); err != nil {
		t.Fatal(err)
	}

	unitPath := filepath.Join(tmp, "stevedore-agent.service")
	b, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatal(err)
	}
	unitText := string(b)
	if !strings.Contains(unitText, "ExecStart=/usr/local/bin/stevedore-agent run") {
		t.Fatalf("expected ExecStart in unit file, got:\n%s", unitText)
	}
	if !strings.Contains(unitText, "Environment=DOCKER_CONFIG="+filepath.Join(homeDir, ".docker")) {
		t.Fatalf("expected default DOCKER_CONFIG in unit file, got:\n%s", unitText)
	}
	if !strings.Contains(unitText, "Environment=HOME="+homeDir) {
		t.Fatalf("expected default HOME in unit file, got:\n%s", unitText)
	}

	if len(commandCalls) != 2 {
		t.Fatalf("expected 2 systemctl calls, got %d: %v", len(commandCalls), commandCalls)
	}
	if commandCalls[0] != "systemctl daemon-reload" {
		t.Fatalf("unexpected first command: %s", commandCalls[0])
	}
	if commandCalls[1] != "systemctl enable --now stevedore-agent.service" {
		t.Fatalf("unexpected second command: %s", commandCalls[1])
	}

	output := out.String()
	if !strings.Contains(output, "installed ") || !strings.Contains(output, "enabled and started") {
		t.Fatalf("unexpected output: %q", output)
	}
}

func TestRunInstallService_WithDockerConfig(t *testing.T) {
	oldServiceName := serviceName
	oldSystemdDir := systemdDirPath
	oldHome := stevedoreHomePath
	oldLog := stevedoreLogDir
	oldResolve := resolveExecutablePath
	oldWrite := writeFileFn
	oldRun := runCommandFn
	oldDockerConfig := dockerConfigPath
	oldDockerHome := dockerHomePath
	defer func() {
		serviceName = oldServiceName
		systemdDirPath = oldSystemdDir
		stevedoreHomePath = oldHome
		stevedoreLogDir = oldLog
		resolveExecutablePath = oldResolve
		writeFileFn = oldWrite
		runCommandFn = oldRun
		dockerConfigPath = oldDockerConfig
		dockerHomePath = oldDockerHome
	}()

	tmp := t.TempDir()
	var commandCalls []string
	t.Setenv("HOME", filepath.Join(tmp, "home"))

	serviceName = "stevedore-agent"
	systemdDirPath = tmp
	stevedoreHomePath = "/etc/stevedore"
	stevedoreLogDir = "/var/log/stevedore"
	dockerConfigPath = "/custom/.docker"
	dockerHomePath = "/custom"

	resolveExecutablePath = func() (string, error) {
		return "/usr/local/bin/stevedore-agent", nil
	}
	writeFileFn = os.WriteFile
	runCommandFn = func(name string, args ...string) ([]byte, error) {
		commandCalls = append(commandCalls, name+" "+strings.Join(args, " "))
		return []byte("ok"), nil
	}

	var out bytes.Buffer
	if err := runInstallService(&out); err != nil {
		t.Fatal(err)
	}

	unitPath := filepath.Join(tmp, "stevedore-agent.service")
	b, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatal(err)
	}
	unitText := string(b)

	// Should include Docker config environment variables
	if !strings.Contains(unitText, "Environment=DOCKER_CONFIG=/custom/.docker") {
		t.Fatalf("expected DOCKER_CONFIG in unit file, got:\n%s", unitText)
	}
	if !strings.Contains(unitText, "Environment=HOME=/custom") {
		t.Fatalf("expected HOME in unit file, got:\n%s", unitText)
	}

	output := out.String()
	if strings.Contains(output, "Docker private repository") {
		t.Fatalf("did not expect Docker configuration guide in output, got: %q", output)
	}
}
