# Stevedore `doctor` — inspect config and validate local setup

The `doctor` command is a non-mutating diagnostic for Stevedore. It loads the
same config that `stevedore-agent run` would use, shows the **effective settings**,
explains where each setting came from, and validates that local prerequisites
are accessible.

```bash
stevedore-agent doctor
STEVEDORE_HOME=/etc/stevedore stevedore-agent doctor
STEVEDORE_DEBUG=true stevedore-agent doctor
```

## What it checks

`stevedore-agent doctor` reports:

- the resolved config path and how it was chosen
- the effective setting values used by Stevedore
- the origin of each value:
  - built-in default
  - config file
  - environment override
  - derived from another setting
- environment overrides that are active, ignored, or invalid
- local filesystem readiness for:
  - the config file
  - the logging directory
  - the Apache sites directory
  - the local source repository and `apps/` directory
  - local secrets store files
  - git workdir / SSH key when `source.git` is configured
- required local binaries:
  - `docker`
  - `git` 1.8.5+ when `source.git` is configured (Stevedore uses `git -C`)
- exposure provider directories:
  - `exposure.apache.sitesDir` is only checked when the apache provider is configured

It does **not** mutate the system and does **not** test remote git connectivity.
It also does **not** pre-create Docker networks or verify container network
attachments; those are handled during `stevedore-agent run` reconciliation.
Likewise, it does **not** pre-create app `volumes[].hostPath` directories; those
are created during reconcile when each container spec is built.

When `source.git.workdir` is omitted, doctor reports and validates the effective
default checkout directory at `<STEVEDORE_HOME>/git-source` (or
`/etc/stevedore/git-source` when `STEVEDORE_HOME` is unset).

## Example output

```text
Stevedore doctor
================

Config lookup
-------------
config.path = /etc/stevedore/config.yml (built-in default /etc/stevedore)

Effective settings
------------------
logging.dir = /var/log/stevedore (built-in default)
logging.debug = true (environment variable STEVEDORE_DEBUG)
source.type = local (derived from source.local)
source.local.path = /srv/stevedore/apps-repo (config file /etc/stevedore/config.yml)
source.repoRoot = /srv/stevedore/apps-repo (derived from source.local.path)
poll.interval = 30s (built-in default)
secrets.providers.local.file = /etc/stevedore/secrets.yml (config file /etc/stevedore/config.yml)
exposure.apache.sitesDir = /etc/apache2/sites-enabled (config file /etc/stevedore/config.yml)

Environment overrides
---------------------
STEVEDORE_HOME [not-set] -> config.path
STEVEDORE_LOG_DIR [not-set] -> logging.dir
STEVEDORE_DEBUG [active] = "true" -> logging.debug
STEVEDORE_POLL_INTERVAL [not-set] -> poll.interval
STEVEDORE_APACHE_SITES_DIR [not-set] -> exposure.apache.sitesDir
STEVEDORE_WORKDIR [not-set] -> source.git.workdir

Checks
------
[PASS] config file — configuration file is readable
[PASS] config validation — configuration loads and validates successfully
[PASS] docker CLI — docker is available
[PASS] logging directory — directory exists and is writable
[PASS] local source repository — directory exists and is readable
[PASS] apps directory — directory exists and is readable
[PASS] local secrets store — local secrets store is readable and parses successfully
[PASS] apache sites directory — directory exists and is writable

Summary: 8 passed, 0 warning(s), 0 failed
```

## Common failure patterns

### Config file missing

If the config file cannot be read, `doctor` shows the exact path it tried and
suggests either:

- creating the missing `config.yml`, or
- pointing `STEVEDORE_HOME` at the correct home directory

### Relative local secrets path resolves somewhere unexpected

Relative values in `secrets.providers.local.file` are resolved from the folder
that contains `config.yml`.

Example:

```yaml
# /opt/stevedore/config.yml
secrets:
  providers:
    local:
      file: ./secrets.local.yml
```

This resolves to:

```text
/opt/stevedore/secrets.local.yml
```

If you accidentally configure:

```yaml
file: ./.home/secrets.local.yml
```

and your config is already in `.home/`, `doctor` will show both:

- the raw configured value
- the resolved absolute path it tried to open

### Invalid environment overrides

If an environment override is set but invalid, `doctor` marks it as a warning.
For example:

- `STEVEDORE_DEBUG=maybe`
- `STEVEDORE_POLL_INTERVAL=ten-seconds`
- `STEVEDORE_LOG_DIR=`

The report explains that these values are ignored and tells you to fix or unset
those variables.

## Recommended workflow

Run `doctor` before starting the long-running agent:

```bash
stevedore-agent doctor
stevedore-agent run
```

This is especially useful after changing:

- `config.yml`
- `STEVEDORE_*` environment overrides
- local secrets files
- source repository paths
- filesystem permissions
