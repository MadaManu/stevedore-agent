package reconciler

import (
	"fmt"
	"sort"
	"strings"

	"stevedore-agent/internal/docker"
	"stevedore-agent/internal/manifest"
)

// DriftReport describes the differences found between the desired manifest state
// and the actual running container state.
type DriftReport struct {
	HasDrift             bool
	ImageChanged         bool
	CurrentImage         string
	DesiredImage         string
	PortDrifts           []PortDrift
	EnvDrifts            map[string]EnvDrift
	IgnoredExtraEnv      map[string]string
	NetworkChanged       bool
	CurrentNetwork       string
	DesiredNetwork       string
	VolumeChanged        bool
	CurrentVolumes       []docker.VolumeMapping
	DesiredVolumes       []docker.VolumeMapping
	CurrentNetworks      []string
	DesiredNetworks      []string
	IgnoredExtraNetworks []string
	ConfigDrift          bool // restart policy or other hashed fields changed
}

// PortDrift describes a single port mapping that differs from desired state.
type PortDrift struct {
	ContainerPort   int
	DesiredHostPort int
	ActualHostPort  int
}

// EnvDrift describes a single environment variable that differs from desired state.
type EnvDrift struct {
	Name    string
	Current string
	Desired string
	Added   bool // true if variable is present in actual but not in desired
	Removed bool // true if variable is present in desired but not in actual
}

// Summary returns a human-readable, single-line description of all detected drift.
// Returns "none" when HasDrift is false.
func (d DriftReport) Summary() string {
	items := d.Items()
	if len(items) == 0 {
		return "none"
	}
	return strings.Join(items, "; ")
}

// Items returns each drift item as its own human-readable entry.
func (d DriftReport) Items() []string {
	if !d.HasDrift {
		return nil
	}
	items := make([]string, 0)
	if d.ImageChanged {
		items = append(items, fmt.Sprintf("image: %s → %s", d.CurrentImage, d.DesiredImage))
	}
	for _, p := range d.PortDrifts {
		items = append(items, fmt.Sprintf("port %d: host %d → %d", p.ContainerPort, p.ActualHostPort, p.DesiredHostPort))
	}
	if len(d.EnvDrifts) > 0 {
		envKeys := make([]string, 0, len(d.EnvDrifts))
		for name := range d.EnvDrifts {
			envKeys = append(envKeys, name)
		}
		sort.Strings(envKeys)
		for _, name := range envKeys {
			drift := d.EnvDrifts[name]
			if drift.Added {
				items = append(items, fmt.Sprintf("env: %s (extra)", name))
			} else if drift.Removed {
				items = append(items, fmt.Sprintf("env: %s (missing)", name))
			} else {
				items = append(items, fmt.Sprintf("env: %s: %q → %q", name, drift.Current, drift.Desired))
			}
		}
	}
	if d.NetworkChanged {
		items = append(items, fmt.Sprintf("networks: %s → %s", summarizeStrings(d.CurrentNetworks), summarizeStrings(d.DesiredNetworks)))
	}
	if d.VolumeChanged {
		items = append(items, fmt.Sprintf("volumes: %s → %s", summarizeVolumes(d.CurrentVolumes), summarizeVolumes(d.DesiredVolumes)))
	}
	if d.ConfigDrift {
		items = append(items, "config changed (restart/other hashed fields)")
	}
	return items
}

// ComputeDrift inspects the running container and compares it against the desired
// manifest state. It returns a DriftReport with specific field-level differences.
//
// The container must already exist; callers should check ContainerExists before calling.
func ComputeDrift(runtime docker.Runtime, app manifest.Application) (DriftReport, error) {
	report := DriftReport{
		EnvDrifts:       make(map[string]EnvDrift),
		IgnoredExtraEnv: make(map[string]string),
	}

	// 1. Image drift - resolve "latest" tag if needed
	currentImage, err := runtime.ContainerImage(app.ContainerName())
	if err != nil {
		return report, err
	}
	desiredImage := app.FullImage()

	// For "latest" tags or when images differ, compare by digest
	if currentImage != desiredImage {
		currentDigest, digestErr := runtime.ImageDigest(currentImage)
		desiredDigest, desiredDigestErr := runtime.ImageDigest(desiredImage)

		// If we can compare by digest (more reliable than tag strings)
		if digestErr == nil && desiredDigestErr == nil {
			if currentDigest != desiredDigest {
				report.HasDrift = true
				report.ImageChanged = true
				report.CurrentImage = currentImage
				report.DesiredImage = desiredImage
			}
		} else {
			// Fall back to string comparison if digest lookup fails
			report.HasDrift = true
			report.ImageChanged = true
			report.CurrentImage = currentImage
			report.DesiredImage = desiredImage
		}
	}

	// 2. Port drift — inspect actual host port bindings
	actualPorts, err := runtime.ContainerPorts(app.ContainerName())
	if err != nil {
		return report, err
	}
	for _, desired := range app.Ports {
		actual, ok := actualPorts[desired.ContainerPort]
		if !ok || actual != desired.HostPort {
			report.HasDrift = true
			report.PortDrifts = append(report.PortDrifts, PortDrift{
				ContainerPort:   desired.ContainerPort,
				DesiredHostPort: desired.HostPort,
				ActualHostPort:  actual,
			})
		}
	}

	// 3. Environment variable drift
	actualEnv, err := runtime.ContainerEnvironment(app.ContainerName())
	if err != nil {
		return report, err
	}
	desiredEnv := app.Environment
	if desiredEnv == nil {
		desiredEnv = make(map[string]string)
	}

	// Check for changed or removed desired variables
	for desiredKey, desiredVal := range desiredEnv {
		if actualVal, ok := actualEnv[desiredKey]; !ok {
			// Environment variable missing from actual container
			report.HasDrift = true
			report.EnvDrifts[desiredKey] = EnvDrift{
				Name:    desiredKey,
				Desired: desiredVal,
				Removed: true,
			}
		} else if actualVal != desiredVal {
			// Environment variable value differs
			report.HasDrift = true
			report.EnvDrifts[desiredKey] = EnvDrift{
				Name:    desiredKey,
				Current: actualVal,
				Desired: desiredVal,
			}
		}
	}

	// Track extra runtime environment variables so callers can log them, but
	// do not treat them as reconciliation drift.
	for actualKey, actualVal := range actualEnv {
		if _, ok := desiredEnv[actualKey]; !ok {
			report.IgnoredExtraEnv[actualKey] = actualVal
		}
	}

	// 4. Network drift — only desired networks that are missing from actual
	// trigger drift. Extra runtime-only networks (e.g. Docker's default
	// bridge) are recorded for informational logging but do not cause
	// reconciliation.
	actualNetworks, err := runtime.ContainerNetworks(app.ContainerName())
	if err != nil {
		return report, err
	}
	desiredNetworks := app.NetworkNames()
	missingNetworks := missingStrings(desiredNetworks, actualNetworks)
	if len(missingNetworks) > 0 {
		report.HasDrift = true
		report.NetworkChanged = true
		report.CurrentNetworks = actualNetworks
		report.DesiredNetworks = desiredNetworks
		report.CurrentNetwork = summarizeStrings(actualNetworks)
		report.DesiredNetwork = summarizeStrings(desiredNetworks)
	}
	report.IgnoredExtraNetworks = extraStrings(actualNetworks, desiredNetworks)

	// 5. Volume drift
	actualVolumes, err := runtime.ContainerVolumes(app.ContainerName())
	if err != nil {
		return report, err
	}
	desiredVolumes := desiredVolumeMappings(app)
	if !volumeMappingsEqual(actualVolumes, desiredVolumes) {
		report.HasDrift = true
		report.VolumeChanged = true
		report.CurrentVolumes = actualVolumes
		report.DesiredVolumes = desiredVolumes
	}

	// 6. Config drift — hash catches restart policy and any remaining spec drift
	if !report.ImageChanged && len(report.PortDrifts) == 0 && len(report.EnvDrifts) == 0 && !report.NetworkChanged && !report.VolumeChanged {
		_, desiredHash, err := desired(app)
		if err != nil {
			return report, err
		}
		currentHash, err := runtime.ContainerLabel(app.ContainerName(), "stevedore.dev/hash")
		if err != nil {
			return report, err
		}
		if currentHash != desiredHash {
			report.HasDrift = true
			report.ConfigDrift = true
		}
	}

	return report, nil
}

func summarizeStrings(items []string) string {
	if len(items) == 0 {
		return ""
	}
	return strings.Join(items, ", ")
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// missingStrings returns elements of needles not present in haystack.
func missingStrings(needles, haystack []string) []string {
	set := make(map[string]struct{}, len(haystack))
	for _, s := range haystack {
		set[s] = struct{}{}
	}
	var missing []string
	for _, s := range needles {
		if _, ok := set[s]; !ok {
			missing = append(missing, s)
		}
	}
	return missing
}

// extraStrings returns elements of actual not present in desired.
func extraStrings(actual, desired []string) []string {
	set := make(map[string]struct{}, len(desired))
	for _, s := range desired {
		set[s] = struct{}{}
	}
	var extra []string
	for _, s := range actual {
		if _, ok := set[s]; !ok {
			extra = append(extra, s)
		}
	}
	return extra
}

func desiredVolumeMappings(app manifest.Application) []docker.VolumeMapping {
	volumes := app.VolumeMappings()
	result := make([]docker.VolumeMapping, 0, len(volumes))
	for _, volume := range volumes {
		result = append(result, docker.VolumeMapping{
			HostPath:  volume.HostPath,
			MountPath: volume.MountPath,
		})
	}
	return result
}

func volumeMappingsEqual(a, b []docker.VolumeMapping) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].HostPath != b[i].HostPath || a[i].MountPath != b[i].MountPath {
			return false
		}
	}
	return true
}

func summarizeVolumes(volumes []docker.VolumeMapping) string {
	if len(volumes) == 0 {
		return ""
	}
	items := make([]string, 0, len(volumes))
	for _, volume := range volumes {
		items = append(items, fmt.Sprintf("%s:%s", volume.HostPath, volume.MountPath))
	}
	return strings.Join(items, ", ")
}
