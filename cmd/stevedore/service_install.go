package stevedore

import (
	"bytes"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/spf13/cobra"
)

const (
	defaultServiceName  = "stevedore-agent"
	defaultSystemdDir   = "/etc/systemd/system"
	defaultStevedoreDir = "/etc/stevedore"
	defaultLogDir       = "/var/log/stevedore"
	systemdTemplateFile = "templates/systemd.service.tmpl"
)

//go:embed templates/systemd.service.tmpl
var systemdTemplateFS embed.FS

var (
	serviceName           = defaultServiceName
	systemdDirPath        = defaultSystemdDir
	stevedoreHomePath     = defaultStevedoreDir
	stevedoreLogDir       = defaultLogDir
	resolveExecutablePath = os.Executable
	writeFileFn           = os.WriteFile
	runCommandFn          = runCommand
	readTemplateFileFn    = readEmbeddedTemplateFile
)

func newInstallServiceCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install-service",
		Short: "Install and enable the stevedore-agent systemd service",
		Long:  "Install a systemd unit for stevedore-agent from a built-in template, reload systemd, then enable and start the service.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInstallService(cmd.OutOrStdout())
		},
	}

	return cmd
}

func runInstallService(w io.Writer) error {
	if strings.TrimSpace(serviceName) == "" {
		return fmt.Errorf("service name is required")
	}
	if strings.TrimSpace(systemdDirPath) == "" {
		return fmt.Errorf("systemd directory is required")
	}
	if strings.TrimSpace(stevedoreHomePath) == "" {
		return fmt.Errorf("stevedore home is required")
	}
	if strings.TrimSpace(stevedoreLogDir) == "" {
		return fmt.Errorf("log directory is required")
	}

	binaryPath, err := resolveExecutablePath()
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}

	binaryPath = strings.TrimSpace(binaryPath)
	if binaryPath == "" {
		return fmt.Errorf("resolved executable path is empty")
	}

	unitFilePath := filepath.Join(filepath.Clean(systemdDirPath), serviceName+".service")
	serviceText, err := renderSystemdService(binaryPath)
	if err != nil {
		return fmt.Errorf("render systemd service: %w", err)
	}

	if err := writeFileFn(unitFilePath, []byte(serviceText), 0o644); err != nil {
		return fmt.Errorf("write systemd unit %s: %w", unitFilePath, err)
	}

	if out, err := runCommandFn("systemctl", "daemon-reload"); err != nil {
		return fmt.Errorf("systemctl daemon-reload failed: %w: %s", err, strings.TrimSpace(string(out)))
	}

	serviceUnit := serviceName + ".service"
	if out, err := runCommandFn("systemctl", "enable", "--now", serviceUnit); err != nil {
		return fmt.Errorf("systemctl enable --now %s failed: %w: %s", serviceUnit, err, strings.TrimSpace(string(out)))
	}

	_, _ = fmt.Fprintf(w, "installed %s\n", unitFilePath)
	_, _ = fmt.Fprintf(w, "enabled and started %s\n", serviceUnit)
	return nil
}

func renderSystemdService(binaryPath string) (string, error) {
	binaryPath = strings.TrimSpace(binaryPath)
	if binaryPath == "" {
		return "", fmt.Errorf("binary path is required")
	}
	templateBytes, err := readTemplateFileFn(systemdTemplateFile)
	if err != nil {
		return "", fmt.Errorf("read systemd template %s: %w", systemdTemplateFile, err)
	}
	templateText := string(templateBytes)
	if strings.TrimSpace(templateText) == "" {
		return "", fmt.Errorf("systemd template %s is empty", systemdTemplateFile)
	}
	data := struct {
		StevedoreHome string
		LogDir        string
		BinaryPath    string
	}{
		StevedoreHome: stevedoreHomePath,
		LogDir:        stevedoreLogDir,
		BinaryPath:    binaryPath,
	}
	tpl, err := template.New("systemd-service").Parse(templateText)
	if err != nil {
		return "", err
	}
	var b bytes.Buffer
	if err := tpl.Execute(&b, data); err != nil {
		return "", err
	}
	return b.String(), nil
}

func runCommand(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	return cmd.CombinedOutput()
}

func readEmbeddedTemplateFile(path string) ([]byte, error) {
	return fs.ReadFile(systemdTemplateFS, path)
}
