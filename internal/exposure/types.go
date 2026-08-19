package exposure

// App is the internal representation of an application that an exposure
// provider receives.
type App struct {
	// Name is the application name derived from the app folder name.
	Name string

	// ContainerName is the name of the running Docker container.
	ContainerName string

	// Ports lists host->container port mappings declared in the manifest.
	Ports []Port

	// Config holds the provider-specific expose.config block from the manifest.
	Config map[string]interface{}
}

// Port represents a single port mapping exposed by the container.
type Port struct {
	Name          string
	HostPort      int
	ContainerPort int
}

// Provider is the interface all bundled exposure implementations must satisfy.
// Providers are selected via spec.expose.provider in stevedore.yml.
type Provider interface {
	// Name returns the provider name used in spec.expose.provider.
	Name() string

	// Validate checks that the app expose config is valid for this provider.
	Validate(config map[string]interface{}) error

	// Apply configures external exposure for the app.
	Apply(app App) error

	// Remove tears down external exposure for the app.
	Remove(app App) error
}
