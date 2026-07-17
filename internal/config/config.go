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
	// ApacheSitesDirEnvVar overrides the apache sites-enabled directory.
	ApacheSitesDirEnvVar = "STEVEDORE_APACHE_SITES_DIR"

	// ConfigFileName is the fixed configuration filename within STEVEDORE_HOME.
	ConfigFileName = "config.yml"
	// DefaultHomeDir is the default stevedore home when STEVEDORE_HOME is unset.
	DefaultHomeDir = "/etc/stevedore"
	// DefaultWorkdir is where git repositories are cloned when not overridden.
	DefaultWorkdir = "/var/lib/stevedore/repo"
	// DefaultInterval is used when poll.interval is not set.
	DefaultInterval = 30 * time.Second
	// DefaultApacheSitesDir mirrors the exposure plugin default.
	DefaultApacheSitesDir = "/etc/apache2/sites-enabled"
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

// Config is the top-level agent configuration. It is loaded and validated once
// at process startup and then treated as immutable for the life of the process.
type Config struct {
	Logging        Logging `yaml:"logging"`
	Source         Source  `yaml:"source"`
	Secrets        Secrets `yaml:"secrets"`
	Poll           Poll    `yaml:"poll"`
	ApacheSitesDir string  `yaml:"apacheSitesDir"`
	path           string  `yaml:"-"`
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

// LocalSource points at a filesystem repository root (containing an apps/ dir).
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

	// Start with base config containing all defaults
	cfg := newBaseConfig()
	cfg.path = path

	// Unmarshal YAML on top of defaults (only set fields override)
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	// Apply git-specific defaults if git source is being used
	if cfg.Source.Git != nil {
		if strings.TrimSpace(cfg.Source.Git.Branch) == "" {
			cfg.Source.Git.Branch = "main"
		}
		if strings.TrimSpace(cfg.Source.Git.Workdir) == "" {
			cfg.Source.Git.Workdir = DefaultWorkdir
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
	return Config{
		Logging: Logging{
			Dir:   DefaultLogDir,
			Debug: false,
		},
		Poll: Poll{
			Interval: DefaultInterval,
		},
		ApacheSitesDir: DefaultApacheSitesDir,
		Source:         Source{}, // Will be populated from YAML; no defaults here to avoid conflicts
	}
}

func resolveConfigPath() string {
	home, _ := env.StringDefault(HomeEnvVar, DefaultHomeDir)
	return filepath.Join(filepath.Clean(home), ConfigFileName)
}

// applyEnvOverrides applies environment variable overrides on top of the config.
func (c *Config) applyEnvOverrides() {
	if logDir, _ := env.StringDefault(LogDirEnvVar, ""); logDir != "" {
		c.Logging.Dir = logDir
	}

	if debug, err := env.BoolDefault(DebugEnvVar, c.Logging.Debug); err == nil {
		c.Logging.Debug = debug
	}

	if interval, err := env.DurationDefault(IntervalEnvVar, 0); err == nil && interval > 0 {
		c.Poll.Interval = interval
	}

	if apacheSitesDir, _ := env.StringDefault(ApacheSitesDirEnvVar, ""); apacheSitesDir != "" {
		c.ApacheSitesDir = apacheSitesDir
	}

	if c.Source.Git != nil {
		if workdir, _ := env.StringDefault(WorkdirEnvVar, ""); workdir != "" {
			c.Source.Git.Workdir = workdir
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

// RepoRoot returns the directory that contains the apps/ folder for the current
// source. For git sources this is the checkout workdir.
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
