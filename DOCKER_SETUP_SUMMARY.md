# Summary: Docker Private Repository Setup for Service Installation

## What Changed

I've made it easier to set up Docker private repository support when installing stevedore-agent as a systemd service. Here's what I implemented:

### 1. **New Command-Line Flags**
Added two optional flags to the `install-service` command:
- `--docker-config <path>` - Path to Docker config directory (e.g., `/root/.docker`)
- `--docker-home <path>` - Home directory for the Docker user (e.g., `/root`)

### 2. **Updated Systemd Template**
The template now conditionally includes Docker environment variables only when specified:
```ini
Environment=DOCKER_CONFIG=/root/.docker     # Only if --docker-config provided
Environment=HOME=/root                      # Only if --docker-home provided
```

### 3. **Helpful Guidance**
When installing the service WITHOUT Docker configuration, users see:
```
ℹ️  Docker private repository support:
To enable pulling images from private Docker repositories, reinstall the service with:
  stevedore-agent install-service --docker-config <path> --docker-home <path>

Example (for root user):
  stevedore-agent install-service --docker-config /root/.docker --docker-home /root

Make sure your Docker credentials are available at the specified paths.
```

### 4. **Comprehensive Documentation**
Created `docs/docker-private-registry.md` with:
- Setup instructions for various scenarios (root user, custom user, centralized config)
- Verification steps
- Troubleshooting guide
- Security considerations
- Examples for complete setup from scratch

## Usage Examples

### Quick Setup (Root User)
```bash
sudo stevedore-agent install-service \
  --docker-config /root/.docker \
  --docker-home /root
```

### Custom User
```bash
sudo stevedore-agent install-service \
  --docker-config /home/stevedore/.docker \
  --docker-home /home/stevedore
```

### Centralized Configuration
```bash
# Store Docker credentials in a shared location
sudo mkdir -p /etc/stevedore/docker
sudo cp /root/.docker/config.json /etc/stevedore/docker/

# Install service pointing to centralized config
sudo stevedore-agent install-service \
  --docker-config /etc/stevedore/docker \
  --docker-home /etc/stevedore
```

### Reinstalling with New Configuration
```bash
# Simply run install-service again with new paths - it will replace the old service
sudo stevedore-agent install-service \
  --docker-config /new/path \
  --docker-home /new/home
```

## Files Modified

1. **cmd/stevedore/service_install.go**
   - Added `dockerConfigPath` and `dockerHomePath` global variables
   - Added command-line flags to `newInstallServiceCommand()`
   - Updated `renderSystemdService()` to include Docker config in template data
   - Enhanced `runInstallService()` with helpful guidance for users

2. **cmd/stevedore/templates/systemd.service.tmpl**
   - Added conditional environment variable lines for DOCKER_CONFIG and HOME

3. **cmd/stevedore/service_install_test.go**
   - Added `TestRenderSystemdService_IncludesDockerConfig()` - verifies Docker config is included when set
   - Added `TestRenderSystemdService_OmitsDockerConfigWhenNotSet()` - verifies it's omitted when not set
   - Updated `TestRunInstallService_WritesUnitAndEnablesService()` - tests without Docker config
   - Added `TestRunInstallService_WithDockerConfig()` - tests with Docker config enabled
   - All new tests verify helpful output is shown appropriately

4. **docs/run.md**
   - Added new section "Private Docker Registry Support" with flags documentation

5. **docs/docker-private-registry.md** (NEW)
   - Complete guide for setting up Docker private repository support

## Benefits

✅ **Easier Setup** - Users don't need to manually edit systemd unit files  
✅ **Clear Guidance** - Helpful messages guide users through the setup  
✅ **Flexible** - Supports root users, custom users, and centralized configurations  
✅ **Safe** - Environment variables only added when explicitly specified  
✅ **Reversible** - Service can be reinstalled with different configuration anytime  
✅ **Well-Tested** - All changes covered with comprehensive tests  
✅ **Well-Documented** - Extensive documentation and troubleshooting guide

## Testing

All tests pass:
```
✓ TestRenderSystemdService_IncludesRequiredDirectives
✓ TestRenderSystemdService_IncludesDockerConfig
✓ TestRenderSystemdService_OmitsDockerConfigWhenNotSet
✓ TestRunInstallService_WritesUnitAndEnablesService
✓ TestRunInstallService_WithDockerConfig
```

## Next Steps for Users

1. Review the new documentation: `docs/docker-private-registry.md`
2. Log in to your Docker registries first: `docker login registry.example.com`
3. Use the new flags when installing the service
4. Verify setup with: `sudo systemctl status stevedore-agent` and `sudo journalctl -u stevedore-agent -f`

