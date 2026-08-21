// Package apache implements the stevedore Apache exposure provider.
//
// The provider writes Apache VirtualHost configuration files to a
// sites-enabled directory and reloads Apache.  When SSL is requested it uses
// certbot (Let's Encrypt) to obtain/renew the TLS certificate before writing
// the HTTPS vhost.
//
// # Required prerequisites on the host
//
//   - Apache2 with mod_proxy, mod_proxy_http, mod_ssl, mod_rewrite, mod_headers
//     all enabled:
//     a2enmod proxy proxy_http ssl rewrite headers
//   - certbot installed and reachable on $PATH (only required when ssl: true)
//   - Port 80 accessible from the internet for the ACME HTTP-01 challenge
//     (only required when ssl: true)
//
// # stevedore.yml expose.config parameters
//
//	domain   (string, required)  – public hostname, e.g. "api.example.com"
//	ssl      (bool,   optional)  – enable HTTPS via Let's Encrypt; default false
//	email    (string, required when ssl: true) – contact e-mail for the Let's
//	                                Encrypt account / expiry notifications
//	webroot  (string, optional)  – directory used for ACME HTTP-01 challenges;
//	                                default /var/www/letsencrypt
//	port     (int,    optional)  – override which hostPort to proxy traffic to;
//	                                defaults to the first port listed under ports:
//	path     (string, optional)  – request path prefix to expose, e.g. "/hello";
//	                                default "/"
//	stripPathPrefix (bool, optional) – when path != "/", strip the prefix before
//	                                proxying to the backend; default true
package apache

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"stevedore-agent/internal/exposure"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

const (
	defaultSitesDir = "/etc/apache2/sites-enabled"
	defaultWebroot  = "/var/www/letsencrypt"
	certBasePath    = "/etc/letsencrypt/live"
)

// Provider is the Apache exposure provider.
type Provider struct {
	SitesDir      string
	ReloadCommand string
}

// New creates a Provider. sitesDir defaults to /etc/apache2/sites-enabled when
// empty.
func New(sitesDir string) *Provider {
	if sitesDir == "" {
		sitesDir = defaultSitesDir
	}
	return &Provider{SitesDir: sitesDir, ReloadCommand: "apachectl graceful"}
}

// Name returns "apache" — the value used in expose.provider.
func (p *Provider) Name() string { return "apache" }

// Validate checks that all required expose.config fields are present.
func (p *Provider) Validate(config map[string]interface{}) error {
	domain, _ := config["domain"].(string)
	if strings.TrimSpace(domain) == "" {
		return fmt.Errorf("apache expose config requires a non-empty 'domain'")
	}
	ssl, _ := config["ssl"].(bool)
	if ssl {
		email, _ := config["email"].(string)
		if strings.TrimSpace(email) == "" {
			return fmt.Errorf("apache expose config requires 'email' when ssl is true (used for Let's Encrypt account registration)")
		}
	}
	if _, err := resolvePathConfig(config); err != nil {
		return err
	}
	return nil
}

// vhostData is the template context passed to both vhost templates.
type vhostData struct {
	AppName             string
	Domain              string
	BackendPort         int
	Webroot             string
	CertDir             string
	ProxyPath           string
	ProxyTarget         string
	HasCustomPathPrefix bool
	PathPrefixRegex     string
	PathWithSlash       string
}

// Apply writes an Apache VirtualHost config for the app and reloads Apache.
//
// When ssl is true the flow is:
//  1. Write an HTTP-only vhost that serves the ACME challenge path from disk.
//  2. Reload Apache (so certbot can reach /.well-known/acme-challenge/).
//  3. Run certbot to obtain/renew the Let's Encrypt certificate.
//  4. Overwrite the vhost with a full HTTP-redirect + HTTPS vhost.
//  5. Reload Apache again.
//
// When ssl is false only a plain HTTP vhost is written and Apache is reloaded
// once.
func (p *Provider) Apply(app exposure.App) error {
	if err := os.MkdirAll(p.SitesDir, 0o755); err != nil {
		return fmt.Errorf("apache: create sites dir %s: %w", p.SitesDir, err)
	}

	domain, _ := app.Config["domain"].(string)
	ssl, _ := app.Config["ssl"].(bool)
	email, _ := app.Config["email"].(string)
	webroot, _ := app.Config["webroot"].(string)
	if webroot == "" {
		webroot = defaultWebroot
	}

	backendPort, err := resolveBackendPort(app)
	if err != nil {
		return err
	}
	pathCfg, err := resolvePathConfig(app.Config)
	if err != nil {
		return err
	}
	route := buildRouteConfig(pathCfg, backendPort)

	data := vhostData{
		AppName:             app.Name,
		Domain:              domain,
		BackendPort:         backendPort,
		Webroot:             webroot,
		CertDir:             filepath.Join(certBasePath, domain),
		ProxyPath:           route.proxyPath,
		ProxyTarget:         route.proxyTarget,
		HasCustomPathPrefix: route.hasCustomPathPrefix,
		PathPrefixRegex:     route.pathPrefixRegex,
		PathWithSlash:       route.pathWithSlash,
	}

	if ssl {
		// Step 1 — write HTTP-only config so certbot can complete the challenge.
		if err := p.writeVHost(app.Name, "vhost-http.conf.tmpl", data); err != nil {
			return err
		}
		// Step 2 — reload so Apache serves /.well-known/acme-challenge/.
		if err := p.reloadApache(); err != nil {
			return err
		}
		// Step 3 — obtain or renew the certificate.
		if err := p.ensureCertificate(domain, email, webroot); err != nil {
			return fmt.Errorf("apache: Let's Encrypt certificate for %s: %w", domain, err)
		}
		// Step 4 — replace with the full SSL config.
		if err := p.writeVHost(app.Name, "vhost-ssl.conf.tmpl", data); err != nil {
			return err
		}
	} else {
		if err := p.writeVHost(app.Name, "vhost-http.conf.tmpl", data); err != nil {
			return err
		}
	}

	// Final reload — activates the definitive vhost config.
	return p.reloadApache()
}

// Remove deletes the vhost config file for the app and reloads Apache.
func (p *Provider) Remove(app exposure.App) error {
	cfgPath := filepath.Join(p.SitesDir, app.Name+".conf")
	_ = os.Remove(cfgPath)
	return p.reloadApacheIfAvailable()
}

// writeVHost renders the named template and writes it to sites-enabled.
func (p *Provider) writeVHost(appName, tmplName string, data vhostData) error {
	tmpl, err := template.ParseFS(templateFS, "templates/"+tmplName)
	if err != nil {
		return fmt.Errorf("apache: parse template %s: %w", tmplName, err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("apache: render template %s: %w", tmplName, err)
	}
	cfgPath := filepath.Join(p.SitesDir, appName+".conf")
	if err := os.WriteFile(cfgPath, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("apache: write config %s: %w", cfgPath, err)
	}
	return nil
}

// ensureCertificate obtains a new certificate or attempts renewal if one
// already exists.
func (p *Provider) ensureCertificate(domain, email, webroot string) error {
	certFile := filepath.Join(certBasePath, domain, "fullchain.pem")
	if _, err := os.Stat(certFile); err == nil {
		// Certificate already exists — attempt a quiet renewal.
		return p.runCertbot("renew",
			"--cert-name", domain,
			"--non-interactive",
			"--quiet",
		)
	}

	// Ensure the webroot challenge directory exists.
	challengeDir := filepath.Join(webroot, ".well-known", "acme-challenge")
	if err := os.MkdirAll(challengeDir, 0o755); err != nil {
		return fmt.Errorf("create acme webroot %s: %w", challengeDir, err)
	}

	return p.runCertbot("certonly",
		"--webroot",
		"-w", webroot,
		"-d", domain,
		"--email", email,
		"--agree-tos",
		"--non-interactive",
		"--keep-until-expiring",
	)
}

// runCertbot executes certbot with the supplied arguments and returns a
// descriptive error (including certbot's combined output) on failure.
func (p *Provider) runCertbot(args ...string) error {
	cmd := exec.Command("certbot", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("certbot %s: %w: %s", args[0], err, strings.TrimSpace(string(out)))
	}
	return nil
}

// reloadApache unconditionally reloads Apache.  Returns an error if Apache is
// unavailable or the reload fails.
func (p *Provider) reloadApache() error {
	parts := strings.Fields(p.ReloadCommand)
	if len(parts) == 0 {
		return nil
	}
	cmd := exec.Command(parts[0], parts[1:]...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("apache reload (%s): %w: %s", p.ReloadCommand, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// reloadApacheIfAvailable reloads Apache only when the reload binary is on
// $PATH.  Used during Remove so that a missing Apache installation does not
// cause an error.
func (p *Provider) reloadApacheIfAvailable() error {
	parts := strings.Fields(p.ReloadCommand)
	if len(parts) == 0 {
		return nil
	}
	if _, err := exec.LookPath(parts[0]); err != nil {
		return nil // Apache not installed on this host — nothing to reload.
	}
	return p.reloadApache()
}

// resolveBackendPort returns the host port the provider should proxy traffic to.
// It honours the optional expose.config.port override, otherwise uses the
// first port declared in the app manifest.
func resolveBackendPort(app exposure.App) (int, error) {
	// Honour explicit override in expose.config.
	if override, ok := app.Config["port"]; ok {
		switch v := override.(type) {
		case int:
			if v > 0 {
				return v, nil
			}
		case float64: // YAML numbers unmarshal as float64 via interface{}
			if int(v) > 0 {
				return int(v), nil
			}
		}
	}
	// Fall back to the first declared host port.
	for _, port := range app.Ports {
		if port.HostPort > 0 {
			return port.HostPort, nil
		}
	}
	return 0, fmt.Errorf("apache expose: app %q has no host port; declare at least one port or set expose.config.port", app.Name)
}

type pathConfig struct {
	pathPrefix      string
	stripPathPrefix bool
}

type routeConfig struct {
	proxyPath           string
	proxyTarget         string
	hasCustomPathPrefix bool
	pathPrefixRegex     string
	pathWithSlash       string
}

func resolvePathConfig(config map[string]interface{}) (pathConfig, error) {
	pathPrefix := "/"
	if raw, ok := config["path"]; ok {
		pathValue, ok := raw.(string)
		if !ok {
			return pathConfig{}, fmt.Errorf("apache expose config field 'path' must be a string")
		}
		normalized, err := normalizePathPrefix(pathValue)
		if err != nil {
			return pathConfig{}, err
		}
		pathPrefix = normalized
	}

	stripPathPrefix := true
	if raw, ok := config["stripPathPrefix"]; ok {
		boolValue, ok := raw.(bool)
		if !ok {
			return pathConfig{}, fmt.Errorf("apache expose config field 'stripPathPrefix' must be a boolean")
		}
		stripPathPrefix = boolValue
	}

	return pathConfig{
		pathPrefix:      pathPrefix,
		stripPathPrefix: stripPathPrefix,
	}, nil
}

func normalizePathPrefix(v string) (string, error) {
	p := strings.TrimSpace(v)
	if p == "" {
		return "/", nil
	}
	if !strings.HasPrefix(p, "/") {
		return "", fmt.Errorf("apache expose config field 'path' must start with '/'")
	}
	if strings.ContainsAny(p, "?#") {
		return "", fmt.Errorf("apache expose config field 'path' must not include query strings or fragments")
	}
	cleaned := path.Clean(p)
	if cleaned == "." {
		cleaned = "/"
	}
	if !strings.HasPrefix(cleaned, "/") {
		cleaned = "/" + cleaned
	}
	if strings.ContainsAny(cleaned, " \t\r\n") {
		return "", fmt.Errorf("apache expose config field 'path' must not contain whitespace")
	}
	return cleaned, nil
}

func buildRouteConfig(pathCfg pathConfig, backendPort int) routeConfig {
	defaultTarget := fmt.Sprintf("http://127.0.0.1:%d/", backendPort)
	if pathCfg.pathPrefix == "/" {
		return routeConfig{
			proxyPath:   "/",
			proxyTarget: defaultTarget,
		}
	}

	proxyPath := pathCfg.pathPrefix + "/"
	proxyTarget := defaultTarget
	if !pathCfg.stripPathPrefix {
		proxyTarget = fmt.Sprintf("http://127.0.0.1:%d%s/", backendPort, pathCfg.pathPrefix)
	}

	return routeConfig{
		proxyPath:           proxyPath,
		proxyTarget:         proxyTarget,
		hasCustomPathPrefix: true,
		pathPrefixRegex:     regexp.QuoteMeta(pathCfg.pathPrefix),
		pathWithSlash:       proxyPath,
	}
}
