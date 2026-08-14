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

## Project status

This project is functional and actively evolving. Interfaces and behavior may change between releases.

## Requirements

- Go 1.24+
- Docker Engine with CLI access
- Linux + systemd for service install/uninstall commands
- Optional: Apache + certbot if using `expose.provider: apache`

## Quick start

1. Build:

```bash
go build -o stevedore-agent .
```

2. Prepare config:

```bash
sudo mkdir -p /etc/stevedore
sudo cp examples/config/local.yml /etc/stevedore/config.yml
```

3. Update `source.local.path` in `/etc/stevedore/config.yml` to your manifests repo.

4. Validate setup:

```bash
./stevedore-agent doctor
```

5. Run:

```bash
./stevedore-agent run
```

## CLI

```text
stevedore-agent
├── run
├── doctor
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
- Linux binaries are attached as downloadable assets on each release.
- A `checksums.txt` file is included with each release for integrity checks.

## Security

Please report vulnerabilities using the process in [SECURITY.md](SECURITY.md).

## Contributing

Please read [CONTRIBUTING.md](CONTRIBUTING.md) and [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) before submitting changes.

## License

Licensed under the [MIT License](LICENSE).
