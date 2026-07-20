package stevedore

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderSystemdService_IncludesRequiredDirectives(t *testing.T) {
	text, err := renderSystemdService("/usr/local/bin/stevedore-agent")
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

func TestRunInstallService_WritesUnitAndEnablesService(t *testing.T) {
	oldServiceName := serviceName
	oldSystemdDir := systemdDirPath
	oldHome := stevedoreHomePath
	oldLog := stevedoreLogDir
	oldResolve := resolveExecutablePath
	oldWrite := writeFileFn
	oldRun := runCommandFn
	defer func() {
		serviceName = oldServiceName
		systemdDirPath = oldSystemdDir
		stevedoreHomePath = oldHome
		stevedoreLogDir = oldLog
		resolveExecutablePath = oldResolve
		writeFileFn = oldWrite
		runCommandFn = oldRun
	}()

	tmp := t.TempDir()
	var commandCalls []string

	serviceName = "stevedore-agent"
	systemdDirPath = tmp
	stevedoreHomePath = "/etc/stevedore"
	stevedoreLogDir = "/var/log/stevedore"

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

	if len(commandCalls) != 2 {
		t.Fatalf("expected 2 systemctl calls, got %d: %v", len(commandCalls), commandCalls)
	}
	if commandCalls[0] != "systemctl daemon-reload" {
		t.Fatalf("unexpected first command: %s", commandCalls[0])
	}
	if commandCalls[1] != "systemctl enable --now stevedore-agent.service" {
		t.Fatalf("unexpected second command: %s", commandCalls[1])
	}

	if !strings.Contains(out.String(), "installed ") || !strings.Contains(out.String(), "enabled and started") {
		t.Fatalf("unexpected output: %q", out.String())
	}
}
