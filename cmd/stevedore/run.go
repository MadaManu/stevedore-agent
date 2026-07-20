package stevedore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"

	"log/slog"

	"stevedore-agent/internal/config"
	"stevedore-agent/internal/docker"
	"stevedore-agent/internal/git"
	"stevedore-agent/internal/logging"
	"stevedore-agent/internal/manifest"
	"stevedore-agent/internal/reconciler"
	"stevedore-agent/internal/secrets"

	"github.com/spf13/cobra"
)

// ConfigError is a user-facing configuration error that should be printed
// cleanly to stderr without structured logging noise.
type ConfigError struct {
	message string
}

func (e *ConfigError) Error() string {
	return e.message
}

func newConfigError(msg string) *ConfigError {
	return &ConfigError{message: msg}
}

type reconcileState struct {
	ManifestHash string
	DesiredHash  string
	RuntimeHash  string
}

func newRunCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the agent continuously, reconciling from a configured source of truth",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAgent()
		},
	}

	return cmd
}

func runAgent() error {
	cfg, err := config.Load()
	if err != nil {
		return explainConfigLoadError(err)
	}

	logging.SetDebug(cfg.Logging.Debug)
	if err := logging.ConfigureDefault(cfg.Logging.Dir); err != nil {
		return fmt.Errorf("failed to initialize logging: %w", err)
	}

	ctx := logging.BusinessContext(context.Background(), "run")

	slog.InfoContext(ctx, "agent starting",
		slog.String("config", cfg.Path()),
		slog.String("sourceType", sourceTypeName(cfg)),
		slog.Duration("interval", cfg.Poll.Interval),
	)

	// Graceful shutdown on SIGINT/SIGTERM.
	signalCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	r := buildReconciler(cfg)
	secretResolver, err := buildSecretResolver(cfg)
	if err != nil {
		return err
	}
	lastState := reconcileState{}

	// Run one cycle immediately, then on each tick.
	ticker := time.NewTicker(cfg.Poll.Interval)
	defer ticker.Stop()

	for {
		newState, err := reconcileCycle(ctx, cfg, r, secretResolver, lastState)
		if err != nil {
			slog.ErrorContext(logging.SecurityContext(context.Background(), "run"), "reconcile cycle failed", slog.String("error", err.Error()))
		} else {
			lastState = newState
		}

		select {
		case <-signalCtx.Done():
			slog.InfoContext(ctx, "shutdown signal received, stopping agent")
			return nil
		case <-ticker.C:
		}
	}
}

func explainConfigLoadError(err error) error {
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to load config: %w", err)
	}

	configPath := config.ConfigPath()
	message := fmt.Sprintf(`failed to load config: %v

Create a configuration file at:
  %s

You can point stevedore to another config location by setting %s to the folder that contains %s:
  export %s=/path/to/stevedore
  stevedore-agent run

Minimal example %s:
  source:
    local:
      path: /path/to/repo-containing-apps
  poll:
    interval: 30s`,
		err,
		configPath,
		config.HomeEnvVar,
		config.ConfigFileName,
		config.HomeEnvVar,
		config.ConfigFileName,
	)
	return newConfigError(message)
}

// reconcileCycle syncs the source of truth, checks for manifest changes, and
// applies them when something changed. It returns the current manifest hash.
func reconcileCycle(ctx context.Context, cfg *config.Config, r *reconciler.Reconciler, secretResolver manifest.SecretResolver, lastState reconcileState) (reconcileState, error) {
	currentState := reconcileState{}

	repoRoot, err := syncSource(ctx, cfg)
	if err != nil {
		return lastState, err
	}

	currentManifestHash, err := manifest.HashManifests(repoRoot)
	if err != nil {
		return lastState, err
	}
	currentState.ManifestHash = currentManifestHash

	var apps []manifest.Application
	loadApps := func() error {
		if apps != nil {
			return nil
		}
		loaded, loadErr := manifest.LoadApplications(repoRoot, secretResolver)
		if loadErr != nil {
			return loadErr
		}
		apps = loaded
		return nil
	}

	if err := loadApps(); err != nil {
		return lastState, err
	}

	desiredHash, err := computeDesiredStateHash(apps)
	if err != nil {
		return lastState, err
	}
	currentState.DesiredHash = desiredHash

	runtimeHash, err := computeRuntimeStateHash(r.Runtime, apps)
	if err != nil {
		return lastState, err
	}
	currentState.RuntimeHash = runtimeHash

	slog.DebugContext(ctx, "manifest hash computed",
		slog.String("repoRoot", repoRoot),
		slog.String("manifestHash", currentManifestHash),
		slog.String("desiredHash", desiredHash),
		slog.String("runtimeHash", runtimeHash),
	)

	if currentManifestHash == lastState.ManifestHash && desiredHash == lastState.DesiredHash && runtimeHash == lastState.RuntimeHash && lastState.ManifestHash != "" && lastState.DesiredHash != "" && lastState.RuntimeHash != "" {
		slog.DebugContext(ctx, "no manifest or runtime changes detected, sleeping", slog.Duration("interval", cfg.Poll.Interval))
		return currentState, nil
	}

	reason := "manifest changed"
	if currentManifestHash == lastState.ManifestHash && desiredHash != lastState.DesiredHash {
		reason = "resolved configuration changed"
	} else if currentManifestHash == lastState.ManifestHash {
		reason = "runtime drift detected"
	}
	slog.InfoContext(ctx, "changes detected, applying",
		slog.String("reason", reason),
		slog.String("manifestHash", currentManifestHash),
		slog.String("runtimeHash", runtimeHash),
	)
	slog.DebugContext(ctx, "applications loaded", slog.Int("count", len(apps)))

	if errs := reconcileApps(ctx, r, apps); len(errs) > 0 {
		for _, e := range errs {
			slog.ErrorContext(logging.SecurityContext(context.Background(), "reconcile"), "reconcile failed", slog.String("error", e.Error()))
		}
		return lastState, errors.Join(errs...)
	}

	postRuntimeHash, err := computeRuntimeStateHash(r.Runtime, apps)
	if err != nil {
		return lastState, err
	}
	currentState.RuntimeHash = postRuntimeHash
	return currentState, nil
}

type runtimeSnapshot struct {
	App     string            `json:"app"`
	Exists  bool              `json:"exists"`
	Running bool              `json:"running,omitempty"`
	Image   string            `json:"image,omitempty"`
	Hash    string            `json:"hash,omitempty"`
	Ports   map[int]int       `json:"ports,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Network string            `json:"network,omitempty"`
}

func computeRuntimeStateHash(runtime docker.Runtime, apps []manifest.Application) (string, error) {
	sortedApps := append([]manifest.Application(nil), apps...)
	sort.Slice(sortedApps, func(i, j int) bool {
		return sortedApps[i].ContainerName() < sortedApps[j].ContainerName()
	})

	snapshots := make([]runtimeSnapshot, 0, len(sortedApps))
	for _, app := range sortedApps {
		snapshot := runtimeSnapshot{App: app.ContainerName()}

		exists, err := runtime.ContainerExists(app.ContainerName())
		if err != nil {
			return "", err
		}
		snapshot.Exists = exists
		if !exists {
			snapshots = append(snapshots, snapshot)
			continue
		}

		running, err := runtime.ContainerRunning(app.ContainerName())
		if err != nil {
			return "", err
		}
		snapshot.Running = running

		image, err := runtime.ContainerImage(app.ContainerName())
		if err != nil {
			return "", err
		}
		snapshot.Image = image

		labelHash, err := runtime.ContainerLabel(app.ContainerName(), "stevedore.dev/hash")
		if err != nil {
			return "", err
		}
		snapshot.Hash = labelHash

		ports, err := runtime.ContainerPorts(app.ContainerName())
		if err != nil {
			return "", err
		}
		snapshot.Ports = ports

		envVars, err := runtime.ContainerEnvironment(app.ContainerName())
		if err != nil {
			return "", err
		}
		snapshot.Env = envVars

		network, err := runtime.ContainerNetwork(app.ContainerName())
		if err != nil {
			return "", err
		}
		snapshot.Network = network

		snapshots = append(snapshots, snapshot)
	}

	b, err := json.Marshal(snapshots)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func computeDesiredStateHash(apps []manifest.Application) (string, error) {
	sortedApps := append([]manifest.Application(nil), apps...)
	sort.Slice(sortedApps, func(i, j int) bool {
		return sortedApps[i].ContainerName() < sortedApps[j].ContainerName()
	})

	b, err := json.Marshal(sortedApps)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// syncSource ensures the manifests are present locally and returns the repo root
// that contains the apps/ directory.
func syncSource(ctx context.Context, cfg *config.Config) (string, error) {
	if cfg.Source.IsGit() {
		g := cfg.Source.Git
		slog.DebugContext(ctx, "syncing git source",
			slog.String("url", g.URL),
			slog.String("branch", g.Branch),
			slog.String("workdir", g.Workdir),
			slog.String("auth", string(g.Auth.Method())),
		)
		return git.Sync(*g)
	}
	slog.DebugContext(ctx, "using local source", slog.String("path", cfg.RepoRoot()))
	return cfg.RepoRoot(), nil
}

func sourceTypeName(cfg *config.Config) string {
	if cfg.Source.IsGit() {
		return "git"
	}
	return "local"
}

// reconcileApps iterates over every application and reconciles each one
// independently. Errors from individual apps are collected and returned so that
// a failure in one container does not prevent the remaining containers from
// being reconciled.
func reconcileApps(ctx context.Context, r *reconciler.Reconciler, apps []manifest.Application) []error {
	var errs []error
	for _, app := range apps {
		slog.DebugContext(ctx, "reconciling app", slog.String("app", app.Metadata.Name), slog.String("container", app.ContainerName()))
		if _, err := r.Reconcile(app); err != nil {
			errs = append(errs, fmt.Errorf("app %s: %w", app.Metadata.Name, err))
			continue
		}
		slog.InfoContext(ctx, "application reconciled", slog.String("app", app.Metadata.Name))
	}
	return errs
}

func buildSecretResolver(cfg *config.Config) (*secrets.Resolver, error) {
	providers := map[string]secrets.Provider{}

	if localFile := cfg.LocalSecretsFile(); localFile != "" {
		provider, err := secrets.NewLocalProvider(localFile)
		if err != nil {
			return nil, fmt.Errorf("build local secrets provider: %w", err)
		}
		providers["local"] = provider
	}

	return secrets.NewResolver(providers), nil
}
