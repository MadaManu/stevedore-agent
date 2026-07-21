package manifest

import (
	"sort"
	"strings"
)

type Application struct {
	Metadata      Metadata          `yaml:"metadata"`
	Image         ImageConfig       `yaml:"image"`
	Container     ContainerConfig   `yaml:"container"`
	RestartPolicy string            `yaml:"restartPolicy"`
	Ports         []PortMapping     `yaml:"ports"`
	Volume        VolumeMapping     `yaml:"volume"`
	Volumes       []VolumeMapping   `yaml:"volumes"`
	Environment   map[string]string `yaml:"environment"`
	Network       NetworkConfig     `yaml:"network"`
	Networks      []NetworkConfig   `yaml:"networks"`
	Expose        ExposureConfig    `yaml:"expose"`
	SourcePath    string            `yaml:"-"`
}

type Metadata struct {
	Name string `yaml:"name"`
}

type ImageConfig struct {
	Repository string `yaml:"repository"`
	Tag        string `yaml:"tag"`
}

type ContainerConfig struct {
	Name string `yaml:"name"`
}

type PortMapping struct {
	Name          string `yaml:"name"`
	ContainerPort int    `yaml:"containerPort"`
	HostPort      int    `yaml:"hostPort"`
}

type VolumeMapping struct {
	Name      string `yaml:"name"`
	HostPath  string `yaml:"hostPath"`
	MountPath string `yaml:"mountPath"`
}

// VolumeMappings returns a normalized, unique, sorted list of desired volume
// mappings. The legacy `volume:` block is treated as an alias for the first
// entry.
func (a Application) VolumeMappings() []VolumeMapping {
	volumes := a.OrderedVolumeMappings()
	sort.Slice(volumes, func(i, j int) bool {
		if volumes[i].MountPath != volumes[j].MountPath {
			return volumes[i].MountPath < volumes[j].MountPath
		}
		if volumes[i].HostPath != volumes[j].HostPath {
			return volumes[i].HostPath < volumes[j].HostPath
		}
		return volumes[i].Name < volumes[j].Name
	})
	return volumes
}

// OrderedVolumeMappings returns a normalized, unique list of desired volume
// mappings in declaration order. The legacy `volume:` block is treated as the
// first entry.
func (a Application) OrderedVolumeMappings() []VolumeMapping {
	return normalizeVolumeMappingsOrdered(a.Volume, a.Volumes)
}

func normalizeVolumeMappingsOrdered(primary VolumeMapping, additional []VolumeMapping) []VolumeMapping {
	seen := map[string]struct{}{}
	volumes := make([]VolumeMapping, 0, 1+len(additional))
	add := func(v VolumeMapping) {
		normalized := VolumeMapping{
			Name:      strings.TrimSpace(v.Name),
			HostPath:  strings.TrimSpace(v.HostPath),
			MountPath: strings.TrimSpace(v.MountPath),
		}
		if normalized.HostPath == "" || normalized.MountPath == "" {
			return
		}
		key := normalized.MountPath + "\x00" + normalized.HostPath
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		volumes = append(volumes, normalized)
	}

	add(primary)
	for _, vol := range additional {
		add(vol)
	}
	return volumes
}

type NetworkConfig struct {
	Name string `yaml:"name"`
}

// NetworkNames returns a normalized, unique, sorted list of desired network
// names. The legacy `network:` block is treated as an alias for the first entry.
func (a Application) NetworkNames() []string {
	names := a.OrderedNetworkNames()
	sort.Strings(names)
	return names
}

// OrderedNetworkNames returns a normalized, unique list of desired networks in
// declaration order. The legacy `network:` block is treated as the first entry.
func (a Application) OrderedNetworkNames() []string {
	return normalizeNetworkNamesOrdered(a.Network.Name, a.Networks)
}

// PrimaryNetworkName returns the first normalized network name, or an empty
// string when no networks are configured.
func (a Application) PrimaryNetworkName() string {
	names := a.OrderedNetworkNames()
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

func normalizeNetworkNamesOrdered(primary string, additional []NetworkConfig) []string {
	seen := map[string]struct{}{}
	names := make([]string, 0, 1+len(additional))
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}

	add(primary)
	for _, net := range additional {
		add(net.Name)
	}
	return names
}

type ExposureConfig struct {
	Enabled  bool                   `yaml:"enabled"`
	Provider string                 `yaml:"provider"`
	Config   map[string]interface{} `yaml:"config"`
}

func (a Application) EffectiveName() string {
	if a.Metadata.Name != "" {
		return a.Metadata.Name
	}
	return a.Container.Name
}

func (a Application) ContainerName() string {
	if a.Container.Name != "" {
		return a.Container.Name
	}
	return a.Metadata.Name
}

func (a Application) FullImage() string {
	tag := a.Image.Tag
	if tag == "" {
		tag = "latest"
	}
	return a.Image.Repository + ":" + tag
}
