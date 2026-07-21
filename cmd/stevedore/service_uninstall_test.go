package stevedore

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunUninstallService_RemovesUnitAndReloadsSystemd(t *testing.T) {
	oldServiceName := serviceName
	oldSystemdDir := systemdDirPath
	oldRun := runCommandFn
	oldStat := statFileFn
	oldRemove := removeFileFn
	defer func() {
		serviceName = oldServiceName
		systemdDirPath = oldSystemdDir
		runCommandFn = oldRun
		statFileFn = oldStat
		removeFileFn = oldRemove
	}()

	tmp := t.TempDir()
	serviceName = "stevedore-agent"
	systemdDirPath = tmp
	unitPath := filepath.Join(tmp, "stevedore-agent.service")
	if err := os.WriteFile(unitPath, []byte("[Unit]\nDescription=test\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var commandCalls []string
	runCommandFn = func(name string, args ...string) ([]byte, error) {
		commandCalls = append(commandCalls, name+" "+strings.Join(args, " "))
		return []byte("ok"), nil
	}
	statFileFn = os.Stat
	removeFileFn = os.Remove

	var out bytes.Buffer
	if err := runUninstallService(&out); err != nil {
		t.Fatal(err)
	}

	if len(commandCalls) != 2 {
		t.Fatalf("expected 2 systemctl calls, got %d: %v", len(commandCalls), commandCalls)
	}
	if commandCalls[0] != "systemctl disable --now stevedore-agent.service" {
		t.Fatalf("unexpected first command: %s", commandCalls[0])
	}
	if commandCalls[1] != "systemctl daemon-reload" {
		t.Fatalf("unexpected second command: %s", commandCalls[1])
	}
	if _, err := os.Stat(unitPath); !os.IsNotExist(err) {
		t.Fatalf("expected unit file removed, stat err=%v", err)
	}

	output := out.String()
	if !strings.Contains(output, "removed ") || !strings.Contains(output, "disabled and stopped stevedore-agent.service") {
		t.Fatalf("unexpected output: %q", output)
	}
}

func TestRunUninstallService_MissingUnit_IsIdempotent(t *testing.T) {
	oldServiceName := serviceName
	oldSystemdDir := systemdDirPath
	oldRun := runCommandFn
	oldStat := statFileFn
	oldRemove := removeFileFn
	defer func() {
		serviceName = oldServiceName
		systemdDirPath = oldSystemdDir
		runCommandFn = oldRun
		statFileFn = oldStat
		removeFileFn = oldRemove
	}()

	tmp := t.TempDir()
	serviceName = "stevedore-agent"
	systemdDirPath = tmp

	var commandCalls []string
	runCommandFn = func(name string, args ...string) ([]byte, error) {
		commandCalls = append(commandCalls, name+" "+strings.Join(args, " "))
		if len(args) >= 2 && args[0] == "disable" && args[1] == "--now" {
			return []byte("Unit stevedore-agent.service not loaded."), fmt.Errorf("exit status 5")
		}
		return []byte("ok"), nil
	}
	statFileFn = os.Stat
	removeFileFn = os.Remove

	var out bytes.Buffer
	if err := runUninstallService(&out); err != nil {
		t.Fatal(err)
	}

	if len(commandCalls) != 2 {
		t.Fatalf("expected 2 systemctl calls, got %d: %v", len(commandCalls), commandCalls)
	}
	if commandCalls[0] != "systemctl disable --now stevedore-agent.service" {
		t.Fatalf("unexpected first command: %s", commandCalls[0])
	}
	if commandCalls[1] != "systemctl daemon-reload" {
		t.Fatalf("unexpected second command: %s", commandCalls[1])
	}

	output := out.String()
	if !strings.Contains(output, "not found") || !strings.Contains(output, "disabled and stopped stevedore-agent.service") {
		t.Fatalf("unexpected output: %q", output)
	}
}

func TestRunUninstallService_FailsOnDisableError(t *testing.T) {
	oldServiceName := serviceName
	oldSystemdDir := systemdDirPath
	oldRun := runCommandFn
	defer func() {
		serviceName = oldServiceName
		systemdDirPath = oldSystemdDir
		runCommandFn = oldRun
	}()

	tmp := t.TempDir()
	serviceName = "stevedore-agent"
	systemdDirPath = tmp

	var commandCalls []string
	runCommandFn = func(name string, args ...string) ([]byte, error) {
		commandCalls = append(commandCalls, name+" "+strings.Join(args, " "))
		return []byte("permission denied"), fmt.Errorf("exit status 1")
	}

	var out bytes.Buffer
	err := runUninstallService(&out)
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if !strings.Contains(err.Error(), "disable --now") {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(commandCalls) != 1 {
		t.Fatalf("expected uninstall to stop after disable failure, got calls: %v", commandCalls)
	}
}
