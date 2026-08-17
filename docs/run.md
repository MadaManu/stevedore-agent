# Running stevedore-agent

The `run` command starts a continuous reconciliation loop.

```bash
stevedore-agent run
```

Each cycle:

1. Sync source of truth (`source.local` or `source.git`)
2. Discover manifests (`<fqdn>/apps/*/stevedore.yml` + `apps/*/stevedore.yml`)
3. Resolve placeholders and compute desired/runtime hashes
4. Reconcile drift by creating/updating containers and plugin resources
5. Sleep until next `poll.interval`

Use `stevedore-agent doctor` before `run` to validate local setup.

For a first-run walkthrough with the bundled demo apps, see
[docs/getting-started.md](getting-started.md).

## Config file and precedence

- Path: `${STEVEDORE_HOME}/config.yml`
- Default home: `/etc/stevedore`
- Precedence: **environment overrides > config file > defaults**

## Configuration reference

| Field | Type | Notes |
|---|---|---|
| `logging.dir` | string | Default `/var/log/stevedore` |
| `logging.debug` | bool | Default `false` |
| `poll.interval` | duration | Default `30s` |
| `source.local.path` | string | Required when `source.local` is used |
| `source.git.url` | string | Required when `source.git` is used |
| `source.git.branch` | string | Default `main` |
| `source.git.workdir` | string | Default `<STEVEDORE_HOME>/git-source` |
| `source.git.auth.token.value` | string | Optional, `${ENV}` supported |
| `source.git.auth.basic.username/password` | string | Optional, `${ENV}` supported |
| `source.git.auth.ssh.keyPath` | string | Optional, `${ENV}` supported |
| `secrets.providers.local.file` | string | Local YAML/JSON secrets store |
| `exposure.apache.sitesDir` | string | Required only when Apache exposure is used |

Rules:

- Exactly one of `source.local` or `source.git`
- At most one of `source.git.auth.token`, `basic`, `ssh`

## Environment overrides

| Variable | Overrides |
|---|---|
| `STEVEDORE_HOME` | config home directory |
| `STEVEDORE_LOG_DIR` | `logging.dir` |
| `STEVEDORE_DEBUG` | `logging.debug` |
| `STEVEDORE_POLL_INTERVAL` | `poll.interval` |
| `STEVEDORE_WORKDIR` | `source.git.workdir` |
| `STEVEDORE_APACHE_SITES_DIR` | `exposure.apache.sitesDir` |

## Manifest model highlights

- App name is derived from folder name.
- Duplicate app names across host/shared trees fail startup.
- `networks` and `volumes` support multi-entry declarations.
- Legacy `network` and `volume` fields are still accepted.
- Secret placeholders use `${provider:path}` (currently `local` provider).

Examples:

- [examples/config/local.yml](../examples/config/local.yml)
- [examples/config/git.yml](../examples/config/git.yml)
- [examples/config/secrets.local.yml](../examples/config/secrets.local.yml)

## Running as a systemd service

Install:

```bash
sudo stevedore-agent install-service
```

Uninstall:

```bash
sudo stevedore-agent uninstall-service
```

The installer writes `/etc/systemd/system/stevedore-agent.service`, reloads
systemd, then enables/starts the service.

## Versioning

- Use `stevedore-agent version` to print the installed build version.
- Release binaries are published as
  `https://github.com/MadaManu/stevedore-agent/releases/download/<tag>/stevedore-agent-linux-<arch>`.

For private image pulls, see [docs/docker-private-registry.md](docker-private-registry.md).
