package stevedore

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"stevedore-agent/internal/config"
	"stevedore-agent/internal/docker"
	"stevedore-agent/internal/logging"
	"stevedore-agent/internal/plugins"
	"stevedore-agent/internal/plugins/apache"
	"stevedore-agent/internal/reconciler"
	"stevedore-agent/pkg/plugin"

	"github.com/spf13/cobra"
)

// Execute runs the CLI.
func Execute() {
	root := newRootCommand()
	slog.Debug("executing root command", slog.String("binary", "stevedore-agent"))
	if err := root.Execute(); err != nil {
		fatal(err)
	}
}

func newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "stevedore-agent",
		Short:         "Stevedore agent",
		SilenceUsage:  true,
		SilenceErrors: true,
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	root.AddCommand(newRunCommand())
	root.AddCommand(newDoctorCommand())
	root.AddCommand(newInstallServiceCommand())

	return root
}

func buildReconciler(cfg *config.Config) *reconciler.Reconciler {
	var exposurePlugins []plugin.ExposurePlugin
	if cfg.Exposure.Apache != nil && cfg.Exposure.Apache.SitesDir != "" {
		exposurePlugins = append(exposurePlugins, apache.New(cfg.Exposure.Apache.SitesDir))
	}
	pm := plugins.NewManager(exposurePlugins...)
	return &reconciler.Reconciler{Runtime: docker.NewDockerRuntime(), Plugins: pm}
}

func fatal(err error) {
	// Config errors are user-facing setup guidance; print cleanly without log prefix.
	var cfgErr *ConfigError
	if errors.As(err, &cfgErr) {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
	// Other errors get structured logging.
	slog.ErrorContext(logging.SecurityContext(context.Background(), "fatal"), "command failed", slog.String("error", err.Error()))
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	os.Exit(1)
}
