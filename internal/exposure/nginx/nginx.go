// Package nginx is an example in-tree exposure provider implementation.
// It is intentionally not wired into the default build.
package nginx

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"stevedore-agent/internal/exposure"
)

// Provider is an example Nginx exposure provider.
type Provider struct {
	SitesDir      string
	ReloadCommand string
}

func New(sitesDir string) *Provider {
	if sitesDir == "" {
		sitesDir = "/etc/nginx/sites-enabled"
	}
	return &Provider{SitesDir: sitesDir, ReloadCommand: "nginx -s reload"}
}

// Name returns "nginx" — this is what manifests must set in expose.provider.
func (p *Provider) Name() string { return "nginx" }

// Validate checks that required config fields are present.
func (p *Provider) Validate(config map[string]interface{}) error {
	domain, _ := config["domain"].(string)
	if strings.TrimSpace(domain) == "" {
		return fmt.Errorf("nginx exposure config: expose.config.domain is required")
	}
	return nil
}

// Apply writes an Nginx server block and reloads Nginx.
func (p *Provider) Apply(app exposure.App) error {
	if err := os.MkdirAll(p.SitesDir, 0o755); err != nil {
		return err
	}

	domain, _ := app.Config["domain"].(string)
	port := 80
	if len(app.Ports) > 0 {
		port = app.Ports[0].ContainerPort
	}

	cfgPath := filepath.Join(p.SitesDir, app.Name+".conf")
	f, err := os.Create(cfgPath)
	if err != nil {
		return err
	}
	defer f.Close()

	tpl := `server {
    listen 80;
    server_name {{.Domain}};

    location / {
        proxy_pass http://{{.ContainerName}}:{{.Port}};
        proxy_set_header Host $host;
    }
}
`
	return template.Must(template.New("nginx").Parse(tpl)).Execute(f, map[string]interface{}{
		"Domain":        domain,
		"ContainerName": app.ContainerName,
		"Port":          port,
	})
}

// Remove deletes the Nginx config file and reloads Nginx.
func (p *Provider) Remove(app exposure.App) error {
	cfgPath := filepath.Join(p.SitesDir, app.Name+".conf")
	_ = os.Remove(cfgPath)
	return p.reload()
}

func (p *Provider) reload() error {
	parts := strings.Split(p.ReloadCommand, " ")
	if _, err := exec.LookPath(parts[0]); err != nil {
		return nil // nginx not installed on this machine, skip
	}
	cmd := exec.Command(parts[0], parts[1:]...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("reload nginx: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
