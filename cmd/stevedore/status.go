package stevedore

import (
	"context"
	"log/slog"

	"stevedore-agent/internal/docker"
	"stevedore-agent/internal/logging"
	"stevedore-agent/internal/manifest"
	"stevedore-agent/internal/reconciler"

	"github.com/spf13/cobra"
)

type statusOptions struct {
	repo string
}

func newStatusCommand() *cobra.Command {
	opts := statusOptions{}

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show application runtime status and drift",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(opts)
		},
	}

	cmd.Flags().StringVar(&opts.repo, "repo", ".", "Path to stevedore apps repository")

	return cmd
}

func runStatus(opts statusOptions) error {
	ctx := logging.BusinessContext(context.Background(), "status")
	slog.DebugContext(ctx, "status options", slog.String("repo", opts.repo))
	slog.InfoContext(ctx, "status scan started")

	apps, err := manifest.LoadApplications(opts.repo)
	if err != nil {
		return err
	}

	rt := docker.NewDockerRuntime()
	slog.InfoContext(ctx, "status header", slog.String("columns", "application,status,drift"))
	for _, app := range apps {
		status := "Missing"
		drift := "-"
		slog.DebugContext(ctx, "checking app", slog.String("app", app.Metadata.Name), slog.String("container", app.ContainerName()))

		exists, err := rt.ContainerExists(app.ContainerName())
		if err != nil {
			status = "Error"
		} else if exists {
			slog.DebugContext(ctx, "container found", slog.String("app", app.Metadata.Name))
			running, runErr := rt.ContainerRunning(app.ContainerName())
			if runErr != nil {
				status = "Error"
			} else if running {
				status = "Running"
			} else {
				status = "Stopped"
			}

			report, driftErr := reconciler.ComputeDrift(rt, app)
			if driftErr != nil {
				drift = "drift check error"
				slog.ErrorContext(logging.SecurityContext(context.Background(), "status"), "drift check failed", slog.String("app", app.Metadata.Name), slog.String("error", driftErr.Error()))
			} else {
				items := report.Items()
				slog.DebugContext(ctx, "drift evaluation complete", slog.String("app", app.Metadata.Name), slog.Int("items", len(items)))
				for _, item := range items {
					slog.InfoContext(ctx, "drift item", slog.String("app", app.Metadata.Name), slog.String("drift", item))
				}
				if len(items) > 0 {
					drift = "found"
				} else {
					drift = "none"
				}
			}
		}

		slog.InfoContext(ctx, "application status",
			slog.String("app", app.Metadata.Name),
			slog.String("status", status),
			slog.String("drift", drift),
		)
	}
	return nil
}
