package stevedore

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"stevedore-agent/internal/logging"
	"stevedore-agent/internal/manifest"

	"github.com/spf13/cobra"
)

type deleteOptions struct {
	repo           string
	apacheSitesDir string
}

func newDeleteCommand() *cobra.Command {
	opts := deleteOptions{}

	cmd := &cobra.Command{
		Use:   "delete <app-name>",
		Short: "Delete an application",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDelete(opts, args[0])
		},
	}

	cmd.Flags().StringVar(&opts.repo, "repo", ".", "Path to stevedore apps repository")
	cmd.Flags().StringVar(&opts.apacheSitesDir, "apache-sites-dir", "/etc/apache2/sites-enabled", "Apache sites-enabled directory")

	return cmd
}

func runDelete(opts deleteOptions, appName string) error {
	ctx := logging.BusinessContext(context.Background(), "delete")
	slog.DebugContext(ctx, "delete options",
		slog.String("repo", opts.repo),
		slog.String("apacheSitesDir", opts.apacheSitesDir),
		slog.String("app", appName),
	)
	apps, err := manifest.LoadApplications(opts.repo)
	if err != nil {
		return err
	}
	slog.DebugContext(ctx, "applications loaded", slog.Int("count", len(apps)))
	var target *manifest.Application
	for i := range apps {
		if strings.EqualFold(apps[i].Metadata.Name, appName) {
			target = &apps[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("application %q not found under %s", appName, filepath.Join(opts.repo, "apps"))
	}

	r := buildReconciler(opts.apacheSitesDir)
	slog.DebugContext(ctx, "deleting app", slog.String("app", target.Metadata.Name), slog.String("container", target.ContainerName()))
	if err := r.Delete(*target); err != nil {
		slog.ErrorContext(logging.SecurityContext(context.Background(), "delete"), "delete failed", slog.String("app", target.Metadata.Name), slog.String("error", err.Error()))
		return err
	}
	slog.InfoContext(ctx, "application deleted", slog.String("app", target.Metadata.Name))
	return nil
}
