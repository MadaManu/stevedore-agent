# Contributing to stevedore-agent

Thanks for your interest in contributing.

## Development setup

1. Install Go 1.24+ and Docker.
2. Clone the repository.
3. Run the same checks as CI:

```bash
make ci
```

4. Build the same Linux release artifacts as the release workflow:

```bash
make build VERSION=dev
```

## Project conventions

- Keep changes focused and small.
- Follow existing package boundaries and naming patterns.
- Add or update tests when behavior changes.
- Do not commit secrets or environment-specific local files.
- Prefer improving docs together with behavioral changes.

## Submitting changes

1. Create a branch from `main`.
2. Make your changes with tests passing.
3. Open a Pull Request with:
   - concise summary of what changed
   - rationale (why)
   - testing notes
   - any operational impact

## Reporting issues

Open an issue with:

- expected behavior
- actual behavior
- reproduction steps
- relevant logs (redacted)
- environment details (OS, Go version, Docker version)

## Security reports

Do not file public issues for vulnerabilities. Follow [SECURITY.md](SECURITY.md).
