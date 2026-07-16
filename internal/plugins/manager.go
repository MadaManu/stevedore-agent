package plugins

import (
	"fmt"

	"stevedore-agent/internal/manifest"
	"stevedore-agent/pkg/plugin"
)

type Manager struct {
	plugins map[string]plugin.ExposurePlugin
}

func NewManager(items ...plugin.ExposurePlugin) *Manager {
	m := &Manager{plugins: map[string]plugin.ExposurePlugin{}}
	for _, p := range items {
		m.plugins[p.Name()] = p
	}
	return m
}

func (m *Manager) Apply(app manifest.Application) error {
	if !app.Expose.Enabled {
		return nil
	}
	p, ok := m.plugins[app.Expose.Provider]
	if !ok {
		return fmt.Errorf("exposure provider %q is not registered", app.Expose.Provider)
	}
	pa := toPluginApp(app)
	if err := p.Validate(pa.ExposeConfig); err != nil {
		return err
	}
	return p.Apply(pa)
}

func (m *Manager) Remove(app manifest.Application) error {
	if !app.Expose.Enabled {
		return nil
	}
	p, ok := m.plugins[app.Expose.Provider]
	if !ok {
		return fmt.Errorf("exposure provider %q is not registered", app.Expose.Provider)
	}
	return p.Remove(toPluginApp(app))
}

// toPluginApp converts an internal Application to the public plugin.App type.
func toPluginApp(app manifest.Application) plugin.App {
	ports := make([]plugin.Port, 0, len(app.Ports))
	for _, p := range app.Ports {
		ports = append(ports, plugin.Port{
			Name:          p.Name,
			HostPort:      p.HostPort,
			ContainerPort: p.ContainerPort,
		})
	}
	return plugin.App{
		Name:          app.Metadata.Name,
		ContainerName: app.ContainerName(),
		Ports:         ports,
		ExposeConfig:  app.Expose.Config,
	}
}
