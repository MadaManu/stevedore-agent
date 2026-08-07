package manifest

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type discoveryDeps struct {
	glob     func(string) ([]string, error)
	stat     func(string) (os.FileInfo, error)
	hostFQDN func() (string, error)
}

// FQDNResolutionError indicates that host FQDN discovery failed.
type FQDNResolutionError struct {
	cause error
}

func (e *FQDNResolutionError) Error() string {
	if e == nil || e.cause == nil {
		return "resolve host fqdn: unknown error"
	}
	return e.cause.Error()
}

func (e *FQDNResolutionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func defaultDiscoveryDeps() discoveryDeps {
	return discoveryDeps{
		glob:     filepath.Glob,
		stat:     os.Stat,
		hostFQDN: HostFQDN,
	}
}

// HostFQDN returns the OS-reported fully-qualified domain name.
func HostFQDN() (string, error) {
	cmd := exec.Command("hostname", "-f")
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			stderr := strings.TrimSpace(string(exitErr.Stderr))
			if stderr != "" {
				return "", &FQDNResolutionError{cause: fmt.Errorf("resolve host fqdn: hostname -f failed: %s", stderr)}
			}
		}
		return "", &FQDNResolutionError{cause: fmt.Errorf("resolve host fqdn: %w", err)}
	}

	fqdn := strings.TrimSpace(string(out))
	if fqdn == "" {
		return "", &FQDNResolutionError{cause: errors.New("resolve host fqdn: hostname -f returned an empty value")}
	}
	return fqdn, nil
}

// DiscoverManifestPaths lists manifest paths for the current host.
//
// Lookup order:
//  1. <repoRoot>/<fqdn>/apps/*/stevedore.yml
//  2. <repoRoot>/apps/*/stevedore.yml
//
// When both directories exist, both sets are returned.
func DiscoverManifestPaths(repoRoot string) ([]string, error) {
	return discoverManifestPathsWithDeps(repoRoot, defaultDiscoveryDeps())
}

func discoverManifestPathsWithDeps(repoRoot string, deps discoveryDeps) ([]string, error) {
	fqdn, err := deps.hostFQDN()
	if err != nil {
		return nil, err
	}

	hostAppsDir := filepath.Join(repoRoot, fqdn, "apps")
	defaultAppsDir := filepath.Join(repoRoot, "apps")

	hostExists, err := directoryExists(hostAppsDir, deps.stat)
	if err != nil {
		return nil, fmt.Errorf("stat host apps directory %s: %w", hostAppsDir, err)
	}
	defaultExists, err := directoryExists(defaultAppsDir, deps.stat)
	if err != nil {
		return nil, fmt.Errorf("stat apps directory %s: %w", defaultAppsDir, err)
	}

	patterns := make([]string, 0, 2)
	if hostExists {
		patterns = append(patterns, filepath.Join(hostAppsDir, "*", "stevedore.yml"))
	}
	if defaultExists {
		patterns = append(patterns, filepath.Join(defaultAppsDir, "*", "stevedore.yml"))
	}

	paths := make([]string, 0)
	for _, pattern := range patterns {
		matches, err := deps.glob(pattern)
		if err != nil {
			return nil, fmt.Errorf("glob manifests: %w", err)
		}
		paths = append(paths, matches...)
	}
	sort.Strings(paths)
	return paths, nil
}

func directoryExists(path string, statFn func(string) (os.FileInfo, error)) (bool, error) {
	info, err := statFn(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return info.IsDir(), nil
}
