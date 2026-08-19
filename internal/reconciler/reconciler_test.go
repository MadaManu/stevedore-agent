package reconciler

import (
	"testing"

	"stevedore-agent/internal/docker"
	"stevedore-agent/internal/exposure"
	"stevedore-agent/internal/manifest"
)

type fakeRuntime struct {
	exists            bool
	running           bool
	image             string
	hash              string
	ports             map[int]int // containerPort → hostPort
	network           bool
	created           bool
	started           bool
	pulledImage       string
	env               map[string]string
	containerNetwork  string
	containerNetworks []string
	containerVolumes  []docker.VolumeMapping
	imageDigests      map[string]string // image name -> digest
}

func (f *fakeRuntime) PullImage(image string) error                    { f.pulledImage = image; return nil }
func (f *fakeRuntime) ContainerExists(name string) (bool, error)       { return f.exists, nil }
func (f *fakeRuntime) ContainerImage(name string) (string, error)      { return f.image, nil }
func (f *fakeRuntime) ContainerLabel(name, key string) (string, error) { return f.hash, nil }
func (f *fakeRuntime) ContainerPorts(name string) (map[int]int, error) {
	return f.ports, nil
}
func (f *fakeRuntime) ContainerEnvironment(name string) (map[string]string, error) {
	if f.env == nil {
		return make(map[string]string), nil
	}
	return f.env, nil
}
func (f *fakeRuntime) ContainerNetwork(name string) (string, error) {
	return f.containerNetwork, nil
}
func (f *fakeRuntime) ContainerNetworks(name string) ([]string, error) {
	if len(f.containerNetworks) > 0 {
		return f.containerNetworks, nil
	}
	if f.containerNetwork != "" {
		return []string{f.containerNetwork}, nil
	}
	return nil, nil
}
func (f *fakeRuntime) ContainerVolumes(name string) ([]docker.VolumeMapping, error) {
	return f.containerVolumes, nil
}
func (f *fakeRuntime) ImageDigest(image string) (string, error) {
	if f.imageDigests == nil {
		f.imageDigests = make(map[string]string)
	}
	if digest, ok := f.imageDigests[image]; ok {
		return digest, nil
	}
	return "sha256:unknown", nil
}
func (f *fakeRuntime) StartContainer(name string) error                    { f.started = true; return nil }
func (f *fakeRuntime) StopContainer(name string) error                     { f.exists = false; return nil }
func (f *fakeRuntime) RemoveContainer(name string) error                   { f.exists = false; return nil }
func (f *fakeRuntime) NetworkExists(name string) (bool, error)             { return f.network, nil }
func (f *fakeRuntime) CreateNetwork(name string) error                     { f.network = true; return nil }
func (f *fakeRuntime) ContainerRunning(name string) (bool, error)          { return f.running, nil }
func (f *fakeRuntime) ContainerLogs(name string, tail int) (string, error) { return "", nil }
func (f *fakeRuntime) CreateContainer(spec docker.ContainerSpec) error {
	f.created = true
	f.exists = true
	f.running = false
	f.image = spec.Image
	f.hash = spec.Labels["stevedore.dev/hash"]
	return nil
}

func testApp() manifest.Application {
	return manifest.Application{
		Metadata:      manifest.Metadata{Name: "demo"},
		Image:         manifest.ImageConfig{Repository: "nginx", Tag: "latest"},
		RestartPolicy: "always",
		Network:       manifest.NetworkConfig{Name: "apps"},
	}
}

func TestReconcileCreatesContainer(t *testing.T) {
	fr := &fakeRuntime{}
	r := &Reconciler{Runtime: fr, Exposure: exposure.NewManager()}

	app := testApp()

	_, err := r.Reconcile(app)
	if err != nil {
		t.Fatal(err)
	}
	if !fr.created || !fr.started {
		t.Fatalf("expected container created and started")
	}
	if !fr.network {
		t.Fatalf("expected network to be created")
	}
}

func TestReconcileStartsExistingStoppedContainer(t *testing.T) {
	app := testApp()
	spec, err := docker.BuildContainerSpec(app, "")
	if err != nil {
		t.Fatal(err)
	}
	h, err := docker.HashFromSpec(spec)
	if err != nil {
		t.Fatal(err)
	}

	fr := &fakeRuntime{
		exists:            true,
		running:           false,
		image:             app.FullImage(),
		hash:              h,
		network:           true,
		containerNetworks: app.NetworkNames(),
	}
	r := &Reconciler{Runtime: fr, Exposure: exposure.NewManager()}

	_, err = r.Reconcile(app)
	if err != nil {
		t.Fatal(err)
	}
	if !fr.started {
		t.Fatalf("expected existing stopped container to be started")
	}
	if fr.created {
		t.Fatalf("did not expect container recreation when config is unchanged")
	}
}

func TestComputeDrift_NoDrift(t *testing.T) {
	app := manifest.Application{
		Metadata:      manifest.Metadata{Name: "demo"},
		Image:         manifest.ImageConfig{Repository: "nginx", Tag: "latest"},
		RestartPolicy: "always",
		Ports: []manifest.PortMapping{
			{Name: "http", ContainerPort: 80, HostPort: 8080},
		},
	}
	spec, _ := docker.BuildContainerSpec(app, "")
	h, _ := docker.HashFromSpec(spec)

	fr := &fakeRuntime{
		image:             app.FullImage(),
		hash:              h,
		ports:             map[int]int{80: 8080},
		env:               make(map[string]string), // no user env vars
		containerNetworks: app.NetworkNames(),
		imageDigests: map[string]string{
			"nginx:latest": "sha256:abc123",
		},
	}
	report, err := ComputeDrift(fr, app)
	if err != nil {
		t.Fatal(err)
	}
	if report.HasDrift {
		t.Fatalf("expected no drift, got: %s", report.Summary())
	}
}

func TestComputeDrift_PortDrift(t *testing.T) {
	app := manifest.Application{
		Metadata:      manifest.Metadata{Name: "demo"},
		Image:         manifest.ImageConfig{Repository: "nginx", Tag: "latest"},
		RestartPolicy: "always",
		Ports: []manifest.PortMapping{
			{Name: "http", ContainerPort: 80, HostPort: 8081}, // desired 8081
		},
	}
	spec, _ := docker.BuildContainerSpec(app, "")
	h, _ := docker.HashFromSpec(spec)

	fr := &fakeRuntime{
		image:             app.FullImage(),
		hash:              h,
		ports:             map[int]int{80: 8080}, // actually running on 8080
		env:               make(map[string]string),
		containerNetworks: app.NetworkNames(),
		imageDigests: map[string]string{
			"nginx:latest": "sha256:abc123",
		},
	}
	report, err := ComputeDrift(fr, app)
	if err != nil {
		t.Fatal(err)
	}
	if !report.HasDrift {
		t.Fatal("expected drift but got none")
	}
	if len(report.PortDrifts) != 1 {
		t.Fatalf("expected 1 port drift, got %d", len(report.PortDrifts))
	}
	pd := report.PortDrifts[0]
	if pd.ContainerPort != 80 || pd.ActualHostPort != 8080 || pd.DesiredHostPort != 8081 {
		t.Fatalf("unexpected port drift: %+v", pd)
	}
	if !containsString(report.Summary(), "8080 → 8081") {
		t.Fatalf("unexpected summary: %s", report.Summary())
	}
}

func TestComputeDrift_ImageDrift(t *testing.T) {
	app := manifest.Application{
		Metadata:      manifest.Metadata{Name: "demo"},
		Image:         manifest.ImageConfig{Repository: "nginx", Tag: "1.27"},
		RestartPolicy: "always",
	}
	fr := &fakeRuntime{
		image: "nginx:1.25", // old image running
		ports: map[int]int{},
		env:   make(map[string]string),
		imageDigests: map[string]string{
			"nginx:1.25": "sha256:old",
			"nginx:1.27": "sha256:new",
		},
	}
	report, err := ComputeDrift(fr, app)
	if err != nil {
		t.Fatal(err)
	}
	if !report.HasDrift || !report.ImageChanged {
		t.Fatal("expected image drift")
	}
	if report.CurrentImage != "nginx:1.25" || report.DesiredImage != "nginx:1.27" {
		t.Fatalf("unexpected image drift values: current=%s desired=%s", report.CurrentImage, report.DesiredImage)
	}
}

func TestComputeDrift_EnvironmentDrift(t *testing.T) {
	app := manifest.Application{
		Metadata:      manifest.Metadata{Name: "demo"},
		Image:         manifest.ImageConfig{Repository: "nginx", Tag: "latest"},
		RestartPolicy: "always",
		Environment: map[string]string{
			"NGINX_ENTRYPOINT_QUIET_LOGS": "1",
			"CUSTOM_VAR":                  "expected_value",
		},
	}
	spec, _ := docker.BuildContainerSpec(app, "")
	h, _ := docker.HashFromSpec(spec)

	fr := &fakeRuntime{
		image: app.FullImage(),
		hash:  h,
		ports: map[int]int{},
		env: map[string]string{
			"NGINX_ENTRYPOINT_QUIET_LOGS": "1",
			"CUSTOM_VAR":                  "actual_value", // different value
			"EXTRA_VAR":                   "extra",        // extra variable
		},
		containerNetworks: app.NetworkNames(),
		imageDigests: map[string]string{
			"nginx:latest": "sha256:abc123",
		},
	}
	report, err := ComputeDrift(fr, app)
	if err != nil {
		t.Fatal(err)
	}
	if !report.HasDrift {
		t.Fatal("expected drift but got none")
	}
	if len(report.EnvDrifts) != 1 {
		t.Fatalf("expected 1 env drift, got %d: %v", len(report.EnvDrifts), report.EnvDrifts)
	}
	if _, exists := report.EnvDrifts["EXTRA_VAR"]; exists {
		t.Fatal("did not expect extra runtime environment variable to be treated as drift")
	}
	if _, exists := report.IgnoredExtraEnv["EXTRA_VAR"]; !exists {
		t.Fatal("expected extra runtime environment variable to be recorded as ignored")
	}
	if !containsString(report.Summary(), "env:") {
		t.Fatalf("expected env drift in summary: %s", report.Summary())
	}
}

func TestComputeDrift_ExtraEnvironmentOnly_IsIgnored(t *testing.T) {
	app := manifest.Application{
		Metadata:      manifest.Metadata{Name: "demo"},
		Image:         manifest.ImageConfig{Repository: "nginx", Tag: "latest"},
		RestartPolicy: "always",
		Environment: map[string]string{
			"CUSTOM_VAR": "expected_value",
		},
	}
	spec, _ := docker.BuildContainerSpec(app, "")
	h, _ := docker.HashFromSpec(spec)

	fr := &fakeRuntime{
		image: app.FullImage(),
		hash:  h,
		ports: map[int]int{},
		env: map[string]string{
			"CUSTOM_VAR": "expected_value",
			"EXTRA_VAR":  "extra",
		},
		containerNetworks: app.NetworkNames(),
		imageDigests: map[string]string{
			"nginx:latest": "sha256:abc123",
		},
	}

	report, err := ComputeDrift(fr, app)
	if err != nil {
		t.Fatal(err)
	}
	if report.HasDrift {
		t.Fatalf("expected no drift when only extra runtime env vars exist, got: %s", report.Summary())
	}
	if len(report.EnvDrifts) != 0 {
		t.Fatalf("expected no env drift entries, got %d", len(report.EnvDrifts))
	}
	if _, exists := report.IgnoredExtraEnv["EXTRA_VAR"]; !exists {
		t.Fatal("expected extra runtime env var to be captured as ignored")
	}
}

func TestComputeDrift_NetworkDrift(t *testing.T) {
	app := manifest.Application{
		Metadata:      manifest.Metadata{Name: "demo"},
		Image:         manifest.ImageConfig{Repository: "nginx", Tag: "latest"},
		RestartPolicy: "always",
		Networks: []manifest.NetworkConfig{
			{Name: "desired-network"},
			{Name: "shared-network"},
		},
	}
	spec, _ := docker.BuildContainerSpec(app, "")
	h, _ := docker.HashFromSpec(spec)

	fr := &fakeRuntime{
		image:             app.FullImage(),
		hash:              h,
		ports:             map[int]int{},
		env:               make(map[string]string),
		containerNetworks: []string{"actual-network", "shared-network"},
		imageDigests: map[string]string{
			"nginx:latest": "sha256:abc123",
		},
	}
	report, err := ComputeDrift(fr, app)
	if err != nil {
		t.Fatal(err)
	}
	if !report.HasDrift {
		t.Fatal("expected drift but got none")
	}
	if !report.NetworkChanged {
		t.Fatal("expected network drift")
	}
	if !containsString(report.Summary(), "networks:") {
		t.Fatalf("expected network drift in summary: %s", report.Summary())
	}
}

func TestComputeDrift_ExtraRuntimeNetwork_IsIgnored(t *testing.T) {
	app := manifest.Application{
		Metadata:      manifest.Metadata{Name: "demo"},
		Image:         manifest.ImageConfig{Repository: "nginx", Tag: "latest"},
		RestartPolicy: "always",
		Networks:      []manifest.NetworkConfig{{Name: "apps"}},
	}
	spec, _ := docker.BuildContainerSpec(app, "")
	h, _ := docker.HashFromSpec(spec)

	fr := &fakeRuntime{
		image: app.FullImage(),
		hash:  h,
		ports: map[int]int{},
		env:   make(map[string]string),
		// bridge is an extra runtime network not in the manifest
		containerNetworks: []string{"apps", "bridge"},
		imageDigests: map[string]string{
			"nginx:latest": "sha256:abc123",
		},
	}

	report, err := ComputeDrift(fr, app)
	if err != nil {
		t.Fatal(err)
	}
	if report.HasDrift {
		t.Fatalf("expected no drift when only extra runtime networks present, got: %s", report.Summary())
	}
	found := false
	for _, n := range report.IgnoredExtraNetworks {
		if n == "bridge" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected 'bridge' in IgnoredExtraNetworks, got: %v", report.IgnoredExtraNetworks)
	}
}

func TestComputeDrift_NoDesiredNetworks_BridgeIgnored(t *testing.T) {
	app := manifest.Application{
		Metadata:      manifest.Metadata{Name: "demo"},
		Image:         manifest.ImageConfig{Repository: "nginx", Tag: "latest"},
		RestartPolicy: "always",
		// no networks declared
	}
	spec, _ := docker.BuildContainerSpec(app, "")
	h, _ := docker.HashFromSpec(spec)

	fr := &fakeRuntime{
		image: app.FullImage(),
		hash:  h,
		ports: map[int]int{},
		env:   make(map[string]string),
		// container only on bridge (Docker default)
		containerNetworks: []string{"bridge"},
		imageDigests: map[string]string{
			"nginx:latest": "sha256:abc123",
		},
	}

	report, err := ComputeDrift(fr, app)
	if err != nil {
		t.Fatal(err)
	}
	if report.HasDrift {
		t.Fatalf("expected no drift when desired has no networks and runtime only has bridge, got: %s", report.Summary())
	}
}

func TestComputeDrift_VolumeDrift(t *testing.T) {
	app := manifest.Application{
		Metadata:      manifest.Metadata{Name: "demo"},
		Image:         manifest.ImageConfig{Repository: "nginx", Tag: "latest"},
		RestartPolicy: "always",
		Volumes: []manifest.VolumeMapping{
			{HostPath: "/data/desired", MountPath: "/srv/data"},
		},
	}
	spec, _ := docker.BuildContainerSpec(app, "")
	h, _ := docker.HashFromSpec(spec)

	fr := &fakeRuntime{
		image: app.FullImage(),
		hash:  h,
		ports: map[int]int{},
		env:   make(map[string]string),
		containerVolumes: []docker.VolumeMapping{
			{HostPath: "/data/actual", MountPath: "/srv/data"},
		},
		imageDigests: map[string]string{
			"nginx:latest": "sha256:abc123",
		},
	}
	report, err := ComputeDrift(fr, app)
	if err != nil {
		t.Fatal(err)
	}
	if !report.HasDrift {
		t.Fatal("expected drift but got none")
	}
	if !report.VolumeChanged {
		t.Fatal("expected volume drift")
	}
	if !containsString(report.Summary(), "volumes:") {
		t.Fatalf("expected volume drift in summary: %s", report.Summary())
	}
}

func TestDriftReportSummaryAndItems_Order(t *testing.T) {
	report := DriftReport{
		HasDrift: true,
		EnvDrifts: map[string]EnvDrift{
			"MADA_TEST": {
				Name:    "MADA_TEST",
				Removed: true,
			},
		},
		NetworkChanged:  true,
		CurrentNetworks: []string{"apps"},
		DesiredNetworks: []string{"apps2"},
	}

	wantSummary := "env: MADA_TEST (missing); networks: apps → apps2"
	if got := report.Summary(); got != wantSummary {
		t.Fatalf("unexpected summary:\nwant: %q\n got: %q", wantSummary, got)
	}

	items := report.Items()
	if len(items) != 2 {
		t.Fatalf("expected 2 drift items, got %d: %v", len(items), items)
	}
	if items[0] != "env: MADA_TEST (missing)" {
		t.Fatalf("unexpected first drift item: %q", items[0])
	}
	if items[1] != "networks: apps → apps2" {
		t.Fatalf("unexpected second drift item: %q", items[1])
	}
}

func containsString(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
