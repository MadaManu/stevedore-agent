package apache

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	//"text/template"

	"stevedore-agent/pkg/plugin"
)

type Plugin struct {
	SitesDir      string
	ReloadCommand string
}

func New(sitesDir string) *Plugin {
	if sitesDir == "" {
		sitesDir = "/etc/apache2/sites-enabled"
	}
	return &Plugin{SitesDir: sitesDir, ReloadCommand: "apachectl graceful"}
}

func (p *Plugin) Name() string { return "apache" }

func (p *Plugin) Validate(config map[string]interface{}) error {
	domain, _ := config["domain"].(string)
	if strings.TrimSpace(domain) == "" {
		return fmt.Errorf("apache expose config requires non-empty domain")
	}
	return nil
}

func (p *Plugin) Apply(app plugin.App) error {
	//if err := os.MkdirAll(p.SitesDir, 0o755); err != nil {
	//	return err
	//}

	//domain, _ := app.ExposeConfig["domain"].(string)
	//ssl, _ := app.ExposeConfig["ssl"].(bool)
	//port := 80
	//if len(app.Ports) > 0 {
	//	port = app.Ports[0].ContainerPort
	//}

	//	cfgPath := filepath.Join(p.SitesDir, app.Name+".conf")
	//	f, err := os.Create(cfgPath)
	//	if err != nil {
	//		return err
	//	}
	//	defer f.Close()
	//
	//	tpl := `<VirtualHost *:{{if .SSL}}443{{else}}80{{end}}>
	//    ServerName {{.Domain}}
	//    ProxyPreserveHost On
	//    ProxyPass / http://{{.ContainerName}}:{{.Port}}/
	//    ProxyPassReverse / http://{{.ContainerName}}:{{.Port}}/
	//</VirtualHost>
	//`
	//	if err := template.Must(template.New("apache").Parse(tpl)).Execute(f, map[string]interface{}{
	//		"Domain":        domain,
	//		"SSL":           ssl,
	//		"ContainerName": app.ContainerName,
	//		"Port":          port,
	//	}); err != nil {
	//		return err
	//	}

	return nil
}

func (p *Plugin) Remove(app plugin.App) error {
	cfgPath := filepath.Join(p.SitesDir, app.Name+".conf")
	_ = os.Remove(cfgPath)
	return p.reloadApacheIfAvailable()
}

func (p *Plugin) reloadApacheIfAvailable() error {
	parts := strings.Split(p.ReloadCommand, " ")
	if len(parts) == 0 {
		return nil
	}
	if _, err := exec.LookPath(parts[0]); err != nil {
		return nil
	}
	cmd := exec.Command(parts[0], parts[1:]...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("reload apache: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
