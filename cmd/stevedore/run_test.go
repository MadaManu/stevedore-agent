package stevedore

import (
	"context"
	"errors"
	"testing"

	"stevedore-agent/internal/docker"
	"stevedore-agent/internal/manifest"
	"stevedore-agent/internal/plugins"
	"stevedore-agent/internal/reconciler"
)

type hashRuntimeFake struct {
	existsByName  map[string]bool
	runningByName map[string]bool
	imageByName   map[string]string
	labelByName   map[string]string
	portsByName   map[string]map[int]int
	envByName     map[string]map[string]string
	networkByName map[string]string
}

func (f *hashRuntimeFake) PullImage(image string) error { return nil }
func (f *hashRuntimeFake) ContainerExists(name string) (bool, error) {
	return f.existsByName[name], nil
}
func (f *hashRuntimeFake) ContainerImage(name string) (string, error) {
	return f.imageByName[name], nil
}
func (f *hashRuntimeFake) ContainerLabel(name, key string) (string, error) {
	_ = key
	return f.labelByName[name], nil
}
func (f *hashRuntimeFake) ContainerPorts(name string) (map[int]int, error) {
	if ports, ok := f.portsByName[name]; ok {
		return ports, nil
	}
	return map[int]int{}, nil
}
func (f *hashRuntimeFake) ContainerEnvironment(name string) (map[string]string, error) {
	if env, ok := f.envByName[name]; ok {
		return env, nil
	}
	return map[string]string{}, nil
}
func (f *hashRuntimeFake) ContainerNetwork(name string) (string, error) {
	return f.networkByName[name], nil
}
func (f *hashRuntimeFake) CreateContainer(spec docker.ContainerSpec) error { _ = spec; return nil }
func (f *hashRuntimeFake) StartContainer(name string) error                { _ = name; return nil }
func (f *hashRuntimeFake) StopContainer(name string) error                 { _ = name; return nil }
func (f *hashRuntimeFake) RemoveContainer(name string) error               { _ = name; return nil }
func (f *hashRuntimeFake) NetworkExists(name string) (bool, error)         { _ = name; return true, nil }
func (f *hashRuntimeFake) CreateNetwork(name string) error                 { _ = name; return nil }
func (f *hashRuntimeFake) ContainerRunning(name string) (bool, error) {
	return f.runningByName[name], nil
}
func (f *hashRuntimeFake) ContainerLogs(name string, tail int) (string, error) {
	_, _ = name, tail
	return "", nil
}
func (f *hashRuntimeFake) ImageDigest(image string) (string, error) {
	_ = image
	return "", nil
}

func TestComputeRuntimeStateHash_DeterministicAcrossAppOrder(t *testing.T) {
	runtime := &hashRuntimeFake{
		existsByName:  map[string]bool{"api": true, "ui": true},
		runningByName: map[string]bool{"api": true, "ui": true},
		imageByName:   map[string]string{"api": "nginx:1.27", "ui": "nginx:1.27"},
		labelByName:   map[string]string{"api": "h1", "ui": "h2"},
		portsByName:   map[string]map[int]int{"api": {80: 8080}, "ui": {80: 8081}},
		envByName:     map[string]map[string]string{"api": {"A": "1"}, "ui": {"B": "2"}},
		networkByName: map[string]string{"api": "apps", "ui": "apps"},
	}

	appsA := []manifest.Application{
		{Metadata: manifest.Metadata{Name: "api"}},
		{Metadata: manifest.Metadata{Name: "ui"}},
	}
	appsB := []manifest.Application{
		{Metadata: manifest.Metadata{Name: "ui"}},
		{Metadata: manifest.Metadata{Name: "api"}},
	}

	hashA, err := computeRuntimeStateHash(runtime, appsA)
	if err != nil {
		t.Fatal(err)
	}
	hashB, err := computeRuntimeStateHash(runtime, appsB)
	if err != nil {
		t.Fatal(err)
	}

	if hashA != hashB {
		t.Fatalf("expected deterministic runtime hash across app order, got %s vs %s", hashA, hashB)
	}
}

func TestComputeRuntimeStateHash_ChangesWhenContainerStops(t *testing.T) {
	runtime := &hashRuntimeFake{
		existsByName:  map[string]bool{"api": true},
		runningByName: map[string]bool{"api": true},
		imageByName:   map[string]string{"api": "nginx:1.27"},
		labelByName:   map[string]string{"api": "h1"},
		portsByName:   map[string]map[int]int{"api": {80: 8080}},
		envByName:     map[string]map[string]string{"api": {"A": "1"}},
		networkByName: map[string]string{"api": "apps"},
	}
	apps := []manifest.Application{{Metadata: manifest.Metadata{Name: "api"}}}

	before, err := computeRuntimeStateHash(runtime, apps)
	if err != nil {
		t.Fatal(err)
	}

	runtime.runningByName["api"] = false

	after, err := computeRuntimeStateHash(runtime, apps)
	if err != nil {
		t.Fatal(err)
	}

	if before == after {
		t.Fatal("expected runtime hash to change when container running state changes")
	}
}

func TestComputeRuntimeStateHash_ChangesWhenEnvironmentChanges(t *testing.T) {
	runtime := &hashRuntimeFake{
		existsByName:  map[string]bool{"api": true},
		runningByName: map[string]bool{"api": true},
		imageByName:   map[string]string{"api": "nginx:1.27"},
		labelByName:   map[string]string{"api": "h1"},
		portsByName:   map[string]map[int]int{"api": {80: 8080}},
		envByName:     map[string]map[string]string{"api": {"A": "1"}},
		networkByName: map[string]string{"api": "apps"},
	}
	apps := []manifest.Application{{Metadata: manifest.Metadata{Name: "api"}}}

	before, err := computeRuntimeStateHash(runtime, apps)
	if err != nil {
		t.Fatal(err)
	}

	runtime.envByName["api"]["A"] = "2"

	after, err := computeRuntimeStateHash(runtime, apps)
	if err != nil {
		t.Fatal(err)
	}

	if before == after {
		t.Fatal("expected runtime hash to change when environment changes")
	}
}

func TestComputeDesiredStateHash_DeterministicAcrossAppOrder(t *testing.T) {
	appsA := []manifest.Application{
		{Metadata: manifest.Metadata{Name: "api"}, Environment: map[string]string{"TOKEN": "a"}},
		{Metadata: manifest.Metadata{Name: "ui"}, Environment: map[string]string{"TOKEN": "b"}},
	}
	appsB := []manifest.Application{
		{Metadata: manifest.Metadata{Name: "ui"}, Environment: map[string]string{"TOKEN": "b"}},
		{Metadata: manifest.Metadata{Name: "api"}, Environment: map[string]string{"TOKEN": "a"}},
	}

	hashA, err := computeDesiredStateHash(appsA)
	if err != nil {
		t.Fatal(err)
	}
	hashB, err := computeDesiredStateHash(appsB)
	if err != nil {
		t.Fatal(err)
	}

	if hashA != hashB {
		t.Fatalf("expected deterministic desired hash across app order, got %s vs %s", hashA, hashB)
	}
}

func TestComputeDesiredStateHash_ChangesWhenDesiredConfigChanges(t *testing.T) {
	apps := []manifest.Application{{Metadata: manifest.Metadata{Name: "api"}, Environment: map[string]string{"TOKEN": "a"}}}

	before, err := computeDesiredStateHash(apps)
	if err != nil {
		t.Fatal(err)
	}

	apps[0].Environment["TOKEN"] = "b"

	after, err := computeDesiredStateHash(apps)
	if err != nil {
		t.Fatal(err)
	}

	if before == after {
		t.Fatal("expected desired hash to change when desired environment changes")
	}
}

func containsStringRun(s, sub string) bool {
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

// pullErrorRuntime wraps hashRuntimeFake but returns an error from PullImage
// for containers whose image matches a configured value.
type pullErrorRuntime struct {
	hashRuntimeFake
	failImage  string
	pulledApps []string
}

func (f *pullErrorRuntime) PullImage(image string) error {
	f.pulledApps = append(f.pulledApps, image)
	if image == f.failImage {
		return errors.New("simulated pull error")
	}
	return nil
}

func TestReconcileApps_ContinuesAfterError(t *testing.T) {
	rt := &pullErrorRuntime{
		failImage: "nginx:fail",
		hashRuntimeFake: hashRuntimeFake{
			existsByName:  map[string]bool{},
			runningByName: map[string]bool{},
			imageByName:   map[string]string{},
			labelByName:   map[string]string{},
			portsByName:   map[string]map[int]int{},
			envByName:     map[string]map[string]string{},
			networkByName: map[string]string{},
		},
	}

	apps := []manifest.Application{
		{Metadata: manifest.Metadata{Name: "failing-app"}, Image: manifest.ImageConfig{Repository: "nginx", Tag: "fail"}},
		{Metadata: manifest.Metadata{Name: "ok-app"}, Image: manifest.ImageConfig{Repository: "nginx", Tag: "ok"}},
	}

	r := &reconciler.Reconciler{Runtime: rt, Plugins: plugins.NewManager()}
	errs := reconcileApps(context.Background(), r, apps)

	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if !containsStringRun(errs[0].Error(), "failing-app") {
		t.Fatalf("expected error to mention 'failing-app', got: %v", errs[0])
	}

	// Both images must have been attempted.
	if len(rt.pulledApps) != 2 {
		t.Fatalf("expected 2 pull attempts, got %d: %v", len(rt.pulledApps), rt.pulledApps)
	}
}

func TestReconcileApps_NoErrors(t *testing.T) {
	rt := &pullErrorRuntime{
		failImage: "", // never fail
		hashRuntimeFake: hashRuntimeFake{
			existsByName:  map[string]bool{},
			runningByName: map[string]bool{},
			imageByName:   map[string]string{},
			labelByName:   map[string]string{},
			portsByName:   map[string]map[int]int{},
			envByName:     map[string]map[string]string{},
			networkByName: map[string]string{},
		},
	}

	apps := []manifest.Application{
		{Metadata: manifest.Metadata{Name: "app-a"}, Image: manifest.ImageConfig{Repository: "nginx", Tag: "1.25"}},
		{Metadata: manifest.Metadata{Name: "app-b"}, Image: manifest.ImageConfig{Repository: "nginx", Tag: "1.27"}},
	}

	r := &reconciler.Reconciler{Runtime: rt, Plugins: plugins.NewManager()}
	errs := reconcileApps(context.Background(), r, apps)

	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %d: %v", len(errs), errs)
	}
	if len(rt.pulledApps) != 2 {
		t.Fatalf("expected 2 pull attempts, got %d", len(rt.pulledApps))
	}
}
