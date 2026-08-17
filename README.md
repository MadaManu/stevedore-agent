# stevedore-agent

[![CI](https://github.com/MadaManu/stevedore-agent/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/MadaManu/stevedore-agent/actions/workflows/ci.yml)

`stevedore-agent` is a host-level reconciliation agent for running containerized apps from declarative manifests.

It continuously syncs a source of truth (local folder or Git repository), discovers app manifests, and converges Docker runtime state to match what is declared.

## Features

- Continuous reconcile loop with configurable poll interval
- Local or Git-backed source of truth
- Host-aware manifest discovery (`<fqdn>/apps/*` + shared `apps/*`)
- Declarative container state (image, ports, env, volumes, networks, restart policy)
- Runtime drift detection and corrective reconciliation
- Secret interpolation for manifests (`${local:path/to/secret}`)
- Apache exposure plugin for HTTP/HTTPS publishing and Let's Encrypt flow
- Built-in `doctor` command for local diagnostics
- Built-in `install-service` and `uninstall-service` for systemd
- Built-in `version` command and release bootstrap installer

## Project status

This project is functional and actively evolving. Interfaces and behavior may change between releases.

## Requirements

- Go 1.24+
- Docker Engine with CLI access
- Linux + systemd for service install/uninstall commands
- Optional: Apache + certbot if using `expose.provider: apache`

## Quick start

1. Set up the demo repo and config:

```bash
sudo mkdir -p /srv/stevedore/apps-repo/apps
sudo cp examples/config/local.yml /etc/stevedore/config.yml
sudo cp -R examples/apps/demo-api /srv/stevedore/apps-repo/apps/
sudo cp -R examples/apps/demo-ui /srv/stevedore/apps-repo/apps/
sudo cp examples/config/secrets.local.yml /etc/stevedore/secrets.local.yml
```

2. Point `/etc/stevedore/config.yml` at `/srv/stevedore/apps-repo` and `/etc/stevedore/secrets.local.yml`.

3. Run the bootstrap installer:

```bash
curl -fsSL https://raw.githubusercontent.com/MadaManu/stevedore-agent/main/install.sh | sudo bash
```

4. Or install a specific release:

```bash
curl -fsSL https://raw.githubusercontent.com/MadaManu/stevedore-agent/main/install.sh | sudo bash -s -- --version v0.1.0
```

5. Validate and run:

```bash
stevedore-agent doctor
stevedore-agent run
```

For a step-by-step first-run guide and the demo stack, see
[docs/getting-started.md](docs/getting-started.md).

## CLI

```text
stevedore-agent
├── run
├── doctor
├── version
├── install-service
└── uninstall-service
```

## Configuration model

- Config path: `${STEVEDORE_HOME}/config.yml` (default home: `/etc/stevedore`)
- Precedence: **environment overrides > config.yml > built-in defaults**
- Source selection is inferred from `source.local` or `source.git`
- Git auth selection is inferred from `source.git.auth.{token|basic|ssh}`

See full details in [docs/run.md](docs/run.md).

## Manifest repository layout

```text
<repo-root>/
  <fqdn>/
    apps/
      host-only-app/
        stevedore.yml
  apps/
    shared-app/
      stevedore.yml
```

## Documentation

- Running and configuration: [docs/run.md](docs/run.md)
- Diagnostics: [docs/doctor.md](docs/doctor.md)
- Apache exposure: [docs/apache-exposure.md](docs/apache-exposure.md)
- Private registries with systemd service: [docs/docker-private-registry.md](docs/docker-private-registry.md)
- Maintainer release process: [docs/development-release.md](docs/development-release.md)

## Releases and binaries

- Release notes are published on the GitHub Releases page:
  <https://github.com/MadaManu/stevedore-agent/releases>
- Linux binaries for `amd64` and `arm64` are attached as downloadable assets on each release.
- Assets follow the pattern `https://github.com/MadaManu/stevedore-agent/releases/download/<tag>/stevedore-agent-linux-<arch>`.
- A `checksums.txt` file is included with each release for integrity checks.

## Security

Please report vulnerabilities using the process in [SECURITY.md](SECURITY.md).

## Contributing

Please read [CONTRIBUTING.md](CONTRIBUTING.md) and [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) before submitting changes.

## License

Licensed under the [MIT License](LICENSE).
