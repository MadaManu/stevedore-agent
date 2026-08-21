# Running stevedore-agent

The `run` command starts a continuous reconciliation loop.

```bash
stevedore-agent run
```

Each cycle:

1. Sync source of truth (`source.local` or `source.git`)
2. Discover manifests (`<fqdn>/apps/*/stevedore.yml` + `apps/*/stevedore.yml`)
3. Resolve placeholders and compute desired/runtime hashes
4. Reconcile drift by creating/updating containers and exposure resources
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
| `source.git.auth.token.value` | string | Optional, `${ENV}` supported. Use token auth for GitHub and similar providers. |
| `source.git.auth.basic.username/password` | string | Optional, `${ENV}` supported. Some providers, like Bitbucket, accept a token as the basic-auth password. |
| `source.git.auth.ssh.keyPath` | string | Optional, `${ENV}` supported |
| `secrets.providers.local.file` | string | Local YAML/JSON secrets store |
| `exposure.apache.sitesDir` | string | Required only when Apache exposure is used |

Stevedore uses `git -C` when `source.git` is configured, so the host must have Git 1.8.5 or newer.

Rules:

- Exactly one of `source.local` or `source.git`
- At most one of `source.git.auth.token`, `basic`, `ssh`
- Prefer `source.git.auth.token` for GitHub; if your provider uses basic auth, some services such as Bitbucket accept a token as the password.

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
- Host-specific manifests can live under `<fqdn>/apps/*/stevedore.yml`.
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

## Bootstrap installer

The bootstrap script is `install.sh` in the repository root. It can be run
locally or piped from GitHub:

```bash
curl -fsSL https://raw.githubusercontent.com/MadaManu/stevedore-agent/main/install.sh | sudo bash
```

Accepted arguments:

| Flag | Values | Default |
|---|---|---|
| `--version` | `latest` or a release tag like `v0.1.0` | `latest` |
| `--install-dir` | Any writable directory | `/usr/local/bin` |
| `--repo` | `OWNER/REPO` | `MadaManu/stevedore-agent` |
| `-h`, `--help` | Prints usage | n/a |

The script only supports Linux, detects the machine architecture, downloads the
matching binary asset for the chosen version, checks `checksums.txt`, and then
installs the binary before reapplying the systemd unit. It also creates
`stevedore-agent` and `stevedore` symlinks.

If set, the script uses these environment variables to create the runtime
folders:

| Env var | Default | Purpose |
|---|---|---|
| `STEVEDORE_HOME` | `/etc/stevedore` | config, runtime state, and `git-source/` |
| `STEVEDORE_LOG_DIR` | `/var/log/stevedore` | log files |

`--upgrade` forces a reinstall of the latest release. Running the installer
again without it is still idempotent and will refresh symlinks when the
requested version changes.

Example:

```bash
curl -fsSL https://raw.githubusercontent.com/MadaManu/stevedore-agent/main/install.sh | \
  sudo STEVEDORE_HOME=/srv/stevedore STEVEDORE_LOG_DIR=/var/log/stevedore bash
```

For private image pulls, see [docs/docker-private-registry.md](docker-private-registry.md).
