package stevedore

import (
	"context"
	"fmt"
	"github.com/spf13/cobra"
	"log/slog"
	"os"
	"stevedore-agent/internal/docker"
	"stevedore-agent/internal/logging"
	"stevedore-agent/internal/plugins"
	"stevedore-agent/internal/plugins/apache"
	"stevedore-agent/internal/reconciler"
)

func Execute() {
	root := newRootCommand()
	slog.Debug("executing root command", slog.String("binary", "stevedore"))
	if err := root.Execute(); err != nil {
		fatal(err)
	}
}

func newRootCommand() *cobra.Command {
	var logDir string
	var debug bool

	root := &cobra.Command{
		Use:           "stevedore",
		Short:         "Stevedore agent",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			logging.SetDebug(debug)
			slog.DebugContext(logging.BusinessContext(context.Background(), "startup"), "command pre-run",
				slog.String("command", cmd.Name()),
				slog.Bool("debug", debug),
				slog.Int("args", len(args)),
			)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	root.PersistentFlags().StringVar(&logDir, "log-dir", "", "Log directory (overrides STEVEDORE_LOG_DIR)")
	root.PersistentFlags().BoolVar(&debug, "debug", false, "Print verbose logs to stderr as well")

	root.AddCommand(newApplyCommand())
	root.AddCommand(newStatusCommand())
	root.AddCommand(newLogsCommand())
	root.AddCommand(newDeleteCommand())

	return root
}

func buildReconciler(apacheSitesDir string) *reconciler.Reconciler {
	pm := plugins.NewManager(apache.New(apacheSitesDir))
	return &reconciler.Reconciler{Runtime: docker.NewDockerRuntime(), Plugins: pm}
}

func usage() {
	ctx := logging.BusinessContext(context.Background(), "usage")
	slog.InfoContext(ctx, "stevedore <command>")
	slog.InfoContext(ctx, "commands: apply, status, logs, delete")
}

func fatal(err error) {
	slog.ErrorContext(logging.SecurityContext(context.Background(), "fatal"), "command failed", slog.String("error", err.Error()))
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	os.Exit(1)
}
