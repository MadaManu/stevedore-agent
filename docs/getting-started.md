# Getting started

This guide shows how to download `stevedore-agent`, point it at a manifest repo,
and run the bundled demo stack.

## Download the binary

Go to the [GitHub Releases page](https://github.com/MadaManu/stevedore-agent/releases)
and download one of:

- `stevedore-agent-linux-amd64`
- `stevedore-agent-linux-arm64`

Then make it executable:

```bash
chmod +x stevedore-agent-linux-amd64
```

## Prepare the demo repo

Create a manifest repo layout like this:

```text
/srv/stevedore/apps-repo/
  apps/
    demo-api/
      stevedore.yml
    demo-ui/
      stevedore.yml
```

Copy the bundled examples:

```bash
sudo mkdir -p /srv/stevedore/apps-repo/apps
sudo cp -R examples/apps/demo-api /srv/stevedore/apps-repo/apps/
sudo cp -R examples/apps/demo-ui /srv/stevedore/apps-repo/apps/
```

For the local secret example:

```bash
sudo mkdir -p /etc/stevedore
sudo cp examples/config/secrets.local.yml /etc/stevedore/secrets.local.yml
```

## Configure stevedore

Start from the example config:

```bash
sudo cp examples/config/local.yml /etc/stevedore/config.yml
```

Update these values in `/etc/stevedore/config.yml`:

- `source.local.path: /srv/stevedore/apps-repo`
- `secrets.providers.local.file: /etc/stevedore/secrets.local.yml`

## Run

First inspect the environment:

```bash
./stevedore-agent-linux-amd64 doctor
```

Then start reconciliation:

```bash
./stevedore-agent-linux-amd64 run
```

## What the demo shows

The demo manifests show:

- multiple apps managed together
- shared networks
- host-mounted volumes
- environment interpolation from a local secrets file
- optional Apache exposure in the SSL example

If you want to see Apache + HTTPS, review
[`examples/apps/demo-api-ssl/stevedore.yml`](../examples/apps/demo-api-ssl/stevedore.yml)
and [docs/apache-exposure.md](apache-exposure.md).
