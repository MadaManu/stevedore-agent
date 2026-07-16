package docker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

type Runtime interface {
	PullImage(image string) error
	ContainerExists(name string) (bool, error)
	ContainerImage(name string) (string, error)
	ContainerLabel(name, key string) (string, error)
	ContainerPorts(name string) (map[int]int, error)
	ContainerEnvironment(name string) (map[string]string, error)
	ContainerNetwork(name string) (string, error)
	CreateContainer(spec ContainerSpec) error
	StartContainer(name string) error
	StopContainer(name string) error
	RemoveContainer(name string) error
	NetworkExists(name string) (bool, error)
	CreateNetwork(name string) error
	ContainerRunning(name string) (bool, error)
	ContainerLogs(name string, tail int) (string, error)
	ImageDigest(image string) (string, error)
}

type ContainerSpec struct {
	Name          string
	Image         string
	RestartPolicy string
	Ports         []PortMapping
	Volumes       []VolumeMapping
	Environment   map[string]string
	NetworkName   string
	Labels        map[string]string
}

type PortMapping struct {
	HostPort      int
	ContainerPort int
}

type VolumeMapping struct {
	HostPath  string
	MountPath string
}

type DockerRuntime struct{}

func NewDockerRuntime() *DockerRuntime {
	return &DockerRuntime{}
}

func (d *DockerRuntime) PullImage(image string) error {
	_, err := runDocker("pull", image)
	return err
}

func (d *DockerRuntime) ContainerExists(name string) (bool, error) {
	out, err := runDocker("ps", "-a", "--filter", "name=^/"+name+"$", "--format", "{{.Names}}")
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.TrimSpace(line) == name {
			return true, nil
		}
	}
	return false, nil
}

func (d *DockerRuntime) ContainerImage(name string) (string, error) {
	out, err := runDocker("inspect", "-f", "{{.Config.Image}}", name)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (d *DockerRuntime) ContainerLabel(name, key string) (string, error) {
	format := fmt.Sprintf("{{ index .Config.Labels %q }}", key)
	out, err := runDocker("inspect", "-f", format, name)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// ContainerPorts returns a map of containerPort -> hostPort for ports that are
// actively published by Docker at runtime.
func (d *DockerRuntime) ContainerPorts(name string) (map[int]int, error) {
	out, err := runDocker("inspect", "--format", "{{json .NetworkSettings.Ports}}", name)
	if err != nil {
		return nil, err
	}
	// Docker returns: {"80/tcp":[{"HostIp":"0.0.0.0","HostPort":"8081"}]}
	// For unbound ports the value can be null or an empty array.
	var raw map[string][]struct {
		HostPort string `json:"HostPort"`
	}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return nil, fmt.Errorf("parse port bindings for %s: %w", name, err)
	}
	result := make(map[int]int)
	for portProto, bindings := range raw {
		parts := strings.SplitN(portProto, "/", 2)
		if len(parts) != 2 {
			continue
		}
		containerPort, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}
		if len(bindings) > 0 && bindings[0].HostPort != "" {
			hostPort, err := strconv.Atoi(bindings[0].HostPort)
			if err != nil {
				continue
			}
			result[containerPort] = hostPort
		}
	}
	return result, nil
}

func (d *DockerRuntime) CreateContainer(spec ContainerSpec) error {
	args := []string{"create", "--name", spec.Name}

	rp := spec.RestartPolicy
	if rp == "" {
		rp = "always"
	}
	args = append(args, "--restart", rp)

	for _, p := range spec.Ports {
		args = append(args, "-p", fmt.Sprintf("%d:%d", p.HostPort, p.ContainerPort))
	}
	for _, v := range spec.Volumes {
		args = append(args, "-v", fmt.Sprintf("%s:%s", v.HostPath, v.MountPath))
	}
	if spec.NetworkName != "" {
		args = append(args, "--network", spec.NetworkName)
	}

	if len(spec.Environment) > 0 {
		keys := make([]string, 0, len(spec.Environment))
		for k := range spec.Environment {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			args = append(args, "-e", fmt.Sprintf("%s=%s", k, spec.Environment[k]))
		}
	}

	if len(spec.Labels) > 0 {
		keys := make([]string, 0, len(spec.Labels))
		for k := range spec.Labels {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			args = append(args, "--label", fmt.Sprintf("%s=%s", k, spec.Labels[k]))
		}
	}

	args = append(args, spec.Image)
	_, err := runDocker(args...)
	return err
}

func (d *DockerRuntime) StartContainer(name string) error {
	_, err := runDocker("start", name)
	return err
}

func (d *DockerRuntime) StopContainer(name string) error {
	_, err := runDocker("stop", name)
	return err
}

func (d *DockerRuntime) RemoveContainer(name string) error {
	_, err := runDocker("rm", name)
	return err
}

func (d *DockerRuntime) NetworkExists(name string) (bool, error) {
	out, err := runDocker("network", "ls", "--filter", "name=^"+name+"$", "--format", "{{.Name}}")
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.TrimSpace(line) == name {
			return true, nil
		}
	}
	return false, nil
}

func (d *DockerRuntime) CreateNetwork(name string) error {
	_, err := runDocker("network", "create", name)
	return err
}

func (d *DockerRuntime) ContainerRunning(name string) (bool, error) {
	out, err := runDocker("inspect", "-f", "{{.State.Running}}", name)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) == "true", nil
}

func (d *DockerRuntime) ContainerLogs(name string, tail int) (string, error) {
	if tail <= 0 {
		tail = 100
	}
	return runDocker("logs", "--tail", fmt.Sprintf("%d", tail), name)
}

// ContainerEnvironment returns the environment variables from a container as a map.
func (d *DockerRuntime) ContainerEnvironment(name string) (map[string]string, error) {
	out, err := runDocker("inspect", "-f", "{{json .Config.Env}}", name)
	if err != nil {
		return nil, err
	}
	var envList []string
	if err := json.Unmarshal([]byte(out), &envList); err != nil {
		return nil, fmt.Errorf("parse environment for %s: %w", name, err)
	}
	env := make(map[string]string)
	for _, pair := range envList {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) == 2 {
			env[parts[0]] = parts[1]
		}
	}
	return env, nil
}

// ContainerNetwork returns the network name the container is connected to.
func (d *DockerRuntime) ContainerNetwork(name string) (string, error) {
	out, err := runDocker("inspect", "-f", "{{json .NetworkSettings.Networks}}", name)
	if err != nil {
		return "", err
	}
	// Docker returns: {"networkname":{...}, "bridge":{...}}
	// We want to get the first key (usually the primary network)
	var networks map[string]interface{}
	if err := json.Unmarshal([]byte(out), &networks); err != nil {
		return "", fmt.Errorf("parse networks for %s: %w", name, err)
	}

	// Get the first (non-bridge) network name, or fall back to bridge
	for netName := range networks {
		if netName != "bridge" {
			return netName, nil
		}
	}
	// If only bridge exists, return it
	if len(networks) > 0 {
		for netName := range networks {
			return netName, nil
		}
	}
	return "", nil
}

// ImageDigest returns the digest (sha256:...) of the given image.
// This is used to resolve "latest" tags to actual image IDs for comparison.
func (d *DockerRuntime) ImageDigest(image string) (string, error) {
	out, err := runDocker("inspect", image, "--format", "{{.Id}}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func runDocker(args ...string) (string, error) {
	cmd := exec.Command("docker", args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("docker %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

func HashFromSpec(spec ContainerSpec) (string, error) {
	b, err := json.Marshal(spec)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", simpleChecksum(b)), nil
}

func simpleChecksum(b []byte) uint64 {
	var sum uint64
	for _, v := range b {
		sum = sum*131 + uint64(v)
	}
	return sum
}
