package stevedore

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"stevedore-agent/internal/git"
	"stevedore-agent/internal/logging"
	"stevedore-agent/internal/manifest"

	"github.com/spf13/cobra"
)

type applyOptions struct {
	repo           string
	watch          bool
	interval       time.Duration
	gitPull        bool
	apacheSitesDir string
}

func newApplyCommand() *cobra.Command {
	opts := applyOptions{}

	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Reconcile applications against manifests",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runApply(opts)
		},
	}

	cmd.Flags().StringVar(&opts.repo, "repo", ".", "Path to stevedore apps repository")
	cmd.Flags().BoolVar(&opts.watch, "watch", false, "Continuously reconcile on interval")
	cmd.Flags().DurationVar(&opts.interval, "interval", 30*time.Second, "Reconciliation interval")
	cmd.Flags().BoolVar(&opts.gitPull, "git-pull", false, "Run git pull --ff-only before each scan")
	cmd.Flags().StringVar(&opts.apacheSitesDir, "apache-sites-dir", "/etc/apache2/sites-enabled", "Apache sites-enabled directory")

	return cmd
}

func runApply(opts applyOptions) error {
	r := buildReconciler(opts.apacheSitesDir)
	ctx := logging.BusinessContext(context.Background(), "apply")
	reconcileErrCtx := logging.SecurityContext(context.Background(), "reconcile")
	slog.DebugContext(ctx, "apply options",
		slog.String("repo", opts.repo),
		slog.Bool("watch", opts.watch),
		slog.Duration("interval", opts.interval),
		slog.Bool("gitPull", opts.gitPull),
		slog.String("apacheSitesDir", opts.apacheSitesDir),
	)
	applyOnce := func() error {
		if opts.gitPull {
			slog.DebugContext(ctx, "running git pull", slog.String("repo", opts.repo))
			if err := git.Pull(opts.repo); err != nil {
				return err
			}
		}
		apps, err := manifest.LoadApplications(opts.repo)
		if err != nil {
			return err
		}
		slog.DebugContext(ctx, "applications loaded", slog.Int("count", len(apps)))
		for _, app := range apps {
			slog.DebugContext(ctx, "reconciling app", slog.String("app", app.Metadata.Name), slog.String("container", app.ContainerName()))
			_, err := r.Reconcile(app)
			if err != nil {
				slog.ErrorContext(reconcileErrCtx, "reconcile failed", slog.String("app", app.Metadata.Name), slog.String("error", err.Error()))
				return fmt.Errorf("reconcile %s: %w", app.Metadata.Name, err)
			}
			slog.InfoContext(ctx, "application reconciled", slog.String("app", app.Metadata.Name))
		}
		return nil
	}

	if !opts.watch {
		return applyOnce()
	}

	for {
		if err := applyOnce(); err != nil {
			slog.ErrorContext(logging.SecurityContext(context.Background(), "apply"), "apply iteration failed", slog.String("error", err.Error()))
		}
		slog.DebugContext(ctx, "watch sleep", slog.Duration("interval", opts.interval))
		time.Sleep(opts.interval)
	}
}
