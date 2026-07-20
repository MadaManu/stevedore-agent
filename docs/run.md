# Stevedore `run` — continuous reconciliation

The `run` command turns Stevedore into a long-running agent. It continuously
watches a **source of truth** (a local directory or a git repository) and applies
any changes it finds, on a configurable schedule.

```bash
stevedore-agent run                 # loads config, then runs continuously
STEVEDORE_HOME=/etc/stevedore stevedore-agent run
STEVEDORE_DEBUG=true stevedore-agent run   # verbose per-cycle tracing
```

Before starting the agent, you can validate the effective configuration and
local prerequisites with [`stevedore-agent doctor`](./doctor.md).

Configuration is **loaded and validated exactly once at startup** and then
treated as immutable for the life of the process. Nothing is hot-reloaded — to
pick up config changes, restart the process. (Manifest changes from the source
of truth are still detected every cycle; only the agent's own config is fixed.)

## High-level flow

```
        ┌────────────────────────────────────────────────────────┐
        │ startup: load + validate config (once)                 │
        └───────────────────────────┬────────────────────────────┘
                                    │
                                    ▼
                 ┌──────────────────────────────────────┐
                 │ sync source of truth                 │
                 │  - git:  clone (first) / fetch+reset  │
                 │  - local: use path directly           │
                 └───────────────┬──────────────────────┘
                                 ▼
                 ┌──────────────────────────────────────┐
                 │ hash all apps/*/stevedore.yml files  │
                 └───────────────┬──────────────────────┘
                                 ▼
                    changed since last cycle?
                     │                      │
                 no  │                      │ yes
                     ▼                      ▼
             sleep(interval)              reconcile
                     │                      │
                     └───────────┬──────────┘
                                 ▼
                          wait for next tick
                        (or exit on SIGINT/SIGTERM)
```

Each cycle:

1. **Config is already loaded** (done once at startup — see below).
2. **Sync the source of truth**
   - `git`: clones into the workdir on first run, then `fetch` + `reset --hard`
     to the configured branch on subsequent runs.
   - `local`: uses the configured filesystem path as-is.
3. **Load and resolve manifests** (including `${provider:path}` placeholders)
   and compute a desired-state hash from the resolved application specs.
4. **Detect changes** by comparing:
   - manifest file hash (`apps/*/stevedore.yml` content + path)
   - resolved desired-state hash
   - runtime snapshot hash
5. **Decide**
   - If all hashes are unchanged, the agent logs a debug line and sleeps for
     the configured interval.
   - If any hash changed (or on the first cycle), it reconciles every
     application (create/update/remove containers to match the manifests).
6. **Repeat** on the next tick. `SIGINT`/`SIGTERM` triggers graceful shutdown.

## Configuration

Configuration is loaded **once at startup** with the precedence:

> **ENV overrides > config file values > built-in defaults**

Resolution order for the config file location:

1. `STEVEDORE_HOME` environment variable (directory)
2. `/etc/stevedore/config.yml` (default)

### Schema

The active source and git auth method are **inferred from which node is
present** — there is no `type` or `method` discriminator field.

- Under `source`, define **exactly one** of `local` or `git`. Defining both, or
  neither, is a validation error.
- Under `source.git.auth`, define **at most one** of `token`, `basic`, or `ssh`.
  Omit `auth` entirely for public repositories. Defining more than one is a
  validation error.

| Field | Type | Description |
|-------|------|-------------|
| `logging.dir` | string | Directory for `stevedore.log`. Default `/var/log/stevedore`. |
| `logging.debug` | bool | Verbose logging. Default `false`. |
| `source.local.path` | string | Repo root (containing `apps/`). Required for a local source. |
| `source.git.url` | string | Git remote URL. Required for a git source. |
| `source.git.branch` | string | Branch to track. Default `main`. |
| `source.git.workdir` | string | Checkout location. Default `<STEVEDORE_HOME>/git-source` (or `/etc/stevedore/git-source` when `STEVEDORE_HOME` is unset). |
| `source.git.auth.token.value` | string | Token (present ⇒ token auth). Supports `${ENV}`. |
| `source.git.auth.basic.username` / `password` | string | Basic auth (present ⇒ basic auth). Supports `${ENV}`. |
| `source.git.auth.ssh.keyPath` | string | Private key path (present ⇒ ssh auth). Supports `${ENV}`. |
| `secrets.providers.local.file` | string | Path to YAML/JSON secrets store used by `${local:...}` placeholders. |
| `poll.interval` | duration | How often to check. Default `30s` (e.g. `15s`, `5m`, `1h`). |
| `exposure.apache.sitesDir` | string | Apache vhost output dir. **Optional** — required only when an app uses `expose.provider: apache`. Overridable by `STEVEDORE_APACHE_SITES_DIR`. |

### Environment overrides

Each of these, when set, wins over the corresponding config-file value:

| Variable | Overrides |
|----------|-----------|
| `STEVEDORE_HOME` | Config home directory (config is read from `<home>/config.yml`). |
| `STEVEDORE_LOG_DIR` | `logging.dir` |
| `STEVEDORE_DEBUG` | `logging.debug` (bool) |
| `STEVEDORE_POLL_INTERVAL` | `poll.interval` (Go duration) |
| `STEVEDORE_APACHE_SITES_DIR` | `exposure.apache.sitesDir` — also activates the apache provider if not already configured in the file. |
| `STEVEDORE_WORKDIR` | `source.git.workdir` |

### Secrets

Secret-bearing git auth fields (`token.value`, `basic.username`,
`basic.password`, `ssh.keyPath`) support `${ENV_VAR}` interpolation. Prefer
injecting secrets via environment variables (or a secrets manager) rather than
committing them to the config file:

```yaml
source:
  git:
    auth:
      token:
        value: ${GIT_TOKEN}
```

Manifests also support runtime secret placeholders in the form `${provider:path}`.
These are resolved during reconciliation (not at startup), so sensitive values
are only loaded when needed.

Today, `local` is supported:

```yaml
# config.yml
secrets:
  providers:
    local:
      file: /etc/stevedore/secrets.yml
```

```yaml
# apps/demo-ui/stevedore.yml
environment:
  API_TOKEN: "${local:api/token}"
```

The local store file can be YAML or JSON and supports nested paths using `/`
(for example `api/token` or `nested/list/0`).

### Exposure providers

Exposure providers are **optional** and configured under `exposure`. A provider
is only required when at least one app in your manifests sets `expose.enabled:
true` and `expose.provider` to that provider name. No provider configuration is
needed if none of your apps use `expose`.

The active provider(s) are inferred from which nodes are present under
`exposure` — there is no explicit list.

#### Apache

The `apache` provider writes Apache VirtualHost configuration files to the
configured `sites-enabled` directory, handles Let's Encrypt certificate
issuance/renewal (when `ssl: true`), and reloads Apache.

Full documentation, all `expose.config` parameters, generated vhost examples,
and a troubleshooting guide: **[`docs/apache-exposure.md`](./apache-exposure.md)**

```yaml
# config.yml
exposure:
  apache:
    sitesDir: /etc/apache2/sites-enabled   # required when using apache
```

Minimal HTTP-only app manifest:

```yaml
# apps/demo-api/stevedore.yml
expose:
  enabled: true
  provider: apache
  config:
    domain: api.example.com
    ssl: false
```

HTTPS app manifest (Let's Encrypt):

```yaml
expose:
  enabled: true
  provider: apache
  config:
    domain: api.example.com
    ssl: true
    email: ops@example.com     # required for Let's Encrypt registration
```

`expose.config` fields:

| Field     | Required              | Default                | Description |
|-----------|-----------------------|------------------------|-------------|
| `domain`  | **yes**               | —                      | Public hostname; must resolve to this server. |
| `ssl`     | no                    | `false`                | Enable HTTPS via Let's Encrypt. |
| `email`   | **yes when ssl:true** | —                      | Let's Encrypt account / expiry notifications. |
| `webroot` | no                    | `/var/www/letsencrypt` | ACME HTTP-01 challenge directory. |
| `port`    | no                    | first `hostPort`       | Override proxy target port. |

The `sitesDir` can also be set via the `STEVEDORE_APACHE_SITES_DIR` environment
variable, which activates the provider even if `exposure.apache` is absent from
`config.yml`.

#### Local secrets store structure (example)

Sample file: [`examples/config/secrets.local.yml`](../examples/config/secrets.local.yml)

```yaml
# examples/config/secrets.local.yml
api:
  token: demo-api-token
database:
  host: localhost
  port: 5432
  credentials:
    username: demo_user
    password: demo_password
services:
  - name: billing
    apiKey: billing-key-1
```

Configure `stevedore` to use that file:

```yaml
# config.yml
source:
  local:
    path: /srv/stevedore/apps-repo
secrets:
  providers:
    local:
      # Absolute path (recommended in production)
      file: /etc/stevedore/secrets.yml
      # Relative paths are resolved from the folder containing config.yml
      # file: ./secrets.local.yml
```

Reference secrets from app manifests:

```yaml
# apps/demo-ui/stevedore.yml
environment:
  API_TOKEN: "${local:api/token}"
  DB_HOST: "${local:database/host}"
  DB_PASSWORD: "${local:database/credentials/password}"
  BILLING_API_KEY: "${local:services/0/apiKey}"
```

Notes:

- Paths are `/`-separated and can include list indices (`services/0/apiKey`).
- Resolved values must be scalar (`string`, `number`, `bool`); maps/lists are rejected.
- Placeholders are resolved during reconcile execution, not at process startup.

## Authentication details

The auth method is inferred from which node is present under `source.git.auth`:

- **token** — the `token.value` is injected into the `https://` remote URL as
  `x-access-token:<token>@…`. Requires an `https` URL.
- **basic** — `basic.username`/`basic.password` are injected into the `https://`
  remote URL.
- **ssh** — an `ssh` remote URL is used with `GIT_SSH_COMMAND` pointing at
  `ssh.keyPath` (`IdentitiesOnly=yes`, `StrictHostKeyChecking=accept-new`).
- **none** — when `auth` is omitted, the URL is used unchanged (public repos).

Git is always run with `GIT_TERMINAL_PROMPT=0` so the daemon never blocks on an
interactive credential prompt.

## Example configs

- Local source: [`examples/config/local.yml`](../examples/config/local.yml)
- Git source: [`examples/config/git.yml`](../examples/config/git.yml)
- Local secrets store sample: [`examples/config/secrets.local.yml`](../examples/config/secrets.local.yml)
- Doctor command: [`docs/doctor.md`](./doctor.md)

## Repository layout expected at the source

The source root must contain an `apps/` directory with one folder per
application, each holding a `stevedore.yml`:

```
<repo-root>/
  apps/
    demo/
      stevedore.yml
    demo-api/
      stevedore.yml
```

This is the standard Stevedore repository layout consumed by the agent.

## Running as a service

Install and enable a `systemd` service using the built-in installer command:

```bash
sudo stevedore-agent install-service
```

This command writes `/etc/systemd/system/stevedore-agent.service`, runs
`systemctl daemon-reload`, then runs `systemctl enable --now stevedore-agent.service`.

The service file is generated from a template in code at
`cmd/stevedore/service_install.go` so it can be changed centrally.

Generated unit content:

```ini
[Unit]
Description=Stevedore agent
After=network-online.target docker.service
Wants=network-online.target
StartLimitIntervalSec=900
StartLimitBurst=5

[Service]
Environment=STEVEDORE_HOME=/etc/stevedore
Environment=STEVEDORE_LOG_DIR=/var/log/stevedore
# Environment=GIT_TOKEN=...   (prefer an EnvironmentFile with 0600 perms)
ExecStart=/usr/local/bin/stevedore-agent run
Restart=on-failure
RestartSec=30

[Install]
WantedBy=multi-user.target
```

