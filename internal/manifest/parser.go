package manifest

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const ()

func LoadApplications(repoRoot string) ([]Application, error) {
	pattern := filepath.Join(repoRoot, "apps", "*", "stevedore.yaml")
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

	if err := resolveAllApplications(apps); err != nil {
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
		if v.HostPath == "" || v.MountPath == "" {
			return fmt.Errorf("%s: spec.volumes[%d] hostPath and mountPath are required", path, i)
		}
	}
	if app.Expose.Enabled && app.Expose.Provider == "" {
		return errors.New("spec.expose.provider is required when expose.enabled=true")
	}
	return nil
}
