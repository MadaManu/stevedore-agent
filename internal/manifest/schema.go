package manifest

type Application struct {
	Metadata      Metadata          `yaml:"metadata"`
	Image         ImageConfig       `yaml:"image"`
	Container     ContainerConfig   `yaml:"container"`
	RestartPolicy string            `yaml:"restartPolicy"`
	Ports         []PortMapping     `yaml:"ports"`
	Volumes       []VolumeMapping   `yaml:"volumes"`
	Environment   map[string]string `yaml:"environment"`
	Network       NetworkConfig     `yaml:"network"`
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

type NetworkConfig struct {
	Name string `yaml:"name"`
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
