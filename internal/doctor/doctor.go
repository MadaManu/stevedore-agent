package doctor

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"stevedore-agent/internal/config"
	"stevedore-agent/internal/secrets"
)

type CheckStatus string

const (
	StatusPass CheckStatus = "PASS"
	StatusWarn CheckStatus = "WARN"
	StatusFail CheckStatus = "FAIL"
)

const (
	accessExecute = 1
	accessWrite   = 2
	accessRead    = 4
)

type EnvOverrideState string

const (
	EnvOverrideNotSet  EnvOverrideState = "not-set"
	EnvOverrideActive  EnvOverrideState = "active"
	EnvOverrideIgnored EnvOverrideState = "ignored"
	EnvOverrideInvalid EnvOverrideState = "invalid"
)

type EnvOverride struct {
	Name    string
	Setting string
	Raw     string
	State   EnvOverrideState
	Detail  string
}

type CheckResult struct {
	Name        string
	Status      CheckStatus
	Summary     string
	Tried       []string
	Detail      string
	Suggestions []string
}

type Report struct {
	ConfigPath       string
	ConfigPathOrigin config.SettingOrigin
	Settings         []config.EffectiveSetting
	EnvOverrides     []EnvOverride
	Checks           []CheckResult
}

type systemDeps struct {
	readFile func(string) ([]byte, error)
	readDir  func(string) ([]fs.DirEntry, error)
	stat     func(string) (fs.FileInfo, error)
	glob     func(string) ([]string, error)
	lookPath func(string) (string, error)
	runCmd   func(string, ...string) ([]byte, error)
	access   func(string, uint32) error
}

func defaultSystemDeps() systemDeps {
	return systemDeps{
		readFile: os.ReadFile,
		readDir:  os.ReadDir,
		stat:     os.Stat,
		glob:     filepath.Glob,
		lookPath: exec.LookPath,
		runCmd:   runSystemCommand,
		access:   syscall.Access,
	}
}

func Run() Report {
	return runWithDeps(defaultSystemDeps())
}

func runWithDeps(deps systemDeps) Report {
	report := Report{
		ConfigPath:       config.ConfigPath(),
		ConfigPathOrigin: configPathOrigin(),
	}

	report.EnvOverrides = inspectEnvOverrides(nil)
	for _, envOverride := range report.EnvOverrides {
		if envOverride.State == EnvOverrideInvalid {
			report.Checks = append(report.Checks, CheckResult{
				Name:    "environment override " + envOverride.Name,
				Status:  StatusWarn,
				Summary: fmt.Sprintf("%s is set but invalid and will be ignored", envOverride.Name),
				Tried:   []string{fmt.Sprintf("%s=%q", envOverride.Name, envOverride.Raw)},
				Detail:  envOverride.Detail,
				Suggestions: []string{
					fixEnvOverrideSuggestion(envOverride.Name, envOverride.Setting),
				},
			})
		}
	}

	if _, err := deps.readFile(report.ConfigPath); err != nil {
		report.Checks = append(report.Checks, CheckResult{
			Name:    "config file",
			Status:  StatusFail,
			Summary: "configuration file is not readable",
			Tried:   []string{report.ConfigPath},
			Detail:  err.Error(),
			Suggestions: []string{
				fmt.Sprintf("Create %s or point %s at another stevedore home directory", report.ConfigPath, config.HomeEnvVar),
				fmt.Sprintf("If %s is set, verify it points at the directory containing %s", config.HomeEnvVar, config.ConfigFileName),
			},
		})
		return report
	}
	report.Checks = append(report.Checks, CheckResult{
		Name:    "config file",
		Status:  StatusPass,
		Summary: "configuration file is readable",
		Tried:   []string{report.ConfigPath},
	})

	cfg, err := config.LoadFromPath(report.ConfigPath)
	if err != nil {
		report.Checks = append(report.Checks, CheckResult{
			Name:    "config validation",
			Status:  StatusFail,
			Summary: "configuration failed to load or validate",
			Tried:   []string{report.ConfigPath},
			Detail:  err.Error(),
			Suggestions: []string{
				fmt.Sprintf("Fix the configuration in %s and run `stevedore-agent doctor` again", report.ConfigPath),
				"Compare your config against examples/config/local.yml or examples/config/git.yml",
			},
		})
		return report
	}
	report.Settings = cfg.EffectiveSettings()
	report.EnvOverrides = inspectEnvOverrides(cfg)
	report.Checks = append(report.Checks, CheckResult{
		Name:    "config validation",
		Status:  StatusPass,
		Summary: "configuration loads and validates successfully",
		Tried:   []string{report.ConfigPath},
	})

	for _, envOverride := range report.EnvOverrides {
		if envOverride.State == EnvOverrideInvalid {
			continue
		}
		if envOverride.State == EnvOverrideIgnored {
			report.Checks = append(report.Checks, CheckResult{
				Name:    "environment override " + envOverride.Name,
				Status:  StatusWarn,
				Summary: fmt.Sprintf("%s is set but not active for this configuration", envOverride.Name),
				Tried:   []string{fmt.Sprintf("%s=%q", envOverride.Name, envOverride.Raw)},
				Detail:  envOverride.Detail,
				Suggestions: []string{
					fixEnvOverrideSuggestion(envOverride.Name, envOverride.Setting),
				},
			})
		}
	}

	dockerCheck := checkBinary("docker CLI", "docker", deps)
	report.Checks = append(report.Checks, dockerCheck)
	if dockerCheck.Status == StatusPass {
		dockerPath := "docker"
		if len(dockerCheck.Tried) > 0 && strings.TrimSpace(dockerCheck.Tried[0]) != "" {
			dockerPath = dockerCheck.Tried[0]
		}
		report.Checks = append(report.Checks, checkDockerDaemon(dockerPath, deps))
	}
	report.Checks = append(report.Checks, checkWritableDirectory("logging directory", cfg.Logging.Dir, deps,
		fmt.Sprintf("Create %s or make it writable for the stevedore process", cfg.Logging.Dir),
	))

	if cfg.Exposure.Apache != nil {
		report.Checks = append(report.Checks, checkWritableDirectory("apache sites directory", cfg.Exposure.Apache.SitesDir, deps,
			fmt.Sprintf("Create %s or update exposure.apache.sitesDir / %s", cfg.Exposure.Apache.SitesDir, config.ApacheSitesDirEnvVar),
		))
	}

	if cfg.Source.IsGit() {
		report.Checks = append(report.Checks, checkBinary("git CLI", "git", deps))
		report.Checks = append(report.Checks, checkWritableDirectory("git workdir", cfg.Source.Git.Workdir, deps,
			fmt.Sprintf("Create %s or override it with %s", cfg.Source.Git.Workdir, config.WorkdirEnvVar),
		))
		switch cfg.Source.Git.Auth.Method() {
		case config.AuthSSH:
			report.Checks = append(report.Checks, checkReadableFile("git SSH key", cfg.Source.Git.Auth.SSH.KeyPath, deps,
				"Ensure source.git.auth.ssh.keyPath points to a readable private key file",
			))
		case config.AuthToken, config.AuthBasic:
			report.Checks = append(report.Checks, CheckResult{
				Name:    "git credentials",
				Status:  StatusPass,
				Summary: fmt.Sprintf("git auth method %q is configured", cfg.Source.Git.Auth.Method()),
				Tried:   []string{"source.git.auth"},
				Detail:  "Remote connectivity is not tested by doctor; only local configuration is validated.",
			})
		default:
			report.Checks = append(report.Checks, CheckResult{
				Name:    "git credentials",
				Status:  StatusPass,
				Summary: "git source uses unauthenticated access",
				Tried:   []string{"source.git.auth"},
			})
		}
	} else {
		report.Checks = append(report.Checks, checkReadableDirectory("local source repository", cfg.Source.Local.Path, deps,
			fmt.Sprintf("Ensure source.local.path points at the directory containing apps/ manifests, or update %s", report.ConfigPath),
		))
		report.Checks = append(report.Checks, checkReadableDirectory("apps directory", filepath.Join(cfg.RepoRoot(), "apps"), deps,
			"Create an apps/ directory in the configured local source repository",
		))
		manifestPaths, globErr := deps.glob(filepath.Join(cfg.RepoRoot(), "apps", "*", "stevedore.yml"))
		if globErr != nil {
			report.Checks = append(report.Checks, CheckResult{
				Name:    "manifest discovery",
				Status:  StatusFail,
				Summary: "failed to enumerate app manifests",
				Tried:   []string{filepath.Join(cfg.RepoRoot(), "apps", "*", "stevedore.yml")},
				Detail:  globErr.Error(),
				Suggestions: []string{
					"Verify the local source path is a normal filesystem path and that app manifests are stored as apps/<name>/stevedore.yml",
				},
			})
		} else if len(manifestPaths) == 0 {
			report.Checks = append(report.Checks, CheckResult{
				Name:    "manifest discovery",
				Status:  StatusWarn,
				Summary: "no app manifests were found",
				Tried:   []string{filepath.Join(cfg.RepoRoot(), "apps", "*", "stevedore.yml")},
				Suggestions: []string{
					"Add manifests at apps/<app>/stevedore.yml so the agent has desired state to reconcile",
				},
			})
		} else {
			sort.Strings(manifestPaths)
			report.Checks = append(report.Checks, CheckResult{
				Name:    "manifest discovery",
				Status:  StatusPass,
				Summary: fmt.Sprintf("found %d app manifest(s)", len(manifestPaths)),
				Tried:   []string{filepath.Join(cfg.RepoRoot(), "apps", "*", "stevedore.yml")},
				Detail:  strings.Join(manifestPaths, "\n"),
			})
		}
	}

	if cfg.Secrets.Providers.Local != nil {
		raw := strings.TrimSpace(cfg.Secrets.Providers.Local.File)
		resolved := cfg.LocalSecretsFile()
		check := checkReadableFile("local secrets store", resolved, deps,
			"Create the file, fix the relative path, or update secrets.providers.local.file",
		)
		if raw != "" && raw != resolved {
			check.Tried = append(check.Tried, fmt.Sprintf("raw config value: %s", raw))
		}
		if check.Status != StatusFail {
			if _, err := secrets.LoadLocalStore(resolved); err != nil {
				check.Status = StatusFail
				check.Summary = "local secrets store exists but could not be parsed"
				check.Detail = err.Error()
				check.Suggestions = []string{
					"Ensure the local secrets store is valid YAML or JSON",
					"Verify secret paths are stored as scalars, maps, and lists in a valid structure",
				}
			} else {
				check.Status = StatusPass
				check.Summary = "local secrets store is readable and parses successfully"
			}
		}
		report.Checks = append(report.Checks, check)
	}

	return report
}

func (r Report) FailureCount() int {
	count := 0
	for _, check := range r.Checks {
		if check.Status == StatusFail {
			count++
		}
	}
	return count
}

func (r Report) WarningCount() int {
	count := 0
	for _, check := range r.Checks {
		if check.Status == StatusWarn {
			count++
		}
	}
	return count
}

func (r Report) PassCount() int {
	count := 0
	for _, check := range r.Checks {
		if check.Status == StatusPass {
			count++
		}
	}
	return count
}

func RenderText(report Report) string {
	var b strings.Builder
	b.WriteString("Stevedore doctor\n")
	b.WriteString("================\n\n")

	b.WriteString("Config lookup\n")
	b.WriteString("-------------\n")
	fmt.Fprintf(&b, "config.path = %s (%s)\n", report.ConfigPath, report.ConfigPathOrigin.Describe())
	b.WriteString("\n")

	if len(report.Settings) > 0 {
		b.WriteString("Effective settings\n")
		b.WriteString("------------------\n")
		for _, setting := range report.Settings {
			fmt.Fprintf(&b, "%s = %s (%s)\n", setting.Path, setting.Value, setting.Origin.Describe())
			if setting.Note != "" {
				fmt.Fprintf(&b, "  note: %s\n", setting.Note)
			}
		}
		b.WriteString("\n")
	}

	b.WriteString("Environment overrides\n")
	b.WriteString("---------------------\n")
	for _, envOverride := range report.EnvOverrides {
		line := fmt.Sprintf("%s [%s]", envOverride.Name, envOverride.State)
		if envOverride.Raw != "" {
			line += fmt.Sprintf(" = %q", envOverride.Raw)
		}
		if envOverride.Setting != "" {
			line += fmt.Sprintf(" -> %s", envOverride.Setting)
		}
		fmt.Fprintln(&b, line)
		if envOverride.Detail != "" {
			fmt.Fprintf(&b, "  %s\n", envOverride.Detail)
		}
	}
	b.WriteString("\n")

	b.WriteString("Checks\n")
	b.WriteString("------\n")
	for _, check := range report.Checks {
		fmt.Fprintf(&b, "[%s] %s — %s\n", check.Status, check.Name, check.Summary)
		for _, tried := range check.Tried {
			fmt.Fprintf(&b, "  tried: %s\n", tried)
		}
		if check.Detail != "" {
			for _, line := range strings.Split(check.Detail, "\n") {
				if strings.TrimSpace(line) == "" {
					continue
				}
				fmt.Fprintf(&b, "  detail: %s\n", line)
			}
		}
		for _, suggestion := range check.Suggestions {
			fmt.Fprintf(&b, "  fix: %s\n", suggestion)
		}
	}
	b.WriteString("\n")

	fmt.Fprintf(&b, "Summary: %d passed, %d warning(s), %d failed\n", report.PassCount(), report.WarningCount(), report.FailureCount())
	return b.String()
}

func configPathOrigin() config.SettingOrigin {
	if home, ok := os.LookupEnv(config.HomeEnvVar); ok && strings.TrimSpace(home) != "" {
		return config.SettingOrigin{Source: config.SettingSourceEnvVar, Reference: config.HomeEnvVar}
	}
	return config.SettingOrigin{Source: config.SettingSourceDefault, Reference: config.DefaultHomeDir}
}

func inspectEnvOverrides(cfg *config.Config) []EnvOverride {
	overrides := []EnvOverride{
		inspectStringEnv(config.HomeEnvVar, "config.path", true),
		inspectStringEnv(config.LogDirEnvVar, "logging.dir", true),
		inspectBoolEnv(config.DebugEnvVar, "logging.debug"),
		inspectDurationEnv(config.IntervalEnvVar, "poll.interval"),
		inspectStringEnv(config.ApacheSitesDirEnvVar, "exposure.apache.sitesDir", true),
	}

	workdir := inspectStringEnv(config.WorkdirEnvVar, "source.git.workdir", true)
	if workdir.State == EnvOverrideActive && (cfg == nil || !cfg.Source.IsGit()) {
		workdir.State = EnvOverrideIgnored
		workdir.Detail = "This override only applies when source.git is configured."
	}
	overrides = append(overrides, workdir)
	return overrides
}

func inspectStringEnv(name, setting string, trim bool) EnvOverride {
	raw, ok := os.LookupEnv(name)
	if !ok {
		return EnvOverride{Name: name, Setting: setting, State: EnvOverrideNotSet}
	}
	value := raw
	if trim {
		value = strings.TrimSpace(value)
	}
	if value == "" {
		return EnvOverride{Name: name, Setting: setting, Raw: raw, State: EnvOverrideInvalid, Detail: "The value is empty, so the override is ignored."}
	}
	return EnvOverride{Name: name, Setting: setting, Raw: raw, State: EnvOverrideActive}
}

func inspectBoolEnv(name, setting string) EnvOverride {
	raw, ok := os.LookupEnv(name)
	if !ok {
		return EnvOverride{Name: name, Setting: setting, State: EnvOverrideNotSet}
	}
	if _, err := strconvParseBool(raw); err != nil {
		return EnvOverride{Name: name, Setting: setting, Raw: raw, State: EnvOverrideInvalid, Detail: fmt.Sprintf("Expected a boolean value such as true or false: %v", err)}
	}
	return EnvOverride{Name: name, Setting: setting, Raw: raw, State: EnvOverrideActive}
}

func inspectDurationEnv(name, setting string) EnvOverride {
	raw, ok := os.LookupEnv(name)
	if !ok {
		return EnvOverride{Name: name, Setting: setting, State: EnvOverrideNotSet}
	}
	d, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil || d <= 0 {
		detail := "Expected a positive Go duration such as 30s or 5m."
		if err != nil {
			detail = fmt.Sprintf("Expected a positive Go duration such as 30s or 5m: %v", err)
		}
		if err == nil && d <= 0 {
			detail = "Expected a positive Go duration such as 30s or 5m; zero or negative values are ignored."
		}
		return EnvOverride{Name: name, Setting: setting, Raw: raw, State: EnvOverrideInvalid, Detail: detail}
	}
	return EnvOverride{Name: name, Setting: setting, Raw: raw, State: EnvOverrideActive}
}

func checkBinary(label, binary string, deps systemDeps) CheckResult {
	path, err := deps.lookPath(binary)
	if err != nil {
		return CheckResult{
			Name:    label,
			Status:  StatusFail,
			Summary: fmt.Sprintf("%s was not found in PATH", binary),
			Tried:   []string{binary},
			Detail:  err.Error(),
			Suggestions: []string{
				fmt.Sprintf("Install %s and ensure it is available on PATH", binary),
			},
		}
	}
	return CheckResult{Name: label, Status: StatusPass, Summary: fmt.Sprintf("%s is available", binary), Tried: []string{path}}
}

func checkDockerDaemon(dockerBinary string, deps systemDeps) CheckResult {
	cmdDisplay := fmt.Sprintf("%s info", dockerBinary)
	out, err := deps.runCmd(dockerBinary, "info")
	if err != nil {
		return CheckResult{
			Name:    "docker daemon",
			Status:  StatusFail,
			Summary: "docker CLI is present but daemon is not reachable",
			Tried:   []string{cmdDisplay},
			Detail:  strings.TrimSpace(string(out)),
			Suggestions: []string{
				"Start Docker (for example Docker Desktop) and retry",
				"Verify the current user can access the Docker socket",
			},
		}
	}
	return CheckResult{
		Name:    "docker daemon",
		Status:  StatusPass,
		Summary: "docker daemon is reachable",
		Tried:   []string{cmdDisplay},
	}
}

func checkReadableDirectory(name, path string, deps systemDeps, suggestion string) CheckResult {
	path = filepath.Clean(path)
	info, err := deps.stat(path)
	if err != nil {
		return CheckResult{Name: name, Status: StatusFail, Summary: "directory could not be accessed", Tried: []string{path}, Detail: err.Error(), Suggestions: []string{suggestion}}
	}
	if !info.IsDir() {
		return CheckResult{Name: name, Status: StatusFail, Summary: "path exists but is not a directory", Tried: []string{path}, Suggestions: []string{suggestion}}
	}
	if err := deps.access(path, accessRead|accessExecute); err != nil {
		return CheckResult{Name: name, Status: StatusFail, Summary: "directory is not readable", Tried: []string{path}, Detail: err.Error(), Suggestions: []string{suggestion}}
	}
	return CheckResult{Name: name, Status: StatusPass, Summary: "directory exists and is readable", Tried: []string{path}}
}

func checkWritableDirectory(name, path string, deps systemDeps, suggestion string) CheckResult {
	path = filepath.Clean(path)
	info, err := deps.stat(path)
	if err == nil {
		if !info.IsDir() {
			return CheckResult{Name: name, Status: StatusFail, Summary: "path exists but is not a directory", Tried: []string{path}, Suggestions: []string{suggestion}}
		}
		if accessErr := deps.access(path, accessWrite|accessExecute); accessErr != nil {
			return CheckResult{Name: name, Status: StatusFail, Summary: "directory exists but is not writable", Tried: []string{path}, Detail: accessErr.Error(), Suggestions: []string{suggestion}}
		}
		return CheckResult{Name: name, Status: StatusPass, Summary: "directory exists and is writable", Tried: []string{path}}
	}
	if !os.IsNotExist(err) {
		return CheckResult{Name: name, Status: StatusFail, Summary: "directory could not be accessed", Tried: []string{path}, Detail: err.Error(), Suggestions: []string{suggestion}}
	}

	parent, parentErr := nearestExistingParent(path, deps)
	if parentErr != nil {
		return CheckResult{Name: name, Status: StatusFail, Summary: "directory does not exist and no writable parent could be verified", Tried: []string{path}, Detail: parentErr.Error(), Suggestions: []string{suggestion}}
	}
	if accessErr := deps.access(parent, accessWrite|accessExecute); accessErr != nil {
		return CheckResult{Name: name, Status: StatusFail, Summary: "directory does not exist and parent is not writable", Tried: []string{path, parent}, Detail: accessErr.Error(), Suggestions: []string{suggestion}}
	}
	return CheckResult{Name: name, Status: StatusWarn, Summary: "directory does not exist yet, but its parent is writable", Tried: []string{path, parent}, Suggestions: []string{suggestion}}
}

func checkReadableFile(name, path string, deps systemDeps, suggestion string) CheckResult {
	path = filepath.Clean(path)
	info, err := deps.stat(path)
	if err != nil {
		return CheckResult{Name: name, Status: StatusFail, Summary: "file could not be accessed", Tried: []string{path}, Detail: err.Error(), Suggestions: []string{suggestion}}
	}
	if info.IsDir() {
		return CheckResult{Name: name, Status: StatusFail, Summary: "path exists but is a directory", Tried: []string{path}, Suggestions: []string{suggestion}}
	}
	if err := deps.access(path, accessRead); err != nil {
		return CheckResult{Name: name, Status: StatusFail, Summary: "file exists but is not readable", Tried: []string{path}, Detail: err.Error(), Suggestions: []string{suggestion}}
	}
	return CheckResult{Name: name, Status: StatusPass, Summary: "file exists and is readable", Tried: []string{path}}
}

func nearestExistingParent(path string, deps systemDeps) (string, error) {
	current := filepath.Clean(path)
	for {
		current = filepath.Dir(current)
		if current == "." || current == string(filepath.Separator) || current == "" {
			break
		}
		info, err := deps.stat(current)
		if err == nil {
			if !info.IsDir() {
				return "", fmt.Errorf("%s exists but is not a directory", current)
			}
			return current, nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
	}
	if _, err := deps.stat(string(filepath.Separator)); err == nil {
		return string(filepath.Separator), nil
	}
	return "", fmt.Errorf("no existing parent directory found for %s", path)
}

func fixEnvOverrideSuggestion(name, setting string) string {
	if setting == "" {
		return fmt.Sprintf("Unset %s if it should not apply", name)
	}
	return fmt.Sprintf("Fix %s or unset it so %s can come from the config file/defaults", name, setting)
}

func strconvParseBool(v string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "t", "true", "y", "yes", "on":
		return true, nil
	case "0", "f", "false", "n", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean %q", v)
	}
}

func runSystemCommand(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	return cmd.CombinedOutput()
}
