package stevedore

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var (
	statFileFn   = os.Stat
	removeFileFn = os.Remove
)

func newUninstallServiceCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "uninstall-service",
		Short: "Disable and remove the stevedore-agent systemd service",
		Long:  "Disable and stop the stevedore-agent systemd unit, remove the unit file, and reload systemd.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUninstallService(cmd.OutOrStdout())
		},
	}

	return cmd
}

func runUninstallService(w io.Writer) error {
	if strings.TrimSpace(serviceName) == "" {
		return fmt.Errorf("service name is required")
	}
	if strings.TrimSpace(systemdDirPath) == "" {
		return fmt.Errorf("systemd directory is required")
	}

	serviceUnit := serviceName + ".service"
	if out, err := runCommandFn("systemctl", "disable", "--now", serviceUnit); err != nil {
		if !isUnitNotLoaded(out) {
			return fmt.Errorf("systemctl disable --now %s failed: %w: %s", serviceUnit, err, strings.TrimSpace(string(out)))
		}
	}

	unitFilePath := filepath.Join(filepath.Clean(systemdDirPath), serviceUnit)
	if _, err := statFileFn(unitFilePath); err == nil {
		if err := removeFileFn(unitFilePath); err != nil {
			return fmt.Errorf("remove systemd unit %s: %w", unitFilePath, err)
		}
		_, _ = fmt.Fprintf(w, "removed %s\n", unitFilePath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat systemd unit %s: %w", unitFilePath, err)
	} else {
		_, _ = fmt.Fprintf(w, "unit file %s not found\n", unitFilePath)
	}

	if out, err := runCommandFn("systemctl", "daemon-reload"); err != nil {
		return fmt.Errorf("systemctl daemon-reload failed: %w: %s", err, strings.TrimSpace(string(out)))
	}

	_, _ = fmt.Fprintf(w, "disabled and stopped %s\n", serviceUnit)
	return nil
}

func isUnitNotLoaded(out []byte) bool {
	message := strings.ToLower(strings.TrimSpace(string(out)))
	return strings.Contains(message, "not loaded") || strings.Contains(message, "not found") || strings.Contains(message, "does not exist")
}
