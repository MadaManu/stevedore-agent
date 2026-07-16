package stevedore

import (
	"context"
	"log/slog"

	"stevedore-agent/internal/docker"
	"stevedore-agent/internal/logging"

	"github.com/spf13/cobra"
)

func newLogsCommand() *cobra.Command {
	var tail int

	cmd := &cobra.Command{
		Use:   "logs <app-name>",
		Short: "Show container logs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLogs(args[0], tail)
		},
	}

	cmd.Flags().IntVar(&tail, "tail", 100, "Number of log lines")
	return cmd
}

func runLogs(appName string, tail int) error {
	runtime := docker.NewDockerRuntime()
	ctx := logging.BusinessContext(context.Background(), "logs")
	slog.DebugContext(ctx, "logs options", slog.String("app", appName), slog.Int("tail", tail))
	out, err := runtime.ContainerLogs(appName, tail)
	if err != nil {
		return err
	}
	slog.DebugContext(ctx, "logs fetched", slog.Int("bytes", len(out)))
	slog.InfoContext(ctx, "container logs", slog.String("app", appName), slog.String("output", out))
	return nil
}
