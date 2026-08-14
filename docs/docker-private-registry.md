# Docker private registry access with systemd service

When `stevedore-agent` runs as a service, Docker image pulls happen in the
service environment. If private registry credentials are not visible there,
pulls fail.

## How install-service sets Docker env

`stevedore-agent install-service` writes these environment variables into the
systemd unit:

- `DOCKER_CONFIG`
- `HOME`

Resolution behavior:

1. If `--docker-home` is set, that value is used for `HOME`.
2. Otherwise `HOME` is inherited from the shell running `install-service`.
3. If `--docker-config` is set, that value is used.
4. Otherwise `DOCKER_CONFIG` defaults to `<HOME>/.docker` when `HOME` is known.

## Typical setups

### Default (use current shell home)

```bash
sudo stevedore-agent install-service
```

### Dedicated service user home

```bash
sudo stevedore-agent install-service \
  --docker-home /home/stevedore \
  --docker-config /home/stevedore/.docker
```

### Centralized credentials directory

```bash
sudo stevedore-agent install-service \
  --docker-home /etc/stevedore \
  --docker-config /etc/stevedore/docker
```

## Verify

```bash
sudo cat /etc/systemd/system/stevedore-agent.service
sudo systemctl status stevedore-agent
sudo journalctl -u stevedore-agent -n 100
```

Confirm the unit file has expected `Environment=DOCKER_CONFIG=...` and
`Environment=HOME=...` lines.

## Common failures

| Symptom | Likely cause | Fix |
|---|---|---|
| `no basic auth credentials` | service cannot read Docker credentials | point `--docker-config` to readable directory |
| image pull auth failures | wrong credentials in `config.json` | run `docker login` for target registry |
| permission denied on Docker config | restrictive file ownership/perms | grant read access to service account |

## Security notes

- Keep Docker credential files readable only by the service account.
- Prefer short-lived tokens where available.
- Rotate credentials periodically.
