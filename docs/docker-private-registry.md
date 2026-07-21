# Docker Private Repository Setup for Service Installation

## Overview

When running `stevedore-agent` as a systemd service, pulling images from private Docker repositories requires proper configuration of Docker credentials. By default, `install-service` sets:

- `DOCKER_CONFIG=$HOME/.docker`
- `HOME=$HOME`

where `$HOME` is the current shell user that runs `install-service`. This guide explains the defaults and how to override them when needed.

## Common Issues

When Docker credentials are not configured correctly for the service user, you may encounter errors like:
- `Error response from daemon: Head "https://registry.example.com/v2/myimage/manifests/latest": no basic auth credentials`
- Authentication failures when pulling private images

## Setup Instructions

### Basic Setup (Root User)

If your current user has Docker credentials in `~/.docker/config.json`, the default install command already configures Docker support:

```bash
sudo stevedore-agent install-service
```

### Custom User Setup

If you run stevedore-agent as a non-root user (e.g., `stevedore`):

```bash
# First, ensure the user has Docker credentials configured
# Copy Docker config to the stevedore user's home directory
sudo cp /root/.docker /home/stevedore/.docker -r
sudo chown -R stevedore:stevedore /home/stevedore/.docker

# Then install the service
sudo stevedore-agent install-service \
  --docker-config /home/stevedore/.docker \
  --docker-home /home/stevedore
```

### Centralized Docker Config

You can also store Docker credentials in a shared location:

```bash
# Create a centralized Docker config directory
sudo mkdir -p /etc/stevedore/docker
sudo cp /root/.docker/config.json /etc/stevedore/docker/

# Install the service pointing to the centralized config
sudo stevedore-agent install-service \
  --docker-config /etc/stevedore/docker \
  --docker-home /etc/stevedore
```

## Environment Variables

The `install-service` command supports override flags:

- `--docker-config <path>`: Path to the Docker configuration directory (default: `$HOME/.docker`)
- `--docker-home <path>`: Home directory for the Docker user (default: `$HOME`)

Both paths must be accessible by the systemd service's user account.

## Verifying the Setup

After installation, verify the environment variables are set correctly:

```bash
# Check the installed systemd unit file
sudo cat /etc/systemd/system/stevedore-agent.service

# Look for lines like:
# Environment=DOCKER_CONFIG=/root/.docker
# Environment=HOME=/root
```

You can also test connectivity to private registries:

```bash
# View the service logs
sudo journalctl -u stevedore-agent -f

# You should see successful image pulls from private repositories
```

## Troubleshooting

### Permission Denied Errors

If you see permission errors when accessing Docker credentials:

```bash
# Ensure the systemd service user can read the config
sudo chmod 644 /path/to/docker/config.json
sudo chmod 755 /path/to/docker
```

### Config File Not Found

If the service can't find the config file:

```bash
# Verify the path exists and is readable
ls -la /path/to/docker/config.json

# Check systemd service status
sudo systemctl status stevedore-agent
sudo journalctl -u stevedore-agent -n 50
```

### Docker Socket Access

Ensure the systemd service user can access the Docker socket:

```bash
# Add the service user to the docker group (use with caution)
sudo usermod -aG docker stevedore
sudo systemctl daemon-reload
sudo systemctl restart stevedore-agent
```

## Example: Complete Setup from Scratch

For a fresh installation with private Docker registry support:

```bash
# 1. Build the stevedore-agent binary
go build -o stevedore-agent ./main.go

# 2. Move to system path (optional)
sudo mv stevedore-agent /usr/local/bin/

# 3. Ensure your Docker is logged into the private registry
docker login registry.example.com

# 4. Install the service (uses current $HOME defaults for Docker env)
sudo /usr/local/bin/stevedore-agent install-service

# 5. Verify the service is running
sudo systemctl status stevedore-agent
```

## Environment Variables in Systemd Service File

The generated systemd service file will include:

```ini
Environment=STEVEDORE_HOME=/etc/stevedore
Environment=STEVEDORE_LOG_DIR=/var/log/stevedore
Environment=DOCKER_CONFIG=/root/.docker
Environment=HOME=/root
```

The `DOCKER_CONFIG` variable tells Docker where to find the authentication credentials, and `HOME` ensures proper home directory resolution for the service.

## Reinstalling with New Configuration

If you need to change the Docker configuration later:

```bash
# Reinstall the service with new paths
sudo stevedore-agent install-service \
  --docker-config /new/docker/path \
  --docker-home /new/home/path

# The systemd daemon will automatically reload and restart the service
```

## Security Considerations

- **Restrict permissions**: Ensure Docker credentials are readable only by the service user
- **Use specific registries**: If possible, limit credentials to specific registries in `config.json`
- **Regular rotation**: Rotate Docker credentials periodically
- **Audit logs**: Monitor systemd logs for authentication failures

For more information, refer to the main documentation in `docs/service_install.md`.

