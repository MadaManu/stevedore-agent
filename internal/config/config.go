// Package config loads the long-running agent configuration that declares the
// source of truth (local path or git repository), authentication for git, and
// how frequently the agent should reconcile.
//
// The active source and git auth strategy are inferred from which node is
// present in the YAML rather than an explicit discriminator field. Declaring
// more than one is a validation error.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/bitstep-ie/mango-go/pkg/env"
	"gopkg.in/yaml.v3"
)

const (
	// HomeEnvVar overrides the default stevedore home directory.
	HomeEnvVar = "STEVEDORE_HOME"
	// WorkdirEnvVar overrides where git sources are checked out.
	WorkdirEnvVar = "STEVEDORE_WORKDIR"
	// LogDirEnvVar overrides the logging directory.
	LogDirEnvVar = "STEVEDORE_LOG_DIR"
	// DebugEnvVar overrides verbose logging (parsed as bool).
	DebugEnvVar = "STEVEDORE_DEBUG"
	// IntervalEnvVar overrides the poll interval (parsed as a Go duration).
	IntervalEnvVar = "STEVEDORE_POLL_INTERVAL"
	// ApacheSitesDirEnvVar overrides the apache exposure provider sites-enabled directory.
	// Setting this activates the apache provider even if exposure.apache is not present
	// in config.yml.
	ApacheSitesDirEnvVar = "STEVEDORE_APACHE_SITES_DIR"

	// ConfigFileName is the fixed configuration filename within STEVEDORE_HOME.
	ConfigFileName = "config.yml"
	// DefaultHomeDir is the default stevedore home when STEVEDORE_HOME is unset.
	DefaultHomeDir = "/etc/stevedore"
	// DefaultGitSourceDirName is the directory under STEVEDORE_HOME used as the
	// default git checkout path when source.git.workdir is omitted.
	DefaultGitSourceDirName = "git-source"
	// DefaultInterval is used when poll.interval is not set.
	DefaultInterval = 30 * time.Second
	// DefaultLogDir is used when no logging directory is configured.
	DefaultLogDir = "/var/log/stevedore"
)

// AuthMethod names the git authentication strategy that is active.
type AuthMethod string

const (
	AuthNone  AuthMethod = "none"
	AuthToken AuthMethod = "token"
	AuthBasic AuthMethod = "basic"
	AuthSSH   AuthMethod = "ssh"
)

// SettingSource describes where an effective setting value came from.
type SettingSource string

const (
	SettingSourceDefault    SettingSource = "default"
	SettingSourceConfigFile SettingSource = "config"
	SettingSourceEnvVar     SettingSource = "env"
	SettingSourceDerived    SettingSource = "derived"
)

// SettingOrigin records how an effective setting value was chosen.
type SettingOrigin struct {
	Source    SettingSource
	Reference string
}

// Describe returns a human-readable description of the setting origin.
func (o SettingOrigin) Describe() string {
	switch o.Source {
	case SettingSourceEnvVar:
		if o.Reference != "" {
			return fmt.Sprintf("environment variable %s", o.Reference)
		}
		return "environment variable"
	case SettingSourceConfigFile:
		if o.Reference != "" {
			return fmt.Sprintf("config file %s", o.Reference)
		}
		return "config file"
	case SettingSourceDerived:
		if o.Reference != "" {
			return fmt.Sprintf("derived from %s", o.Reference)
		}
		return "derived value"
	default:
		if o.Reference != "" {
			return fmt.Sprintf("built-in default %s", o.Reference)
		}
		return "built-in default"
	}
}

// EffectiveSetting is a single resolved configuration setting suitable for
// diagnostics such as `stevedore-agent doctor`.
type EffectiveSetting struct {
	Path      string
	Value     string
	Origin    SettingOrigin
	Sensitive bool
	Note      string
}

// Config is the top-level agent configuration. It is loaded and validated once
// at process startup and then treated as immutable for the life of the process.
type Config struct {
	Logging  Logging                  `yaml:"logging"`
	Source   Source                   `yaml:"source"`
	Secrets  Secrets                  `yaml:"secrets"`
	Poll     Poll                     `yaml:"poll"`
	Exposure ExposureConfig           `yaml:"exposure"`
	path     string                   `yaml:"-"`
	origins  map[string]SettingOrigin `yaml:"-"`
}

// ExposureConfig groups all exposure provider configuration. Each provider is
// optional and only required when at least one app uses that provider.
type ExposureConfig struct {
	Apache *ApacheExposureConfig `yaml:"apache"`
}

// ApacheExposureConfig configures the Apache virtual-host exposure provider.
// Required only when at least one app sets expose.provider to "apache".
type ApacheExposureConfig struct {
	SitesDir string `yaml:"sitesDir"`
}

// Secrets configures secret provider backends used by manifest interpolation.
type Secrets struct {
	Providers SecretProviders `yaml:"providers"`
}

// SecretProviders lists supported secret backends.
type SecretProviders struct {
	Local *LocalSecretProvider `yaml:"local"`
}

// LocalSecretProvider loads secrets from a local YAML/JSON file.
type LocalSecretProvider struct {
	File string `yaml:"file"`
}

// Logging configures where and how logs are written.
type Logging struct {
	Dir   string `yaml:"dir"`
	Debug bool   `yaml:"debug"`
}

// Source describes where manifests are read from. Exactly one of Local or Git
// must be present; the active source is inferred from which node is set.
type Source struct {
	Local *LocalSource `yaml:"local"`
	Git   *GitSource   `yaml:"git"`
}

// LocalSource points at a filesystem repository root containing shared apps/
// and/or host-specific <fqdn>/apps/ manifests.
type LocalSource struct {
	Path string `yaml:"path"`
}

// GitSource describes a git repository source of truth.
type GitSource struct {
	URL     string  `yaml:"url"`
	Branch  string  `yaml:"branch"`
	Workdir string  `yaml:"workdir"`
	Auth    GitAuth `yaml:"auth"`
}

// GitAuth carries credentials used to connect to the git repository. At most one
// of Token, Basic, or SSH may be present; the active method is inferred from
// which node is set. When none are set, no authentication is used.
//
// Secret-bearing fields support ${ENV_VAR} interpolation so plaintext secrets
// can be kept out of the config file.
type GitAuth struct {
	Token *TokenAuth `yaml:"token"`
	Basic *BasicAuth `yaml:"basic"`
	SSH   *SSHAuth   `yaml:"ssh"`
}

// TokenAuth authenticates with a personal access / app token over https.
type TokenAuth struct {
	Value string `yaml:"value"`
}

// BasicAuth authenticates with a username/password over https.
type BasicAuth struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

// SSHAuth authenticates with an ssh private key.
type SSHAuth struct {
	KeyPath string `yaml:"keyPath"`
}

// Poll controls how often the agent checks the source of truth.
type Poll struct {
	Interval time.Duration `yaml:"interval"`
}

// Path returns the resolved path the config was loaded from.
func (c *Config) Path() string { return c.path }

// IsGit reports whether the active source of truth is a git repository.
func (s Source) IsGit() bool { return s.Git != nil }

// Method returns the active authentication method inferred from the config.
func (a GitAuth) Method() AuthMethod {
	switch {
	case a.Token != nil:
		return AuthToken
	case a.Basic != nil:
		return AuthBasic
	case a.SSH != nil:
		return AuthSSH
	default:
		return AuthNone
	}
}

// Load reads and validates configuration from STEVEDORE_HOME/config.yml (or
// /etc/stevedore/config.yml when STEVEDORE_HOME is unset).
func Load() (*Config, error) {
	return LoadFromPath(resolveConfigPath())
}

// ConfigPath returns the resolved config path using STEVEDORE_HOME (or the
// default home) joined with config.yml.
func ConfigPath() string {
	return resolveConfigPath()
}

// LoadFromPath reads and validates configuration from an explicit path.
// Precedence: ENV overrides > config file values > built-in defaults.
func LoadFromPath(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	// Start with base config containing all defaults
	cfg := newBaseConfig()
	cfg.path = path

	// Unmarshal YAML on top of defaults (only set fields override)
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	cfg.applyFileOrigins(path, &doc)

	// Apply git-specific defaults if git source is being used
	if cfg.Source.Git != nil {
		if strings.TrimSpace(cfg.Source.Git.Branch) == "" {
			cfg.Source.Git.Branch = "main"
		}
		if strings.TrimSpace(cfg.Source.Git.Workdir) == "" {
			cfg.Source.Git.Workdir = defaultGitWorkdir()
			cfg.setOrigin("source.git.workdir", gitWorkdirDefaultOrigin())
		}
	}

	// Apply environment variable overrides on top of everything
	cfg.applyEnvOverrides()

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config %s: %w", path, err)
	}
	return &cfg, nil
}

// newBaseConfig returns a Config with all default values already set.
func newBaseConfig() Config {
	cfg := Config{
		Logging: Logging{
			Dir:   DefaultLogDir,
			Debug: false,
		},
		Poll:   Poll{Interval: DefaultInterval},
		Source: Source{}, // Will be populated from YAML; no defaults here to avoid conflicts
	}
	cfg.initOrigins()
	return cfg
}

func resolveConfigPath() string {
	home := resolveHomeDir()
	return filepath.Join(filepath.Clean(home), ConfigFileName)
}

func resolveHomeDir() string {
	home, _ := env.StringDefault(HomeEnvVar, DefaultHomeDir)
	return home
}

func defaultGitWorkdir() string {
	return filepath.Join(filepath.Clean(resolveHomeDir()), DefaultGitSourceDirName)
}

func gitWorkdirDefaultOrigin() SettingOrigin {
	if home, ok := lookupNonEmptyEnv(HomeEnvVar); ok {
		return SettingOrigin{Source: SettingSourceDerived, Reference: fmt.Sprintf("%s (%s)", HomeEnvVar, home)}
	}
	return SettingOrigin{Source: SettingSourceDerived, Reference: DefaultHomeDir}
}

// applyEnvOverrides applies environment variable overrides on top of the config.
func (c *Config) applyEnvOverrides() {
	if logDir, ok := lookupNonEmptyEnv(LogDirEnvVar); ok {
		c.Logging.Dir = logDir
		c.setOrigin("logging.dir", envOrigin(LogDirEnvVar))
	}

	if raw, ok := os.LookupEnv(DebugEnvVar); ok {
		if debug, err := strconv.ParseBool(strings.TrimSpace(raw)); err == nil {
			c.Logging.Debug = debug
			c.setOrigin("logging.debug", envOrigin(DebugEnvVar))
		}
	}

	if raw, ok := os.LookupEnv(IntervalEnvVar); ok {
		if interval, err := time.ParseDuration(strings.TrimSpace(raw)); err == nil && interval > 0 {
			c.Poll.Interval = interval
			c.setOrigin("poll.interval", envOrigin(IntervalEnvVar))
		}
	}

	if apacheSitesDir, ok := lookupNonEmptyEnv(ApacheSitesDirEnvVar); ok {
		if c.Exposure.Apache == nil {
			c.Exposure.Apache = &ApacheExposureConfig{}
		}
		c.Exposure.Apache.SitesDir = apacheSitesDir
		c.setOrigin("exposure.apache.sitesDir", envOrigin(ApacheSitesDirEnvVar))
	}

	if c.Source.Git != nil {
		if workdir, ok := lookupNonEmptyEnv(WorkdirEnvVar); ok {
			c.Source.Git.Workdir = workdir
			c.setOrigin("source.git.workdir", envOrigin(WorkdirEnvVar))
		}
		// Expand env references in secret-bearing fields.
		if c.Source.Git.Auth.Token != nil {
			c.Source.Git.Auth.Token.Value = expandEnv(c.Source.Git.Auth.Token.Value)
		}
		if c.Source.Git.Auth.Basic != nil {
			c.Source.Git.Auth.Basic.Username = expandEnv(c.Source.Git.Auth.Basic.Username)
			c.Source.Git.Auth.Basic.Password = expandEnv(c.Source.Git.Auth.Basic.Password)
		}
		if c.Source.Git.Auth.SSH != nil {
			c.Source.Git.Auth.SSH.KeyPath = expandEnv(c.Source.Git.Auth.SSH.KeyPath)
		}
	}
}

// expandEnv replaces a "${VAR}" or "$VAR" value with the environment value.
// Plain (non-reference) values are returned unchanged.
func expandEnv(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if strings.Contains(v, "$") {
		return os.ExpandEnv(v)
	}
	return v
}

func (c *Config) validate() error {
	if err := c.Source.validate(); err != nil {
		return err
	}
	if c.Source.Git != nil {
		if err := c.Source.Git.validate(); err != nil {
			return err
		}
	}
	if c.Secrets.Providers.Local != nil && strings.TrimSpace(c.Secrets.Providers.Local.File) == "" {
		return errors.New("secrets.providers.local.file is required when local secrets provider is configured")
	}
	if c.Exposure.Apache != nil && strings.TrimSpace(c.Exposure.Apache.SitesDir) == "" {
		return errors.New("exposure.apache.sitesDir is required when the apache exposure provider is configured")
	}
	return nil
}

func (s Source) validate() error {
	switch {
	case s.Local != nil && s.Git != nil:
		return errors.New("source must define exactly one of 'local' or 'git', not both")
	case s.Local == nil && s.Git == nil:
		return errors.New("source must define one of 'local' or 'git'")
	}

	if s.Local != nil && strings.TrimSpace(s.Local.Path) == "" {
		return errors.New("source.local.path is required")
	}
	return nil
}

func (g GitSource) validate() error {
	if strings.TrimSpace(g.URL) == "" {
		return errors.New("source.git.url is required")
	}
	return g.Auth.validate()
}

func (a GitAuth) validate() error {
	count := 0
	if a.Token != nil {
		count++
	}
	if a.Basic != nil {
		count++
	}
	if a.SSH != nil {
		count++
	}
	if count > 1 {
		return errors.New("source.git.auth must define at most one of 'token', 'basic', or 'ssh'")
	}

	switch {
	case a.Token != nil && strings.TrimSpace(a.Token.Value) == "":
		return errors.New("source.git.auth.token.value is required")
	case a.Basic != nil && (strings.TrimSpace(a.Basic.Username) == "" || strings.TrimSpace(a.Basic.Password) == ""):
		return errors.New("source.git.auth.basic requires username and password")
	case a.SSH != nil && strings.TrimSpace(a.SSH.KeyPath) == "":
		return errors.New("source.git.auth.ssh.keyPath is required")
	}
	return nil
}

// RepoRoot returns the manifest repository root for the current source.
// For git sources this is the checkout workdir.
func (c *Config) RepoRoot() string {
	if c.Source.Git != nil {
		return c.Source.Git.Workdir
	}
	return filepath.Clean(c.Source.Local.Path)
}

// LocalSecretsFile returns the configured local secrets store path, resolved
// relative to the config directory when it is not absolute.
func (c *Config) LocalSecretsFile() string {
	if c.Secrets.Providers.Local == nil {
		return ""
	}
	p := strings.TrimSpace(c.Secrets.Providers.Local.File)
	if p == "" {
		return ""
	}
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	if c.path == "" {
		return filepath.Clean(p)
	}
	return filepath.Clean(filepath.Join(filepath.Dir(c.path), p))
}

// SettingOrigin returns the provenance for an effective setting.
func (c *Config) SettingOrigin(path string) SettingOrigin {
	if c == nil {
		return SettingOrigin{}
	}
	if origin, ok := c.origins[path]; ok {
		return origin
	}
	return SettingOrigin{}
}

// EffectiveSettings returns a stable list of effective settings and where each
// value came from.
func (c *Config) EffectiveSettings() []EffectiveSetting {
	if c == nil {
		return nil
	}

	settings := []EffectiveSetting{
		{Path: "logging.dir", Value: c.Logging.Dir, Origin: c.SettingOrigin("logging.dir")},
		{Path: "logging.debug", Value: strconv.FormatBool(c.Logging.Debug), Origin: c.SettingOrigin("logging.debug")},
		{Path: "poll.interval", Value: c.Poll.Interval.String(), Origin: c.SettingOrigin("poll.interval")},
	}

	if c.Source.Local != nil {
		settings = append(settings,
			EffectiveSetting{Path: "source.type", Value: "local", Origin: SettingOrigin{Source: SettingSourceDerived, Reference: "source.local"}},
			EffectiveSetting{Path: "source.local.path", Value: c.Source.Local.Path, Origin: c.SettingOrigin("source.local.path")},
			EffectiveSetting{Path: "source.repoRoot", Value: c.RepoRoot(), Origin: SettingOrigin{Source: SettingSourceDerived, Reference: "source.local.path"}},
		)
	}

	if c.Source.Git != nil {
		settings = append(settings,
			EffectiveSetting{Path: "source.type", Value: "git", Origin: SettingOrigin{Source: SettingSourceDerived, Reference: "source.git"}},
			EffectiveSetting{Path: "source.git.url", Value: c.Source.Git.URL, Origin: c.SettingOrigin("source.git.url")},
			EffectiveSetting{Path: "source.git.branch", Value: c.Source.Git.Branch, Origin: c.SettingOrigin("source.git.branch")},
			EffectiveSetting{Path: "source.git.workdir", Value: c.Source.Git.Workdir, Origin: c.SettingOrigin("source.git.workdir")},
			EffectiveSetting{Path: "source.git.auth.method", Value: string(c.Source.Git.Auth.Method()), Origin: authMethodOrigin(c)},
			EffectiveSetting{Path: "source.repoRoot", Value: c.RepoRoot(), Origin: SettingOrigin{Source: SettingSourceDerived, Reference: "source.git.workdir"}},
		)

		switch c.Source.Git.Auth.Method() {
		case AuthToken:
			settings = append(settings, EffectiveSetting{Path: "source.git.auth.token.value", Value: redactValue(c.Source.Git.Auth.Token.Value), Origin: c.SettingOrigin("source.git.auth.token.value"), Sensitive: true})
		case AuthBasic:
			settings = append(settings,
				EffectiveSetting{Path: "source.git.auth.basic.username", Value: redactValue(c.Source.Git.Auth.Basic.Username), Origin: c.SettingOrigin("source.git.auth.basic.username"), Sensitive: true},
				EffectiveSetting{Path: "source.git.auth.basic.password", Value: redactValue(c.Source.Git.Auth.Basic.Password), Origin: c.SettingOrigin("source.git.auth.basic.password"), Sensitive: true},
			)
		case AuthSSH:
			settings = append(settings, EffectiveSetting{Path: "source.git.auth.ssh.keyPath", Value: c.Source.Git.Auth.SSH.KeyPath, Origin: c.SettingOrigin("source.git.auth.ssh.keyPath")})
		}
	}

	if c.Secrets.Providers.Local != nil {
		note := ""
		resolved := c.LocalSecretsFile()
		if raw := strings.TrimSpace(c.Secrets.Providers.Local.File); raw != "" && raw != resolved {
			note = fmt.Sprintf("resolved from %q relative to config file directory", raw)
		}
		settings = append(settings, EffectiveSetting{Path: "secrets.providers.local.file", Value: resolved, Origin: c.SettingOrigin("secrets.providers.local.file"), Note: note})
	}

	if c.Exposure.Apache != nil {
		settings = append(settings, EffectiveSetting{Path: "exposure.apache.sitesDir", Value: c.Exposure.Apache.SitesDir, Origin: c.SettingOrigin("exposure.apache.sitesDir")})
	}

	return settings
}

func (c *Config) initOrigins() {
	c.origins = map[string]SettingOrigin{
		"logging.dir":   {Source: SettingSourceDefault},
		"logging.debug": {Source: SettingSourceDefault},
		"poll.interval": {Source: SettingSourceDefault},
	}
}

func (c *Config) setOrigin(path string, origin SettingOrigin) {
	if c.origins == nil {
		c.initOrigins()
	}
	c.origins[path] = origin
}

func (c *Config) applyFileOrigins(path string, doc *yaml.Node) {
	for _, settingPath := range []string{
		"logging.dir",
		"logging.debug",
		"source.local.path",
		"source.git.url",
		"source.git.branch",
		"source.git.workdir",
		"source.git.auth.token.value",
		"source.git.auth.basic.username",
		"source.git.auth.basic.password",
		"source.git.auth.ssh.keyPath",
		"secrets.providers.local.file",
		"poll.interval",
		"exposure.apache.sitesDir",
	} {
		if yamlPathNode(doc, strings.Split(settingPath, ".")...) != nil {
			c.setOrigin(settingPath, configFileOrigin(path))
		}
	}
}

func yamlPathNode(root *yaml.Node, segments ...string) *yaml.Node {
	if root == nil {
		return nil
	}
	node := root
	if node.Kind == yaml.DocumentNode {
		if len(node.Content) == 0 {
			return nil
		}
		node = node.Content[0]
	}
	for _, segment := range segments {
		if node.Kind != yaml.MappingNode {
			return nil
		}
		found := false
		for i := 0; i+1 < len(node.Content); i += 2 {
			if node.Content[i].Value == segment {
				node = node.Content[i+1]
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	}
	return node
}

func configFileOrigin(path string) SettingOrigin {
	return SettingOrigin{Source: SettingSourceConfigFile, Reference: path}
}

func envOrigin(name string) SettingOrigin {
	return SettingOrigin{Source: SettingSourceEnvVar, Reference: name}
}

func lookupNonEmptyEnv(name string) (string, bool) {
	v, ok := os.LookupEnv(name)
	if !ok {
		return "", false
	}
	v = strings.TrimSpace(v)
	if v == "" {
		return "", false
	}
	return v, true
}

func authMethodOrigin(c *Config) SettingOrigin {
	if c == nil || c.Source.Git == nil {
		return SettingOrigin{Source: SettingSourceDerived, Reference: "source.git.auth"}
	}
	switch c.Source.Git.Auth.Method() {
	case AuthToken:
		return c.SettingOrigin("source.git.auth.token.value")
	case AuthBasic:
		return c.SettingOrigin("source.git.auth.basic.username")
	case AuthSSH:
		return c.SettingOrigin("source.git.auth.ssh.keyPath")
	default:
		return SettingOrigin{Source: SettingSourceDerived, Reference: "source.git.auth"}
	}
}

func redactValue(v string) string {
	if strings.TrimSpace(v) == "" {
		return ""
	}
	return "<redacted>"
}
