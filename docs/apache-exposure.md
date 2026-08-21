# Apache exposure provider

The bundled `apache` exposure provider makes stevedore-agent manage Apache VirtualHost
configuration files for your applications. On each reconcile cycle it writes a
`.conf` file to the configured `sites-enabled` directory, handles Let's Encrypt
certificate issuance/renewal when SSL is requested, and reloads Apache.

Apache proxying is based on the app's host port, so exposure behavior is the
same whether an app is attached to one or many Docker networks.
Exposure behavior is also independent of the number of bind mounts configured.

---

## Host prerequisites

Install and enable the required Apache modules once:

```bash
# Debian / Ubuntu
sudo apt-get install -y apache2 certbot python3-certbot-apache

sudo a2enmod proxy proxy_http ssl rewrite headers
sudo systemctl reload apache2
```

> `certbot` is only required when at least one app sets `ssl: true`.

Port **80** must be reachable from the internet for the Let's Encrypt HTTP-01
challenge. Port **443** must also be open for HTTPS traffic.

---

## stevedore-agent config.yml

```yaml
exposure:
  apache:
    sitesDir: /etc/apache2/sites-enabled   # required when any app uses expose.provider: apache
```

`sitesDir` can also be set via the `STEVEDORE_APACHE_SITES_DIR` environment
variable (activates the provider even if the `exposure.apache` block is absent
from `config.yml`).

---

## stevedore.yml — expose block

All `expose.config` fields understood by the Apache provider:

| Field             | Type   | Required          | Default                  | Description                                                                                                                           |
|-------------------|--------|-------------------|--------------------------|---------------------------------------------------------------------------------------------------------------------------------------|
| `domain`          | string | ✅                 | —                        | Public hostname Apache will serve, e.g. `api.example.com`. Must resolve to this host's IP address.                                    |
| `ssl`             | bool   | ❌                 | `false`                  | Enable HTTPS via Let's Encrypt. When `true` certbot obtains/renews a certificate automatically.                                       |
| `email`           | string | ✅ when `ssl:true` | —                        | E-mail address for the Let's Encrypt account and expiry notifications.                                                                |
| `webroot`         | string | ❌                 | `/var/www/letsencrypt`   | Directory used for the ACME HTTP-01 challenge files. Apache must be able to serve `/.well-known/acme-challenge/` from this path.      |
| `port`            | int    | ❌                 | first `ports[].hostPort` | Override which host port stevedore proxies traffic to. Useful when the app exposes multiple ports.                                    |
| `path`            | string | ❌                 | `/`                      | Path prefix to expose on the domain, e.g. `/hello`.                                                                                   |
| `stripPathPrefix` | bool   | ❌                 | `true`                   | When `path` is not `/`, strip the prefix before forwarding to the container. If `false`, the backend receives the full prefixed path. |

### Path-based exposure example

Repository layout:

```text
<repo-root>/
  example.com/
    apps/
      demo-api/
        stevedore.yml
```

Example manifest:

```yaml
expose:
  enabled: true
  provider: apache
  config:
    domain: example.com
    path: /hello
    stripPathPrefix: true
```

This exposes only `example.com/hello` (and children like `example.com/hello/api`) to
the app. Requests to `/hello` are redirected to `/hello/`.

### HTTP-only example

```yaml
# apps/demo-api/stevedore.yml
image:
  repository: nginx
  tag: 1.31.3
container:
  name: demo-api
restartPolicy: always
ports:
  - name: http
    containerPort: 80
    hostPort: 8081
volumes:
  - name: data
    hostPath: /var/lib/stevedore/demo-api/data
    mountPath: /srv/data
networks:
  - name: demo
  - name: demo-shared
expose:
  enabled: true
  provider: apache
  config:
    domain: demo-api.example.com
    ssl: false
```

Generated vhost (`/etc/apache2/sites-enabled/demo-api.conf`):

```apache
<VirtualHost *:80>
    ServerName demo-api.example.com

    Alias /.well-known/acme-challenge/ /var/www/letsencrypt/.well-known/acme-challenge/
    <Directory "/var/www/letsencrypt/.well-known/acme-challenge">
        Options None
        AllowOverride None
        Require all granted
    </Directory>

    ProxyPreserveHost On
    ProxyPass /.well-known/acme-challenge/ !
    ProxyPass        / http://127.0.0.1:8081/
    ProxyPassReverse / http://127.0.0.1:8081/

    ErrorLog  ${APACHE_LOG_DIR}/demo-api-error.log
    CustomLog ${APACHE_LOG_DIR}/demo-api-access.log combined
</VirtualHost>
```

### HTTPS example (Let's Encrypt)

```yaml
# apps/demo-api/stevedore.yml
image:
  repository: nginx
  tag: 1.31.3
container:
  name: demo-api
restartPolicy: always
ports:
  - name: http
    containerPort: 80
    hostPort: 8081
volumes:
  - name: data
    hostPath: /var/lib/stevedore/demo-api/data
    mountPath: /srv/data
networks:
  - name: demo
  - name: demo-shared
expose:
  enabled: true
  provider: apache
  config:
    domain: demo-api.example.com    # must resolve to this server
    ssl: true
    email: ops@example.com          # Let's Encrypt account / expiry alerts
```

What stevedore-agent does on the **first** reconcile cycle:

1. Writes an HTTP-only vhost with the ACME challenge alias.
2. Reloads Apache so `/.well-known/acme-challenge/` is served from disk.
3. Runs `certbot certonly --webroot` to obtain the certificate.
4. Overwrites the vhost with the full HTTP-redirect + HTTPS config.
5. Reloads Apache again.

On **subsequent** reconcile cycles, if the certificate already exists certbot
runs `certbot renew --non-interactive --quiet` (which is a no-op unless the cert
is close to expiry).

Generated vhost after certificate is obtained:

```apache
<VirtualHost *:80>
    ServerName demo-api.example.com

    Alias /.well-known/acme-challenge/ /var/www/letsencrypt/.well-known/acme-challenge/
    <Directory "/var/www/letsencrypt/.well-known/acme-challenge">
        Options None
        AllowOverride None
        Require all granted
    </Directory>

    RewriteEngine On
    RewriteCond %{REQUEST_URI} !^/.well-known/acme-challenge/
    RewriteRule ^(.*)$ https://%{HTTP_HOST}$1 [R=301,L]
</VirtualHost>

<VirtualHost *:443>
    ServerName demo-api.example.com

    SSLEngine On
    SSLCertificateFile    /etc/letsencrypt/live/demo-api.example.com/fullchain.pem
    SSLCertificateKeyFile /etc/letsencrypt/live/demo-api.example.com/privkey.pem

    SSLProtocol             all -SSLv3 -TLSv1 -TLSv1.1
    SSLCipherSuite          ECDHE-ECDSA-AES128-GCM-SHA256:...
    SSLHonorCipherOrder     off
    SSLSessionTickets       off

    Header always set Strict-Transport-Security "max-age=63072000; includeSubDomains; preload"

    ProxyPreserveHost On
    ProxyPass        / http://127.0.0.1:8081/
    ProxyPassReverse / http://127.0.0.1:8081/

    ErrorLog  ${APACHE_LOG_DIR}/demo-api-error.log
    CustomLog ${APACHE_LOG_DIR}/demo-api-access.log combined
</VirtualHost>
```

---

## Certificate storage

Certificates are stored by certbot at:

```
/etc/letsencrypt/live/<domain>/fullchain.pem
/etc/letsencrypt/live/<domain>/privkey.pem
```

stevedore-agent does **not** manage certificate deletion when an app is removed.
If you decommission an app, remove the certificate manually with:

```bash
certbot delete --cert-name <domain>
```

---

## Renewal

stevedore-agent calls `certbot renew` on every reconcile cycle where `ssl: true`
and the certificate already exists. certbot only actually renews when the
certificate is within 30 days of expiry (certbot's default), so the extra calls
are cheap no-ops the vast majority of the time.

For additional safety you can also set up the standard certbot systemd timer:

```bash
systemctl enable --now certbot.timer
```

---

## Troubleshooting

| Symptom                                   | Likely cause                               | Fix                                                        |
|-------------------------------------------|--------------------------------------------|------------------------------------------------------------|
| `certbot: command not found`              | certbot not installed                      | `apt-get install certbot`                                  |
| `certbot certonly: … connection refused`  | Port 80 not open or Apache not running     | Open port 80, ensure Apache is active                      |
| `certbot certonly: … DNS problem`         | `domain` does not resolve to this host     | Update your DNS A record                                   |
| `apache reload … failed`                  | Apache config syntax error                 | Run `apachectl configtest`                                 |
| `apache expose: app "x" has no host port` | `ports` block missing from `stevedore.yml` | Add at least one `ports` entry or set `expose.config.port` |
| `apache expose config requires 'email'`   | `ssl: true` but no `email` in config       | Add `email: you@example.com` under `expose.config`         |

---

## Example files

- Plain HTTP: [`examples/apps/demo-api/stevedore.yml`](../examples/apps/demo-api/stevedore.yml)
- HTTPS / SSL: [`examples/apps/demo-api-ssl/stevedore.yml`](../examples/apps/demo-api-ssl/stevedore.yml)
- VHost templates (embedded in binary):
    - `internal/exposure/apache/templates/vhost-http.conf.tmpl`
    - `internal/exposure/apache/templates/vhost-ssl.conf.tmpl`
