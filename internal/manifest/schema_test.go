package manifest

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestApplicationOrderedNetworkNamesPreservesDeclarationOrder(t *testing.T) {
	app := Application{
		Network: NetworkConfig{Name: "frontend"},
		Networks: []NetworkConfig{
			{Name: "backend"},
			{Name: "frontend"},
			{Name: "internal"},
		},
	}

	got := app.OrderedNetworkNames()
	want := []string{"frontend", "backend", "internal"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected ordered networks: want %v got %v", want, got)
	}
}

func TestApplicationNetworkNamesReturnsStableSortedSet(t *testing.T) {
	app := Application{
		Networks: []NetworkConfig{
			{Name: "zeta"},
			{Name: "alpha"},
			{Name: "zeta"},
		},
	}

	got := app.NetworkNames()
	want := []string{"alpha", "zeta"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected normalized networks: want %v got %v", want, got)
	}
}

func TestApplicationPrimaryNetworkNameUsesFirstDeclaredNetwork(t *testing.T) {
	app := Application{
		Networks: []NetworkConfig{
			{Name: "zeta"},
			{Name: "alpha"},
		},
	}
	if got := app.PrimaryNetworkName(); got != "zeta" {
		t.Fatalf("expected first declared network to be primary, got %q", got)
	}
}

func TestApplicationOrderedVolumeMappingsPreservesDeclarationOrder(t *testing.T) {
	app := Application{
		Volume: VolumeMapping{HostPath: "/data/primary", MountPath: "/srv/data"},
		Volumes: []VolumeMapping{
			{HostPath: "/data/cache", MountPath: "/srv/cache"},
			{HostPath: "/data/primary", MountPath: "/srv/data"},
			{HostPath: "/data/logs", MountPath: "/srv/logs"},
		},
	}

	got := app.OrderedVolumeMappings()
	want := []VolumeMapping{
		{HostPath: "/data/primary", MountPath: "/srv/data"},
		{HostPath: "/data/cache", MountPath: "/srv/cache"},
		{HostPath: "/data/logs", MountPath: "/srv/logs"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected ordered volumes: want %v got %v", want, got)
	}
}

func TestApplicationVolumeMappingsReturnsStableSortedSet(t *testing.T) {
	app := Application{
		Volumes: []VolumeMapping{
			{HostPath: "/data/z", MountPath: "/srv/z"},
			{HostPath: "/data/a", MountPath: "/srv/a"},
		},
	}

	got := app.VolumeMappings()
	want := []VolumeMapping{
		{HostPath: "/data/a", MountPath: "/srv/a"},
		{HostPath: "/data/z", MountPath: "/srv/z"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected normalized volumes: want %v got %v", want, got)
	}
}

func TestApplicationResolvedImageUsesGeneratedBuildTag(t *testing.T) {
	app := Application{
		Metadata: Metadata{Name: "Demo_UI"},
		Build:    BuildConfig{Context: "."},
	}

	if got := app.ResolvedImage(); got != "stevedore/demo-ui:local" {
		t.Fatalf("unexpected build image tag: %q", got)
	}
}

func TestApplicationBuildPathsResolveFromManifestDirectory(t *testing.T) {
	app := Application{
		Metadata:   Metadata{Name: "demo"},
		Build:      BuildConfig{Context: "../context", Dockerfile: "docker/Dockerfile"},
		SourcePath: filepath.Join("/repo", "apps", "demo", "stevedore.yml"),
	}

	if got := app.BuildContextPath(); got != filepath.Clean("/repo/apps/context") {
		t.Fatalf("unexpected build context path: %q", got)
	}
	if got := app.BuildDockerfilePath(); got != filepath.Clean("/repo/apps/demo/docker/Dockerfile") {
		t.Fatalf("unexpected dockerfile path: %q", got)
	}
}
