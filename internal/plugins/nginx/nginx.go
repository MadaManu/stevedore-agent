// Package example demonstrates how to write an external stevedore exposure plugin.
//
// To use this pattern:
//
//  1. Create a new Go module (separate repository), e.g. github.com/you/stevedore-nginx-plugin
//
//  2. Add stevedore-agent as a dependency:
//
//     go get stevedore-agent@latest
//
//  3. Implement plugin.ExposurePlugin (see NginxPlugin below).
//
//  4. Fork stevedore-agent (or maintain a thin wrapper main.go), and register
//     your plugin in buildReconciler:
//
//     pm := plugins.NewManager(
//     apache.New(...),
//     nginxplugin.New(...),  // <- your plugin
//     )
//
//  5. Rebuild and deploy your custom stevedore binary.
package example

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"stevedore-agent/pkg/plugin"
)

// NginxPlugin is an example external plugin that configures Nginx.
// It satisfies plugin.ExposurePlugin.
type NginxPlugin struct {
	SitesDir      string
	ReloadCommand string
}

func New(sitesDir string) *NginxPlugin {
	if sitesDir == "" {
		sitesDir = "/etc/nginx/sites-enabled"
	}
	return &NginxPlugin{SitesDir: sitesDir, ReloadCommand: "nginx -s reload"}
}

// Name returns "nginx" — this is what manifests must set in expose.provider.
func (p *NginxPlugin) Name() string { return "nginx" }

// Validate checks that required config fields are present.
func (p *NginxPlugin) Validate(config map[string]interface{}) error {
	domain, _ := config["domain"].(string)
	if strings.TrimSpace(domain) == "" {
		return fmt.Errorf("nginx plugin: expose.config.domain is required")
	}
	return nil
}

// Apply writes an Nginx server block and reloads Nginx.
func (p *NginxPlugin) Apply(app plugin.App) error {
	if err := os.MkdirAll(p.SitesDir, 0o755); err != nil {
		return err
	}

	domain, _ := app.ExposeConfig["domain"].(string)
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
func (p *NginxPlugin) Remove(app plugin.App) error {
	cfgPath := filepath.Join(p.SitesDir, app.Name+".conf")
	_ = os.Remove(cfgPath)
	return p.reload()
}

func (p *NginxPlugin) reload() error {
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
