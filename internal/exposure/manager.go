package exposure

import (
	"fmt"

	"stevedore-agent/internal/manifest"
)

type Manager struct {
	providers map[string]Provider
}

func NewManager(items ...Provider) *Manager {
	m := &Manager{providers: map[string]Provider{}}
	for _, p := range items {
		m.providers[p.Name()] = p
	}
	return m
}

func (m *Manager) Apply(app manifest.Application) error {
	if !app.Expose.Enabled {
		return nil
	}
	p, ok := m.providers[app.Expose.Provider]
	if !ok {
		return fmt.Errorf("exposure provider %q is not registered", app.Expose.Provider)
	}
	exposureApp := toExposureApp(app)
	if err := p.Validate(exposureApp.Config); err != nil {
		return err
	}
	return p.Apply(exposureApp)
}

func (m *Manager) Remove(app manifest.Application) error {
	if !app.Expose.Enabled {
		return nil
	}
	p, ok := m.providers[app.Expose.Provider]
	if !ok {
		return fmt.Errorf("exposure provider %q is not registered", app.Expose.Provider)
	}
	return p.Remove(toExposureApp(app))
}

// toExposureApp converts an internal Application to the internal exposure App.
func toExposureApp(app manifest.Application) App {
	ports := make([]Port, 0, len(app.Ports))
	for _, p := range app.Ports {
		ports = append(ports, Port{
			Name:          p.Name,
			HostPort:      p.HostPort,
			ContainerPort: p.ContainerPort,
		})
	}
	return App{
		Name:          app.Metadata.Name,
		ContainerName: app.ContainerName(),
		Ports:         ports,
		Config:        app.Expose.Config,
	}
}
