// Package plugin defines the public contract for stevedore exposure plugins.
//
// External authors can implement this interface and register their plugin
// with stevedore. A plugin receives an App value describing the application
// to expose, and is responsible for configuring the reverse proxy or any
// other exposure mechanism.
//
// To write an external plugin:
//
//  1. Import this package:
//
//     import "stevedore-agent/pkg/plugin"
//
//  2. Implement the ExposurePlugin interface.
//
//  3. Wire it into a custom stevedore binary by calling plugins.NewManager with
//     your plugin instance alongside the built-in ones.
package plugin

// App is the public representation of an application that a plugin receives.
// It contains only the fields a plugin needs; internal reconciliation details
// are not exposed.
type App struct {
	// Name is the application name derived from the folder under apps/.
	//
	// Example: apps/demo/stevedore.yaml => Name == "demo".
	Name string

	// ContainerName is the name of the running Docker container.
	ContainerName string

	// Ports lists the host→container port mappings declared in the manifest.
	Ports []Port

	// ExposeConfig holds the free-form provider-specific config block from the manifest.
	//
	// Example manifest:
	//
	//   expose:
	//     provider: apache
	//     config:
	//       domain: app.example.com
	//       ssl: true
	//
	// This maps directly to ExposeConfig["domain"] and ExposeConfig["ssl"].
	ExposeConfig map[string]interface{}
}

// Port represents a single port mapping exposed by the container.
type Port struct {
	// Name is the optional label given to this port (e.g. "http", "metrics").
	Name string

	// HostPort is the port bound on the Docker host.
	HostPort int

	// ContainerPort is the port the process inside the container listens on.
	ContainerPort int
}

// ExposurePlugin is the interface all stevedore exposure plugins must implement.
//
// Plugins are registered by name and selected via the `expose.provider` field
// in the application manifest.
type ExposurePlugin interface {
	// Name returns the provider name that manifests reference in expose.provider.
	// Must be unique across all registered plugins.
	Name() string

	// Validate checks that the app's ExposeConfig contains all required fields
	// for this plugin. Called before Apply.
	Validate(config map[string]interface{}) error

	// Apply configures external exposure for the given app (e.g. writes a
	// reverse-proxy config and reloads the proxy daemon).
	Apply(app App) error

	// Remove tears down external exposure for the given app (e.g. deletes the
	// proxy config file and reloads the proxy daemon).
	Remove(app App) error
}
