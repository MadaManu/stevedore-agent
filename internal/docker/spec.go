package docker

import (
	"os"

	"stevedore-agent/internal/manifest"
)

func BuildContainerSpec(app manifest.Application, hash string) (ContainerSpec, error) {
	orderedVolumes := app.OrderedVolumeMappings()
	for _, v := range orderedVolumes {
		if err := os.MkdirAll(v.HostPath, 0o755); err != nil {
			return ContainerSpec{}, err
		}
	}

	ports := make([]PortMapping, 0, len(app.Ports))
	for _, p := range app.Ports {
		ports = append(ports, PortMapping{HostPort: p.HostPort, ContainerPort: p.ContainerPort})
	}

	volumes := make([]VolumeMapping, 0, len(orderedVolumes))
	for _, v := range orderedVolumes {
		volumes = append(volumes, VolumeMapping{HostPath: v.HostPath, MountPath: v.MountPath})
	}

	labels := map[string]string{
		"stevedore.dev/managed": "true",
		"stevedore.dev/app":     app.Metadata.Name,
		"stevedore.dev/hash":    hash,
	}
	networkNames := app.OrderedNetworkNames()

	return ContainerSpec{
		Name:          app.ContainerName(),
		Image:         app.FullImage(),
		RestartPolicy: app.RestartPolicy,
		Ports:         ports,
		Volumes:       volumes,
		Environment:   app.Environment,
		NetworkName:   app.PrimaryNetworkName(),
		NetworkNames:  networkNames,
		Labels:        labels,
	}, nil
}
