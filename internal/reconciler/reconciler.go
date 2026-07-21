package reconciler

import (
	"context"
	"fmt"
	"log/slog"
	"sort"

	"stevedore-agent/internal/docker"
	"stevedore-agent/internal/logging"
	"stevedore-agent/internal/manifest"
	"stevedore-agent/internal/plugins"
)

type Reconciler struct {
	Runtime docker.Runtime
	Plugins *plugins.Manager
}

// ReconcileResult contains the result of a reconciliation including any drift detected.
type ReconcileResult struct {
	Drift DriftReport
}

func (r *Reconciler) Reconcile(app manifest.Application) (*ReconcileResult, error) {
	ctx := context.Background()
	result := &ReconcileResult{}

	slog.InfoContext(logging.BusinessContext(ctx, "reconcile"), "reconcile start", slog.String("app", app.Metadata.Name), slog.String("image", app.FullImage()))

	desiredSpec, desiredHash, err := desired(app)
	if err != nil {
		return result, err
	}

	if err := r.Runtime.PullImage(desiredSpec.Image); err != nil {
		return result, err
	}

	for _, networkName := range app.NetworkNames() {
		exists, err := r.Runtime.NetworkExists(networkName)
		if err != nil {
			return result, err
		}
		if !exists {
			slog.InfoContext(logging.BusinessContext(ctx, "reconcile"), "reconcile action", slog.String("app", app.Metadata.Name), slog.String("action", "creating network: "+networkName))
			if err := r.Runtime.CreateNetwork(networkName); err != nil {
				return result, err
			}
		}
	}

	exists, err := r.Runtime.ContainerExists(app.ContainerName())
	if err != nil {
		return result, err
	}

	startExisting := false
	if exists {
		needs, err := r.needsUpdate(app, desiredHash, desiredSpec.Image)
		if err != nil {
			return result, err
		}
		if needs {
			// Compute and log the specific drift before acting on it
			if drift, driftErr := ComputeDrift(r.Runtime, app); driftErr == nil {
				result.Drift = drift
				if len(drift.IgnoredExtraEnv) > 0 {
					extraEnvKeys := make([]string, 0, len(drift.IgnoredExtraEnv))
					for key := range drift.IgnoredExtraEnv {
						extraEnvKeys = append(extraEnvKeys, key)
					}
					sort.Strings(extraEnvKeys)
					slog.InfoContext(
						logging.BusinessContext(ctx, "reconcile"),
						"ignoring runtime env vars not declared in manifest",
						slog.String("app", app.Metadata.Name),
						slog.String("envVars", fmt.Sprintf("%v", extraEnvKeys)),
					)
				}
				if len(drift.IgnoredExtraNetworks) > 0 {
					extra := append([]string(nil), drift.IgnoredExtraNetworks...)
					sort.Strings(extra)
					slog.InfoContext(
						logging.BusinessContext(ctx, "reconcile"),
						"ignoring runtime networks not declared in manifest",
						slog.String("app", app.Metadata.Name),
						slog.String("networks", fmt.Sprintf("%v", extra)),
					)
				}
				slog.InfoContext(logging.BusinessContext(ctx, "reconcile"), "drift detected", slog.String("app", app.Metadata.Name), slog.String("changes", drift.Summary()))
				fmt.Printf("Drift detected in %s: %s\n", app.Metadata.Name, drift.Summary())
				for _, item := range drift.Items() {
					slog.InfoContext(logging.BusinessContext(ctx, "reconcile"), "drift item", slog.String("app", app.Metadata.Name), slog.String("drift", item))
				}
			}
			slog.InfoContext(logging.BusinessContext(ctx, "reconcile"), "reconcile action", slog.String("app", app.Metadata.Name), slog.String("action", "stopping container"))
			if err := r.Runtime.StopContainer(app.ContainerName()); err != nil {
				return result, err
			}
			slog.InfoContext(logging.BusinessContext(ctx, "reconcile"), "reconcile action", slog.String("app", app.Metadata.Name), slog.String("action", "removing container"))
			if err := r.Runtime.RemoveContainer(app.ContainerName()); err != nil {
				return result, err
			}
			exists = false
		} else {
			running, err := r.Runtime.ContainerRunning(app.ContainerName())
			if err != nil {
				return result, err
			}
			startExisting = !running
		}
	}

	if !exists {
		slog.InfoContext(logging.BusinessContext(ctx, "reconcile"), "reconcile action", slog.String("app", app.Metadata.Name), slog.String("action", "creating container"))
		if err := r.Runtime.CreateContainer(desiredSpec); err != nil {
			return result, err
		}
		slog.InfoContext(logging.BusinessContext(ctx, "reconcile"), "reconcile action", slog.String("app", app.Metadata.Name), slog.String("action", "starting container"))
		if err := r.Runtime.StartContainer(app.ContainerName()); err != nil {
			return result, err
		}
	} else if startExisting {
		slog.InfoContext(logging.BusinessContext(ctx, "reconcile"), "reconcile action", slog.String("app", app.Metadata.Name), slog.String("action", "starting stopped container"))
		if err := r.Runtime.StartContainer(app.ContainerName()); err != nil {
			return result, err
		}
	} else if exists {
		slog.InfoContext(logging.BusinessContext(ctx, "reconcile"), "reconcile action", slog.String("app", app.Metadata.Name), slog.String("action", "no-op: already converged"))
	}

	if err := r.Plugins.Apply(app); err != nil {
		return result, err
	}

	slog.InfoContext(logging.BusinessContext(ctx, "reconcile"), "reconcile success", slog.String("app", app.Metadata.Name), slog.String("action", "done"))

	return result, nil
}

func (r *Reconciler) Delete(app manifest.Application) error {
	ctx := context.Background()

	slog.InfoContext(logging.BusinessContext(ctx, "delete"), "delete start", slog.String("app", app.Metadata.Name))

	exists, err := r.Runtime.ContainerExists(app.ContainerName())
	if err != nil {
		return err
	}
	if exists {
		_ = r.Runtime.StopContainer(app.ContainerName())
		if err := r.Runtime.RemoveContainer(app.ContainerName()); err != nil {
			return err
		}
	}

	if err := r.Plugins.Remove(app); err != nil {
		return err
	}

	slog.InfoContext(logging.BusinessContext(ctx, "delete"), "delete success", slog.String("app", app.Metadata.Name))

	return nil
}

func desired(app manifest.Application) (docker.ContainerSpec, string, error) {
	spec, err := docker.BuildContainerSpec(app, "")
	if err != nil {
		return docker.ContainerSpec{}, "", err
	}
	h, err := docker.HashFromSpec(spec)
	if err != nil {
		return docker.ContainerSpec{}, "", err
	}
	spec.Labels["stevedore.dev/hash"] = h
	return spec, h, nil
}

func (r *Reconciler) needsUpdate(app manifest.Application, desiredHash, desiredImage string) (bool, error) {
	currentImage, err := r.Runtime.ContainerImage(app.ContainerName())
	if err != nil {
		return false, err
	}
	if currentImage != desiredImage {
		return true, nil
	}
	currentHash, err := r.Runtime.ContainerLabel(app.ContainerName(), "stevedore.dev/hash")
	if err != nil {
		return false, err
	}
	if currentHash != desiredHash {
		return true, nil
	}
	currentNetworks, err := r.Runtime.ContainerNetworks(app.ContainerName())
	if err != nil {
		return false, err
	}
	// Only trigger an update when a desired network is absent from the running
	// container. Extra runtime networks (e.g. Docker's bridge) are ignored.
	if len(missingStrings(app.NetworkNames(), currentNetworks)) > 0 {
		return true, nil
	}
	currentVolumes, err := r.Runtime.ContainerVolumes(app.ContainerName())
	if err != nil {
		return false, err
	}
	return !volumeMappingsEqual(currentVolumes, desiredVolumeMappings(app)), nil
}
