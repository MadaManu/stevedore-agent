package secrets

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// LocalProvider resolves secret paths from a local YAML/JSON file.
type LocalProvider struct {
	filePath string
}

func NewLocalProvider(filePath string) (*LocalProvider, error) {
	p := strings.TrimSpace(filePath)
	if p == "" {
		return nil, fmt.Errorf("local secrets file path is required")
	}
	return &LocalProvider{filePath: p}, nil
}

func (p *LocalProvider) Resolve(path string) (string, error) {
	root, err := p.loadStore()
	if err != nil {
		return "", err
	}

	value, err := lookup(root, path)
	if err != nil {
		return "", err
	}

	switch typed := value.(type) {
	case nil:
		return "", fmt.Errorf("secret path %q resolved to null", path)
	case map[string]any, []any:
		return "", fmt.Errorf("secret path %q resolved to non-scalar value", path)
	case string:
		return typed, nil
	default:
		return fmt.Sprint(typed), nil
	}
}

func (p *LocalProvider) loadStore() (any, error) {
	return LoadLocalStore(p.filePath)
}

// LoadLocalStore reads and parses a local YAML/JSON secrets store file.
func LoadLocalStore(filePath string) (any, error) {
	b, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read local secrets store %s: %w", filePath, err)
	}

	var raw any
	if err := yaml.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("parse local secrets store %s: %w", filePath, err)
	}
	return normalize(raw), nil
}

func lookup(root any, path string) (any, error) {
	segments, err := splitPath(path)
	if err != nil {
		return nil, err
	}

	cur := root
	for _, segment := range segments {
		switch node := cur.(type) {
		case map[string]any:
			next, ok := node[segment]
			if !ok {
				return nil, fmt.Errorf("secret path %q not found", path)
			}
			cur = next
		case []any:
			idx, convErr := strconv.Atoi(segment)
			if convErr != nil || idx < 0 || idx >= len(node) {
				return nil, fmt.Errorf("invalid list index %q in secret path %q", segment, path)
			}
			cur = node[idx]
		default:
			return nil, fmt.Errorf("cannot descend into %q for secret path %q", segment, path)
		}
	}

	return cur, nil
}

func splitPath(path string) ([]string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return nil, fmt.Errorf("secret path is required")
	}

	if strings.Contains(trimmed, "/") {
		parts := strings.Split(trimmed, "/")
		segments := make([]string, 0, len(parts))
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			segments = append(segments, part)
		}
		if len(segments) == 0 {
			return nil, fmt.Errorf("secret path is required")
		}
		return segments, nil
	}

	parts := strings.Split(trimmed, ".")
	segments := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, fmt.Errorf("invalid secret path %q", path)
		}
		segments = append(segments, part)
	}
	return segments, nil
}

func normalize(v any) any {
	switch node := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(node))
		for k, value := range node {
			out[k] = normalize(value)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(node))
		for k, value := range node {
			out[fmt.Sprint(k)] = normalize(value)
		}
		return out
	case []any:
		for i := range node {
			node[i] = normalize(node[i])
		}
		return node
	default:
		return node
	}
}
