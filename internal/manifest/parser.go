package manifest

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const ()

func LoadApplications(repoRoot string, secretResolver SecretResolver) ([]Application, error) {
	pattern := filepath.Join(repoRoot, "apps", "*", "stevedore.yml")
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob manifests: %w", err)
	}

	apps := make([]Application, 0, len(paths))
	seen := map[string]string{}
	for _, p := range paths {
		app, err := LoadApplicationFromPath(p)
		if err != nil {
			return nil, err
		}
		name := app.Metadata.Name
		if prev, ok := seen[name]; ok {
			return nil, fmt.Errorf("duplicate application name %q in %s and %s", name, prev, p)
		}
		seen[name] = p
		apps = append(apps, app)
	}

	if err := resolveAllApplications(apps, secretResolver); err != nil {
		return nil, err
	}

	return apps, nil
}

func LoadApplicationFromPath(path string) (Application, error) {
	folderName := filepath.Base(filepath.Dir(path))

	b, err := os.ReadFile(path)
	if err != nil {
		return Application{}, fmt.Errorf("read manifest %s: %w", path, err)
	}

	var app Application
	if err := yaml.Unmarshal(b, &app); err != nil {
		return Application{}, fmt.Errorf("parse yaml %s: %w", path, err)
	}
	app.SourcePath = path

	if err := validate(app, path, folderName); err != nil {
		return Application{}, err
	}

	app.Metadata.Name = folderName

	return app, nil
}

func validate(app Application, path, folderName string) error {
	if app.Metadata.Name != "" && app.Metadata.Name != folderName {
		return fmt.Errorf("%s: metadata.name %q must match app folder name %q or be omitted", path, app.Metadata.Name, folderName)
	}
	if app.Image.Repository == "" {
		return fmt.Errorf("%s: spec.image.repository is required", path)
	}
	if app.RestartPolicy == "" {
		app.RestartPolicy = "always"
	}
	switch app.RestartPolicy {
	case "always", "unless-stopped", "on-failure", "none", "":
	default:
		return fmt.Errorf("%s: invalid restartPolicy %q", path, app.RestartPolicy)
	}

	for i, p := range app.Ports {
		if p.ContainerPort <= 0 || p.HostPort <= 0 {
			return fmt.Errorf("%s: spec.ports[%d] must have positive hostPort/containerPort", path, i)
		}
	}
	for i, v := range app.Volumes {
		hostPath := strings.TrimSpace(v.HostPath)
		mountPath := strings.TrimSpace(v.MountPath)
		if hostPath == "" || mountPath == "" {
			return fmt.Errorf("%s: spec.volumes[%d] hostPath and mountPath are required", path, i)
		}
	}
	volumeConfigured := strings.TrimSpace(app.Volume.Name) != "" || strings.TrimSpace(app.Volume.HostPath) != "" || strings.TrimSpace(app.Volume.MountPath) != ""
	if volumeConfigured {
		hostPath := strings.TrimSpace(app.Volume.HostPath)
		mountPath := strings.TrimSpace(app.Volume.MountPath)
		if hostPath == "" || mountPath == "" {
			return fmt.Errorf("%s: spec.volume.hostPath and spec.volume.mountPath are required when volume is set", path)
		}
	}
	seenMountPaths := map[string]string{}
	if volumeConfigured {
		primaryMountPath := strings.TrimSpace(app.Volume.MountPath)
		seenMountPaths[primaryMountPath] = "spec.volume"
	}
	for i, v := range app.Volumes {
		mountPath := strings.TrimSpace(v.MountPath)
		if prev, ok := seenMountPaths[mountPath]; ok {
			return fmt.Errorf("%s: duplicate mountPath %q at spec.volumes[%d] (already declared at %s)", path, mountPath, i, prev)
		}
		seenMountPaths[mountPath] = fmt.Sprintf("spec.volumes[%d]", i)
	}
	for i, net := range app.Networks {
		if strings.TrimSpace(net.Name) == "" {
			return fmt.Errorf("%s: spec.networks[%d].name is required", path, i)
		}
	}
	if app.Network.Name != "" && strings.TrimSpace(app.Network.Name) == "" {
		return fmt.Errorf("%s: spec.network.name is required when network is set", path)
	}
	seenNetworks := map[string]struct{}{}
	if primary := strings.TrimSpace(app.Network.Name); primary != "" {
		seenNetworks[primary] = struct{}{}
	}
	for i, net := range app.Networks {
		name := strings.TrimSpace(net.Name)
		if _, ok := seenNetworks[name]; ok {
			return fmt.Errorf("%s: duplicate network name %q at spec.networks[%d]", path, name, i)
		}
		seenNetworks[name] = struct{}{}
	}
	if app.Expose.Enabled && app.Expose.Provider == "" {
		return errors.New("spec.expose.provider is required when expose.enabled=true")
	}
	return nil
}
